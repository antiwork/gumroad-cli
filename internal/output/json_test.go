package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func setStageOutputThresholdForTest(t *testing.T, threshold int64) {
	t.Helper()
	orig := stageOutputMemoryThreshold
	stageOutputMemoryThreshold = threshold
	t.Cleanup(func() { stageOutputMemoryThreshold = orig })
}

func setStageOutputTempFileFactoryForTest(t *testing.T, factory func(string) (*os.File, error)) {
	t.Helper()
	orig := createStageOutputTempFile
	createStageOutputTempFile = factory
	t.Cleanup(func() { createStageOutputTempFile = orig })
}

type sliceIter struct {
	values []any
	index  int
}

func (i *sliceIter) Next() (any, bool) {
	if i.index >= len(i.values) {
		return nil, false
	}
	value := i.values[i.index]
	i.index++
	return value, true
}

func TestPrintJSON_Pretty(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"name":"test","value":42}`)
	if err := PrintJSON(&buf, data, ""); err != nil {
		t.Fatalf("PrintJSON failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["name"] != "test" {
		t.Errorf("got name=%v, want test", parsed["name"])
	}
}

func TestPrintJSON_JQ(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"user":{"email":"test@example.com"}}`)
	if err := PrintJSON(&buf, data, ".user.email"); err != nil {
		t.Fatalf("PrintJSON with jq failed: %v", err)
	}
	got := bytes.TrimSpace(buf.Bytes())
	if string(got) != `"test@example.com"` {
		t.Errorf("got %s, want %q", got, "test@example.com")
	}
}

func TestPrintJSON_JQArray(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"items":[1,2,3]}`)
	if err := PrintJSON(&buf, data, ".items[]"); err != nil {
		t.Fatalf("PrintJSON with jq array failed: %v", err)
	}
	got := bytes.TrimSpace(buf.Bytes())
	if string(got) != "1\n2\n3" {
		t.Errorf("got %q, want %q", got, "1\n2\n3")
	}
}

func TestPrintJSON_InvalidJQ(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{}`)
	if err := PrintJSON(&buf, data, ".[invalid"); err == nil {
		t.Fatal("expected error for invalid jq expression, got nil")
	}
}

func TestPrintJSON_JQRuntimeErrorLeavesNoPartialOutput(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"value":"not-a-number"}`)
	err := PrintJSON(&buf, data, ".value, (.value | tonumber)")
	if err == nil {
		t.Fatal("expected jq runtime error")
	}
	if buf.Len() != 0 {
		t.Fatalf("partial jq output = %q", buf.String())
	}
}

func TestPrintJSON_InvalidJSON_Errors(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`not json at all`)
	err := PrintJSON(&buf, data, "")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "could not format JSON output") {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output on invalid JSON, got %q", buf.String())
	}
}

func TestValidateJQExpression(t *testing.T) {
	for _, expr := range []string{"", ".media.url"} {
		if err := ValidateJQExpression(expr); err != nil {
			t.Fatalf("ValidateJQExpression(%q): %v", expr, err)
		}
	}
	if err := ValidateJQExpression(".["); err == nil {
		t.Fatal("invalid jq expression returned nil")
	}
	if err := ValidateJQExpression("foo"); err == nil {
		t.Fatal("jq compile error returned nil")
	}
}

func TestFilterJQ_InvalidInputJSON(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{not json}`)
	err := PrintJSON(&buf, data, ".foo")
	if err == nil {
		t.Fatal("expected error for invalid JSON with jq, got nil")
	}
	if !strings.Contains(err.Error(), "could not parse JSON") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrintJSON_Nested(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"a":{"b":{"c":1}}}`)
	if err := PrintJSON(&buf, data, ".a.b.c"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "1" {
		t.Errorf("got %q, want 1", got)
	}
}

func TestFilterJQ_RuntimeError(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`"a string"`)
	// .[] on a string triggers a jq runtime error
	err := PrintJSON(&buf, data, ".[]")
	if err == nil {
		t.Fatal("expected jq runtime error, got nil")
	}
	if !strings.Contains(err.Error(), "jq error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrintJSON_NullJQResult(t *testing.T) {
	var buf bytes.Buffer
	data := json.RawMessage(`{"a":1}`)
	if err := PrintJSON(&buf, data, ".nonexistent"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "null" {
		t.Errorf("got %q, want null", got)
	}
}

func TestPrintJSON_WriteError(t *testing.T) {
	err := PrintJSON(errWriter{}, json.RawMessage(`{"a":1}`), "")
	if !errors.Is(err, errTestWrite) {
		t.Fatalf("got %v, want %v", err, errTestWrite)
	}
}

func TestStreamJSONArrayEnvelope(t *testing.T) {
	var buf bytes.Buffer
	err := StreamJSONArrayEnvelope(&buf, "items", func(writeItem func(any) error) error {
		if err := writeItem(map[string]any{"id": "one"}); err != nil {
			return err
		}
		return writeItem(map[string]any{"id": "two"})
	})
	if err != nil {
		t.Fatalf("StreamJSONArrayEnvelope failed: %v", err)
	}

	var resp struct {
		Success bool                `json:"success"`
		Items   []map[string]string `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("streamed output is not valid JSON: %v\n%s", err, buf.String())
	}
	if !resp.Success {
		t.Fatal("expected success=true")
	}
	if len(resp.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(resp.Items))
	}
}

func TestStreamJSONArrayEnvelope_Empty(t *testing.T) {
	var buf bytes.Buffer
	if err := StreamJSONArrayEnvelope(&buf, "items", func(func(any) error) error { return nil }); err != nil {
		t.Fatalf("StreamJSONArrayEnvelope failed: %v", err)
	}

	var resp struct {
		Items []any `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("streamed output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(resp.Items) != 0 {
		t.Fatalf("got %d items, want 0", len(resp.Items))
	}
}

func TestStreamJSONArrayEnvelope_WriteError(t *testing.T) {
	err := StreamJSONArrayEnvelope(errWriter{}, "items", func(writeItem func(any) error) error {
		return writeItem(map[string]any{"id": "one"})
	})
	if !errors.Is(err, errTestWrite) {
		t.Fatalf("got %v, want %v", err, errTestWrite)
	}
}

func TestStreamJSONArrayEnvelope_CallbackError(t *testing.T) {
	want := errors.New("boom")
	err := StreamJSONArrayEnvelope(&bytes.Buffer{}, "items", func(func(any) error) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestStreamJSONArrayEnvelope_MarshalError(t *testing.T) {
	err := StreamJSONArrayEnvelope(&bytes.Buffer{}, "items", func(writeItem func(any) error) error {
		return writeItem(make(chan int))
	})
	if err == nil || !strings.Contains(err.Error(), "could not encode JSON output") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteJSONStreamTo(t *testing.T) {
	var buf bytes.Buffer
	err := WriteJSONStreamTo(&buf, "items", func(writeItem func(any) error) error {
		if err := writeItem(map[string]any{"id": "one"}); err != nil {
			return err
		}
		return writeItem(map[string]any{"id": "two"})
	})
	if err != nil {
		t.Fatalf("WriteJSONStreamTo failed: %v", err)
	}

	var resp struct {
		Success bool                `json:"success"`
		Items   []map[string]string `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("streamed output is not valid JSON: %v\n%s", err, buf.String())
	}
	if !resp.Success {
		t.Fatal("expected success=true")
	}
	if len(resp.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(resp.Items))
	}
}

func TestWriteJSONStreamTo_WriteError(t *testing.T) {
	err := WriteJSONStreamTo(errWriter{}, "items", func(writeItem func(any) error) error {
		return writeItem(map[string]any{"id": "one"})
	})
	if !errors.Is(err, errTestWrite) {
		t.Fatalf("got %v, want %v", err, errTestWrite)
	}
}

func TestWriteJSONStreamTo_CallbackError(t *testing.T) {
	want := errors.New("boom")
	err := WriteJSONStreamTo(&bytes.Buffer{}, "items", func(func(any) error) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestWriteJSONStreamTo_MarshalError(t *testing.T) {
	err := WriteJSONStreamTo(&bytes.Buffer{}, "items", func(writeItem func(any) error) error {
		return writeItem(make(chan int))
	})
	if err == nil || !strings.Contains(err.Error(), "could not encode JSON output") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintJSONStream_CallbackError(t *testing.T) {
	want := errors.New("boom")
	err := PrintJSONStream(&bytes.Buffer{}, "items", func(func(any) error) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestPrintJSONStream_CallbackErrorAfterPartialWriteLeavesNoOutput(t *testing.T) {
	var buf bytes.Buffer
	want := errors.New("boom")
	err := PrintJSONStream(&buf, "items", func(writeItem func(any) error) error {
		if err := writeItem(map[string]any{"id": "one"}); err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output on error, got %q", buf.String())
	}
}

func TestPrintJSONStream_StaysInMemoryBelowThreshold(t *testing.T) {
	setStageOutputThresholdForTest(t, 1<<20)

	tempCalls := 0
	setStageOutputTempFileFactoryForTest(t, func(string) (*os.File, error) {
		tempCalls++
		return nil, errors.New("temp file should not be created")
	})

	var buf bytes.Buffer
	if err := PrintJSONStream(&buf, "items", func(writeItem func(any) error) error {
		return writeItem(map[string]any{"id": "one"})
	}); err != nil {
		t.Fatalf("PrintJSONStream failed: %v", err)
	}
	if tempCalls != 0 {
		t.Fatalf("temp file factory called %d times, want 0", tempCalls)
	}
}

func TestPrintJSONStream_SpillsToDiskAboveThreshold(t *testing.T) {
	setStageOutputThresholdForTest(t, 32)

	var tempCalls int
	tempDir := t.TempDir()
	setStageOutputTempFileFactoryForTest(t, func(pattern string) (*os.File, error) {
		tempCalls++
		return os.CreateTemp(tempDir, pattern)
	})

	largeValue := strings.Repeat("x", 256)
	var buf bytes.Buffer
	if err := PrintJSONStream(&buf, "items", func(writeItem func(any) error) error {
		return writeItem(map[string]any{"id": largeValue})
	}); err != nil {
		t.Fatalf("PrintJSONStream failed: %v", err)
	}
	if tempCalls != 1 {
		t.Fatalf("temp file factory called %d times, want 1", tempCalls)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 0 {
		var names []string
		for _, entry := range entries {
			names = append(names, filepath.Join(tempDir, entry.Name()))
		}
		t.Fatalf("expected temp files to be cleaned up, found %v", names)
	}
}

func TestPrintJSONStream_TempFileCreationError(t *testing.T) {
	setStageOutputThresholdForTest(t, 32)
	setStageOutputTempFileFactoryForTest(t, func(string) (*os.File, error) {
		return nil, errors.New("temp disabled")
	})

	largeValue := strings.Repeat("x", 256)
	err := PrintJSONStream(&bytes.Buffer{}, "items", func(writeItem func(any) error) error {
		return writeItem(map[string]any{"id": largeValue})
	})
	if err == nil || !strings.Contains(err.Error(), "could not create temp output") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintJSONStream_CopyError(t *testing.T) {
	err := PrintJSONStream(errWriter{}, "items", func(writeItem func(any) error) error {
		return writeItem(map[string]any{"id": "one"})
	})
	if !errors.Is(err, errTestWrite) {
		t.Fatalf("got %v, want %v", err, errTestWrite)
	}
}

func TestStageOutputBuffer_CopyToRewindError(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stage-output-*")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}

	buffer := &stageOutputBuffer{file: file}
	name := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(name) })

	err = buffer.CopyTo(&bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "could not rewind staged output") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStageOutputBuffer_SpillToDiskWriteError(t *testing.T) {
	buffer := newStageOutputBuffer("stage-output-*", 0)
	if _, err := buffer.buffer.WriteString("hello"); err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}

	file, err := os.CreateTemp(t.TempDir(), "stage-output-*")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	setStageOutputTempFileFactoryForTest(t, func(string) (*os.File, error) {
		return file, nil
	})

	err = buffer.spillToDisk()
	if err == nil {
		t.Fatal("expected spillToDisk error")
	}
	if _, statErr := os.Stat(name); !os.IsNotExist(statErr) {
		t.Fatalf("expected temp file cleanup, stat error=%v", statErr)
	}
}

func TestPrintJSONStreamWithJQ(t *testing.T) {
	var buf bytes.Buffer
	err := PrintJSONStreamWithJQ(&buf, "items", ".items | length", func(writeItem func(any) error) error {
		if err := writeItem(map[string]any{"id": "one"}); err != nil {
			return err
		}
		return writeItem(map[string]any{"id": "two"})
	})
	if err != nil {
		t.Fatalf("PrintJSONStreamWithJQ failed: %v", err)
	}

	if got := strings.TrimSpace(buf.String()); got != "2" {
		t.Fatalf("got %q, want %q", got, "2")
	}
}

func TestPrintJSONStreamWithJQ_InvalidJQ(t *testing.T) {
	err := PrintJSONStreamWithJQ(&bytes.Buffer{}, "items", ".[invalid", func(func(any) error) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected invalid jq error")
	}
}

func TestPrintJSONStreamWithJQ_CallbackError(t *testing.T) {
	want := errors.New("boom")
	err := PrintJSONStreamWithJQ(&bytes.Buffer{}, "items", ".items | length", func(func(any) error) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestPrintJSONStreamWithJQ_CallbackErrorAfterPartialWriteLeavesNoOutput(t *testing.T) {
	var buf bytes.Buffer
	want := errors.New("boom")
	err := PrintJSONStreamWithJQ(&buf, "items", ".items[] | .id", func(writeItem func(any) error) error {
		if err := writeItem(map[string]any{"id": "one"}); err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output on error, got %q", buf.String())
	}
}

func TestPrintJSONStreamWithJQ_ItemIteration(t *testing.T) {
	var buf bytes.Buffer
	err := PrintJSONStreamWithJQ(&buf, "items", ".items[] | .id", func(writeItem func(any) error) error {
		if err := writeItem(map[string]any{"id": "one"}); err != nil {
			return err
		}
		return writeItem(map[string]any{"id": "two"})
	})
	if err != nil {
		t.Fatalf("PrintJSONStreamWithJQ failed: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "\"one\"\n\"two\"" {
		t.Fatalf("got %q, want %q", got, "\"one\"\n\"two\"")
	}
}

func TestPrintJSONStreamWithJQ_EmptyExpressionFallsBackToJSONStream(t *testing.T) {
	var buf bytes.Buffer
	err := PrintJSONStreamWithJQ(&buf, "items", "", func(writeItem func(any) error) error {
		return writeItem(map[string]any{"id": "one"})
	})
	if err != nil {
		t.Fatalf("PrintJSONStreamWithJQ failed: %v", err)
	}

	var resp struct {
		Success bool                `json:"success"`
		Items   []map[string]string `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("streamed output is not valid JSON: %v\n%s", err, buf.String())
	}
	if !resp.Success || len(resp.Items) != 1 || resp.Items[0]["id"] != "one" {
		t.Fatalf("unexpected payload: %+v", resp)
	}
}

func TestRunJQ_NilQuery(t *testing.T) {
	err := runJQ(io.Discard, strings.NewReader(`{"ok":true}`), nil)
	if err == nil || !strings.Contains(err.Error(), "nil query") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunJQValues_MarshalError(t *testing.T) {
	err := runJQValues(io.Discard, &sliceIter{values: []any{make(chan int)}})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveStreamError(t *testing.T) {
	queryErr := errors.New("query failed")
	writeErr := errors.New("write failed")

	if got := resolveStreamError(queryErr, io.ErrClosedPipe); !errors.Is(got, queryErr) {
		t.Fatalf("got %v, want %v", got, queryErr)
	}
	if got := resolveStreamError(queryErr, writeErr); !errors.Is(got, writeErr) {
		t.Fatalf("got %v, want %v", got, writeErr)
	}
	if got := resolveStreamError(nil, syscall.EPIPE); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
	if got := resolveStreamError(nil, writeErr); !errors.Is(got, writeErr) {
		t.Fatalf("got %v, want %v", got, writeErr)
	}
}
