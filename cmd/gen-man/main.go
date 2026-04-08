package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/cmd"
	cobradoc "github.com/spf13/cobra/doc"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	outputDir := "man"
	if len(args) > 0 {
		outputDir = args[0]
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if err := removeExistingManPages(outputDir); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	root := cmd.NewRootCmd()
	root.DisableAutoGenTag = true

	if err := cobradoc.GenManTree(root, &cobradoc.GenManHeader{
		Title:   "GR",
		Section: "1",
		Source:  "Gumroad",
		Manual:  "Gumroad CLI",
	}, outputDir); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Man pages written to %s/\n", outputDir)
	return 0
}

func removeExistingManPages(outputDir string) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".1" || !strings.HasPrefix(entry.Name(), "gumroad") {
			continue
		}

		path := filepath.Join(outputDir, entry.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}
