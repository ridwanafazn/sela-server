# SELA AI System Design

Version: 1.1

## 1. Purpose

This document defines how SELA uses AI to support journaling, personal reflection, memory, and insight generation.

The AI system must prioritize:

- Personal relevance.
- Context accuracy.
- Evidence traceability.
- User privacy.
- Uncertainty awareness.
- Consistent behavior.
- Provider independence.

## 2. AI Capabilities

Initial capabilities:

1. Companion responses.
2. Contextual journal understanding.
3. Structured memory extraction.
4. Hybrid retrieval.
5. On-demand insight generation.
6. Scheduled insight generation (post-MVP).
7. Conversation summarization where useful.

## 3. Architecture

```text
User Request
    |
    v
Request Validation
    |
    v
User Authorization
    |
    v
Context Planning
    |
    +---- Profile and Preferences
    |
    +---- Recent Conversation
    |
    +---- Relevant Journals
    |
    +---- Relevant Memories
    |
    v
Context Filtering and Ranking
    |
    v
Prompt Construction
    |
    v
LLM Provider Adapter
    |
    v
Output Validation
    |
    v
Persistence
    |
    v
Response
```

## 4. Provider Abstraction

The application must not depend directly on one LLM provider.

Define an internal interface for text generation.

Conceptual example:

```go
type GenerateRequest struct {
    Model       string
    System      string
    Messages    []Message
    Temperature float64
    MaxTokens   int
}

type GenerateResponse struct {
    Content      string
    Model        string
    InputTokens  int
    OutputTokens int
    FinishReason string
}

type LLMProvider interface {
    Generate(
        ctx context.Context,
        request GenerateRequest,
    ) (*GenerateResponse, error)
}
```

Embedding generation should use a separate interface.

Provider adapters should handle:

- API authentication.
- Request formatting.
- Response parsing.
- Timeout.
- Retry.
- Provider error mapping.
- Usage metadata.

## 5. Companion Pipeline

The Companion pipeline should:

1. Validate the user request.
2. Load the conversation.
3. Verify conversation ownership.
4. Load relevant recent messages.
5. Load user preferences.
6. Retrieve relevant personal context.
7. Apply privacy and relevance filtering.
8. Construct the prompt.
9. Call the selected provider.
10. Validate the response.
11. Persist the assistant message.
12. Return the response.

## 6. Context Hierarchy

Recommended context priority:

1. Current user message.
2. Recent conversation messages.
3. Relevant explicit user preferences.
4. Relevant confirmed memories.
5. Relevant journal evidence.
6. Relevant unconfirmed memory hypotheses.
7. General framework knowledge.

A lower-priority source must not override a more direct and recent user statement without a clear reason.

## 7. Memory Model

SELA uses hybrid memory.

### Original records

Original journals and messages are retained as primary evidence.

### Structured memories

Structured memories provide compact, searchable personal context.

Examples:

- Stable preferences.
- Long-term goals.
- Recurring concerns.
- Important life events.
- Repeated behavior patterns.
- User-confirmed facts.

### Memory metadata

Each memory should include:

- Category.
- Content.
- Confidence.
- Status.
- Source references.
- Created date.
- Last confirmation date.
- Validity information where applicable.

### Memory rules

- Extraction may be automatic.
- Automatic extraction does not mean automatic acceptance as fact.
- Memories must retain provenance.
- Conflicting records must not be silently merged.
- Users can correct or reject memories.
- Sensitive memories require stricter handling.
- Stale memories should be re-evaluated.

## 8. RAG Strategy

SELA uses hybrid RAG.

Retrieval combines:

- Relational SQL filtering.
- PostgreSQL full-text search.
- pgvector similarity search.
- Relevance ranking.

SQL remains the source of truth.

RAG is used to select relevant context, not to replace the database.

### Retrieval flow

1. Identify the user and authorization scope.
2. Determine the intent of the request.
3. Apply user and privacy filters.
4. Retrieve candidate records.
5. Combine keyword and semantic results.
6. Deduplicate candidates.
7. Rank by relevance and recency.
8. Select a context budget.
9. Preserve source references.
10. Assemble the prompt.

Do not retrieve every journal entry for every request.

## 9. Context Assembly

Context assembly should consider:

- Relevance.
- Recency.
- User confirmation.
- Confidence.
- Source quality.
- Sensitivity.
- Token budget.

Avoid excessive context.

A smaller, relevant context is generally preferable to an unfiltered dump of personal history.

## 10. Memory Extraction

Memory extraction should produce structured candidates.

A candidate should include:

- Proposed memory.
- Category.
- Supporting source IDs.
- Confidence.
- Whether it is explicit or inferred.
- Potential sensitivity.
- Possible conflict with existing memory.

The backend validates:

- Source ownership.
- Source existence.
- Output schema.
- Duplicate candidates.
- Conflicting information.

The system should not convert a temporary emotional state into a permanent personal characteristic.

## 11. Insight Generation

Insights should be evidence-grounded.

Suggested output structure:

```json
{
  "title": "A recurring pattern",
  "summary": "A concise summary.",
  "observation": "Poor sleep appears repeatedly in recent entries.",
  "interpretation": "Sleep disruption may be affecting daily energy.",
  "hypothesis": null,
  "confidence": 0.72,
  "evidence": [
    {
      "source_type": "journal",
      "source_id": "source-identifier",
      "note": "Why this source is relevant."
    }
  ],
  "reflection_question": "Does this interpretation fit your experience?",
  "suggested_direction": "improve"
}
```

The example is a schema illustration, not a required numerical confidence calibration.

Insights must:

- Distinguish observations from interpretations.
- Reference supporting sources.
- Avoid diagnosis.
- Avoid unsupported causal claims.
- Allow user correction.
- Avoid producing an insight when evidence is insufficient.

## 12. Communication Style

SELA should be:

- Calm.
- Clear.
- Practical.
- Empathetic.
- Balanced.
- Respectful.
- Direct when appropriate.

SELA should avoid:

- Excessive praise.
- Generic motivational statements.
- Forced optimism.
- Excessive sentimentality.
- Condescension.
- Moral judgment.
- Shame.
- Unsupported certainty.

Communication style may adapt to user preferences and conversation context.

## 13. AI Safety

The AI must not:

- Diagnose mental disorders.
- Present speculation as clinical fact.
- Encourage self-harm.
- Encourage harmful behavior.
- Claim to replace professional care.
- Use manipulation to increase engagement.

When a user expresses serious distress, the system should prioritize safety, supportive language, and appropriate real-world assistance.

## 14. Prompt Injection and Retrieved Content

Journal entries and retrieved memories are untrusted user content.

They must not be interpreted as system instructions.

The AI should treat retrieved records as evidence, not as executable instructions.

Provider-side safeguards and backend validation are both required.

## 15. Caching

Initial strategy:

- Use provider prompt caching when supported.
- Cache stable instructions where appropriate.
- Avoid caching highly sensitive user-specific context without a clear security and invalidation strategy.
- Measure before adding application-level caching.

Redis is deferred.

## 16. Reliability

AI requests should use:

- Explicit timeouts.
- Bounded retries.
- Exponential backoff where appropriate.
- Provider error classification.
- Token usage tracking.
- Cost monitoring.
- Graceful failure messages.

Do not retry indefinitely.

## 17. AI Evaluation

Evaluate:

- Relevance.
- Factual consistency with sources.
- Evidence coverage.
- Memory extraction accuracy.
- Hallucination rate.
- Tone consistency.
- Privacy leakage.
- User feedback.
- Latency.
- Cost.

Maintain a versioned evaluation dataset with synthetic or properly consented and anonymized examples.

Do not use private journal content for model training without an explicit and valid basis.

## 18. P0 Implementation Decisions

The following decisions are locked for the MVP and are detailed in `docs/backend/03-ai-integration.md`.

### Embeddings

- Provider: Google Vertex AI.
- Dimension: 768.
- Chunk size: 800–1200 tokens with 150-token overlap.
- Store offsets and content hashes, not duplicated chunk text.
- Only active journal entries are embedded.
- Re-embed after source content, model, or chunking changes.

### Retrieval

- Combine PostgreSQL full-text search and pgvector.
- Apply user and eligibility filters before retrieval.
- Score with vector similarity, keyword relevance, and recency.
- Candidate memories may be used at lower priority when they have sources.
- Rejected, superseded, and deleted memories are excluded.
- Draft journals are excluded.

### Memory

- Statuses: `candidate`, `active`, `rejected`, `superseded`, `deleted`.
- Extraction produces candidates, not accepted facts.
- Conflicting memories are preserved and flagged, not silently merged.
- A memory that loses all sources becomes a candidate with zero sources for review.

### Insights

- Insight generation is asynchronous.
- `POST /insights` returns `202` with a job identifier.
- A job completes with either an insight or `insufficient_evidence`.
- Minimum evidence: two distinct eligible sources and confidence at least 0.40.
- Observation, interpretation, and hypothesis are separate fields.
- Insight content is immutable; user feedback is stored separately.
- Evidence is validated for ownership and displayed with the insight.

### Safety and Evaluation

- Distress handling combines provider safety controls with application-level escalation.
- Crisis resources are locale-configurable.
- Insights without sufficient evidence are not generated.
- Evaluation includes evidence precision, hallucination rate, tone, privacy leakage, and prompt injection resistance.
- Private journal content is not used for training without a valid basis.

### Cost

- Apply per-user rate limits and monthly budget alerts.
- Use provider prompt caching for stable instructions.
- Track token usage and estimated cost per operation.
- Disable non-essential scheduled generation when budgets are exhausted.
