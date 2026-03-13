package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/itchyny/gojq"
)

const stageOutputMemoryThresholdDefault int64 = 8 * 1024 * 1024

var (
	stageOutputMemoryThreshold = stageOutputMemoryThresholdDefault
	createStageOutputTempFile  = func(pattern string) (*os.File, error) {
		return os.CreateTemp("", pattern)
	}
)

func PrintJSON(w io.Writer, data json.RawMessage, jqExpr string) error {
	if jqExpr != "" {
		return filterJQBytes(w, data, jqExpr)
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return fmt.Errorf("could not format JSON output: %w", err)
	}
	_, err := fmt.Fprintln(w, buf.String())
	return err
}

func StreamJSONArrayEnvelope(w io.Writer, key string, writeItems func(func(any) error) error) error {
	if _, err := fmt.Fprintln(w, "{"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, `  "success": true,`); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  %q: [\n", key); err != nil {
		return err
	}

	first := true
	writeItem := func(item any) error {
		data, err := json.MarshalIndent(item, "    ", "  ")
		if err != nil {
			return fmt.Errorf("could not encode JSON output: %w", err)
		}
		if !first {
			if _, err := fmt.Fprintln(w, ","); err != nil {
				return err
			}
		}
		first = false
		_, err = fmt.Fprint(w, string(data))
		return err
	}

	if err := writeItems(writeItem); err != nil {
		return err
	}

	if !first {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "  ]"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "}")
	return err
}

// WriteJSONStreamTo writes a JSON envelope directly to w without staging.
// Prefer PrintJSONStream for user-facing command output; this helper stays raw
// for internal composition and focused unit tests.
func WriteJSONStreamTo(w io.Writer, key string, writeItems func(func(any) error) error) error {
	return StreamJSONArrayEnvelope(w, key, writeItems)
}

// PrintJSONStreamWithJQ stages user-visible output before copying it to the
// final writer. This is intentional: `gumroad ... --all --jq ...` must fail
// atomically so late pagination/jq errors never leave partial machine-readable
// output on stdout. Do not switch this back to direct stdout streaming unless
// the CLI contract changes.
func PrintJSONStreamWithJQ(w io.Writer, key, jqExpr string, writeItems func(func(any) error) error) error {
	if jqExpr == "" {
		return PrintJSONStream(w, key, writeItems)
	}

	return stageOutput(w, "gumroad-jq-stream-*", func(stage io.Writer) error {
		return streamJSONWithJQ(stage, key, jqExpr, writeItems)
	})
}

// PrintJSONStream keeps the same atomic-output contract as
// PrintJSONStreamWithJQ for user-visible `--all --json` flows.
func PrintJSONStream(w io.Writer, key string, writeItems func(func(any) error) error) error {
	return stageOutput(w, "gumroad-json-stream-*", func(stage io.Writer) error {
		return StreamJSONArrayEnvelope(stage, key, writeItems)
	})
}

func filterJQBytes(w io.Writer, data json.RawMessage, expr string) error {
	return filterJQ(w, bytes.NewReader(data), expr)
}

func filterJQ(w io.Writer, r io.Reader, expr string) error {
	query, err := parseJQ(expr)
	if err != nil {
		return err
	}
	return runJQ(w, r, query)
}

func parseJQ(expr string) (*gojq.Query, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression: %w", err)
	}
	return query, nil
}

func runJQ(w io.Writer, r io.Reader, query *gojq.Query) error {
	if query == nil {
		return fmt.Errorf("invalid jq expression: nil query")
	}

	var input interface{}
	if err := json.NewDecoder(r).Decode(&input); err != nil {
		return fmt.Errorf("could not parse JSON for filtering: %w", err)
	}

	iter := query.Run(input)
	return runJQValues(w, iter)
}

func runJQValues(w io.Writer, iter gojq.Iter) error {
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return fmt.Errorf("jq error: %w", err)
		}
		out, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, string(out)); err != nil {
			return err
		}
	}
	return nil
}

func streamJSONWithJQ(w io.Writer, key, jqExpr string, writeItems func(func(any) error) error) error {
	reader, writer := io.Pipe()
	iter := newJSONStreamInputIter(reader)
	code, err := compileStreamJQ(jqExpr, iter)
	if err != nil {
		_ = iter.Close()
		_ = writer.Close()
		return err
	}

	writeErrCh := make(chan error, 1)
	go func() {
		err := StreamJSONArrayEnvelope(writer, key, writeItems)
		_ = writer.CloseWithError(err)
		writeErrCh <- err
	}()

	queryErr := runJQValues(w, code.Run(nil))
	_ = iter.Close()
	writeErr := <-writeErrCh

	return resolveStreamError(queryErr, writeErr)
}

func compileStreamJQ(jqExpr string, inputIter gojq.Iter) (*gojq.Code, error) {
	query, err := gojq.Parse(fmt.Sprintf("fromstream(inputs) | (%s)", jqExpr))
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression: %w", err)
	}

	code, err := gojq.Compile(query, gojq.WithInputIter(inputIter))
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression: %w", err)
	}
	return code, nil
}

// Stage output off to the side first so failures never leave partial JSON/JQ on
// the final writer. Keep common-sized responses in memory and spill to a temp
// file only when they outgrow the threshold.
func stageOutput(w io.Writer, pattern string, write func(io.Writer) error) (err error) {
	stage := newStageOutputBuffer(pattern, stageOutputMemoryThreshold)
	defer func() {
		closeErr := stage.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	if err := write(stage); err != nil {
		return err
	}
	if err := stage.CopyTo(w); err != nil {
		return err
	}
	return nil
}

func resolveStreamError(queryErr, writeErr error) error {
	if queryErr != nil {
		if writeErr != nil && !IsBrokenPipeError(writeErr) {
			return writeErr
		}
		return queryErr
	}
	if writeErr != nil && !IsBrokenPipeError(writeErr) {
		return writeErr
	}
	return nil
}

type stageOutputBuffer struct {
	pattern   string
	threshold int64
	buffer    bytes.Buffer
	file      *os.File
}

func newStageOutputBuffer(pattern string, threshold int64) *stageOutputBuffer {
	return &stageOutputBuffer{
		pattern:   pattern,
		threshold: threshold,
	}
}

func (b *stageOutputBuffer) Write(p []byte) (int, error) {
	if b.file != nil {
		return b.file.Write(p)
	}

	if int64(b.buffer.Len()+len(p)) <= b.threshold {
		return b.buffer.Write(p)
	}

	if err := b.spillToDisk(); err != nil {
		return 0, err
	}

	return b.file.Write(p)
}

func (b *stageOutputBuffer) CopyTo(w io.Writer) error {
	if b.file == nil {
		_, err := io.Copy(w, bytes.NewReader(b.buffer.Bytes()))
		return err
	}

	if _, err := b.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("could not rewind staged output: %w", err)
	}
	_, err := io.Copy(w, b.file)
	return err
}

func (b *stageOutputBuffer) Close() error {
	if b.file == nil {
		return nil
	}

	name := b.file.Name()
	closeErr := b.file.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return closeErr
	}
	if removeErr != nil && !os.IsNotExist(removeErr) {
		return removeErr
	}
	return nil
}

func (b *stageOutputBuffer) spillToDisk() (err error) {
	file, err := createStageOutputTempFile(b.pattern)
	if err != nil {
		return fmt.Errorf("could not create temp output: %w", err)
	}

	defer func() {
		if err == nil {
			return
		}
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
	}()

	if _, err = file.Write(b.buffer.Bytes()); err != nil {
		return err
	}

	b.buffer.Reset()
	b.file = file
	return nil
}
