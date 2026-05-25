package cmdutil

import (
	"bytes"
	"testing"
)

func TestWriteIdentifierLineWritesWhenMessageDoesNotContainIdentifier(t *testing.T) {
	var buf bytes.Buffer

	if err := WriteIdentifierLine(&buf, "User ID", "User flagged", "2245593582708"); err != nil {
		t.Fatalf("WriteIdentifierLine returned error: %v", err)
	}

	if got, want := buf.String(), "User ID: 2245593582708\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWriteIdentifierLineSkipsEmptyOrAlreadyPresentIdentifier(t *testing.T) {
	cases := []struct {
		name       string
		message    string
		identifier string
	}{
		{name: "empty", message: "User flagged", identifier: ""},
		{name: "already present", message: "User 2245593582708 flagged", identifier: "2245593582708"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteIdentifierLine(&buf, "User ID", tc.message, tc.identifier); err != nil {
				t.Fatalf("WriteIdentifierLine returned error: %v", err)
			}
			if buf.Len() != 0 {
				t.Fatalf("expected no output, got %q", buf.String())
			}
		})
	}
}
