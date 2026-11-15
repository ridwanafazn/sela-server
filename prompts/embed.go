// Package prompts embeds versioned prompt templates.
package prompts

import _ "embed"

// CompanionV1 is the version 1 Companion system prompt.
//
//go:embed companion/v1.md
var CompanionV1 string

// CompanionVersion is stored with generated assistant messages and jobs.
const CompanionVersion = "companion/v1"

// MemoryExtractionV1 is the version 1 memory extraction system prompt.
//
//go:embed memory_extraction/v1.md
var MemoryExtractionV1 string

// MemoryExtractionV2 is the version 2 memory extraction system prompt. It
// supports both journal entries and user messages.
//
//go:embed memory_extraction/v2.md
var MemoryExtractionV2 string

// MemoryExtractionVersion is recorded in extraction job metadata.
const MemoryExtractionVersion = "memory_extraction/v2"

// InsightV1 is the version 1 insight generation system prompt.
//
//go:embed insight/v1.md
var InsightV1 string

// InsightVersion is stored with generated insights and jobs.
const InsightVersion = "insight/v1"
