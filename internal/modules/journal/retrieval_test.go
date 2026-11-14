package journal

import (
	"strings"
	"testing"
)

func TestBuildTextQueryKeepsSignificantTerms(t *testing.T) {
	query := buildTextQuery("What have I written about sleep and work?")
	if query == "" {
		t.Fatal("buildTextQuery returned an empty query")
	}
	if !strings.Contains(query, "sleep") || !strings.Contains(query, "work") {
		t.Errorf("query = %q, want sleep and work", query)
	}
	if strings.Contains(query, "what") || strings.Contains(query, "about") {
		t.Errorf("query = %q, contains stopwords", query)
	}
	if !strings.Contains(query, " OR ") {
		t.Errorf("query = %q, want OR semantics", query)
	}
}

func TestBuildTextQueryIndonesian(t *testing.T) {
	query := buildTextQuery("Apa yang saya tulis tentang tidur?")
	if !strings.Contains(query, "tulis") || !strings.Contains(query, "tidur") {
		t.Errorf("query = %q, want tulis and tidur", query)
	}
}

func TestBuildTextQueryStopwordsOnly(t *testing.T) {
	if query := buildTextQuery("what about the and please"); query != "" {
		t.Errorf("query = %q, want empty", query)
	}
}

func TestBuildTextQueryLimitsTerms(t *testing.T) {
	query := buildTextQuery(strings.Repeat("alpha beta gamma delta ", 10))
	if terms := strings.Count(query, " OR ") + 1; terms > 12 {
		t.Errorf("terms = %d, want at most 12", terms)
	}
}
