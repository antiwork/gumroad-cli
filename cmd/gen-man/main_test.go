package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRemovesStaleManPagesAndGeneratesCurrentTree(t *testing.T) {
	outputDir := t.TempDir()
	stalePath := filepath.Join(outputDir, "gumroad-skus.1")
	if err := os.WriteFile(stalePath, []byte("stale"), 0600); err != nil {
		t.Fatalf("write stale man page: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{outputDir}, &stdout, &stderr); code != 0 {
		t.Fatalf("run returned %d, stderr=%q", code, stderr.String())
	}

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale man page to be removed, got err=%v", err)
	}

	productSKUsPath := filepath.Join(outputDir, "gumroad-products-skus.1")
	if _, err := os.Stat(productSKUsPath); err != nil {
		t.Fatalf("expected nested sku man page, got err=%v", err)
	}

	productPageData, err := os.ReadFile(filepath.Join(outputDir, "gumroad-products.1"))
	if err != nil {
		t.Fatalf("read product man page: %v", err)
	}
	productPageText := string(productPageData)
	if !strings.Contains(productPageText, "gumroad-products-skus(1)") {
		t.Fatalf("product man page missing skus see-also entry: %q", productPageText)
	}
	if !strings.Contains(productPageText, "gumroad products skus <id>") {
		t.Fatalf("product man page missing skus example: %q", productPageText)
	}

	if !strings.Contains(stdout.String(), "Man pages written to "+outputDir+"/") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}
