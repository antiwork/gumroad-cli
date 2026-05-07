package products

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func sampleViewPayload() map[string]any {
	p := sampleProduct("abc123", "Art Pack")
	p["files"] = []map[string]any{
		{
			"id":           "f_1",
			"display_name": "Big Guide",
			"file_name":    "big-guide.pdf",
			"extension":    "PDF",
			"filegroup":    "document",
			"file_size":    1048576,
			"created_at":   "2026-05-01T12:00:00Z",
			"deleted_at":   nil,
		},
		{
			"id":           "f_2",
			"display_name": "Cheat Sheet",
			"file_name":    "cheat-sheet.pdf",
			"extension":    "PDF",
			"filegroup":    "document",
			"file_size":    70016,
			"created_at":   "2026-05-02T08:00:00Z",
			"deleted_at":   nil,
		},
	}
	return map[string]any{"product": p}
}

func TestViewRequiresProductID(t *testing.T) {
	cmd := newViewCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing argument error")
	}
	if !strings.Contains(err.Error(), "missing required argument") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestViewUsesInternalAdminEndpointAndRendersHumanOutput(t *testing.T) {
	var gotMethod, gotPath, gotAuth string

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		testutil.JSON(t, w, sampleViewPayload())
	})

	cmd := testutil.Command(newViewCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"abc123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "GET" || gotPath != "/internal/admin/products/abc123" {
		t.Fatalf("got %s %s, want GET /internal/admin/products/abc123", gotMethod, gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("got auth %q, want Bearer admin-token", gotAuth)
	}
	for _, want := range []string{
		"Art Pack",
		"ID: abc123",
		"URL: https://gumroad.com/l/abc123",
		"Seller: seller@example.com (u_123)",
		"Price: 200.00 USD",
		"Status: alive",
		"Created: 2026-05-01T12:00:00Z",
		"Description:",
		"A short description",
		"Files (2):",
		"Big Guide",
		"big-guide.pdf",
		"1.0 MB",
		"Cheat Sheet",
		"68.4 KB",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestViewMarksDeletedProductAndFile(t *testing.T) {
	payload := sampleViewPayload()
	prod := payload["product"].(map[string]any)
	prod["deleted_at"] = "2026-04-15T10:00:00Z"
	prod["alive"] = false
	files := prod["files"].([]map[string]any)
	files[1]["deleted_at"] = "2026-04-16T11:00:00Z"

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newViewCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"abc123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	for _, want := range []string{
		"Art Pack (deleted)",
		"Status: deleted",
		"Deleted: 2026-04-15T10:00:00Z",
		"Cheat Sheet (deleted)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestViewRendersFilesNoneWhenEmpty(t *testing.T) {
	payload := sampleViewPayload()
	prod := payload["product"].(map[string]any)
	prod["files"] = []map[string]any{}

	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, payload)
	})

	cmd := testutil.Command(newViewCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"abc123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if !strings.Contains(out, "Files: none") {
		t.Errorf("expected 'Files: none' for product without files: %q", out)
	}
}

func TestViewProductNotFoundSurfacesMessage(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"message": "Product not found",
		})
	})

	cmd := testutil.Command(newViewCmd())
	cmd.SetArgs([]string{"missing"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected product-not-found error")
	}
	if !strings.Contains(err.Error(), "Product not found") {
		t.Errorf("missing underlying message: %v", err)
	}
}

func TestViewPlainOutput(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleViewPayload())
	})

	cmd := testutil.Command(newViewCmd(), testutil.PlainOutput())
	cmd.SetArgs([]string{"abc123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	want := "abc123\tArt Pack\t200.00 USD\talive\tseller@example.com\t2\t2026-05-01T12:00:00Z\thttps://gumroad.com/l/abc123"
	if strings.TrimSpace(out) != want {
		t.Fatalf("unexpected plain output:\n got: %q\nwant: %q", out, want)
	}
}

func TestViewJSONPreservesResponse(t *testing.T) {
	testutil.SetupAdmin(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, sampleViewPayload())
	})

	cmd := testutil.Command(newViewCmd(), testutil.JSONOutput())
	cmd.SetArgs([]string{"abc123"})
	out := testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	var resp struct {
		Success bool                   `json:"success"`
		Product map[string]interface{} `json:"product"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out)
	}
	if !resp.Success {
		t.Fatalf("expected success=true: %s", out)
	}
	if resp.Product["id"] != "abc123" {
		t.Errorf("expected product.id=abc123, got %v", resp.Product["id"])
	}
	files, ok := resp.Product["files"].([]interface{})
	if !ok {
		t.Fatalf("expected files array, got %T", resp.Product["files"])
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestProductsCmdRegistersSubcommands(t *testing.T) {
	cmd := NewProductsCmd()
	if cmd.Use != "products" {
		t.Fatalf("got Use=%q, want products", cmd.Use)
	}
	want := map[string]bool{"list": false, "view": false}
	for _, c := range cmd.Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected subcommand %q to be registered", name)
		}
	}
}

func TestSellerLabelHandlesPartialIdentifiers(t *testing.T) {
	cases := []struct {
		seller productSeller
		want   string
	}{
		{productSeller{ID: "u_1", Email: "seller@example.com"}, "seller@example.com (u_1)"},
		{productSeller{Email: "seller@example.com"}, "seller@example.com"},
		{productSeller{ID: "u_1"}, "u_1"},
		{productSeller{}, ""},
	}
	for _, tc := range cases {
		if got := sellerLabel(tc.seller); got != tc.want {
			t.Errorf("sellerLabel(%+v) = %q, want %q", tc.seller, got, tc.want)
		}
	}
}

func TestProductStatusLabel(t *testing.T) {
	cases := []struct {
		p    product
		want string
	}{
		{product{Alive: true}, "alive"},
		{product{Alive: false}, "unpublished"},
		{product{Alive: true, DeletedAt: "2026-04-15T10:00:00Z"}, "deleted"},
	}
	for _, tc := range cases {
		if got := productStatusLabel(tc.p); got != tc.want {
			t.Errorf("productStatusLabel(%+v) = %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestProductNameWithDeletedHandlesEmptyName(t *testing.T) {
	cases := []struct {
		p    product
		want string
	}{
		{product{}, ""},
		{product{Name: "Foo"}, "Foo"},
		{product{Name: "Foo", DeletedAt: "x"}, "Foo (deleted)"},
		{product{DeletedAt: "x"}, "(deleted)"},
	}
	for _, tc := range cases {
		if got := productNameWithDeleted(tc.p); got != tc.want {
			t.Errorf("productNameWithDeleted(%+v) = %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestFileDisplayNameFallsBackToFileName(t *testing.T) {
	cases := []struct {
		f    productFile
		want string
	}{
		{productFile{DisplayName: "Big Guide", FileName: "big.pdf"}, "Big Guide"},
		{productFile{FileName: "big.pdf"}, "big.pdf"},
		{productFile{DisplayName: "Big Guide", DeletedAt: "x"}, "Big Guide (deleted)"},
		{productFile{DeletedAt: "x"}, "(deleted)"},
	}
	for _, tc := range cases {
		if got := fileDisplayNameWithDeleted(tc.f); got != tc.want {
			t.Errorf("fileDisplayNameWithDeleted(%+v) = %q, want %q", tc.f, got, tc.want)
		}
	}
}

func TestFormatPriceWithoutCurrency(t *testing.T) {
	if got := formatPrice(20000, ""); got != "200.00" {
		t.Errorf("formatPrice(20000, \"\") = %q, want %q", got, "200.00")
	}
	if got := formatPrice(1000, "jpy"); got != "1000 JPY" {
		t.Errorf("formatPrice(1000, jpy) = %q, want %q", got, "1000 JPY")
	}
}

func TestPaginationFooterIsEmptyWhenZero(t *testing.T) {
	if got := paginationFooter(productPagination{}); got != "" {
		t.Errorf("paginationFooter({}) = %q, want \"\"", got)
	}
	if got := paginationFooter(productPagination{Page: 3, Pages: 0, Count: 0}); got != "Page 3 of 1 (0 total)" {
		t.Errorf("paginationFooter mid-state = %q", got)
	}
}

func TestFormatFileSize(t *testing.T) {
	cases := []struct {
		bytes int
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{70016, "68.4 KB"},
		{1048524, "1023.9 KB"},
		{1048575, "1.0 MB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741823, "1.0 GB"},
		{1073741824, "1.0 GB"},
		{1099511627775, "1.0 TB"},
		{1099511627776, "1.0 TB"},
		{-1, "0 B"},
	}
	for _, tc := range cases {
		got := formatFileSize(tc.bytes)
		if got != tc.want {
			t.Errorf("formatFileSize(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}
