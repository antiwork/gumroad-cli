package mcp_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/cmd"
	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestMarketingToolsRequireExplicitConfirmation(t *testing.T) {
	writes := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writes++
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.PostForm.Get("idempotency_key") != "same-key" {
				t.Error("missing idempotency key")
			}
		}
		testutil.RawJSON(t, w, `{"success":true,"handle":"seller","marketing_action":{"id":"action-id","idempotency_key":"same-key","confirmation_token":"preview-token","status":"approved","post_text":"Launch\n\nhttps://example.com/link","link_url":"https://example.com/link"}}`)
	})
	session, ctx := connect(t, cmd.NewRootCmd)
	tools := listTools(t, session, ctx)
	for _, name := range []string{"marketing_recommend", "marketing_approve", "marketing_schedule", "marketing_cancel", "marketing_status"} {
		if tools[name] == nil {
			t.Fatalf("missing %s", name)
		}
	}
	for _, name := range []string{"marketing_approve", "marketing_schedule", "marketing_cancel"} {
		tool := tools[name]
		if tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint || !strings.Contains(tool.Description, "yes=true only after confirmation") {
			t.Fatalf("unsafe tool: %+v", tool)
		}
		for _, args := range []map[string]any{{"args": []string{"action-id"}}, {"args": []string{"action-id"}, "yes": false}} {
			before := writes
			text := call(t, session, ctx, name, args, true)
			if writes != before || !strings.Contains(text, "confirmation required") || !strings.Contains(text, "@seller") || !strings.Contains(text, "Launch") || !strings.Contains(text, "https://example.com/link") {
				t.Fatalf("unconfirmed mutation or missing preview: %s", text)
			}
		}
		before := writes
		call(t, session, ctx, name, map[string]any{"args": []string{"action-id"}, "yes": true}, false)
		if writes != before+1 {
			t.Fatalf("expected one confirmed write")
		}
	}
}
