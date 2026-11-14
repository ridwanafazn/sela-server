package journal

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/ridwanafazn/sela-server/internal/modules/ai"
)

const (
	vectorWeight  = 0.55
	textWeight    = 0.35
	recencyWeight = 0.10
	minScore      = 0.25
)

// Candidate is a retrieval result from one search channel.
type Candidate struct {
	JournalID string    `gorm:"column:journal_id"`
	Title     *string   `gorm:"column:title"`
	Content   string    `gorm:"column:content"`
	EntryDate time.Time `gorm:"column:entry_date"`
	Score     float64   `gorm:"column:score"`
}

// RetrievedJournal is a ranked journal with score breakdown.
type RetrievedJournal struct {
	JournalID    string
	Title        *string
	Content      string
	EntryDate    time.Time
	Score        float64
	VectorScore  float64
	TextScore    float64
	RecencyScore float64
}

// RetrievalService performs hybrid FTS + pgvector retrieval.
type RetrievalService struct {
	repo       *Repository
	embeddings ai.EmbeddingProvider
	model      string
	dimension  int
	log        *slog.Logger
	now        func() time.Time
}

// NewRetrievalService creates the retrieval service.
func NewRetrievalService(
	repo *Repository,
	embeddings ai.EmbeddingProvider,
	model string,
	dimension int,
	log *slog.Logger,
) *RetrievalService {
	if dimension <= 0 {
		dimension = 768
	}
	if log == nil {
		log = slog.Default()
	}
	return &RetrievalService{
		repo:       repo,
		embeddings: embeddings,
		model:      model,
		dimension:  dimension,
		log:        log,
		now:        time.Now,
	}
}

// Retrieve returns the top ranked active journals for a query.
func (s *RetrievalService) Retrieve(ctx context.Context, userID, query string, limit int) ([]RetrievedJournal, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []RetrievedJournal{}, nil
	}
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}

	textCandidates, err := s.repo.TextCandidates(ctx, userID, buildTextQuery(query), 30)
	if err != nil {
		return nil, err
	}

	vectorCandidates, vectorErr := s.vectorCandidates(ctx, userID, query)
	if vectorErr != nil {
		s.log.WarnContext(ctx, "vector retrieval unavailable; using text candidates only",
			slog.String("error", vectorErr.Error()),
		)
	}

	merged := make(map[string]*scoredCandidate, len(textCandidates)+len(vectorCandidates))
	for _, candidate := range textCandidates {
		merged[candidate.JournalID] = &scoredCandidate{candidate: candidate, text: candidate.Score}
	}
	for _, candidate := range vectorCandidates {
		existing, ok := merged[candidate.JournalID]
		if ok {
			existing.vector = candidate.Score
			continue
		}
		merged[candidate.JournalID] = &scoredCandidate{candidate: candidate, vector: candidate.Score}
	}

	maxText := 0.0
	for _, item := range merged {
		if item.text > maxText {
			maxText = item.text
		}
	}

	now := s.now().UTC()
	results := make([]RetrievedJournal, 0, len(merged))
	for _, item := range merged {
		textNorm := 0.0
		if maxText > 0 {
			textNorm = item.text / maxText
		}
		if textNorm < 0 {
			textNorm = 0
		}
		if textNorm > 1 {
			textNorm = 1
		}

		days := now.Sub(item.candidate.EntryDate).Hours() / 24
		if days < 0 {
			days = 0
		}
		recency := math.Pow(0.5, days/90)

		score := vectorWeight*item.vector + textWeight*textNorm + recencyWeight*recency
		if score < minScore {
			continue
		}

		results = append(results, RetrievedJournal{
			JournalID:    item.candidate.JournalID,
			Title:        item.candidate.Title,
			Content:      item.candidate.Content,
			EntryDate:    item.candidate.EntryDate,
			Score:        score,
			VectorScore:  item.vector,
			TextScore:    textNorm,
			RecencyScore: recency,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if !results[i].EntryDate.Equal(results[j].EntryDate) {
			return results[i].EntryDate.After(results[j].EntryDate)
		}
		return results[i].JournalID > results[j].JournalID
	})

	if len(results) > limit {
		results = results[:limit]
	}

	s.log.DebugContext(ctx, "journal retrieval completed",
		slog.Int("text_candidates", len(textCandidates)),
		slog.Int("vector_candidates", len(vectorCandidates)),
		slog.Int("returned", len(results)),
	)

	return results, nil
}

type scoredCandidate struct {
	candidate Candidate
	vector    float64
	text      float64
}

func (s *RetrievalService) vectorCandidates(ctx context.Context, userID, query string) ([]Candidate, error) {
	if s.embeddings == nil {
		return nil, nil
	}

	response, err := s.embeddings.Embed(ctx, ai.EmbeddingRequest{
		Model: s.model,
		Texts: []string{query},
	})
	if err != nil {
		return nil, err
	}
	if len(response.Embeddings) != 1 {
		return nil, ai.ErrInvalidResponse
	}
	if len(response.Embeddings[0]) != s.dimension {
		return nil, ai.ErrDimensionMismatch
	}

	return s.repo.VectorCandidates(ctx, userID, vectorString(response.Embeddings[0]), 30)
}

var retrievalStopwords = map[string]bool{
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

// buildTextQuery converts a natural-language message into an OR tsquery so a
// single matching term can retrieve relevant journals.
func buildTextQuery(query string) string {
	fields := strings.FieldsFunc(strings.ToLower(query), func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsDigit(value)
	})

	seen := make(map[string]bool, len(fields))
	tokens := make([]string, 0, 12)

	for _, field := range fields {
		if len([]rune(field)) < 3 || retrievalStopwords[field] || seen[field] {
			continue
		}
		seen[field] = true
		tokens = append(tokens, field)
		if len(tokens) == 12 {
			break
		}
	}

	return strings.Join(tokens, " OR ")
}
