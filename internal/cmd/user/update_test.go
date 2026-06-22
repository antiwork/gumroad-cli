package user

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestUpdate_SendsNameAndBio(t *testing.T) {
	var gotMethod, gotPath, gotName, gotBio string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotName = r.PostForm.Get("name")
		gotBio = r.PostForm.Get("bio")
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{
				"name":        "Jane Doe",
				"email":       "jane@example.com",
				"bio":         "I make great things.",
				"profile_url": "https://gumroad.com/jane",
			},
		})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--name", "Jane Doe", "--bio", "I make great things."})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != http.MethodPut || gotPath != userPath {
		t.Fatalf("got %s %s, want PUT %s", gotMethod, gotPath, userPath)
	}
	if gotName != "Jane Doe" {
		t.Fatalf("name = %q, want Jane Doe", gotName)
	}
	if gotBio != "I make great things." {
		t.Fatalf("bio = %q, want I make great things.", gotBio)
	}
	if !strings.Contains(out, "Updated user.") || !strings.Contains(out, "Jane Doe") {
		t.Fatalf("output missing updated user: %q", out)
	}
}

func TestUpdate_OmitsBioWhenFlagNotProvided(t *testing.T) {
	var gotName string
	var hasBio bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotName = r.PostForm.Get("name")
		_, hasBio = r.PostForm["bio"]
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{"name": "Solo Name", "email": "solo@example.com"},
		})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--name", "Solo Name"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotName != "Solo Name" {
		t.Fatalf("name = %q, want Solo Name", gotName)
	}
	if hasBio {
		t.Fatal("bio must be omitted when --bio is not provided")
	}
}

func TestUpdate_EmptyBioClearsField(t *testing.T) {
	var gotValues []string
	var hasBio bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm failed: %v", err)
		}
		gotValues, hasBio = r.PostForm["bio"]
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{"name": "Cleared", "email": "cleared@example.com"},
		})
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--bio", ""})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !hasBio {
		t.Fatal("bio must be sent when --bio is provided")
	}
	if len(gotValues) != 1 || gotValues[0] != "" {
		t.Fatalf("bio values = %#v, want one empty string", gotValues)
	}
}

func TestUpdate_RequiresAtLeastOneFlag(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("update without flags must not reach API")
	})

	cmd := newUpdateCmd()
	err := cmd.Execute()

	if err == nil || !strings.Contains(err.Error(), "at least one field to update must be provided") {
		t.Fatalf("expected missing flag error, got: %v", err)
	}
}

func TestUpdate_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{"name": "JSON Name", "email": "json@example.com", "bio": "JSON bio"},
		})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--name", "JSON Name"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp userResponse
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if resp.User.Name != "JSON Name" || resp.User.Bio != "JSON bio" {
		t.Fatalf("unexpected JSON response: %+v", resp.User)
	}
}

func TestUpdate_QuietSuppressesHumanOutput(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"user": map[string]any{"name": "Quiet", "email": "quiet@example.com"},
		})
	})

	cmd := testutil.Command(newUpdateCmd())
	cmd.SetArgs([]string{"--name", "Quiet"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if out != "" {
		t.Fatalf("quiet update should not print human output, got %q", out)
	}
}

func TestUpdate_DryRunDoesNotReachAPI(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry-run must not reach API")
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.DryRun(true), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Dry Run", "--bio", "Preview bio"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Dry run: PUT /user",
		"name: Dry Run",
		"bio: Preview bio",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q: %q", want, out)
		}
	}
}

func TestUpdate_ValidationErrorSurfaces(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"success":false,"message":"Name cannot contain colons (:) as it causes email delivery problems."}`)
	})

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--name", "Bad: Name"})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected API validation error")
	}
	if !strings.Contains(err.Error(), "colons") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewUserCmd_RegistersUpdate(t *testing.T) {
	cmd := NewUserCmd()

	found, _, err := cmd.Find([]string{"update"})
	if err != nil {
		t.Fatalf("Find(update) failed: %v", err)
	}
	if found == nil || found.Name() != "update" {
		t.Fatalf("update subcommand not registered, got %v", found)
	}
}
