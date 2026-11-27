package products

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/antiwork/gumroad-cli/internal/testutil"
)

func TestCreate_AdditionalDetailsSendRows(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{"id": "p1", "name": "E-Book", "formatted_price": "$5"},
		})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "E-Book", "--price", "5", "--additional-detail", "Pages: 40", "--additional-detail", "Format: PDF"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "POST" || gotPath != "/products" {
		t.Errorf("got %s %s, want POST /products", gotMethod, gotPath)
	}
	assertAdditionalDetailForm(t, gotForm, []string{"Pages", "Format"}, []string{"40", "PDF"})
}

func TestCreate_AdditionalDetailNameWithoutValueIsSent(t *testing.T) {
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{"id": "p1", "name": "E-Book", "formatted_price": "$5"},
		})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "E-Book", "--additional-detail", "Pages:"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	assertAdditionalDetailForm(t, gotForm, []string{"Pages"}, []string{""})
}

func TestCreate_AdditionalDetailsOmittedWhenFlagUnset(t *testing.T) {
	var hasRows bool
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		for key := range r.PostForm {
			if additionalDetailParamPattern.MatchString(key) {
				hasRows = true
			}
		}
		testutil.JSON(t, w, map[string]any{
			"product": map[string]any{"id": "p1", "name": "Art Pack", "formatted_price": "$10"},
		})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "Art Pack", "--price", "10"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if hasRows {
		t.Errorf("additional detail params should not be sent when the flag is unset")
	}
}

func TestCreate_AdditionalDetailWithoutNameIsRejected(t *testing.T) {
	reached := false
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newCreateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"--name", "E-Book", "--additional-detail", ": 40"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected an error for a value with no name")
	}
	if !strings.Contains(err.Error(), "--additional-detail \": 40\" needs a name") {
		t.Errorf("unexpected error: %v", err)
	}
	if reached {
		t.Errorf("API should not be reached when a row is refused")
	}
}

func TestUpdate_AdditionalDetailsReplaceRows(t *testing.T) {
	var gotMethod, gotPath string
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--additional-detail", "Format: PDF"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	if gotMethod != "PUT" || gotPath != "/products/prod1" {
		t.Errorf("got %s %s, want PUT /products/prod1", gotMethod, gotPath)
	}
	assertAdditionalDetailForm(t, gotForm, []string{"Format"}, []string{"PDF"})
}

func TestUpdate_AdditionalDetailEmptyValueClears(t *testing.T) {
	var gotForm url.Values
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--additional-detail", ""})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })

	assertAdditionalDetailForm(t, gotForm, []string{""}, []string{""})
}

func TestUpdate_AdditionalDetailAloneSatisfiesRequireAnyFlag(t *testing.T) {
	testutil.Setup(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.JSON(t, w, map[string]any{})
	})

	cmd := testutil.Command(newUpdateCmd(), testutil.Quiet(false))
	cmd.SetArgs([]string{"prod1", "--additional-detail", "Pages: 40"})
	testutil.CaptureStdout(func() { testutil.MustExecute(t, cmd) })
}

func TestBuildProductJSONBody_FoldsAdditionalDetailRows(t *testing.T) {
	params := url.Values{}
	setAdditionalDetailParams(params, []additionalDetail{{Name: "Pages", Value: "40"}, {Name: "Format", Value: "PDF"}})

	body := buildProductJSONBody(params, nil)

	for key := range body {
		if additionalDetailParamPattern.MatchString(key) {
			t.Fatalf("the form keys should not reach the body: %v", body)
		}
	}
	want := []map[string]any{{"name": "Pages", "value": "40"}, {"name": "Format", "value": "PDF"}}
	if !reflect.DeepEqual(body["additional_details"], want) {
		t.Errorf("got %v, want %v", body["additional_details"], want)
	}
}

func TestBuildProductJSONBody_CarriesTheClearingRow(t *testing.T) {
	params := url.Values{}
	setAdditionalDetailParams(params, nil)

	body := buildProductJSONBody(params, nil)

	want := []map[string]any{{"name": "", "value": ""}}
	if !reflect.DeepEqual(body["additional_details"], want) {
		t.Errorf("got %v, want %v", body["additional_details"], want)
	}
}

func assertAdditionalDetailForm(t *testing.T, form url.Values, names, values []string) {
	t.Helper()
	for i := range names {
		nameKey := fmt.Sprintf("additional_details[%d][name]", i)
		valueKey := fmt.Sprintf("additional_details[%d][value]", i)
		if got := form.Get(nameKey); got != names[i] {
			t.Errorf("%s: got %q, want %q", nameKey, got, names[i])
		}
		if got := form.Get(valueKey); got != values[i] {
			t.Errorf("%s: got %q, want %q", valueKey, got, values[i])
		}
	}
}
