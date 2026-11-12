package pageutil

import (
	"fmt"
	"net/url"
	"unicode/utf8"
)

const (
	// MaxCustomHTMLLength is the server's cap on a saved custom page
	// (Page::MAX_CUSTOM_HTML_LENGTH). It applies to a product landing page, the
	// profile page and a storefront page alike.
	MaxCustomHTMLLength = 500_000

	// maxEncodedBodyBytes is Rack's cap on a form-encoded request body:
	// RACK_QUERY_PARSER_BYTESIZE_LIMIT, 4,194,304 bytes by default. It applies
	// only to application/x-www-form-urlencoded bodies, not JSON, and a larger
	// body comes back as a 400 with an empty body. Measured against the custom
	// HTML endpoint: 4,190,048 bytes were accepted and 4,210,048 refused.
	maxEncodedBodyBytes = 4 << 20

	// CustomHTMLLimitHelp is the limit note every command that writes a custom
	// page carries in its --help text, so an agent building a page knows the
	// budget before it writes one.
	CustomHTMLLimitHelp = "A custom page is capped at 500,000 characters, and the request that carries it at 4 MiB once form-encoded. Both are checked before anything is sent."
)

// ValidateCustomHTML rejects a page locally, before the request is sent, when
// the API would refuse it for its size. params must be the request body the
// caller is about to send, because a page that fits on its own can still be
// pushed over the body limit by the fields sent beside it.
//
// Both of the API's refusals are confusing from the outside, which is why the
// check lives here rather than being left to the response. Past the character
// cap the API answers with a validation message about a limit the seller never
// chose, and past Rack's form body cap — which a page carrying an inlined image
// reaches easily, because base64 inflates the file — the request is refused with
// a bare "API error (HTTP 400)" carrying no message at all, which reads as a
// broken CLI or a disabled feature. Naming the size, the limit and the usual
// cause locally turns that into something a seller can act on.
func ValidateCustomHTML(input HTMLInput, params url.Values) error {
	if err := ValidateCustomHTMLLength(input); err != nil {
		return err
	}

	if encoded := len(params.Encode()); encoded > maxEncodedBodyBytes {
		return fmt.Errorf(
			"%s is %s (%d characters) and sends %s of form data, over the %s the API accepts in one request. An inlined image (a data: URI) is the usual cause: reference the image by URL instead",
			input.Source, humanBytes(len(input.HTML)), utf8.RuneCountInString(input.HTML), humanBytes(encoded), humanBytes(maxEncodedBodyBytes),
		)
	}

	return nil
}

// ValidateCustomHTMLLength checks only the character cap. It is for a page sent
// in a JSON body, which Rack's form body limit does not apply to.
func ValidateCustomHTMLLength(input HTMLInput) error {
	characters := utf8.RuneCountInString(input.HTML)
	if characters > MaxCustomHTMLLength {
		return fmt.Errorf(
			"%s is %s (%d characters), over the %d-character limit for a custom page. An inlined image (a data: URI) is the usual cause: reference the image by URL instead",
			input.Source, humanBytes(len(input.HTML)), characters, MaxCustomHTMLLength,
		)
	}
	return nil
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
