package sales

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleSales() []Sale {
	return []Sale{
		{ID: "sale_1", Email: "alice@example.com", Product: "Art Pack", FormattedCost: "$25.00", CreatedAt: "2026-04-29T12:00:00Z"},
		{ID: "sale_2", Email: "bob@example.com", Product: "Music Pack", FormattedCost: "$10.00", CreatedAt: "2026-04-28T12:00:00Z", Refunded: true},
		{ID: "sale_3", Email: "carol@example.com", Product: "Art Pack", FormattedCost: "$15.00", CreatedAt: "2026-04-27T12:00:00Z"},
	}
}

func TestNewModel_SortsNewestFirst(t *testing.T) {
	m := NewModel(sampleSales())
	if got := m.sales[0].ID; got != "sale_1" {
		t.Fatalf("expected newest first, got %q", got)
	}
}

func TestModel_TotalRevenueExcludesRefunded(t *testing.T) {
	m := NewModel(sampleSales())
	rev, count := m.totalRevenue()
	if rev != "$40.00" {
		t.Fatalf("revenue: got %q, want $40.00", rev)
	}
	if count != 2 {
		t.Fatalf("count: got %d, want 2", count)
	}
}

func TestModel_NavigationKeys(t *testing.T) {
	m := NewModel(sampleSales())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursor != 1 {
		t.Fatalf("down: cursor = %d, want 1", m.cursor)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursor != 2 {
		t.Fatalf("down clamp: cursor = %d, want 2", m.cursor)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.cursor != 1 {
		t.Fatalf("up: cursor = %d, want 1", m.cursor)
	}
}

func TestModel_SearchFiltersBuyerAndProduct(t *testing.T) {
	m := NewModel(sampleSales())

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(Model)
	if !m.searchOpen {
		t.Fatal("expected search to open on /")
	}

	for _, r := range "music" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}

	if len(m.filtered) != 1 {
		t.Fatalf("filtered: got %d rows, want 1", len(m.filtered))
	}
	if got, _ := m.SelectedSale(); got.ID != "sale_2" {
		t.Fatalf("selected: got %q, want sale_2", got.ID)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.searchOpen {
		t.Fatal("expected esc inside search to close it")
	}
}

func TestModel_TimeFilterCyclesAndClears(t *testing.T) {
	m := NewModel(sampleSales())
	m.now = time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = updated.(Model)
	if m.timeFilter != filterToday {
		t.Fatalf("timeFilter: got %v, want filterToday", m.timeFilter)
	}
	if len(m.filtered) != 1 {
		t.Fatalf("today filter rows: got %d, want 1", len(m.filtered))
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.timeFilter != filterAll {
		t.Fatalf("esc must reset to filterAll, got %v", m.timeFilter)
	}
}

func TestModel_QuitKey(t *testing.T) {
	m := NewModel(sampleSales())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected quit command on q")
	}
}

func TestModel_Init_StartsRevealAnimation(t *testing.T) {
	m := NewModel(sampleSales())
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a tick cmd for the reveal animation")
	}
}

func TestModel_RevealAdvances(t *testing.T) {
	m := NewModel(sampleSales())
	updated, cmd := m.Update(revealMsg{})
	m = updated.(Model)
	if m.revealStep != 1 {
		t.Fatalf("revealStep: got %d, want 1", m.revealStep)
	}
	if cmd == nil {
		t.Fatal("expected reveal to schedule another tick")
	}
}

func TestModel_RevealStopsAtMaxAndIgnoresFurtherTicks(t *testing.T) {
	m := NewModel(sampleSales())
	for i := 0; i <= sparklineWidth+1; i++ {
		updated, _ := m.Update(revealMsg{})
		m = updated.(Model)
	}
	if m.revealing {
		t.Fatal("reveal must stop after sparklineWidth ticks")
	}
	updated, cmd := m.Update(revealMsg{})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no more ticks once reveal completes")
	}
}

func TestModel_View_ContainsBrandAndSelected(t *testing.T) {
	m := NewModel(sampleSales())
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = upd.(Model)
	view := m.View()

	for _, sub := range []string{"gumroad", "SALES", "alice@example.com", "Art Pack"} {
		if !strings.Contains(view, sub) {
			t.Fatalf("view missing %q", sub)
		}
	}
}

func TestModel_View_NoSalesShowsHelpfulMessage(t *testing.T) {
	m := NewModel(nil)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = upd.(Model)
	view := m.View()

	if !strings.Contains(view, "no sales") {
		t.Fatalf("expected empty-state hint, got: %s", view)
	}
}

func TestParseDollarAmount(t *testing.T) {
	cases := map[string]struct {
		val float64
		ok  bool
	}{
		"$10.00":    {10, true},
		"$1,234.56": {1234.56, true},
		"-$5":       {-5, true},
		"":          {0, false},
		"USD 10.00": {0, false},
		"$abc":      {0, false},
	}
	for in, want := range cases {
		got, ok := parseDollarAmount(in)
		if ok != want.ok {
			t.Errorf("parseDollarAmount(%q): ok = %v, want %v", in, ok, want.ok)
		}
		if want.ok && got != want.val {
			t.Errorf("parseDollarAmount(%q): got %v, want %v", in, got, want.val)
		}
	}
}

func TestFormatThousands(t *testing.T) {
	cases := map[float64]string{
		0:        "0.00",
		1:        "1.00",
		100:      "100.00",
		1000:     "1,000.00",
		1234567:  "1,234,567.00",
		-1234.56: "-1,234.56",
	}
	for in, want := range cases {
		if got := formatThousands(in); got != want {
			t.Errorf("formatThousands(%v): got %q, want %q", in, got, want)
		}
	}
}

func TestParseSaleTime(t *testing.T) {
	if !parseSaleTime("").IsZero() {
		t.Fatal("empty input must yield zero time")
	}
	if !parseSaleTime("not a date").IsZero() {
		t.Fatal("unparseable input must yield zero time")
	}
	if t1 := parseSaleTime("2026-04-29T12:00:00Z"); t1.IsZero() {
		t.Fatal("RFC3339 must parse")
	}
	if t1 := parseSaleTime("2026-04-29"); t1.IsZero() {
		t.Fatal("date-only must parse")
	}
}

func TestShortIDAndFitting(t *testing.T) {
	if got := shortID("short"); got != "short" {
		t.Fatalf("shortID short: %q", got)
	}
	if got := shortID("a-very-long-purchase-id"); got != "a-very-lon…" {
		t.Fatalf("shortID truncated: %q", got)
	}
	if got := fitLeft("ab", 5); got != "ab   " {
		t.Fatalf("fitLeft pad: %q", got)
	}
	if got := fitLeft("abcdef", 4); got != "abc…" {
		t.Fatalf("fitLeft trunc: %q", got)
	}
	if got := fitRight("ab", 5); got != "   ab" {
		t.Fatalf("fitRight pad: %q", got)
	}
}

func TestRenderSparklineEmptyAndPopulated(t *testing.T) {
	m := NewModel(nil)
	if got := m.renderSparkline(8); !strings.Contains(got, "·") {
		t.Fatalf("empty sparkline must be dots, got %q", got)
	}

	m = NewModel(sampleSales())
	if got := m.renderSparkline(16); got == "" {
		t.Fatal("populated sparkline must not be empty")
	}
}
