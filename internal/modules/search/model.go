// Package search provides unified full-text search across user-owned content.
package search

// Search types used in requests and counts.
const (
	TypeJournals = "journals"
	TypeMemories = "memories"
	TypeInsights = "insights"
	TypeAnalyses = "analyses"
)

// Result is one unified search hit.
type Result struct {
	Type        string  `json:"type"`
	ID          string  `json:"id"`
	ParentID    *string `json:"parent_id"`
	Title       *string `json:"title"`
	Snippet     string  `json:"snippet"`
	Date        string  `json:"date"`
	Status      *string `json:"status"`
	Sensitivity *string `json:"sensitivity"`
	Score       float64 `json:"score"`
}

// Response is the unified search payload.
type Response struct {
	Query   string         `json:"query"`
	Results []Result       `json:"results"`
	Counts  map[string]int `json:"counts"`
	Limit   int            `json:"limit"`
}

// Filter is a validated search request.
type Filter struct {
	Query string
	Types []string
	Limit int
}

// Row is one raw search row before snippet generation.
type Row struct {
	Type        string
	ID          string
	ParentID    *string
	Title       *string
	Text        string
	Status      *string
	Sensitivity *string
	Date        string
	Score       float64
}
