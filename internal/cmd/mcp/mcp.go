package mcp

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/config"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const instructions = "Prices are in whole currency units. IDs are opaque base64 strings returned by list calls; use *_list before *_view. Mutations run immediately without interactive confirmation. File paths refer to the machine running gumroad; stdin input is unavailable."

// The factory avoids an import cycle with cmd and gives each call its own flag state.
func NewMcpCmd(newRoot func() *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:     "mcp",
		Short:   "Serve Gumroad tools over MCP stdio",
		Long:    "Start a Model Context Protocol server over stdin/stdout. Tools are generated from the public CLI commands. Uses the same stored login or GUMROAD_ACCESS_TOKEN as the CLI. Mutations run without interactive confirmation. Only connect trusted clients; tools can read and write local files.",
		Example: "  gumroad mcp",
		Args:    cobra.NoArgs,
		// Starting the protocol must not refresh skills or launch update-check processes.
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(c *cobra.Command, _ []string) error {
			server := NewServer(newRoot)
			return server.Run(c.Context(), &sdk.IOTransport{
				Reader: io.NopCloser(c.InOrStdin()),
				Writer: writerCloser{c.OutOrStdout()},
			})
		},
	}
}

type unavailableInput struct{}

func (unavailableInput) Read([]byte) (int, error) {
	return 0, fmt.Errorf("stdin is unavailable over MCP; supply a file path or a flag instead")
}

type writerCloser struct{ io.Writer }

func (writerCloser) Close() error { return nil }

func NewServer(newRoot func() *cobra.Command) *sdk.Server {
	root := newRoot()
	server := sdk.NewServer(&sdk.Implementation{Name: "gumroad", Version: root.Version}, &sdk.ServerOptions{Instructions: instructions})
	walk(root, nil, func(c *cobra.Command, path []string) {
		flags := commandFlags(c)
		tool := commandTool(c, path, flags)
		server.AddTool(tool, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			if _, err := config.ResolveToken(); err != nil {
				return toolResult("Authentication unavailable. Run `gumroad auth login` or set GUMROAD_ACCESS_TOKEN.", true), nil
			}
			args, err := commandArgs(path, flags, req.Params.Arguments)
			if err != nil {
				return toolResult(err.Error(), true), nil
			}
			var stdout, stderr bytes.Buffer
			cmd := newRoot()
			cmd.SetArgs(args)
			// Commands that read files from stdin must not consume protocol messages.
			cmd.SetIn(unavailableInput{})
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			if err := cmd.ExecuteContext(ctx); err != nil {
				return toolResult(strings.TrimSpace(stderr.String()+"\n"+err.Error()), true), nil
			}
			return toolResult(stdout.String(), false), nil
		})
	})
	return server
}

func walk(c *cobra.Command, path []string, visit func(*cobra.Command, []string)) {
	if c.Hidden || c.Deprecated != "" || excludedCommand(c.Name()) {
		return
	}
	// A runnable group such as `gumroad user` is a tool in its own right as well as a
	// parent: it reads the account, while its children edit it.
	if len(path) > 0 && c.Runnable() {
		visit(c, path)
	}
	for _, child := range c.Commands() {
		walk(child, append(append([]string(nil), path...), child.Name()), visit)
	}
}

func excludedCommand(name string) bool {
	switch name {
	case "auth", "completion", "skill", "mcp", "admin", "help":
		return true
	}
	return false
}

func commandFlags(c *cobra.Command) *pflag.FlagSet {
	flags := pflag.NewFlagSet(c.Name(), pflag.ContinueOnError)
	flags.AddFlagSet(c.LocalFlags())
	flags.AddFlagSet(c.InheritedFlags())
	return flags
}

func flagType(flag *pflag.Flag) string {
	switch flag.Value.Type() {
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "count":
		return "integer"
	case "float32", "float64":
		return "number"
	case "stringArray", "stringSlice":
		return "array"
	default:
		return "string"
	}
}

func commandTool(c *cobra.Command, path []string, flags *pflag.FlagSet) *sdk.Tool {
	properties := map[string]any{
		"args": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Positional arguments in command order: " + c.Use},
	}
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden || flag.Deprecated != "" || flag.Name == "help" {
			return
		}
		property := map[string]any{"type": flagType(flag), "description": flag.Usage}
		if flagType(flag) == "array" {
			property["items"] = map[string]any{"type": "string"}
		}
		properties[flag.Name] = property
	})
	description := strings.TrimSpace(c.Short + "\n" + c.Long)
	const maxDescriptionRunes = 2000
	if text := []rune(description); len(text) > maxDescriptionRunes {
		description = string(text[:maxDescriptionRunes]) + "…"
	}
	if flags.Lookup("yes") != nil {
		description += "\nRuns without interactive confirmation."
	}
	description += "\nAlways runs with --json --no-input --quiet; stdin is unavailable."
	// Hints are conservative: anything not clearly a read is flagged destructive, because
	// calls pass --yes and MCP clients gate their own confirmation on these flags.
	// `verify` stays out of the read-only set: `licenses verify` increments the use count
	// unless --no-increment is passed.
	annotations := &sdk.ToolAnnotations{}
	switch c.Name() {
	case "list", "view", "get", "show", "status", "preview", "url", "search", "download":
		annotations.ReadOnlyHint = true
	case "pull":
		if len(path) != 2 || path[0] != "pages" {
			annotations.ReadOnlyHint = true
		} else {
			value := true
			annotations.DestructiveHint = &value
		}
	default:
		value := true
		annotations.DestructiveHint = &value
	}
	if c.Annotations["readOnlyHint"] == "true" {
		annotations.ReadOnlyHint = true
		annotations.DestructiveHint = nil
	}
	return &sdk.Tool{
		Name:        strings.ReplaceAll(strings.Join(path, "_"), "-", "_"),
		Description: description,
		InputSchema: map[string]any{"type": "object", "properties": properties, "additionalProperties": false},
		Annotations: annotations,
	}
}

func commandArgs(path []string, flags *pflag.FlagSet, raw json.RawMessage) ([]string, error) {
	values := map[string]json.RawMessage{}
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &values); err != nil || values == nil {
			return nil, fmt.Errorf("tool arguments must be a JSON object")
		}
	}
	var positional []string
	if rawArgs, ok := values["args"]; ok {
		if err := decodeValue(rawArgs, &positional); err != nil {
			return nil, fmt.Errorf("args must be an array of strings")
		}
		delete(values, "args")
	}
	args := append([]string(nil), path...)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		flag := flags.Lookup(key)
		if flag == nil || flag.Hidden || flag.Deprecated != "" || key == "help" {
			return nil, fmt.Errorf("unknown flag %q", key)
		}
		parts, err := flagValues(flag, values[key])
		if err != nil {
			return nil, fmt.Errorf("--%s must be of type %s", key, flagType(flag))
		}
		for _, part := range parts {
			args = append(args, "--"+key+"="+part)
		}
	}
	args = append(args, "--json", "--no-input", "--quiet")
	if flags.Lookup("yes") != nil {
		args = append(args, "--yes")
	}
	// Flag-looking IDs and file names are data, never additional CLI options.
	args = append(args, "--")
	return append(args, positional...), nil
}

func flagValues(flag *pflag.Flag, raw json.RawMessage) ([]string, error) {
	switch flagType(flag) {
	case "boolean":
		var value bool
		err := decodeValue(raw, &value)
		return []string{strconv.FormatBool(value)}, err
	case "integer", "number":
		var number json.Number
		if len(raw) == 0 || raw[0] == '"' {
			return nil, fmt.Errorf("expected number")
		}
		err := decodeValue(raw, &number)
		if err == nil && flagType(flag) == "integer" && strings.ContainsAny(number.String(), ".eE") {
			err = fmt.Errorf("expected integer")
		}
		return []string{number.String()}, err
	case "array":
		var values []string
		if err := decodeValue(raw, &values); err != nil {
			return nil, err
		}
		if flag.Value.Type() == "stringSlice" {
			var encoded bytes.Buffer
			writer := csv.NewWriter(&encoded)
			_ = writer.Write(values)
			writer.Flush()
			return []string{strings.TrimSuffix(encoded.String(), "\n")}, nil
		}
		return values, nil
	default:
		var value string
		err := decodeValue(raw, &value)
		return []string{value}, err
	}
}

func decodeValue(raw json.RawMessage, target any) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("null is not a flag value")
	}
	if values, ok := target.(*[]string); ok {
		var elements []json.RawMessage
		if err := json.Unmarshal(raw, &elements); err != nil {
			return err
		}
		for _, element := range elements {
			var value string
			if err := decodeValue(element, &value); err != nil {
				return err
			}
			*values = append(*values, value)
		}
		return nil
	}
	return json.Unmarshal(raw, target)
}

func toolResult(text string, isError bool) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}, IsError: isError}
}
