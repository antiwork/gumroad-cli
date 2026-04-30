package sales

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	colorAccent  = "#FF90E8"
	colorAccent2 = "#FFC9F0"
	colorSuccess = "#00C896"
	colorWarning = "#FFC23C"
	colorError   = "#FF5A5F"
	colorInfo    = "#5B8DEF"
	colorMuted   = "#8B8D98"
	colorBorder  = "#3A3A42"
	colorSoft    = "#C7C9D1"
	colorBgPanel = "#16161A"
)

const (
	sparklineWidth   = 36
	defaultRowsShown = 12
	minWidth         = 80
	minHeight        = 24
)

var sparkRunes = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

var (
	stylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorBorder)).
			Padding(0, 2)

	styleAccent     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent))
	styleAccentBold = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent)).Bold(true)
	styleSoft       = lipgloss.NewStyle().Foreground(lipgloss.Color(colorSoft))
	styleMuted      = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
	styleInfo       = lipgloss.NewStyle().Foreground(lipgloss.Color(colorInfo))
	styleHeaderRow  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted)).Bold(true)
	stylePill       = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0E0E11")).
			Background(lipgloss.Color(colorAccent)).
			Bold(true).
			Padding(0, 1)
	styleCursorRow = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0E0E11")).
			Background(lipgloss.Color(colorAccent2)).
			Bold(true)
	styleRefunded = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError)).
			Italic(true)
	styleHelpKey  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent)).Bold(true)
	styleHelpDesc = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
)

// View implements tea.Model.
func (m Model) View() string {
	width := m.width
	if width < minWidth {
		width = minWidth
	}
	innerWidth := width - 4 // account for outer padding

	header := m.renderHeader(innerWidth)
	controls := m.renderControls(innerWidth)
	table := m.renderTable(innerWidth)
	detail := m.renderDetail(innerWidth)
	footer := m.renderFooter(innerWidth)

	parts := []string{header, controls, table, detail, footer}
	return strings.Join(parts, "\n")
}

func (m Model) renderHeader(innerWidth int) string {
	revenue, count := m.totalRevenue()
	pill := stylePill.Render("SALES")
	brand := styleAccentBold.Render("gumroad")
	dot := styleMuted.Render(" › ")

	leftSide := brand + dot + pill
	right := fmt.Sprintf("%s · %s",
		styleAccentBold.Render(revenue),
		styleSoft.Render(fmt.Sprintf("%d sales", count)),
	)
	if m.timeFilter != filterAll {
		right = styleMuted.Render(m.timeFilter.label()+" · ") + right
	}

	gap := strings.Repeat(" ", padGap(innerWidth, lipgloss.Width(leftSide), lipgloss.Width(right)))
	headLine := leftSide + gap + right

	spark := m.renderSparkline(sparklineWidth)
	subtitle := styleMuted.Render(fmt.Sprintf("%d loaded · %s", len(m.sales), windowDescription(m.sales)))
	gap2 := strings.Repeat(" ", padGap(innerWidth, lipgloss.Width(spark), lipgloss.Width(subtitle)))
	sparkLine := spark + gap2 + subtitle

	body := headLine + "\n" + sparkLine
	return stylePanel.Width(innerWidth).Render(body)
}

func (m Model) renderControls(innerWidth int) string {
	var search string
	if m.searchOpen {
		search = styleAccent.Render("/ ") + m.search.View()
	} else if v := m.search.Value(); v != "" {
		search = styleMuted.Render("/ ") + styleSoft.Render(v) + styleMuted.Render("  (esc to clear)")
	} else {
		search = styleMuted.Render("press / to search")
	}
	right := styleMuted.Render(fmt.Sprintf("[t] %s", m.timeFilter.label()))
	gap := strings.Repeat(" ", padGap(innerWidth, lipgloss.Width(search), lipgloss.Width(right)))
	return "  " + search + gap + right
}

func (m Model) renderTable(innerWidth int) string {
	if len(m.filtered) == 0 {
		return stylePanel.Width(innerWidth).Render(styleMuted.Render("  no sales match the current filter"))
	}

	idW := 12
	totalW := 9
	dateW := 12
	gutter := 2
	flexible := innerWidth - 4 - idW - totalW - dateW - 4*gutter
	if flexible < 20 {
		flexible = 20
	}
	emailW := flexible / 2
	productW := flexible - emailW

	header := styleHeaderRow.Render(
		fitLeft("ID", idW) + spacer() +
			fitLeft("BUYER", emailW) + spacer() +
			fitLeft("PRODUCT", productW) + spacer() +
			fitRight("TOTAL", totalW) + spacer() +
			fitLeft("DATE", dateW),
	)
	sep := styleMuted.Render(
		strings.Repeat("─", idW) + spacer() +
			strings.Repeat("─", emailW) + spacer() +
			strings.Repeat("─", productW) + spacer() +
			strings.Repeat("─", totalW) + spacer() +
			strings.Repeat("─", dateW),
	)

	rowsToShow := defaultRowsShown
	if m.height > minHeight {
		rowsToShow = max(rowsToShow, m.height-18)
	}
	start := 0
	if m.cursor >= rowsToShow {
		start = m.cursor - rowsToShow + 1
	}
	end := start + rowsToShow
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	var lines []string
	for i := start; i < end; i++ {
		idx := m.filtered[i]
		s := m.sales[idx]
		idCell := fitLeft(shortID(s.ID), idW)
		emailCell := fitLeft(s.Email, emailW)
		productLabel := s.Product
		if s.Refunded {
			productLabel = s.Product + " ↩"
		}
		productCell := fitLeft(productLabel, productW)
		totalCell := fitRight(s.FormattedCost, totalW)
		dateCell := fitLeft(humanDate(s.createdTime, s.CreatedAt), dateW)

		row := idCell + spacer() + emailCell + spacer() + productCell + spacer() + totalCell + spacer() + dateCell

		if i == m.cursor {
			marker := styleAccent.Render("▶ ")
			row = marker + styleCursorRow.Render(row)
		} else {
			marker := "  "
			if s.Refunded {
				row = styleRefunded.Render(row)
			} else {
				row = styleSoft.Render(row)
			}
			row = marker + row
		}
		lines = append(lines, row)
	}

	body := "  " + header + "\n  " + sep + "\n" + strings.Join(lines, "\n")
	return body
}

func (m Model) renderDetail(innerWidth int) string {
	sale, ok := m.SelectedSale()
	if !ok {
		return ""
	}

	var statusBadge string
	switch {
	case sale.Refunded:
		statusBadge = stylePill.
			Background(lipgloss.Color(colorError)).
			Render("REFUNDED")
	default:
		statusBadge = stylePill.
			Background(lipgloss.Color(colorSuccess)).
			Render("DELIVERED")
	}

	heading := styleAccentBold.Render(sale.ID) + "  " + statusBadge

	rows := [][2]string{
		{"Buyer", styleSoft.Render(sale.Email)},
		{"Product", styleSoft.Render(sale.Product)},
		{"Total", styleAccentBold.Render(sale.FormattedCost)},
		{"Date", styleSoft.Render(humanDate(sale.createdTime, sale.CreatedAt))},
	}
	keyW := 0
	for _, r := range rows {
		if w := lipgloss.Width(r[0]); w > keyW {
			keyW = w
		}
	}
	var b strings.Builder
	b.WriteString(heading + "\n")
	for i, r := range rows {
		key := styleMuted.Render(padRight(r[0], keyW))
		b.WriteString(key + "  " + r[1])
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	return stylePanel.Width(innerWidth).Render(b.String())
}

func (m Model) renderFooter(innerWidth int) string {
	hints := []struct{ key, desc string }{
		{"↑↓", "navigate"},
		{"/", "search"},
		{"t", "time"},
		{"esc", "clear"},
		{"q", "quit"},
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, styleHelpKey.Render(h.key)+" "+styleHelpDesc.Render(h.desc))
	}
	hint := strings.Join(parts, styleMuted.Render("  ·  "))
	notice := ""
	if m.notice != "" {
		notice = "  " + styleInfo.Render(m.notice)
	}
	gap := strings.Repeat(" ", padGap(innerWidth, lipgloss.Width(hint), lipgloss.Width(notice)))
	return "  " + hint + gap + notice
}

// renderSparkline buckets sales-per-bucket across the loaded date range and
// renders an ascending block sparkline. The reveal animation grows the visible
// width on each tick during model startup.
func (m Model) renderSparkline(width int) string {
	if width <= 0 || len(m.sales) == 0 {
		return styleMuted.Render(strings.Repeat("·", width))
	}

	// Bucket counts.
	earliest := m.sales[0].createdTime
	latest := earliest
	for _, s := range m.sales {
		if s.createdTime.IsZero() {
			continue
		}
		if s.createdTime.Before(earliest) || earliest.IsZero() {
			earliest = s.createdTime
		}
		if s.createdTime.After(latest) {
			latest = s.createdTime
		}
	}
	if earliest.IsZero() || latest.Equal(earliest) {
		latest = earliest.Add(24 * time.Hour)
	}
	span := latest.Sub(earliest)
	if span <= 0 {
		span = time.Hour
	}

	buckets := make([]int, width)
	for _, s := range m.sales {
		if s.createdTime.IsZero() {
			continue
		}
		ratio := float64(s.createdTime.Sub(earliest)) / float64(span)
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		idx := int(ratio * float64(width-1))
		buckets[idx]++
	}
	maxCount := 0
	for _, c := range buckets {
		if c > maxCount {
			maxCount = c
		}
	}
	if maxCount == 0 {
		return styleMuted.Render(strings.Repeat("·", width))
	}

	visible := width
	if m.revealing {
		visible = m.revealStep
	}

	var b strings.Builder
	for i, c := range buckets {
		if i >= visible {
			b.WriteString(styleMuted.Render("·"))
			continue
		}
		level := int(float64(c) / float64(maxCount) * float64(len(sparkRunes)-1))
		if c > 0 && level == 0 {
			level = 1
		}
		b.WriteString(styleAccent.Render(string(sparkRunes[level])))
	}
	return b.String()
}

func windowDescription(sales []Sale) string {
	if len(sales) == 0 {
		return ""
	}
	var earliest, latest time.Time
	for _, s := range sales {
		if s.createdTime.IsZero() {
			continue
		}
		if earliest.IsZero() || s.createdTime.Before(earliest) {
			earliest = s.createdTime
		}
		if s.createdTime.After(latest) {
			latest = s.createdTime
		}
	}
	if earliest.IsZero() {
		return ""
	}
	span := latest.Sub(earliest)
	switch {
	case span < 24*time.Hour:
		return "today"
	case span < 7*24*time.Hour:
		return fmt.Sprintf("over %d days", int(span.Hours()/24)+1)
	default:
		return fmt.Sprintf("over %d days", int(span.Hours()/24))
	}
}

func shortID(id string) string {
	const max = 11
	if len(id) <= max {
		return id
	}
	return id[:max-1] + "…"
}

func humanDate(t time.Time, fallback string) string {
	if t.IsZero() {
		return fallback
	}
	return t.Format("Jan 2 15:04")
}

func fitLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	visible := lipgloss.Width(s)
	if visible == w {
		return s
	}
	if visible > w {
		if w == 1 {
			return "…"
		}
		return s[:w-1] + "…"
	}
	return s + strings.Repeat(" ", w-visible)
}

func fitRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	visible := lipgloss.Width(s)
	if visible == w {
		return s
	}
	if visible > w {
		if w == 1 {
			return "…"
		}
		return s[:w-1] + "…"
	}
	return strings.Repeat(" ", w-visible) + s
}

func padRight(s string, w int) string {
	visible := lipgloss.Width(s)
	if visible >= w {
		return s
	}
	return s + strings.Repeat(" ", w-visible)
}

func spacer() string { return "  " }

func padGap(width, leftW, rightW int) int {
	gap := width - leftW - rightW - 4 // 4 = panel padding (2 each side)
	if gap < 1 {
		gap = 1
	}
	return gap
}
