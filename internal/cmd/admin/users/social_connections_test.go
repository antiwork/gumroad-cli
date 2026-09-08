package users

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

const socialEvidenceFixture = `{"success":true,"user_id":"2245593582708","future_field":"preserved","social_connections":[{"platform":"instagram","uid":"1234","handle":"seller","currently_linked":false,"account_created_at":null,"follower_count":null,"post_count":0,"last_posted_at":null,"last_verified_at":"2026-09-01T12:00:00Z","shared_identity_user_count":2}]}`

func TestSocialConnectionsLookupAndOutput(t *testing.T) {
	for _, flag := range []string{"user-id", "email", "username"} {
		t.Run(flag, func(t *testing.T) {
			value := map[string]string{"user-id": "2245593582708", "email": "seller@example.com", "username": "seller"}[flag]
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/internal/admin/users/social_connections" || r.URL.Query().Get(strings.ReplaceAll(flag, "-", "_")) != value {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				testutil.RawJSON(t, w, socialEvidenceFixture)
			})
			var out bytes.Buffer
			cmd := testutil.Command(NewUsersCmd(), testutil.Quiet(false), testutil.Stdout(&out))
			cmd.SetArgs([]string{"social-connections", "--" + flag, value})
			testutil.MustExecute(t, cmd)
			for _, want := range []string{"not payout approval", "Currently linked: false", "Followers: unknown", "Posts: 0", "Account created: unknown", "Last verified: 2026-09-01T12:00:00Z", "Other users sharing identity: 2"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q in %s", want, out.String())
				}
			}
		})
	}
}

func TestSocialConnectionsOutputModes(t *testing.T) {
	for _, mode := range []string{"json", "plain", "quiet", "jq"} {
		t.Run(mode, func(t *testing.T) {
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) { testutil.RawJSON(t, w, socialEvidenceFixture) })
			var out bytes.Buffer
			cmd := testutil.Command(newSocialConnectionsCmd(), testutil.Stdout(&out), testutil.JSONOutput())
			if mode == "plain" {
				cmd = testutil.Command(newSocialConnectionsCmd(), testutil.Stdout(&out), testutil.PlainOutput())
			}
			if mode == "quiet" {
				cmd = testutil.Command(newSocialConnectionsCmd(), testutil.Stdout(&out), testutil.Quiet(true))
			}
			if mode == "jq" {
				cmd = testutil.Command(newSocialConnectionsCmd(), testutil.Stdout(&out), testutil.JQ(".social_connections[0].currently_linked"))
			}
			cmd.SetArgs([]string{"--user-id", "2245593582708"})
			testutil.MustExecute(t, cmd)
			switch mode {
			case "json":
				var got, want any
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(socialEvidenceFixture), &want); err != nil {
					t.Fatal(err)
				}
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(want)
				if string(a) != string(b) {
					t.Fatalf("JSON changed: %s", out.String())
				}
			case "plain":
				want := "instagram\t1234\tseller\tfalse\t2026-09-01T12:00:00Z\t\tunknown\t0\t\t2\n"
				if out.String() != want {
					t.Fatalf("plain = %q", out.String())
				}
			case "quiet":
				if out.Len() != 0 {
					t.Fatalf("quiet output: %s", out.String())
				}
			case "jq":
				if strings.TrimSpace(out.String()) != "false" {
					t.Fatalf("jq: %s", out.String())
				}
			}
		})
	}
}

func TestSocialConnectionsEmptyAndEscaping(t *testing.T) {
	for _, empty := range []bool{true, false} {
		t.Run(map[bool]string{true: "empty", false: "escaping"}[empty], func(t *testing.T) {
			testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
				if empty {
					testutil.RawJSON(t, w, `{"success":true,"social_connections":[]}`)
					return
				}
				testutil.RawJSON(t, w, strings.Replace(socialEvidenceFixture, `"seller"`, `"seller\n\u001b[31m"`, 1))
			})
			var out bytes.Buffer
			cmd := testutil.Command(newSocialConnectionsCmd(), testutil.Stdout(&out), testutil.Quiet(false))
			cmd.SetArgs([]string{"--username", "seller"})
			testutil.MustExecute(t, cmd)
			if empty && !strings.Contains(out.String(), "No stored social verification evidence.") {
				t.Fatal(out.String())
			}
			if !empty && (strings.Contains(out.String(), "seller\n") || strings.Contains(out.String(), "\x1b[31m")) {
				t.Fatalf("unescaped output %q", out.String())
			}
		})
	}
}

func TestSocialConnectionsErrors(t *testing.T) {
	t.Run("missing lookup", func(t *testing.T) {
		testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") })
		cmd := testutil.Command(newSocialConnectionsCmd())
		if cmd.Execute() == nil {
			t.Fatal("expected missing target error")
		}
	})
	t.Run("server refusal", func(t *testing.T) {
		testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			testutil.RawJSON(t, w, `{"success":false,"message":"denied"}`)
		})
		cmd := testutil.Command(newSocialConnectionsCmd())
		cmd.SetArgs([]string{"--username", "seller"})
		if cmd.Execute() == nil {
			t.Fatal("expected API error")
		}
	})
}
