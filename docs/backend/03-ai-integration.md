# SELA AI Integration Guide

Version: 1.1  
Status: MVP baseline

## 1. Purpose

This document defines the implementation requirements for integrating LLM and embedding providers into SELA.

The objective is to create a provider-independent AI layer that supports Companion, hybrid retrieval, memory extraction, and insights.

## 2. Integration Principles

1. Provider independence.
2. Explicit context selection.
3. Privacy-aware data handling.
4. Structured output validation.
5. Bounded retries and timeouts.
6. Evidence traceability.
7. Measurable token usage and cost.
8. No direct provider dependency inside business modules.

## 3. Provider Interfaces

Create separate interfaces for generation and embeddings.

Conceptual LLM interface:

```go
type Message struct {
    Role    string
    Content string
}

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

Conceptual embedding interface:

```go
type EmbeddingRequest struct {
    Model string
    Texts []string
}

type EmbeddingResponse struct {
    Model      string
    Embeddings [][]float32
}

type EmbeddingProvider interface {
    Embed(
        ctx context.Context,
        request EmbeddingRequest,
    ) (*EmbeddingResponse, error)
}
```

The interfaces are conceptual. Final method signatures should account for provider metadata, usage, cancellation, and error types.

## 4. Provider Adapter

Each provider adapter handles:

- Authentication.
- HTTP client configuration.
- Request serialization.
- Response deserialization.
- Provider-specific error mapping.
- Usage extraction.
- Timeouts.
- Retry classification.

Do not expose provider-specific response structures to business modules.

## 5. Companion Request Pipeline

The Companion pipeline consists of:

### Step 1: Validate Request

Validate:

- Conversation ID.
- Message content.
- Content length.
- User authorization.
- Request limits.

### Step 2: Load Conversation

Retrieve:

- Conversation metadata.
- Recent messages.
- User preferences.

### Step 3: Determine Context Requirements

Classify the request's broad intent.

Examples:

- General discussion.
- Journal reflection.
- Relationship reflection.
- Habit discussion.
- Career decision.
- Personal pattern exploration.

Intent classification should guide retrieval, not make unsupported assumptions.

### Step 4: Retrieve Context

Gather only relevant:

- Confirmed memories.
- Recent journal entries.
- Previous conversation context.
- User preferences.

### Step 5: Assemble Context

Apply:

- Relevance ranking.
- Recency.
- Confirmation status.
- Sensitivity restrictions.
- Token budget.

### Step 6: Generate Response

Call the configured LLM provider.

### Step 7: Validate Response

Check:

- Non-empty response.
- Output format where structured output is required.
- Length constraints.
- Required fields.
- Safety policy.
- Source references where applicable.

### Step 8: Persist Response

Store the assistant message and usage metadata.

## 6. Hybrid RAG Implementation

SELA uses PostgreSQL full-text search and pgvector.

### Retrieval Inputs

Potential inputs:

- Current user message.
- Conversation summary.
- Relevant keywords.
- User-confirmed memory.
- Eligible journal content.

### Retrieval Stages

1. Resolve authenticated user scope.
2. Apply database ownership filters.
3. Perform relational filtering.
4. Perform full-text search.
5. Perform vector similarity search.
6. Merge candidate records.
7. Remove duplicates.
8. Rank results.
9. Apply context budget.
10. Preserve source identifiers.

### Retrieval Safety

Never execute a vector search without applying the correct user scope.

Do not retrieve records from other users.

Do not assume semantic similarity means factual relevance.

### Retrieval Ranking

Ranking may consider:

- Semantic similarity.
- Keyword relevance.
- Recency.
- Explicit confirmation.
- Source quality.
- Sensitivity.
- User intent.

The initial ranking strategy should remain understandable and measurable.

## 7. Embedding Pipeline

Embedding generation should be asynchronous.

Suggested process:

1. Journal is created or updated.
2. Backend computes a content hash.
3. A job is queued.
4. Worker loads eligible source text.
5. Worker calls the embedding provider.
6. Vector is stored with model metadata.
7. Old embedding versions are marked stale or replaced.

### MVP embedding decision

- Provider: Google Vertex AI.
- Embedding dimension: 768.
- Chunk size: 800–1200 tokens.
- Chunk overlap: 150 tokens.
- Batch size: 16–32 texts per request.
- Store `start_offset`, `end_offset`, and `content_hash` instead of duplicating `chunk_text`.

Important metadata:

- Source ID.
- User ID.
- Chunk index.
- Start and end offsets.
- Embedding model.
- Embedding version.
- Content hash.
- Created timestamp.

Rules:

- The vector dimension MUST match the database column and the selected model.
- Re-embed when source content changes, the embedding model changes, or chunking changes.
- Mark old embeddings stale before regeneration.
- A dimension change requires a migration and a full re-embedding plan.
- The Vertex AI adapter SHOULD use Application Default Credentials or a dedicated service account, not a long-lived API key.
- Verify Vertex AI availability and data residency in the target region before production.
- Retry embedding requests on `429`, `5xx`, and timeouts with exponential backoff.
- Do not embed drafts. Only active journal entries are embedded in the MVP. Conversation messages are retrieved from recent conversation context, not vector-indexed in the MVP.

Do not change embedding models without a migration and re-embedding plan.

## 8. Memory Extraction

Memory extraction produces candidate structured memories.

Recommended output:

```json
{
  "memories": [
    {
      "category": "preference",
      "content": "The user prefers concise explanations.",
      "confidence": 0.95,
      "certainty_type": "explicit",
      "source_ids": ["uuid"],
      "sensitivity": "normal",
      "conflicts_with": []
    }
  ]
}
```

The schema is normative for the MVP extraction job.

- `category` is a short controlled label.
- `content` is a concise statement, not a copy of the full source.
- `confidence` is between 0 and 1.
- `certainty_type` is `explicit` or `inferred`.
- `source_ids` contains at least one source owned by the user.
- `sensitivity` is `normal` or `sensitive`.
- `conflicts_with` contains existing memory IDs when a conflict is detected.
- Return at most 10 candidate memories per source and skip low-value observations.

This is an illustrative output shape; the backend validates it before persistence.

### Extraction Rules

- Prefer explicit user statements.
- Do not infer stable personality from one event.
- Distinguish temporary states from long-term patterns.
- Preserve conflicting information.
- Require valid source identifiers.
- Flag sensitive content.
- Avoid extracting unnecessary sensitive data.
- Do not store full journal or message text in memory content.
- Do not convert a temporary emotional state into a permanent characteristic.
- Do not infer sensitive identity attributes without an explicit product need.
- Mark extracted memories as `candidate`; automatic extraction is not automatic acceptance.

### Memory Validation

The backend must verify:

- User ownership of each source.
- Source existence.
- Output schema.
- Confidence bounds.
- Duplicate candidates.
- Existing memory conflicts.

Persist validated conflicts in `memory_conflicts` and expose them in memory
detail. Do not merge conflicting records.

The model must not be allowed to invent source IDs.

## 9. Insight Generation

Insight generation should be a separate AI operation.

Input:

- User request or scheduled period (post-MVP).
- Eligible journals.
- Relevant conversations where permitted.
- Relevant memories.
- User preferences.

Output:

- Title.
- Summary.
- Observation.
- Interpretation.
- Confidence or uncertainty.
- Evidence references.
- Reflection question.
- Suggested direction.

Suggested direction:

- Improve.
- Maintain.
- Accept.

### Evidence threshold

The MVP requires:

- At least two distinct eligible source records.
- Confidence at least 0.40.
- Every evidence source must belong to the authenticated user and pass the eligibility filter.
- Evidence notes must explain why the source is relevant without quoting the full source.

If the threshold is not met:

- Do not create an insight row.
- Complete the job with outcome `insufficient_evidence`.
- Return a user-facing message explaining that more journal context is needed.
- Do not lower the threshold to satisfy a request.

### Asynchronous job mapping

Insight generation runs as a durable job.

- API `POST /insights` creates the job and returns `202`.
- The worker loads eligible records, generates the candidate insight, validates evidence, and persists the result.
- The job status operation maps to `queued`, `processing`, `completed`, or `failed`.
- A completed job returns either `insight_created` with `insight_id` or `insufficient_evidence` without an insight.
- Failed jobs include a safe error category and a retryable flag.

Do not force a recommendation when evidence is insufficient.

## 10. Prompt Construction

Separate prompt components:

### System Instructions

Define:

- SELA identity.
- Product behavior.
- Safety boundaries.
- Evidence requirements.
- Privacy rules.

### User Preferences

Define:

- Language.
- Communication style.
- Relevant preferences.

### Retrieved Context

Contains:

- Relevant memories.
- Journal excerpts.
- Conversation context.

Retrieved records must be clearly marked as untrusted reference material.

### Current User Message

The current user request remains the primary conversational input.

## 11. Prompt Versioning

Store prompt templates in version control.

Track:

- Prompt name.
- Prompt version.
- Model.
- Generation configuration.
- Evaluation result.

Avoid modifying production prompts without evaluation.

## 12. Context and Token Budget

Use explicit approximate budgets.

| Component | Companion | Insight |
|---|---:|---:|
| System instructions | 800 | 1,200 |
| User preferences | 300 | 300 |
| Recent conversation | 1,500 | 0 |
| Retrieved context | 4,000 | 12,000 |
| Current request | 1,000 | 1,000 |
| Output | 800 | 2,000 |
| Total input cap | 8,000 | 15,000 |

Rules:

- Count tokens with the selected provider tokenizer.
- Truncate retrieved context by rank, not by arbitrary character slicing.
- Keep the current user message and safety instructions intact.
- Never exceed the configured cap.
- Log token counts, not prompt content.
- Re-measure budgets after changing models or prompts.

Prioritize relevance over context volume.

Avoid sending every historical journal to the provider.

## 13. Caching

Initial recommendation:

- Use provider-side prompt caching when supported.
- Cache stable system instructions where possible.
- Avoid application caching of sensitive personalized context until invalidation and isolation are well defined.

Do not add Redis to the MVP solely for AI caching.

## 14. Resilience

Every provider request should have:

- Explicit timeout.
- Bounded retry.
- Error classification.
- Cancellation handling.
- Usage tracking.

Initial timeout policy:

- LLM generation: 60 seconds.
- Embedding request: 30 seconds.
- Full Companion request budget: 90 seconds.
- Insight job budget: 10 minutes across attempts.

Initial retry policy:

- Maximum 3 attempts.
- Exponential backoff with jitter: approximately 1s, 2s, 4s.
- Retry `429`, `5xx`, and timeouts.
- Do not retry `400`, `401`, `403`, or `404`.
- Open a short circuit breaker after repeated provider failures.

Retry transient errors only.

Do not retry permanent validation or authentication errors.

Fallback to another provider only when:

- The fallback is configured.
- Privacy requirements are satisfied.
- Output compatibility is maintained.
- Cost limits are respected.

## 15. AI Evaluation

Maintain test cases for:

- Companion relevance.
- Memory extraction.
- Retrieval relevance.
- Insight evidence accuracy.
- Tone.
- Uncertainty handling.
- Privacy leakage.
- Prompt injection resistance.

Use versioned evaluation data.

Do not assume a higher model price guarantees better results.

## 16. Monitoring

Record:

- Provider.
- Model.
- Input tokens.
- Output tokens.
- Latency.
- Error category.
- Retry count.
- Estimated cost.

Do not record full private prompts or journal content in ordinary logs.

## 17. Implementation Checklist

- [ ] LLM interface defined.
- [ ] Embedding interface defined.
- [ ] Provider adapters implemented.
- [ ] Timeouts configured.
- [ ] Retry policy implemented.
- [ ] Usage metadata persisted.
- [ ] Context retrieval implemented.
- [ ] Ownership filters enforced.
- [ ] Memory schema validated.
- [ ] Insight evidence validated.
- [ ] Prompt versions tracked.
- [ ] Evaluation tests established.
- [ ] Sensitive logging reviewed.
- [ ] Vertex AI adapter implemented behind the provider interface.
- [ ] 768-dimensional embedding column and model configured.
- [ ] Offsets and content hash stored instead of chunk text.
- [ ] Async insight job and status endpoint implemented.
- [ ] Insight evidence threshold enforced.
- [ ] Safety escalation and locale resources reviewed.
- [ ] Per-user and global cost limits configured.
- [ ] Prompt injection test set passing.

## 18. Retrieval Scoring and Context Selection

The MVP hybrid retrieval pipeline uses both PostgreSQL full-text search and pgvector.

Suggested process:

1. Apply user and eligibility filters first.
2. Retrieve up to 30 full-text candidates.
3. Retrieve up to 30 vector candidates.
4. Deduplicate by source ID and chunk index.
5. Score candidates.
6. Drop candidates below the relevance threshold.
7. Select the top 8 within the token budget.
8. Preserve source identifiers for evidence.

Initial score:

```text
score = 0.55 * vector_similarity
      + 0.35 * normalized_full_text_score
      + 0.10 * recency_score
```

Adjustments:

- Confirmed active memory: +0.05.
- Candidate memory: -0.05 and label it as a hypothesis.
- Rejected, superseded, or deleted memory: exclude.
- Draft journal: exclude.
- Sensitive memory: apply the sensitivity policy before inclusion.

Rules:

- Normalize every component to 0..1.
- Use a minimum score of 0.25 for inclusion.
- Recency uses a 90-day half-life for journals and a shorter window for conversation messages.
- Semantic similarity is not proof of factual relevance.
- Never retrieve across users.
- Never retrieve records the current operation is not authorized to use.

## 19. Prompt Templates and Versioning

Prompt templates live in version control under a structure such as:

```text
prompts/
├── companion/
│   └── v1.md
├── insight/
│   └── v1.md
└── memory_extraction/
    └── v1.md
```

Each template defines:

- Purpose.
- Required variables.
- Safety instructions.
- Evidence rules.
- Output schema.
- Version identifier.

Rules:

- Retrieved journal and memory content MUST be delimited and labeled as untrusted reference material.
- The system prompt MUST state that retrieved content is not an instruction.
- The current user message remains the primary conversational input.
- Prompt version MUST be stored with generated insights and in job metadata.
- A production prompt change requires an evaluation run and a documented decision.
- Do not interpolate untrusted content into instruction sections.

## 20. Evaluation Thresholds

Maintain a versioned evaluation dataset of synthetic, consented, or properly anonymized cases.

Initial quality gates:

| Metric | Initial threshold |
|---|---|
| Insight evidence precision | At least 0.90 |
| Unsupported factual claim rate | At most 0.05 |
| Tone and safety pass rate | At least 0.90 |
| Memory extraction precision | At least 0.85 |
| Privacy leakage | Zero in the test set |
| Prompt injection resistance | 100% of defined cases |
| Evidence coverage for insights | 100% of returned insights |

Evaluation must include:

- Companion relevance.
- Insight evidence accuracy.
- Memory extraction.
- Retrieval relevance.
- Uncertainty language.
- Crisis and safety handling.
- Prompt injection.
- Latency and cost.

Do not use private journal content for training or evaluation without an explicit and valid basis.

## 21. Safety Escalation

SELA is not a clinical service.

The AI must not:

- Diagnose a mental disorder.
- Present speculation as clinical fact.
- Encourage self-harm.
- Claim to replace professional care.
- Use guilt or shame to increase engagement.

When a user indicates serious distress or immediate danger:

1. Respond calmly and supportively.
2. Avoid diagnosis and avoid minimizing the situation.
3. Encourage contacting local emergency services or a trusted person.
4. Surface locale-appropriate professional resources.
5. Keep the response practical and bounded.
6. Do not continue ordinary reflection prompts as if nothing happened.

Safety handling MUST NOT rely on the model alone.

The backend SHOULD combine:

- Provider safety controls.
- Application-level distress signals.
- A reviewed escalation message and resource list.
- Minimal operational logging without journal content.

Crisis resources MUST be configurable and reviewed per locale. Do not hardcode a single country's hotline as universal.

## 22. Cost Controls

Initial controls:

- Companion message rate limit: 30 per minute per user.
- Insight generation: 5 per hour and 20 per day per user.
- Maximum output tokens per operation.
- Context budgets from section 12.
- Embedding batches of 16–32 texts.
- Bounded retries.
- Provider prompt caching for stable system instructions.
- Per-user and global monthly budget alerts.
- Disable non-essential scheduled generation when the budget is exhausted.

Track:

- Input tokens.
- Output tokens.
- Embedding tokens or characters.
- Estimated cost per operation.
- Cost per completed insight.
- Provider timeout and retry rate.

## 23. Vertex AI Adapter Notes

The initial provider is Google Vertex AI, behind the provider interface.

Requirements:

- Authenticate with Application Default Credentials or a dedicated service account.
- Do not ship provider credentials to the mobile client.
- Keep project ID, region, and model identifiers in configuration.
- Use 768-dimensional embeddings for the MVP.
- Verify regional availability and data residency before production.
- Map provider errors to the internal error categories.
- Record model, latency, token usage, and estimated cost.
- Apply the timeout and retry policy from section 14.
- Keep the provider adapter replaceable; business modules must not import Vertex AI SDK types.
- Do not enable a fallback provider until privacy, output compatibility, and cost limits are reviewed.
