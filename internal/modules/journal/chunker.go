package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"unicode"
	"unicode/utf8"
)

// CharsPerToken is the conservative approximation used for chunk sizing.
// Provider tokenizers differ; this bound keeps chunks in the intended range.
const CharsPerToken = 4

// ChunkOptions controls content chunking.
type ChunkOptions struct {
	MaxTokens     int
	MinTokens     int
	OverlapTokens int
}

// DefaultChunkOptions returns the MVP chunking configuration.
func DefaultChunkOptions() ChunkOptions {
	return ChunkOptions{
		MaxTokens:     1200,
		MinTokens:     800,
		OverlapTokens: 150,
	}
}

// Chunk is a slice of source content with byte offsets.
type Chunk struct {
	Index       int
	StartOffset int
	EndOffset   int
	Text        string
	ContentHash string
}

// ChunkContent splits content into overlapping, word-aligned chunks.
func ChunkContent(content string, options ChunkOptions) []Chunk {
	if content == "" {
		return nil
	}
	if options.MaxTokens <= 0 {
		options = DefaultChunkOptions()
	}
	if options.MinTokens <= 0 || options.MinTokens > options.MaxTokens {
		options.MinTokens = options.MaxTokens / 2
	}
	if options.OverlapTokens < 0 || options.OverlapTokens >= options.MinTokens {
		options.OverlapTokens = 0
	}

	runes := []rune(content)
	byteOffsets := make([]int, len(runes)+1)
	offset := 0
	for index, value := range runes {
		byteOffsets[index] = offset
		offset += utf8.RuneLen(value)
	}
	byteOffsets[len(runes)] = offset

	maxRunes := options.MaxTokens * CharsPerToken
	minRunes := options.MinTokens * CharsPerToken
	overlapRunes := options.OverlapTokens * CharsPerToken

	if len(runes) <= maxRunes {
		text := string(runes)
		return []Chunk{{
			Index:       0,
			StartOffset: 0,
			EndOffset:   len(content),
			Text:        text,
			ContentHash: chunkHash(text),
		}}
	}

	chunks := make([]Chunk, 0, len(runes)/maxRunes+1)
	index := 0
	start := 0

	for start < len(runes) {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}

		if end < len(runes) {
			boundary := end
			for boundary > start+minRunes && !unicode.IsSpace(runes[boundary-1]) {
				boundary--
			}
			if boundary > start {
				end = boundary
			}
		}

		for start < end && unicode.IsSpace(runes[start]) {
			start++
		}
		for end > start && unicode.IsSpace(runes[end-1]) {
			end--
		}

		if end > start {
			text := string(runes[start:end])
			chunks = append(chunks, Chunk{
				Index:       index,
				StartOffset: byteOffsets[start],
				EndOffset:   byteOffsets[end],
				Text:        text,
				ContentHash: chunkHash(text),
			})
			index++
		}

		if end >= len(runes) {
			break
		}

		next := end - overlapRunes
		if next <= start {
			next = end
		}
		for next < len(runes) && !unicode.IsSpace(runes[next]) {
			next++
		}
		if next <= start {
			next = end
		}
		start = next
	}

	return chunks
}

func chunkHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func chunkTexts(chunks []Chunk) []string {
	texts := make([]string, len(chunks))
	for index, chunk := range chunks {
		texts[index] = chunk.Text
	}
	return texts
}
