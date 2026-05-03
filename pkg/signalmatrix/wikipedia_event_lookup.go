package signalmatrix

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"phantom/pkg/shared"
)

const (
	WarEventType      shared.EventType = "war"
	ElectionEventType shared.EventType = "election"
	SanctionEventType shared.EventType = "sanction"
)

var yearRe = regexp.MustCompile(`(\d{4})`)

type pageConfig struct {
	Title   string
	NameCol int
	DateCol int
}

var wikiPages = map[shared.EventType]pageConfig{
	WarEventType: {
		Title:   "List of wars involving the United States in the 21st century",
		NameCol: 0,
		DateCol: 0,
	},
	ElectionEventType: {
		Title:   "List of United States presidential elections",
		NameCol: 0,
		DateCol: 1,
	},
	SanctionEventType: {
		Title:   "International sanctions during the Russo-Ukrainian War",
		NameCol: 0,
		DateCol: 1,
	},
}

type WikiEventLookup struct {
	client *http.Client
	pages  map[shared.EventType]pageConfig
}

func NewWikiEventLookup() *WikiEventLookup {
	return &WikiEventLookup{
		client: &http.Client{Timeout: 15 * time.Second},
		pages:  wikiPages,
	}
}

func (w *WikiEventLookup) LoadHistorical(ctx context.Context, eventType shared.EventType) ([]shared.Event, error) {
	cfg, ok := w.pages[eventType]
	if !ok {
		return nil, fmt.Errorf("unknown event type: %s", eventType)
	}

	rows, err := w.fetchTable(ctx, cfg.Title)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", cfg.Title, err)
	}

	var events []shared.Event
	seen := map[string]bool{}
	for _, row := range rows {
		if len(row) <= cfg.DateCol || len(row) <= cfg.NameCol {
			continue
		}
		name := stripHTML(row[cfg.NameCol])
		if name == "" {
			continue
		}
		dateStr := name
		if cfg.NameCol != cfg.DateCol {
			dateStr = stripHTML(row[cfg.DateCol])
		}
		if dateStr == "" {
			continue
		}
		t, ok := parseDate(dateStr)
		if !ok {
			// Try parenthesized date in name: "War Name (2001–2021)"
			m := regexp.MustCompile(`\((\d{4}[-–]\d{4}|[\w\s]+[-–][\w\s]+)\)`).FindString(name)
			if m != "" {
				t, ok = parseDate(strings.Trim(m, "()"))
			}
			if !ok {
				continue
			}
		}
		id := fmt.Sprintf("%s-%s", eventType, slugify(name))
		if seen[id] {
			continue
		}
		seen[id] = true
		events = append(events, shared.Event{
			ID:        id,
			Type:      eventType,
			Timestamp: t,
			Asset:     "",
		})
	}
	return events, nil
}

func (w *WikiEventLookup) SaveHistorical(_ context.Context, _ shared.Event) error {
	return nil
}

func (w *WikiEventLookup) fetchTable(ctx context.Context, title string) ([][]string, error) {
	apiURL := fmt.Sprintf("https://en.wikipedia.org/api/rest_v1/page/html/%s", url.PathEscape(title))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "phantom-benchmark/1.0")
	req.Header.Set("Accept", "text/html; charset=utf-8")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseWikitable(string(body)), nil
}

func findWikitable(html string) int {
	re := regexp.MustCompile(`class="[^"]*wikitable[^"]*"`)
	m := re.FindStringIndex(html)
	if m == nil {
		return -1
	}
	return m[0]
}

func parseWikitable(html string) [][]string {
	tableStart := findWikitable(html)
	if tableStart < 0 {
		return nil
	}

	tableEnd := strings.Index(html[tableStart:], `</table>`)
	if tableEnd < 0 {
		return nil
	}
	tableEnd += tableStart + len(`</table>`)

	tableHTML := html[tableStart:tableEnd]

	var rows [][]string
	for {
		trStart := findTag(tableHTML, "<tr")
		if trStart < 0 {
			break
		}
		trEnd := findTagEnd(tableHTML, "</tr>", trStart+3)
		if trEnd < 0 {
			break
		}
		trContent := tableHTML[trStart:trEnd]

		// Skip header rows (only <th>, no <td>)
		if findTag(trContent, "<th") >= 0 && findTag(trContent, "<td") < 0 {
			tableHTML = tableHTML[trEnd:]
			continue
		}

		var cells []string
		findCells(trContent, &cells)

		if len(cells) > 0 {
			rows = append(rows, cells)
		}
		tableHTML = tableHTML[trEnd:]
	}

	return rows
}

func findTag(s, tag string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '<' && i+len(tag) < len(s) {
			candidate := s[i : i+len(tag)]
			if strings.EqualFold(candidate, tag) {
				if i+len(tag) >= len(s) || s[i+len(tag)] == '>' || s[i+len(tag)] == ' ' || s[i+len(tag)] == '\n' {
					return i
				}
			}
		}
	}
	return -1
}

func findTagEnd(s, tag string, start int) int {
	idx := strings.Index(strings.ToLower(s[start:]), strings.ToLower(tag))
	if idx < 0 {
		return -1
	}
	return start + idx + len(tag)
}

func findCells(s string, cells *[]string) {
	pos := 0
	tags := []string{"<td", "<th"}
	for {
		best := -1
		bestTag := ""
		for _, tag := range tags {
			idx := findTag(s[pos:], tag)
			if idx >= 0 && (best < 0 || idx < best) {
				best = idx
				bestTag = tag
			}
		}
		if best < 0 {
			break
		}

		closeTag := "</td>"
		if bestTag == "<th" {
			closeTag = "</th>"
		}

		cellStart := pos + best
		cellEnd := findTagEnd(s, closeTag, cellStart)
		if cellEnd < 0 {
			break
		}
		cellContent := s[cellStart:cellEnd]
		cellContent = stripHTMLTags(cellContent)
		*cells = append(*cells, strings.TrimSpace(cellContent))

		pos = cellEnd
	}
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

func stripHTMLTags(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&ndash;", "–")
	s = strings.ReplaceAll(s, "&mdash;", "—")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return s
}

func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "–", "-")
	s = strings.ReplaceAll(s, "—", "-")

	// Try ISO date first
	if m := regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`).FindStringSubmatch(s); m != nil {
		t, err := time.Parse("2006-01-02", m[1]+"-"+m[2]+"-"+m[3])
		if err == nil {
			return t, true
		}
	}

	// Try text dates
	textParsers := []struct {
		re *regexp.Regexp
		fn func(m []string) (string, string, string) // year, monthName, day
	}{
		{regexp.MustCompile(`(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2}),?\s+(\d{4})`), func(m []string) (string, string, string) {
			return m[3], m[1], m[2]
		}},
		{regexp.MustCompile(`(\d{1,2})\s+(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{4})`), func(m []string) (string, string, string) {
			return m[3], m[2], m[1]
		}},
		{regexp.MustCompile(`(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{4})`), func(m []string) (string, string, string) {
			return m[2], m[1], "1"
		}},
	}

	for _, p := range textParsers {
		m := p.re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		year, monthName, day := p.fn(m)
		t, err := time.Parse("2006-1-2", fmt.Sprintf("%s-%d-%s", year, monthNum(monthName), day))
		if err == nil {
			return t, true
		}
	}

	// Try date range: take start date
	parts := strings.Split(s, "-")
	if len(parts) >= 2 {
		first := strings.TrimSpace(parts[0])
		if t, ok := parseDate(first); ok {
			return t, true
		}
	}

	// Fallback: extract first 4-digit year
	m := yearRe.FindString(s)
	if m != "" {
		t, err := time.Parse("2006", m)
		if err == nil {
			return t, true
		}
	}

	return time.Time{}, false
}

func monthNum(s string) int {
	months := map[string]int{
		"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6,
		"july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12,
	}
	return months[strings.ToLower(s)]
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 50 {
		s = s[:50]
	}
	return s
}
