// Package textquery extracts significant search terms from natural-language
// input shared by journal and memory retrieval.
package textquery

import (
	"strings"
	"unicode"
)

// MaxTerms bounds the number of search terms used in a query.
const MaxTerms = 12

var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true,
	"this": true, "what": true, "have": true, "has": true, "had": true,
	"written": true, "write": true, "about": true, "please": true,
	"answer": true, "briefly": true, "can": true, "could": true,
	"would": true, "should": true, "you": true, "your": true, "was": true,
	"were": true, "are": true, "is": true, "from": true, "not": true,
	"but": true, "all": true, "any": true, "how": true, "why": true,
	"saya": true, "aku": true, "dan": true, "yang": true, "untuk": true,
	"dari": true, "ini": true, "itu": true, "apa": true, "apakah": true,
	"tentang": true, "bisa": true, "tolong": true, "jawab": true,
	"singkat": true, "adalah": true, "di": true, "ke": true, "nya": true,
	"juga": true, "saja": true, "masih": true, "sudah": true, "belum": true,
	"tidak": true, "tak": true, "ada": true, "sama": true, "dengan": true,
	"pada": true, "dalam": true, "akan": true, "atau": true, "karena": true,
	"jadi": true, "kalau": true, "jika": true,
}

// Terms returns up to max significant lowercase terms from the query.
// Repetition and short tokens are discarded.
func Terms(query string, max int) []string {
	if max <= 0 || max > MaxTerms {
		max = MaxTerms
	}

	fields := strings.FieldsFunc(strings.ToLower(query), func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsDigit(value)
	})

	seen := make(map[string]bool, len(fields))
	terms := make([]string, 0, max)

	for _, field := range fields {
		if len([]rune(field)) < 3 || stopwords[field] || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
		if len(terms) == max {
			break
		}
	}

	return terms
}

// Query renders significant terms as an OR tsquery. It returns an empty
// string when the input contains only stopwords.
func Query(query string) string {
	return strings.Join(Terms(query, MaxTerms), " OR ")
}
