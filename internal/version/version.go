package version

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Version struct {
	Major int
	Minor int
	Patch int
}

func Parse(raw string) (Version, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	if i := strings.IndexAny(raw, "-+"); i >= 0 {
		raw = raw[:i]
	}

	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Version{}, false
	}

	major, ok := parsePart(parts[0])
	if !ok {
		return Version{}, false
	}
	minor, ok := parsePart(parts[1])
	if !ok {
		return Version{}, false
	}
	patch, ok := parsePart(parts[2])
	if !ok {
		return Version{}, false
	}

	return Version{Major: major, Minor: minor, Patch: patch}, true
}

func Compare(a, b Version) int {
	switch {
	case a.Major != b.Major:
		return a.Major - b.Major
	case a.Minor != b.Minor:
		return a.Minor - b.Minor
	default:
		return a.Patch - b.Patch
	}
}

func Display(raw string) string {
	trimmed := strings.TrimSpace(raw)
	parts := displayParts(trimmed)
	if len(parts) != 3 || parts[0] != "0" || len(parts[1]) != 8 {
		return trimmed
	}

	releaseDate, err := time.Parse("20060102", parts[1])
	if err != nil {
		return trimmed
	}
	patch, ok := parsePart(parts[2])
	if !ok {
		return trimmed
	}

	display := releaseDate.Format("2006.01.02")
	if patch > 0 {
		display = fmt.Sprintf("%s.%d", display, patch)
	}
	return display
}

func displayParts(raw string) []string {
	raw = strings.TrimPrefix(raw, "v")
	if i := strings.IndexAny(raw, "-+"); i >= 0 {
		raw = raw[:i]
	}
	return strings.Split(raw, ".")
}

func parsePart(part string) (int, bool) {
	if part == "" {
		return 0, false
	}
	for _, r := range part {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(part)
	if err != nil {
		return 0, false
	}
	return n, true
}
