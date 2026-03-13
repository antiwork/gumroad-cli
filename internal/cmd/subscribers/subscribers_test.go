package subscribers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestList_SendsPaginatedTrue(t *testing.T) {
	var gotPath, gotQuery string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{
			"subscribers": []map[string]any{
				{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
			},
		})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "prod1"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotPath != "/products/prod1/subscribers" {
		t.Errorf("got path %q, want /products/prod1/subscribers", gotPath)
	}
	if !strings.Contains(gotQuery, "paginated=true") {
		t.Errorf("should send paginated=true by default, got: %q", gotQuery)
	}
}

func TestList_ProductRequired(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach API without --product")
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error without --product")
	}
	if !strings.Contains(err.Error(), "--product") {
		t.Errorf("error should mention --product: %v", err)
	}
}

func TestList_EmailFilter(t *testing.T) {
	var gotQuery string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.JSON(t, w, map[string]any{"subscribers": []any{}})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1", "--email", "test@example.com"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(gotQuery, "email=test%40example.com") {
		t.Errorf("query missing email filter: %q", gotQuery)
	}
}

func TestList_Pagination(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"subscribers": []map[string]any{
				{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
			},
			"next_page_key": "next123",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "next123") {
		t.Errorf("expected pagination hint, got: %q", out)
	}
}

func TestList_AllFetchesAllPages(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
				},
				"next_page_key": "next123",
			})
		case "next123":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub2", "email_address": "b@c.com", "status": "alive", "created_at": "2024-01-16"},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Subscribers []map[string]any `json:"subscribers"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if len(resp.Subscribers) != 2 {
		t.Fatalf("got %d subscribers, want 2", len(resp.Subscribers))
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestList_SinglePageDoesNotWalkPages(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if pageKey := r.URL.Query().Get("page_key"); pageKey != "" {
			t.Fatalf("unexpected page_key %q", pageKey)
		}
		testutil.JSON(t, w, map[string]any{
			"subscribers": []map[string]any{
				{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
			},
			"next_page_key": "next123",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Subscribers []map[string]any `json:"subscribers"`
		NextPageKey string           `json:"next_page_key"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if requests != 1 {
		t.Fatalf("got %d requests, want 1", requests)
	}
	if len(resp.Subscribers) != 1 {
		t.Fatalf("got %d subscribers, want 1", len(resp.Subscribers))
	}
	if resp.NextPageKey != "next123" {
		t.Fatalf("got next_page_key=%q, want next123", resp.NextPageKey)
	}
}

func TestList_AllJQFetchesAllPages(t *testing.T) {
	requests := 0
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
				},
				"next_page_key": "next123",
			})
		case "next123":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub2", "email_address": "b@c.com", "status": "alive", "created_at": "2024-01-16"},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.JQ(".subscribers | length"))
	cmd.SetArgs([]string{"--product", "p1", "--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if strings.TrimSpace(out) != "2" {
		t.Fatalf("got %q, want 2", out)
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

func TestList_AllPlainOutputStreamsAllPages(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
				},
				"next_page_key": "next123",
			})
		case "next123":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub2", "email_address": "b@c.com", "status": "cancelled", "created_at": "2024-01-16"},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1", "--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "sub1") || !strings.Contains(out, "sub2") {
		t.Fatalf("expected both pages in plain output, got %q", out)
	}
}

func TestList_AllOutputStreamsAllPages(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
				},
				"next_page_key": "next123",
			})
		case "next123":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub2", "email_address": "b@c.com", "status": "cancelled", "created_at": "2024-01-16"},
				},
			})
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "sub1") || !strings.Contains(out, "sub2") || !strings.Contains(out, "cancelled") {
		t.Fatalf("expected streamed table output, got %q", out)
	}
}

func TestList_AllOutputEmpty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"subscribers": []any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--all"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "No subscribers found") {
		t.Fatalf("expected empty message, got %q", out)
	}
}

func TestList_PaginationHintPreservesFilters(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"subscribers": []map[string]any{
				{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
			},
			"next_page_key": "next123",
		})
	})
	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "gumroad subscribers list --product p1 --email buyer@example.com --page-key next123"
	if !strings.Contains(out, want) {
		t.Fatalf("expected replayable pagination hint %q in %q", want, out)
	}
}

func TestView_CorrectEndpoint(t *testing.T) {
	var gotPath string
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		testutil.JSON(t, w, map[string]any{
			"subscriber": map[string]any{
				"id": "sub1", "email_address": "a@b.com", "status": "alive",
				"product_name": "Membership", "created_at": "2024-01-15",
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"sub1"}) })

	if gotPath != "/subscribers/sub1" {
		t.Errorf("got path %q, want /subscribers/sub1", gotPath)
	}
	if !strings.Contains(out, "a@b.com") || !strings.Contains(out, "Membership") {
		t.Errorf("output missing subscriber data: %q", out)
	}
}

func TestView_StatusColoring(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"subscriber": map[string]any{
				"id": "sub1", "email_address": "a@b.com", "status": "cancelled",
				"product_name": "Pro", "created_at": "2024-01-15",
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"sub1"}) })
	if !strings.Contains(out, "cancelled") {
		t.Errorf("output should show cancelled status: %q", out)
	}
}

func TestList_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"subscribers": []map[string]any{
				{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestList_RawFixture(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, testutil.Fixture(t, "testdata/list_raw.json"))
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "raw@example.com") || !strings.Contains(out, "alive") {
		t.Errorf("raw fixture output missing subscriber data: %q", out)
	}
}

func TestList_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"subscribers": []map[string]any{
				{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
			},
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "sub1") {
		t.Errorf("plain output missing data: %q", out)
	}
}

func TestList_Empty(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{"subscribers": []any{}})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "No subscribers found") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestList_EmptyPageStillShowsPaginationHint(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"subscribers":   []any{},
			"next_page_key": "next123",
		})
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--email", "buyer@example.com"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "No subscribers found on this page.") {
		t.Fatalf("expected empty-page message, got %q", out)
	}
	want := "gumroad subscribers list --product p1 --email buyer@example.com --page-key next123"
	if !strings.Contains(out, want) {
		t.Fatalf("expected pagination hint %q in %q", want, out)
	}
}

func TestView_JSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"subscriber": map[string]any{
				"id": "sub1", "email_address": "a@b.com", "status": "alive",
				"product_name": "Membership", "created_at": "2024-01-15",
			},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"sub1"}) })
	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestView_Plain(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"subscriber": map[string]any{
				"id": "sub1", "email_address": "a@b.com", "status": "alive",
				"product_name": "Membership", "created_at": "2024-01-15",
			},
		})
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"sub1"}) })
	if !strings.Contains(out, "sub1") || !strings.Contains(out, "Membership") {
		t.Errorf("plain view missing data: %q", out)
	}
}

func TestView_AliveStatus(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"subscriber": map[string]any{
				"id": "sub1", "email_address": "a@b.com", "status": "alive",
				"product_name": "Pro", "created_at": "2024-01-15",
			},
		})
	})

	cmd := newViewCmd()
	out := testutil.CaptureStdout(func() { _ = cmd.RunE(cmd, []string{"sub1"}) })
	if !strings.Contains(out, "alive") {
		t.Errorf("should show alive status: %q", out)
	}
}

func TestList_AliveStatusColoring(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{
			"subscribers": []map[string]any{
				{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
				{"id": "sub2", "email_address": "b@c.com", "status": "cancelled", "created_at": "2024-02-15"},
			},
		})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
	if !strings.Contains(out, "alive") || !strings.Contains(out, "cancelled") {
		t.Errorf("should show both statuses: %q", out)
	}
}

func TestList_APIError(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		testutil.JSON(t, w, map[string]any{"message": "Error"})
	})

	cmd := newListCmd()
	cmd.SetArgs([]string{"--product", "p1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewSubscribersCmd_Subcommands(t *testing.T) {
	cmd := NewSubscribersCmd()

	if cmd.Use != "subscribers" {
		t.Fatalf("got use=%q, want %q", cmd.Use, "subscribers")
	}
	for _, name := range []string{"list", "view"} {
		if child, _, err := cmd.Find([]string{name}); err != nil || child == nil || child.Name() != name {
			t.Fatalf("expected subcommand %q to be registered, got child=%v err=%v", name, child, err)
		}
	}
}

func TestList_All_InvalidJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"subscribers":`)
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--all"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}

func TestList_All_SecondPageInvalidJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
				},
				"next_page_key": "next123",
			})
		case "next123":
			testutil.RawJSON(t, w, `{"subscribers":`)
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"--product", "p1", "--all"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected second-page parse error, got: %v", err)
	}
}

func TestList_AllPlainOutput_SecondPageInvalidJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
				},
				"next_page_key": "next123",
			})
		case "next123":
			testutil.RawJSON(t, w, `{"subscribers":`)
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"--product", "p1", "--all"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected second-page parse error, got: %v", err)
	}
}

func TestList_AllOutput_SecondPageInvalidJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page_key") {
		case "":
			testutil.JSON(t, w, map[string]any{
				"subscribers": []map[string]any{
					{"id": "sub1", "email_address": "a@b.com", "status": "alive", "created_at": "2024-01-15"},
				},
				"next_page_key": "next123",
			})
		case "next123":
			testutil.RawJSON(t, w, `{"subscribers":`)
		default:
			t.Fatalf("unexpected page_key %q", r.URL.Query().Get("page_key"))
		}
	})

	cmd := testutil.Command(newListCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--product", "p1", "--all"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected second-page parse error, got: %v", err)
	}
}

func TestView_InvalidJSON(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.RawJSON(t, w, `{"subscriber":`)
	})

	cmd := newViewCmd()
	err := cmd.RunE(cmd, []string{"sub1"})
	if err == nil || !strings.Contains(err.Error(), "could not parse response") {
		t.Fatalf("expected parse error, got: %v", err)
	}
}
