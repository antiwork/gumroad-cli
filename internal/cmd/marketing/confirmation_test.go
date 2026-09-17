package marketing

import (
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestNoninteractiveConfirmationBindsReviewedPreview(t *testing.T) {
	for _, verb := range []string{"approve", "schedule"} {
		for _, token := range []string{"", "old-preview", "current-preview"} {
			t.Run(verb+"/"+token, func(t *testing.T) {
				writes := 0
				testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPost {
						writes++
						if err := r.ParseForm(); err != nil {
							t.Error(err)
						}
						if r.PostForm.Get("confirmation_token") != token {
							t.Error("did not submit the reviewed token")
						}
					}
					item := actionPayload()
					item["confirmation_token"] = "current-preview"
					item["post_text"] = "Changed after recommendation"
					testutil.JSON(t, w, map[string]any{"marketing_action": item, "handle": "new_account"})
				})
				command := testutil.Command(NewMarketingCmd(), testutil.Yes(true))
				command.SetArgs([]string{verb, "action-id", "--confirmation-token", token})
				err := command.Execute()
				if token == "current-preview" {
					if err != nil || writes != 1 {
						t.Fatalf("confirmed request: writes=%d, err=%v", writes, err)
					}
				} else {
					if err == nil || writes != 0 {
						t.Fatalf("unreviewed request: writes=%d, err=%v", writes, err)
					}
					if token != "" && !strings.Contains(err.Error(), "post changed") {
						t.Fatalf("wrong error: %v", err)
					}
				}
			})
		}
	}
}
