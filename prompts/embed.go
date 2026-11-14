// Package prompts embeds versioned prompt templates.
package prompts

import _ "embed"

// CompanionV1 is the version 1 Companion system prompt.
//
//go:embed companion/v1.md
var CompanionV1 string

// CompanionVersion is stored with generated assistant messages and jobs.
const CompanionVersion = "companion/v1"
