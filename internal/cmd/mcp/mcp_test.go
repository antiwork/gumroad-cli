package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/antiwork/gumroad-cli/internal/cmd"
	mcpcmd "github.com/antiwork/gumroad-cli/internal/cmd/mcp"
	"github.com/antiwork/gumroad-cli/internal/config"
	"github.com/antiwork/gumroad-cli/internal/testutil"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func connect(t *testing.T, factory func() *cobra.Command) (*sdk.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := mcpcmd.NewServer(factory).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	clientSession, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession, ctx
}

func listTools(t *testing.T, session *sdk.ClientSession, ctx context.Context) map[string]*sdk.Tool {
	t.Helper()
	tools := map[string]*sdk.Tool{}
	params := &sdk.ListToolsParams{}
	for {
		result, err := session.ListTools(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range result.Tools {
			if tools[tool.Name] != nil {
				t.Fatalf("duplicate tool %s", tool.Name)
			}
			tools[tool.Name] = tool
		}
		if result.NextCursor == "" {
			break
		}
		params.Cursor = result.NextCursor
	}
	return tools
}

func call(t *testing.T, session *sdk.ClientSession, ctx context.Context, name string, args any, wantError bool) string {
	t.Helper()
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("protocol error: %v", err)
	}
	if result.IsError != wantError {
		t.Fatalf("IsError = %v, content = %+v", result.IsError, result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("unexpected content: %+v", result.Content)
	}
	return result.Content[0].(*sdk.TextContent).Text
}

func TestEnumerationAndMetadata(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	session, ctx := connect(t, cmd.NewRootCmd)
	tools := listTools(t, session, ctx)
	for _, name := range []string{"products_list", "products_create", "products_view", "sales_refund", "licenses_verify", "pages_push", "offer_codes_list", "variant_categories_list", "products_content_get", "user"} {
		if tools[name] == nil {
			t.Errorf("missing %s", name)
		}
	}
	validName := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for name, tool := range tools {
		if !validName.MatchString(name) {
			t.Errorf("invalid name: %s", name)
		}
		for _, excluded := range []string{"admin", "auth", "completion", "skill", "mcp", "help", "__update"} {
			if name == excluded || strings.HasPrefix(name, excluded+"_") {
				t.Errorf("exposed excluded command %s", name)
			}
		}
		if tool.Description == "" {
			t.Errorf("empty description for %s", name)
		}
	}
	for _, name := range []string{"products_list", "products_view", "pages_pull"} {
		if tools[name] == nil || !tools[name].Annotations.ReadOnlyHint {
			t.Errorf("missing read-only hint: %s", name)
		}
	}
	for _, name := range []string{"sales_refund", "products_delete", "files_abort", "licenses_verify", "pages_push", "emails_send", "products_create"} {
		a := tools[name].Annotations
		if a.DestructiveHint == nil || !*a.DestructiveHint {
			t.Errorf("missing destructive hint: %s", name)
		}
	}
	if !strings.Contains(tools["sales_refund"].Description, "without interactive confirmation") {
		t.Fatal("missing confirmation warning")
	}
	info := session.InitializeResult()
	if info.ServerInfo.Name != "gumroad" || info.ServerInfo.Version != cmd.NewRootCmd().Version {
		t.Fatalf("wrong server info: %+v", info.ServerInfo)
	}
	for _, text := range []string{"whole currency units", "opaque base64", "*_list", "*_view", "immediately"} {
		if !strings.Contains(info.Instructions, text) {
			t.Errorf("missing instruction %s", text)
		}
	}
}

func TestProductSchema(t *testing.T) {
	session, ctx := connect(t, cmd.NewRootCmd)
	tool := listTools(t, session, ctx)["products_create"]
	schema := tool.InputSchema.(map[string]any)
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("bad schema: %+v", schema)
	}
	props := schema["properties"].(map[string]any)
	for name, want := range map[string]string{"name": "string", "tag": "array", "json": "boolean", "args": "array"} {
		p := props[name].(map[string]any)
		if p["type"] != want || p["description"] == "" {
			t.Errorf("%s: %+v", name, p)
		}
		if want == "array" && p["items"].(map[string]any)["type"] != "string" {
			t.Errorf("wrong array items: %+v", p)
		}
	}
	for name, value := range props {
		if _, ok := value.(map[string]any)["default"]; ok {
			t.Errorf("default exposed for %s", name)
		}
	}
	if required, ok := schema["required"]; ok {
		t.Errorf("unexpected required: %v", required)
	}
}

func TestProductsListAndFreshFlags(t *testing.T) {
	const response = `{"success":true,"products":[{"id":"opaque-id","name":"Art Pack","new_field":"preserved"}]}`
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/products" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("wrong authentication")
		}
		testutil.RawJSON(t, w, response)
	})
	session, ctx := connect(t, cmd.NewRootCmd)
	out := call(t, session, ctx, "products_list", map[string]any{"jq": ".products[0].name"}, false)
	if strings.TrimSpace(out) != `"Art Pack"` {
		t.Fatalf("unexpected jq output: %s", out)
	}
	out = call(t, session, ctx, "products_list", map[string]any{"json": false, "quiet": false, "no-input": false}, false)
	var got, want any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(response), &want); err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("response mismatch: %s", out)
	}
}

func TestAPIErrorIsToolError(t *testing.T) {
	for _, status := range []int{400, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				testutil.RawJSON(t, w, `{"success":false,"message":"Invalid product request"}`)
			})
			session, ctx := connect(t, cmd.NewRootCmd)
			out := call(t, session, ctx, "products_list", map[string]any{}, true)
			if !strings.Contains(out, "Invalid product request") {
				t.Fatal(out)
			}
		})
	}
}

func TestNoTokenAndLoginAfterStart(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) { testutil.JSON(t, w, map[string]any{"products": []any{}}) })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvAccessToken, " \t\n")
	session, ctx := connect(t, cmd.NewRootCmd)
	for _, name := range []string{"products_list", "pages_push"} {
		out := call(t, session, ctx, name, map[string]any{}, true)
		if !strings.Contains(out, "gumroad auth login") || !strings.Contains(out, "GUMROAD_ACCESS_TOKEN") {
			t.Fatal(out)
		}
	}
	if err := config.Save(&config.Config{AccessToken: "new-test-token"}); err != nil {
		t.Fatal(err)
	}
	call(t, session, ctx, "products_list", map[string]any{}, false)
}

func TestMutationUsesYesAndPositionalArguments(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/resource_subscriptions/-opaque-id" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		testutil.JSON(t, w, map[string]any{})
	})
	session, ctx := connect(t, cmd.NewRootCmd)
	call(t, session, ctx, "webhooks_delete", map[string]any{"args": []string{"-opaque-id"}, "yes": false}, false)
}

func TestCommandValidationAndStdinIsolation(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected API request") })
	session, ctx := connect(t, cmd.NewRootCmd)
	call(t, session, ctx, "products_view", map[string]any{}, true)
	call(t, session, ctx, "pages_preview", map[string]any{"args": []string{"-"}}, true)
	call(t, session, ctx, "products_list", map[string]any{"args": []string{"--help"}}, true)
}

func testRoot() *cobra.Command {
	root := &cobra.Command{Use: "gumroad", Version: "fixture-version", SilenceErrors: true, SilenceUsage: true}
	for _, flag := range []string{"json", "quiet", "no-input", "yes"} {
		root.PersistentFlags().Bool(flag, false, flag)
	}
	leaf := &cobra.Command{Use: "show [id]", Short: "Test command", Long: strings.Repeat("é", 2100), RunE: func(c *cobra.Command, args []string) error {
		in, err := io.ReadAll(c.InOrStdin())
		if err == nil || errors.Is(err, io.EOF) {
			return fmt.Errorf("expected unavailable stdin error")
		}
		values := map[string]any{"args": args, "stdin": string(in)}
		values["count"], _ = c.Flags().GetInt("count")
		values["ratio"], _ = c.Flags().GetFloat64("ratio")
		values["tag"], _ = c.Flags().GetStringArray("tag")
		values["slice"], _ = c.Flags().GetStringSlice("slice")
		values["title"], _ = c.Flags().GetString("title")
		values["enabled"], _ = c.Flags().GetBool("enabled")
		values["yes"], _ = c.Flags().GetBool("yes")
		return json.NewEncoder(c.OutOrStdout()).Encode(values)
	}}
	leaf.Flags().Int("count", 1, "Count")
	leaf.Flags().Float64("ratio", 1, "Ratio")
	leaf.Flags().StringArray("tag", nil, "Tags")
	leaf.Flags().StringSlice("slice", nil, "CSV tags")
	leaf.Flags().String("title", "default", "Title")
	leaf.Flags().Bool("enabled", true, "Enabled")
	leaf.Flags().String("hidden", "", "Hidden")
	_ = leaf.Flags().MarkHidden("hidden")
	leaf.Flags().String("old", "", "Old")
	_ = leaf.Flags().MarkDeprecated("old", "use title")
	root.AddCommand(leaf)
	root.AddCommand(&cobra.Command{Use: "hidden", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }})
	root.AddCommand(&cobra.Command{Use: "old", Deprecated: "use show", RunE: func(*cobra.Command, []string) error { return nil }})
	root.AddCommand(&cobra.Command{Use: "empty"})
	root.AddCommand(&cobra.Command{Use: "custom", Annotations: map[string]string{"readOnlyHint": "true"}, RunE: func(c *cobra.Command, _ []string) error {
		_, _ = fmt.Fprintln(c.ErrOrStderr(), "diagnostic")
		return fmt.Errorf("command failed")
	}})
	return root
}

func TestFlagTypesAndExclusions(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "test-token")
	session, ctx := connect(t, testRoot)
	tools := listTools(t, session, ctx)
	if len(tools) != 2 || tools["show"] == nil || tools["custom"] == nil {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	if !tools["custom"].Annotations.ReadOnlyHint {
		t.Fatal("annotation ignored")
	}
	if len([]rune(tools["show"].Description)) > 2200 {
		t.Fatal("description not truncated")
	}
	props := tools["show"].InputSchema.(map[string]any)["properties"].(map[string]any)
	if props["hidden"] != nil || props["old"] != nil {
		t.Fatal("hidden/deprecated flags exposed")
	}
	for key, typ := range map[string]string{"count": "integer", "ratio": "number", "title": "string", "tag": "array", "enabled": "boolean"} {
		if props[key].(map[string]any)["type"] != typ {
			t.Errorf("%s type mismatch", key)
		}
	}
	out := call(t, session, ctx, "show", map[string]any{"args": []string{"-id", "a b"}, "count": -7, "ratio": 1.25, "title": "--quiet=false", "tag": []string{"a,b", "--help", "space value", ""}, "enabled": false, "slice": []string{"a,b", "line\nbreak", "quoted\"value"}}, false)
	var values map[string]any
	if err := json.Unmarshal([]byte(out), &values); err != nil {
		t.Fatal(err)
	}
	if values["count"] != float64(-7) || values["ratio"] != 1.25 || values["enabled"] != false || values["yes"] != true || values["title"] != "--quiet=false" || values["stdin"] != "" {
		t.Fatal(out)
	}
	if len(values["tag"].([]any)) != 4 || values["tag"].([]any)[0] != "a,b" {
		t.Fatal(out)
	}
	if len(values["slice"].([]any)) != 3 || values["slice"].([]any)[0] != "a,b" || values["slice"].([]any)[1] != "line\nbreak" {
		t.Fatal(out)
	}
	if values["args"].([]any)[0] != "-id" {
		t.Fatal(out)
	}
	out = call(t, session, ctx, "custom", nil, true)
	if !strings.Contains(out, "diagnostic") || !strings.Contains(out, "command failed") {
		t.Fatal(out)
	}
}

func TestInvalidToolArguments(t *testing.T) {
	t.Setenv(config.EnvAccessToken, "test-token")
	session, ctx := connect(t, testRoot)
	for _, raw := range []string{`[]`, `null`, `{"args":"id"}`, `{"args":null}`, `{"missing":"x"}`, `{"hidden":"x"}`, `{"old":"x"}`, `{"help":true}`, `{"title":5}`, `{"title":null}`, `{"enabled":"true"}`, `{"count":1.5}`, `{"count":"5"}`, `{"count":true}`, `{"ratio":"1"}`, `{"tag":"one"}`, `{"tag":[5]}`, `{"tag":[null]}`, `{"args":[null]}`} {
		t.Run(raw, func(t *testing.T) { call(t, session, ctx, "show", json.RawMessage(raw), true) })
	}
}

func TestStdioCommandInProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	defer serverRead.Close()
	defer serverWrite.Close()
	root := cmd.NewRootCmd()
	root.SetArgs([]string{"mcp"})
	root.SetIn(serverRead)
	root.SetOut(serverWrite)
	root.SetErr(io.Discard)
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()
	client, err := sdk.NewClient(&sdk.Implementation{Name: "stdio-test", Version: "1"}, nil).Connect(ctx, &sdk.IOTransport{Reader: clientRead, Writer: clientWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tools := listTools(t, client, ctx); tools["products_list"] == nil {
		t.Fatal("missing products_list")
	}
	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("server did not stop on EOF")
	}
}
