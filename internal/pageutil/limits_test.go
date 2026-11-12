package pageutil

import (
	"strings"
	"testing"
)

func TestValidateCustomHTMLAcceptsAPageAtTheLimit(t *testing.T) {
	input := HTMLInput{Source: "landing.html", HTML: strings.Repeat("a", MaxCustomHTMLLength)}

	if err := ValidateCustomHTML(input, HTMLParams(input.HTML)); err != nil {
		t.Fatalf("a page exactly at the character limit should pass, got %v", err)
	}
}

func TestValidateCustomHTMLCountsCharactersNotBytes(t *testing.T) {
	// The server validates a String's length, which counts characters, so a
	// page of multi-byte characters must be measured the same way here.
	input := HTMLInput{Source: "landing.html", HTML: strings.Repeat("é", MaxCustomHTMLLength)}

	if err := ValidateCustomHTML(input, HTMLParams(input.HTML)); err != nil {
		t.Fatalf("a page of %d characters should pass however many bytes it takes, got %v", MaxCustomHTMLLength, err)
	}
}

func TestValidateCustomHTMLRejectsAPageOverTheCharacterLimit(t *testing.T) {
	input := HTMLInput{Source: "/tmp/landing.html", HTML: strings.Repeat("a", MaxCustomHTMLLength+1)}

	err := ValidateCustomHTML(input, HTMLParams(input.HTML))
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
	// encoded body, so the request would be refused by Rack with no message.
	input := HTMLInput{Source: "landing.html", HTML: strings.Repeat("😀", MaxCustomHTMLLength)}

	err := ValidateCustomHTML(input, HTMLParams(input.HTML))
	if err == nil {
		t.Fatal("want an error when the encoded body is past the API's request limit")
	}
	if !strings.Contains(err.Error(), "form data") {
		t.Errorf("error %q should name the encoded body size", err)
	}
}

func TestValidateCustomHTMLChecksTheWholeRequestNotJustThePage(t *testing.T) {
	// 349,524 escaped four-byte characters encode to 4,194,300 bytes, which
	// fits; the fields sent beside the page — here `pages create --title` — are
	// enough to push the same request over the limit.
	input := HTMLInput{Source: "about.html", HTML: strings.Repeat("😀", 349_524)}

	if err := ValidateCustomHTML(input, HTMLParams(input.HTML)); err != nil {
		t.Fatalf("the page on its own fits, got %v", err)
	}

	params := HTMLParams(input.HTML)
	params.Set("title", "About")
	if err := ValidateCustomHTML(input, params); err == nil {
		t.Fatal("want an error once the fields sent with the page push the body past the limit")
	}
}

func TestValidateCustomHTMLAcceptsABodyExactlyAtTheLimit(t *testing.T) {
	input := HTMLInput{Source: "about.html", HTML: strings.Repeat("😀", 349_524)}
	params := HTMLParams(input.HTML)
	params.Set("a", "b")
	if encoded := len(params.Encode()); encoded != maxEncodedBodyBytes {
		t.Fatalf("encoded body is %d bytes, want exactly %d", encoded, maxEncodedBodyBytes)
	}

	if err := ValidateCustomHTML(input, params); err != nil {
		t.Fatalf("a body exactly at the limit should pass, got %v", err)
	}

	params.Set("a", "bc")
	if err := ValidateCustomHTML(input, params); err == nil {
		t.Fatal("want an error for a body one byte past the limit")
	}
}
