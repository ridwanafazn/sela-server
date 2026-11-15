# Safety, Evaluation, and Cost Controls

## Safety escalation

SELA applies deterministic application-level distress signals before calling a
model:

- `crisis`: explicit self-harm, suicide, or immediate danger. The Companion
  returns a bounded safety message without calling the model, suggests local
  emergency services and professional resources, and does not continue ordinary
  reflection prompts. Message memory extraction is skipped.
- `elevated`: hopelessness or severe distress. The Companion receives an active
  safety instruction and locale resources in its system prompt. If the model
  does not mention a configured resource, a bounded resource footer is appended
  deterministically. Message memory extraction is skipped.
- `none`: ordinary Companion behavior.

Crisis resources live in `internal/platform/safety/resources.json` and can be
overridden with `SAFETY_RESOURCES_FILE`. The embedded resources are examples and
must be reviewed for the deployment locale before production. The backend never
logs message content for safety events; only level, category, and identifiers.

## Evaluation

`internal/evaluation/cases.json` is the versioned deterministic evaluation
dataset. Run:

```bash
make eval
```

The current gates enforce 100% on:

- safety classification cases,
- prompt-injection escaping and untrusted-content labeling.

Existing unit and integration suites additionally enforce insight evidence
coverage, rejected/superseded memory exclusion, source ownership, and memory
validation. Live model quality (tone, factual accuracy, hallucination rate)
requires a separate run with a real provider and human review; deterministic
tests cannot prove those qualities.

## Cost controls

Every AI operation writes an `ai_usage_events` row with provider, model, token
usage, latency, success, error category, and estimated cost. Prompt and source
content are never stored in this table.

Pricing and budgets are environment configuration:

```env
AI_INPUT_COST_PER_MILLION=0
AI_OUTPUT_COST_PER_MILLION=0
AI_EMBEDDING_COST_PER_MILLION=0
AI_USER_MONTHLY_BUDGET_USD=0
AI_GLOBAL_MONTHLY_BUDGET_USD=0
```

A value of zero disables pricing or a budget. When a monthly budget is
exhausted:

- Companion and insight requests return `429 RATE_LIMITED`;
- background memory extraction completes without calling the provider.

Usage rows cascade when the account is deleted. Cost per insight is available
by joining `ai_usage_events.reference_id` to `insights.id` for
`operation = 'insight.generate'`.

Provider retry counts are not yet exposed by the adapter interface; latency and
error categories are tracked.
