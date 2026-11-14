# SELA Product Specification

Version: 1.1  
Status: MVP baseline

## 1. Product Overview

SELA is an AI journaling and personal reflection companion.

SELA helps users:

- Express experiences through writing.
- Understand their thoughts and emotions.
- Recognize recurring behavioral and situational patterns.
- Connect current experiences with relevant past experiences.
- Explore possible contributing factors.
- Make more intentional decisions.
- Track personal development without reducing life to scores.

SELA is not a productivity tracker, clinical diagnostic application, or motivational quote generator.

## 2. Product Identity

Product name: SELA

Tagline:

> A space to understand yourself.

Product philosophy:

> Remember. Understand. Grow.

Core positioning:

A personal space where users can reflect on their lives with the assistance of an AI companion that remembers relevant context, identifies patterns, and helps them think more clearly.

## 3. Product Principles

### 3.1 Personal-first

The user's journal is the primary source of personal context.

SELA must prioritize the user's actual experiences over generic assumptions.

### 3.2 Reflection over judgment

SELA should help users understand their circumstances without moralizing, shaming, or labeling them.

### 3.3 Balanced companionship

The AI should be empathetic but not blindly agreeable.

It may respectfully challenge assumptions when supported by context.

### 3.4 Evidence-aware insights

Personal insights must be connected to available evidence.

SELA must distinguish:

- User-stated facts.
- Repeated observations.
- Possible interpretations.
- Unverified hypotheses.

### 3.5 User agency

Users must be able to inspect, correct, reject, and delete memories and insights.

### 3.6 Simple experience

The product interface should remain focused and avoid unnecessary complexity.

### 3.7 Privacy by default

Personal journals and conversations are private user data.

## 4. Target User Needs

SELA addresses several recurring needs:

1. A place to write freely without needing to organize everything.
2. A companion to discuss personal experiences.
3. Help identifying patterns across multiple journal entries.
4. Continuity between conversations and past experiences.
5. A structured way to reflect on decisions and personal challenges.
6. Practical suggestions without pressure to constantly improve.

## 5. Core Product Modules

SELA has four primary modules.

### 5.1 Journal

Purpose: capture personal experiences and thoughts.

Features:

- Free-form journal writing.
- Guided writing prompts.
- Journal history.
- Journal detail.
- Search and filtering.
- Optional AI analysis initiated by the user (post-MVP).

Requirements:

- Original journal content must remain unchanged by AI.
- AI analysis must be stored separately.
- Users can edit and delete their own entries.
- Journal entries must not automatically become definitive psychological facts.

### 5.2 Companion

Purpose: provide conversational reflection with personal context.

Features:

- Text-based AI conversation.
- Contextual responses.
- Relevant memory retrieval.
- Follow-up questions.
- Reflection and decision support.

Behavior:

- Empathetic but not excessively sentimental.
- Direct but not harsh.
- Willing to challenge unsupported assumptions.
- Avoids generic praise.
- Avoids forced advice.
- Adapts communication style to the user's needs.

### 5.3 Insights

Purpose: identify meaningful patterns from journals and conversations.

Features:

- On-demand insight generation.
- Weekly summaries (post-MVP).
- Monthly reflections (post-MVP).
- Pattern observations.
- Evidence references.
- User feedback.

Each insight should include:

- Title.
- Summary.
- Supporting evidence.
- Interpretation.
- Confidence level or uncertainty description.
- Suggested reflection.
- User feedback status.

Insights must not present hypotheses as established facts.

### 5.4 My Profile

Purpose: manage identity, preferences, and personal context.

Features:

- Basic profile.
- Communication preferences.
- Personal goals.
- Reflection preferences.
- Memory management.
- Privacy settings.
- Account management.

Users should have control over what personal context is used by SELA.

## 6. MVP Scope

### Included

- Authentication.
- User profile.
- Journal CRUD.
- Server-side journal drafts.
- Companion conversations.
- AI-generated responses.
- Context retrieval.
- Basic structured memory.
- User memory feedback.
- On-demand insights.
- Account deletion.
- Basic privacy controls.
- API documentation.

### Deferred

- Voice journaling.
- Audio transcription.
- Social networking.
- Community features.
- Public journal sharing.
- Advanced gamification.
- Wearable integrations.
- Complex financial or productivity integrations.
- Multi-agent architecture.
- Dedicated Python AI service.
- Subscription and payment gateway.
- Granular privacy preference center.
- Scheduled insight generation.

## 7. Functional Requirements

| ID | Requirement |
|---|---|
| FR-001 | Users can register and authenticate. |
| FR-002 | Users can manage their profile. |
| FR-003 | Users can create journal entries. |
| FR-004 | Users can view journal history. |
| FR-005 | Users can edit and delete their own journals. |
| FR-006 | Users can start Companion conversations. |
| FR-007 | Users can send messages and receive AI responses. |
| FR-008 | Companion can retrieve relevant personal context. |
| FR-009 | The system can extract structured memories. |
| FR-010 | Users can inspect and provide feedback on memories. |
| FR-011 | Users can generate insights on demand. |
| FR-012 | Post-MVP: the system can generate scheduled insights. |
| FR-013 | Users can inspect insight evidence. |
| FR-014 | Users can manage basic communication preferences in MVP; granular privacy preferences are post-MVP. |
| FR-015 | Users can delete their account and associated data. |

## 8. Non-functional Requirements

### Security

- All authenticated resources must enforce ownership.
- Passwords must be securely hashed.
- Tokens must have defined expiration and revocation behavior.
- Sensitive journal content must not appear in ordinary application logs.

### Reliability

- API errors must follow a consistent structure.
- Background jobs must be durable and retryable.
- Duplicate job execution must not create duplicate results.
- Database migrations must be versioned.

### Performance

Initial engineering targets:

- Ordinary API requests: p95 below 500 ms, excluding AI generation.
- AI response latency: measure separately by provider and model.
- Pagination required for potentially large collections.
- Database queries must use appropriate indexes.

These are initial targets, not guaranteed service-level commitments.

### Maintainability

- Modular monolith.
- Clear package boundaries.
- Unit and integration tests.
- Versioned API contracts.
- Automated CI checks.

## 9. Primary User Journeys

### Journey A: Write a Journal

1. User opens Journal.
2. User creates an entry.
3. User writes freely.
4. User saves the entry.
5. Entry is stored as the original source.
6. Optional background processing may extract memories.

### Journey B: Talk to Companion

1. User opens Companion.
2. User starts or continues a conversation.
3. Backend retrieves relevant context.
4. AI generates a response.
5. Conversation is stored.
6. Relevant memory extraction may occur asynchronously.

### Journey C: Generate an Insight

1. User opens Insights.
2. User requests an analysis.
3. Backend gathers eligible personal context.
4. AI identifies possible patterns.
5. Backend stores the insight and evidence.
6. User reviews and provides feedback.

## 10. Product Success Metrics

Initial product metrics:

- Weekly active users.
- Journal creation frequency.
- Companion conversation frequency.
- Insight generation frequency.
- User feedback on insight usefulness.
- Memory correction and rejection rate.
- Retention over time.
- User-reported clarity after reflection.

Avoid optimizing solely for time spent in the application.

## 11. Safety Boundaries

SELA:

- MUST NOT diagnose mental disorders.
- MUST NOT claim to replace a psychologist, psychiatrist, or medical professional.
- MUST NOT encourage self-harm.
- MUST NOT use guilt or shame to increase engagement.
- MUST NOT present uncertain interpretations as facts.
- SHOULD encourage appropriate professional support when the situation exceeds the product's scope.

## 12. MVP Acceptance Criteria

The MVP is acceptable when:

- A user can securely register and log in.
- A user can create, read, update, and delete journals.
- A user can hold persistent Companion conversations.
- Companion can use relevant authorized context.
- The system can store structured memories with provenance.
- Users can inspect and correct memories.
- Users can request an insight and inspect its evidence.
- Users can request account deletion with re-authentication.
- Users cannot access another user's data.
- The API is documented.
- Automated tests cover critical flows.
- The application can be deployed through the defined CI/CD process.
