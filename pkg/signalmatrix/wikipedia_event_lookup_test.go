package signalmatrix

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"phantom/pkg/shared"
)

type mockTransport struct {
	fn func(req *http.Request) *http.Response
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.fn(req), nil
}

func TestParseWikitable(t *testing.T) {
	html := `<html><body>
<table class="wikitable">
<tbody>
<tr>
<th>Conflict</th>
<th>Date</th>
<th>Location</th>
</tr>
<tr>
<td>American Revolutionary War</td>
<td>1775–1783</td>
<td>North America</td>
</tr>
<tr>
<td>War of 1812</td>
<td>1812–1815</td>
<td>North America</td>
</tr>
<tr>
<td>Iraq War</td>
<td>2003–2011</td>
<td>Iraq</td>
</tr>
</tbody>
</table>
</body></html>`

	rows := parseWikitable(html)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if len(rows[0]) != 3 {
		t.Fatalf("expected 3 cells in row 0, got %d: %v", len(rows[0]), rows[0])
	}
	if rows[0][0] != "American Revolutionary War" {
		t.Errorf("expected 'American Revolutionary War', got '%s'", rows[0][0])
	}
	if rows[1][0] != "War of 1812" {
		t.Errorf("expected 'War of 1812', got '%s'", rows[1][0])
	}
	if rows[2][0] != "Iraq War" {
		t.Errorf("expected 'Iraq War', got '%s'", rows[2][0])
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1775–1783", "1775-01-01"},
		{"2003–2011", "2003-01-01"},
		{"2001-09-11", "2001-09-11"},
		{"September 11, 2001", "2001-09-11"},
		{"11 September 2001", "2001-09-11"},
		{"March 2003 – December 2011", "2003-03-01"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := parseDate(tc.input)
			if !ok {
				t.Fatalf("parseDate(%q) failed", tc.input)
			}
			if got.Format("2006-01-02") != tc.want {
				t.Errorf("parseDate(%q) = %s, want %s", tc.input, got.Format("2006-01-02"), tc.want)
			}
		})
	}
}

func TestWikiEventLookup_Wars(t *testing.T) {
	lookup := NewWikiEventLookup()
	lookup.client = &http.Client{
		Transport: &mockTransport{
			fn: func(req *http.Request) *http.Response {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`<html><body><table class="wikitable"><tr><th>C</th><th>D</th></tr><tr><td>Test War</td><td>2001–2021</td></tr></table></body></html>`)),
				}
			},
		},
	}
	lookup.pages = map[shared.EventType]pageConfig{
		WarEventType: {Title: "Mock", NameCol: 0, DateCol: 1},
	}

	events, err := lookup.LoadHistorical(context.Background(), WarEventType)
	if err != nil {
		t.Fatalf("LoadHistorical: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d: %+v", len(events), events)
	}
	if events[0].ID != "war-test-war" {
		t.Errorf("expected ID 'war-test-war', got '%s'", events[0].ID)
	}
	if events[0].Type != WarEventType {
		t.Errorf("expected type 'war', got '%s'", events[0].Type)
	}
	if events[0].Timestamp.Year() != 2001 {
		t.Errorf("expected year 2001, got %d", events[0].Timestamp.Year())
	}
}
