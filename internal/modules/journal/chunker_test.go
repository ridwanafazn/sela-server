package journal

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestChunkContentShort(t *testing.T) {
	content := "A short journal entry."
	chunks := ChunkContent(content, DefaultChunkOptions())
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}

	chunk := chunks[0]
	if chunk.Text != content {
		t.Errorf("text = %q, want %q", chunk.Text, content)
	}
	if chunk.StartOffset != 0 || chunk.EndOffset != len(content) {
		t.Errorf("offsets = %d..%d, want 0..%d", chunk.StartOffset, chunk.EndOffset, len(content))
	}
	if chunk.ContentHash != chunkHash(content) {
		t.Error("content hash does not match the chunk")
	}
}

func TestChunkContentLongIsOverlappingAndExact(t *testing.T) {
	content := strings.Repeat("sleep and work and reflection ", 400)
	chunks := ChunkContent(content, DefaultChunkOptions())
	if len(chunks) < 2 {
		t.Fatalf("chunks = %d, want at least 2", len(chunks))
	}

	for index, chunk := range chunks {
		if chunk.Index != index {
			t.Errorf("chunk index = %d, want %d", chunk.Index, index)
		}
		if chunk.Text == "" {
			t.Error("empty chunk text")
		}
		if got := content[chunk.StartOffset:chunk.EndOffset]; got != chunk.Text {
			t.Errorf("chunk %d offsets do not match text", index)
		}
		if chunk.ContentHash != chunkHash(chunk.Text) {
			t.Errorf("chunk %d hash mismatch", index)
		}
		if index > 0 && chunk.StartOffset >= chunks[index-1].EndOffset {
			t.Errorf("chunk %d does not overlap the previous chunk", index)
		}
		if runeCount := utf8.RuneCountInString(chunk.Text); runeCount > DefaultChunkOptions().MaxTokens*CharsPerToken {
			t.Errorf("chunk %d has %d runes, above the maximum", index, runeCount)
		}
	}
}

func TestChunkContentUnicodeOffsets(t *testing.T) {
	content := strings.Repeat("éngat tidur ", 1000)
	chunks := ChunkContent(content, DefaultChunkOptions())
	if len(chunks) < 2 {
		t.Fatalf("chunks = %d, want at least 2", len(chunks))
	}

	for index, chunk := range chunks {
		if got := content[chunk.StartOffset:chunk.EndOffset]; got != chunk.Text {
			t.Errorf("chunk %d byte offsets do not match the unicode text", index)
		}
	}
}

func TestChunkContentEmpty(t *testing.T) {
	if chunks := ChunkContent("", DefaultChunkOptions()); chunks != nil {
		t.Errorf("empty content produced %d chunks", len(chunks))
	}
}
