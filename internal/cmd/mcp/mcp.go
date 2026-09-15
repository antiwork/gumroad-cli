package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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

func NewDispatchCmd(newRoot func() *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:               "mcp-dispatch",
		Short:             "Serve one Gumroad MCP command dispatcher over stdio",
		Long:              "Start a compact Model Context Protocol server over stdin/stdout. The gumroad tool dispatches public CLI operations. Non-read operations require an exact-request confirmation token.",
		Args:              cobra.NoArgs,
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(c *cobra.Command, _ []string) error {
			return NewDispatchServer(newRoot).Run(c.Context(), &sdk.IOTransport{
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
			return executeCommand(ctx, newRoot, args)
		})
	})
	return server
}

type dispatchOperation struct {
	path        []string
	flags       *pflag.FlagSet
	readOnly    bool
	usage       string
	description string
	inputSchema map[string]any
}

const (
	dispatchConfirmationTTL        = 5 * time.Minute
	dispatchConfirmationTokenBytes = 24
	maxDispatchConfirmations       = 100
)

type dispatchConfirmation struct {
	request   string
	expiresAt time.Time
}

type dispatchConfirmations struct {
	mu      sync.Mutex
	pending map[string]dispatchConfirmation
}

type dispatchPlan struct {
	Operation    string   `json:"operation"`
	Command      []string `json:"command"`
	Confirmation string   `json:"confirmation"`
	ExpiresIn    string   `json:"expires_in"`
}

type dispatchRequest struct {
	Operation    string          `json:"operation"`
	Arguments    json.RawMessage `json:"arguments"`
	Confirm      bool            `json:"confirm"`
	Confirmation string          `json:"confirmation"`
}

type dispatchHelpRequest struct {
	Operation string `json:"operation"`
}

type dispatchDescription struct {
	Operation   string         `json:"operation"`
	Usage       string         `json:"usage"`
	Description string         `json:"description"`
	ReadOnly    bool           `json:"read_only"`
	InputSchema map[string]any `json:"input_schema"`
}

func newDispatchConfirmations() *dispatchConfirmations {
	return &dispatchConfirmations{pending: map[string]dispatchConfirmation{}}
}

func dispatchConfirmationRequest(operation string, command []string) string {
	request, _ := json.Marshal(struct {
		Operation string   `json:"operation"`
		Command   []string `json:"command"`
	}{operation, command})
	return string(request)
}

func (confirmations *dispatchConfirmations) issue(operation string, command []string) (string, error) {
	now := time.Now()
	confirmations.mu.Lock()
	defer confirmations.mu.Unlock()
	for token, confirmation := range confirmations.pending {
		if !confirmation.expiresAt.After(now) {
			delete(confirmations.pending, token)
		}
	}
	if len(confirmations.pending) >= maxDispatchConfirmations {
		return "", fmt.Errorf("too many pending confirmations; retry shortly")
	}
	for {
		raw := make([]byte, dispatchConfirmationTokenBytes)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		token := base64.RawURLEncoding.EncodeToString(raw)
		if _, exists := confirmations.pending[token]; !exists {
			confirmations.pending[token] = dispatchConfirmation{request: dispatchConfirmationRequest(operation, command), expiresAt: now.Add(dispatchConfirmationTTL)}
			return token, nil
		}
	}
}

func (confirmations *dispatchConfirmations) consume(token, operation string, command []string) bool {
	confirmations.mu.Lock()
	defer confirmations.mu.Unlock()
	confirmation, ok := confirmations.pending[token]
	if !ok {
		return false
	}
	if !confirmation.expiresAt.After(time.Now()) {
		delete(confirmations.pending, token)
		return false
	}
	if confirmation.request != dispatchConfirmationRequest(operation, command) {
		return false
	}
	delete(confirmations.pending, token)
	return true
}

func dispatchHelp(operations map[string]dispatchOperation, raw json.RawMessage) (*sdk.CallToolResult, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("{}")) {
		names := make([]string, 0, len(operations))
		for name := range operations {
			names = append(names, name)
		}
		sort.Strings(names)
		return toolResult(strings.Join(names, "\n"), false), nil
	}
	var input dispatchHelpRequest
	if err := json.Unmarshal(raw, &input); err != nil || input.Operation == "" {
		return toolResult("help arguments must include a non-empty operation", true), nil
	}
	op, ok := operations[input.Operation]
	if !ok {
		return toolResult(fmt.Sprintf("unknown operation %q; use operation: help", input.Operation), true), nil
	}
	detail, _ := json.Marshal(dispatchDescription{Operation: input.Operation, Usage: op.usage, Description: op.description, ReadOnly: op.readOnly, InputSchema: op.inputSchema})
	return toolResult(string(detail), false), nil
}

func NewDispatchServer(newRoot func() *cobra.Command) *sdk.Server {
	root := newRoot()
	server := sdk.NewServer(&sdk.Implementation{Name: "gumroad", Version: root.Version}, &sdk.ServerOptions{Instructions: "Use the gumroad tool with operation: help to discover CLI operations, then operation: help with arguments: {\"operation\":\"<name>\"} for a command schema. Reads execute immediately. Other operations return a plan with a short-lived confirmation token; repeat the exact request with confirm: true and the token to execute. File paths refer to the machine running the server; stdin is unavailable."})
	operations := map[string]dispatchOperation{}
	confirmations := newDispatchConfirmations()
	walk(root, nil, func(c *cobra.Command, path []string) {
		name := strings.ReplaceAll(strings.Join(path, "_"), "-", "_")
		flags := commandFlags(c)
		tool := commandTool(c, path, flags)
		readOnly := dispatchReadOnly(c, path)
		operations[name] = dispatchOperation{path: append([]string(nil), path...), flags: flags, readOnly: readOnly, usage: c.Use, description: dispatchOperationDescription(c, readOnly), inputSchema: tool.InputSchema.(map[string]any)}
	})
	server.AddTool(&sdk.Tool{
		Name:        "gumroad",
		Description: "Dispatch a Gumroad CLI operation. Use operation: help to list operations, then help arguments: {\"operation\":\"<name>\"} for its schema. Non-read operations require a plan confirmation token.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{
			"operation":    map[string]any{"type": "string", "description": "An operation from help, such as products_list."},
			"arguments":    map[string]any{"type": "object", "description": "CLI positional args and flags; for help, set operation to the name to describe."},
			"confirm":      map[string]any{"type": "boolean", "description": "Required with confirmation to execute an operation that is not read-only."},
			"confirmation": map[string]any{"type": "string", "description": "The plan confirmation token required with confirm: true."},
		}, "required": []string{"operation"}, "additionalProperties": false},
	}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		var input dispatchRequest
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil || input.Operation == "" {
			return toolResult("operation must be a non-empty string", true), nil
		}
		if input.Operation == "help" {
			return dispatchHelp(operations, input.Arguments)
		}
		op, ok := operations[input.Operation]
		if !ok {
			return toolResult(fmt.Sprintf("unknown operation %q; use operation: help", input.Operation), true), nil
		}
		args, err := commandArgs(op.path, op.flags, input.Arguments)
		if err != nil {
			return toolResult(err.Error(), true), nil
		}
		if !op.readOnly {
			if !input.Confirm {
				token, err := confirmations.issue(input.Operation, args)
				if err != nil {
					return toolResult(err.Error(), true), nil
				}
				plan, _ := json.Marshal(dispatchPlan{Operation: input.Operation, Command: args, Confirmation: token, ExpiresIn: dispatchConfirmationTTL.String()})
				return toolResult(string(plan), false), nil
			}
			if _, err := config.ResolveToken(); err != nil {
				return toolResult("Authentication unavailable. Run `gumroad auth login` or set GUMROAD_ACCESS_TOKEN.", true), nil
			}
			if input.Confirmation == "" || !confirmations.consume(input.Confirmation, input.Operation, args) {
				return toolResult("confirmation does not match an unexpired plan for this exact request", true), nil
			}
		} else if _, err := config.ResolveToken(); err != nil {
			return toolResult("Authentication unavailable. Run `gumroad auth login` or set GUMROAD_ACCESS_TOKEN.", true), nil
		}
		return executeCommand(ctx, newRoot, args)
	})
	return server
}

func dispatchReadOnly(c *cobra.Command, path []string) bool {
	if len(path) == 2 && path[0] == "pages" && path[1] == "pull" {
		return false
	}
	if c.Annotations["readOnlyHint"] == "true" {
		return true
	}
	switch c.Name() {
	case "list", "view", "get", "show", "status", "preview", "pull", "url", "search", "download":
		return true
	}
	return false
}

func dispatchOperationDescription(c *cobra.Command, readOnly bool) string {
	description := commandDescription(c)
	if !readOnly {
		description += "\nRequires a plan confirmation token."
	}
	return description + "\nAlways runs with --json --no-input --quiet; stdin is unavailable."
}

func executeCommand(ctx context.Context, newRoot func() *cobra.Command, args []string) (*sdk.CallToolResult, error) {
	var stdout, stderr bytes.Buffer
	cmd := newRoot()
	cmd.SetArgs(args)
	cmd.SetIn(unavailableInput{})
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.ExecuteContext(ctx); err != nil {
		return toolResult(strings.TrimSpace(stderr.String()+"\n"+err.Error()), true), nil
	}
	return toolResult(stdout.String(), false), nil
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
	case "auth", "completion", "skill", "mcp", "mcp-dispatch", "admin", "help":
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

func commandDescription(c *cobra.Command) string {
	description := strings.TrimSpace(c.Short + "\n" + c.Long)
	const maxDescriptionRunes = 2000
	if text := []rune(description); len(text) > maxDescriptionRunes {
		return string(text[:maxDescriptionRunes]) + "…"
	}
	return description
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
	description := commandDescription(c)
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
	case "list", "view", "get", "show", "status", "preview", "pull", "url", "search", "download":
		annotations.ReadOnlyHint = true
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
