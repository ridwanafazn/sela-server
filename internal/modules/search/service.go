package search

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ridwanafazn/sela-server/internal/shared/apierrors"
)

const (
	defaultLimit    = 10
	maxLimit        = 20
	maxQueryRunes   = 200
	minQueryRunes   = 2
	snippetRadius   = 90
	maxSnippetRunes = 220
)

var searchTypes = []string{TypeJournals, TypeMemories, TypeInsights, TypeAnalyses}

// Service validates and merges per-type search results.
type Service struct {
	repo *Repository
	log  *slog.Logger
}

// NewService creates the search service.
func NewService(repo *Repository, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, log: log}
}

// Search returns unified results for one user.
func (s *Service) Search(ctx context.Context, userID string, filter Filter) (*Response, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	length := utf8.RuneCountInString(filter.Query)
	if length < minQueryRunes {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "q",
			Code:    "TOO_SHORT",
			Message: "Search query must be at least 2 characters.",
		})
	}
	if length > maxQueryRunes {
		return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
			Field:   "q",
			Code:    "TOO_LONG",
			Message: "Search query must be at most 200 characters.",
		})
	}

	types, err := normalizeTypes(filter.Types)
	if err != nil {
		return nil, err
	}
	if filter.Limit <= 0 {
		filter.Limit = defaultLimit
	}
	if filter.Limit > maxLimit {
		filter.Limit = maxLimit
	}

	response := &Response{
		Query:   filter.Query,
		Results: []Result{},
		Counts:  map[string]int{},
		Limit:   filter.Limit,
	}
	for _, item := range types {
		response.Counts[item] = 0
	}

	rows := make([]Row, 0, len(types)*filter.Limit)
	for _, item := range types {
		var (
			found []Row
			err   error
		)
		switch item {
		case TypeJournals:
			found, err = s.repo.Journals(ctx, userID, filter.Query, filter.Limit)
		case TypeMemories:
			found, err = s.repo.Memories(ctx, userID, filter.Query, filter.Limit)
		case TypeInsights:
			found, err = s.repo.Insights(ctx, userID, filter.Query, filter.Limit)
		case TypeAnalyses:
			found, err = s.repo.Analyses(ctx, userID, filter.Query, filter.Limit)
		}
		if err != nil {
			return nil, err
		}
		response.Counts[item] = len(found)
		for index := range found {
			found[index].Type = singularType(item)
		}
		rows = append(rows, found...)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		if rows[i].Date != rows[j].Date {
			return rows[i].Date > rows[j].Date
		}
		if rows[i].Type != rows[j].Type {
			return rows[i].Type < rows[j].Type
		}
		return rows[i].ID > rows[j].ID
	})

	for _, row := range rows {
		response.Results = append(response.Results, Result{
			Type:        row.Type,
			ID:          row.ID,
			ParentID:    row.ParentID,
			Title:       row.Title,
			Snippet:     buildSnippet(filter.Query, row.Text),
			Date:        row.Date,
			Status:      row.Status,
			Sensitivity: row.Sensitivity,
			Score:       row.Score,
		})
	}

	s.log.DebugContext(ctx, "search completed",
		slog.Int("types", len(types)),
		slog.Int("results", len(response.Results)),
	)
	return response, nil
}

func normalizeTypes(values []string) ([]string, error) {
	if len(values) == 0 {
		return append([]string(nil), searchTypes...), nil
	}

	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		valid := false
		for _, candidate := range searchTypes {
			if value == candidate {
				valid = true
				break
			}
		}
		if !valid {
			return nil, apierrors.Validation("The request is invalid.", apierrors.Detail{
				Field:   "types",
				Code:    "INVALID_VALUE",
				Message: "Type must be journals, memories, insights, or analyses.",
			})
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return append([]string(nil), searchTypes...), nil
	}
	return result, nil
}

func singularType(value string) string {
	switch value {
	case TypeJournals:
		return "journal"
	case TypeMemories:
		return "memory"
	case TypeInsights:
		return "insight"
	case TypeAnalyses:
		return "analysis"
	default:
		return value
	}
}

func buildSnippet(query, text string) string {
	clean := strings.Join(strings.Fields(text), " ")
	if clean == "" {
		return ""
	}

	lower := strings.ToLower(clean)
	best := -1
	for _, token := range queryTokens(query) {
		index := strings.Index(lower, token)
		if index < 0 {
			continue
		}
		if best < 0 || index < best {
			best = index
		}
	}
	if best < 0 {
		return truncateRunes(clean, maxSnippetRunes)
	}

	runeIndex := utf8.RuneCountInString(clean[:best])
	runes := []rune(clean)
	start := runeIndex - snippetRadius
	if start < 0 {
		start = 0
	}
	end := runeIndex + snippetRadius
	if end > len(runes) {
		end = len(runes)
	}
	snippet := strings.TrimSpace(string(runes[start:end]))
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(runes) {
		snippet += "..."
	}
	return snippet
}

func queryTokens(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, `"'()-`)
		if field == "" || field == "or" || field == "and" || field == "not" {
			continue
		}
		tokens = append(tokens, field)
	}
	return tokens
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}
