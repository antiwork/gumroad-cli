package sales

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	hoursPerDay          = 24
	daysPerWeek          = 7
	daysPerMonth         = 30
	revealTickInterval   = 35 * time.Millisecond
	searchInputCharLimit = 80
)

type Sale struct {
	ID            string
	Email         string
	Product       string
	FormattedCost string
	CreatedAt     string
	Refunded      bool
	createdTime   time.Time
}

type timeFilter int

const (
	filterAll timeFilter = iota
	filterToday
	filterWeek
	filterMonth
	timeFilterCount
)

func (f timeFilter) label() string {
	switch f {
	case filterToday:
		return "today"
	case filterWeek:
		return "7 days"
	case filterMonth:
		return "30 days"
	default:
		return "all time"
	}
}

func (f timeFilter) cutoff(now time.Time) (time.Time, bool) {
	switch f {
	case filterToday:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), true
	case filterWeek:
		return now.Add(-daysPerWeek * hoursPerDay * time.Hour), true
	case filterMonth:
		return now.Add(-daysPerMonth * hoursPerDay * time.Hour), true
	default:
		return time.Time{}, false
	}
}

type keymap struct {
	Up     key.Binding
	Down   key.Binding
	Search key.Binding
	Time   key.Binding
	Open   key.Binding
	Clear  key.Binding
	Quit   key.Binding
}

func defaultKeymap() keymap {
	return keymap{
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Search: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Time:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "time")),
		Open:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "details")),
		Clear:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear")),
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

type Model struct {
	sales      []Sale
	filtered   []int
	cursor     int
	width      int
	height     int
	now        time.Time
	timeFilter timeFilter
	search     textinput.Model
	searchOpen bool
	keys       keymap
	revealStep int
	revealing  bool
	notice     string
}

func NewModel(sales []Sale) Model {
	ti := textinput.New()
	ti.Placeholder = "filter by email or product"
	ti.Prompt = ""
	ti.CharLimit = searchInputCharLimit

	for i := range sales {
		sales[i].createdTime = parseSaleTime(sales[i].CreatedAt)
	}
	sortSalesNewestFirst(sales)

	m := Model{
		sales:      sales,
		now:        time.Now(),
		timeFilter: filterAll,
		search:     ti,
		keys:       defaultKeymap(),
		revealing:  true,
	}
	m.applyFilters()
	return m
}

func (m Model) Init() tea.Cmd {
	return revealTick()
}

func revealTick() tea.Cmd {
	return tea.Tick(revealTickInterval, func(t time.Time) tea.Msg { return revealMsg{} })
}

type revealMsg struct{}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case revealMsg:
		if m.revealing {
			m.revealStep++
			if m.revealStep >= sparklineWidth {
				m.revealing = false
			}
			return m, revealTick()
		}
		return m, nil

	case tea.KeyMsg:
		if m.searchOpen {
			return m.updateSearchKey(msg)
		}
		return m.updateBrowseKey(msg)
	}
	return m, nil
}

func (m Model) updateSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchOpen = false
		m.search.Blur()
		return m, nil
	case "enter":
		m.searchOpen = false
		m.search.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.applyFilters()
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	return m, cmd
}

func (m Model) updateBrowseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case key.Matches(msg, m.keys.Search):
		m.searchOpen = true
		m.search.Focus()
		return m, textinput.Blink
	case key.Matches(msg, m.keys.Time):
		m.timeFilter = (m.timeFilter + 1) % timeFilterCount
		m.applyFilters()
		if m.cursor >= len(m.filtered) {
			m.cursor = max(0, len(m.filtered)-1)
		}
		m.notice = "time filter: " + m.timeFilter.label()
	case key.Matches(msg, m.keys.Clear):
		if m.search.Value() != "" {
			m.search.SetValue("")
			m.applyFilters()
		} else if m.timeFilter != filterAll {
			m.timeFilter = filterAll
			m.applyFilters()
		}
	}
	return m, nil
}

func (m Model) SelectedSale() (Sale, bool) {
	if len(m.filtered) == 0 {
		return Sale{}, false
	}
	return m.sales[m.filtered[m.cursor]], true
}

func (m *Model) applyFilters() {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	cutoff, hasCutoff := m.timeFilter.cutoff(m.now)

	m.filtered = m.filtered[:0]
	for i, s := range m.sales {
		if hasCutoff && !s.createdTime.IsZero() && s.createdTime.Before(cutoff) {
			continue
		}
		if q != "" {
			haystack := strings.ToLower(s.Email + " " + s.Product + " " + s.ID)
			if !strings.Contains(haystack, q) {
				continue
			}
		}
		m.filtered = append(m.filtered, i)
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func parseSaleTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func sortSalesNewestFirst(sales []Sale) {
	sort.SliceStable(sales, func(i, j int) bool {
		return sales[i].createdTime.After(sales[j].createdTime)
	})
}

func (m Model) totalRevenue() (string, int) {
	var sum float64
	count := 0
	for _, idx := range m.filtered {
		s := m.sales[idx]
		if s.Refunded {
			continue
		}
		v, ok := parseDollarAmount(s.FormattedCost)
		if !ok {
			continue
		}
		sum += v
		count++
	}
	return fmt.Sprintf("$%s", formatThousands(sum)), count
}

func parseDollarAmount(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}
	s = strings.TrimPrefix(s, "$")
	s = strings.ReplaceAll(s, ",", "")
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err != nil {
		return 0, false
	}
	if negative {
		v = -v
	}
	return v, true
}

func formatThousands(v float64) string {
	const groupSize = 3
	s := fmt.Sprintf("%.2f", v)
	dot := strings.Index(s, ".")
	intPart := s[:dot]
	decPart := s[dot:]
	negative := strings.HasPrefix(intPart, "-")
	if negative {
		intPart = intPart[1:]
	}
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%groupSize == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if negative {
		return "-" + b.String() + decPart
	}
	return b.String() + decPart
}
