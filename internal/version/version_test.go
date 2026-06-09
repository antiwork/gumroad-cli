package version

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		raw  string
		want Version
		ok   bool
	}{
		{raw: "v0.20260609.0", want: Version{Major: 0, Minor: 20260609, Patch: 0}, ok: true},
		{raw: "0.20260609.1", want: Version{Major: 0, Minor: 20260609, Patch: 1}, ok: true},
		{raw: "v0.20260609.0+build", want: Version{Major: 0, Minor: 20260609, Patch: 0}, ok: true},
		{raw: "v0.21.0", want: Version{Major: 0, Minor: 21, Patch: 0}, ok: true},
		{raw: "not-semver", ok: false},
		{raw: "v0.20260609", ok: false},
		{raw: "v0.20260609.x", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, ok := Parse(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		name string
		a    Version
		b    Version
		want int
	}{
		{
			name: "date release after legacy semver",
			a:    Version{Major: 0, Minor: 20260609, Patch: 0},
			b:    Version{Major: 0, Minor: 21, Patch: 0},
			want: 1,
		},
		{
			name: "same day sequence",
			a:    Version{Major: 0, Minor: 20260609, Patch: 1},
			b:    Version{Major: 0, Minor: 20260609, Patch: 0},
			want: 1,
		},
		{
			name: "older date",
			a:    Version{Major: 0, Minor: 20260608, Patch: 0},
			b:    Version{Major: 0, Minor: 20260609, Patch: 0},
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareSign(Compare(tt.a, tt.b)); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDisplay(t *testing.T) {
	tests := map[string]string{
		"v0.20260609.0":       "2026.06.09",
		"0.20260609.1":        "2026.06.09.1",
		"v0.20280229.0":       "2028.02.29",
		"v0.20261309.0":       "v0.20261309.0",
		"v0.21.0":             "v0.21.0",
		"0.21.0":              "0.21.0",
		"not-semver":          "not-semver",
		" v0.20260609.0 \n\t": "2026.06.09",
	}

	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			if got := Display(raw); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func compareSign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}
