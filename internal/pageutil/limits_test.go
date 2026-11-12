package pageutil

import (
	"strings"
	"testing"
)

func TestValidateCustomHTMLAcceptsAPageAtTheLimit(t *testing.T) {
	input := HTMLInput{Source: "landing.html", HTML: strings.Repeat("a", MaxCustomHTMLLength)}

	if err := ValidateCustomHTML(input); err != nil {
		t.Fatalf("a page exactly at the character limit should pass, got %v", err)
	}
}

func TestValidateCustomHTMLCountsCharactersNotBytes(t *testing.T) {
	// The server validates a String's length, which counts characters, so a
	// page of multi-byte characters must be measured the same way here.
	input := HTMLInput{Source: "landing.html", HTML: strings.Repeat("é", MaxCustomHTMLLength)}

	if err := ValidateCustomHTML(input); err != nil {
		t.Fatalf("a page of %d characters should pass however many bytes it takes, got %v", MaxCustomHTMLLength, err)
	}
}

func TestValidateCustomHTMLRejectsAPageOverTheCharacterLimit(t *testing.T) {
	input := HTMLInput{Source: "/tmp/landing.html", HTML: strings.Repeat("a", MaxCustomHTMLLength+1)}

	err := ValidateCustomHTML(input)
	if err == nil {
		t.Fatal("want an error for a page over the character limit")
	}
	for _, want := range []string{"/tmp/landing.html", "500000", "data: URI"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestValidateCustomHTMLRejectsAPageThatEncodesPastTheBodyLimit(t *testing.T) {
	// Under the character cap, but every byte of the page escapes in the
	// encoded body, so the request would be refused at ingress with no message.
	input := HTMLInput{Source: "landing.html", HTML: strings.Repeat("😀", MaxCustomHTMLLength)}

	err := ValidateCustomHTML(input)
	if err == nil {
		t.Fatal("want an error when the encoded body is past the API's request limit")
	}
	if !strings.Contains(err.Error(), "form data") {
		t.Errorf("error %q should name the encoded body size", err)
	}
}
