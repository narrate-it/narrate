# narration-craft-v1

Prompt version: `narrate-craft-v1`. Modules 1–8 are embedded in
`internal/prompts/*.txt` and assembled by `internal/narration/prompts.go`:
fidelity, voice, concreteness, pacing, listening, then the style module
(conversational or coach), then review.

## Provenance and deliberate adaptations

This is a consolidated adaptation of prior skill instructions the requester
described, not a verbatim archival copy of them. These names are attribution
only — narrate does not depend on or load any of these skills at runtime.

| Original skill or reference | How narrate preserves or handles it |
|---|---|
| `coaching-audio` | Fidelity, single-narrator script, coach structure, evidence limits, resumable rendering, measured timing, honest verification. Defaults become conversational and natural runtime. |
| `voice-conversation-craft` | Approachable register, contractions, fragments, specificity, precise exceptions. Live turn-taking, drift, teasing, and interruption are excluded from a complete monologue. |
| `podcast-craft-concreteness` | Evidence-backed quantities, examples, useful callbacks, and actionable advice. Interview drilling becomes source checking; missing evidence is never invented. |
| `podcast-craft-emotional-pacing` | Breathing room, build and release where supported, practical relief, genre sensitivity. No staged guest emotions, spoken stage directions, forced humor, or fabricated disclosure. |
| Project-briefing and sitrep instructions | Source-aware structure and honest status language in the conversational module, applied only to supplied text. No repository inspection or evidence gathering. |
| `podcast-generation` | Plain spoken text, accessible explanations, source language, ordered synthesis, transcript delivery. Its Python pipeline, two hosts, greeting, fixed duration default, and blanket metadata omissions do not transfer. |
| `spark` | Infrastructure-specific workload management is excluded; cancellation, bounded retries, progress, and cleanup are implemented directly in Go. No Spark service or operational skill is embedded. |
| `podcast-script-writing` | Plan/draft/review discipline is adapted into chunk context and the review module. Its interview format and mandatory guest devices are excluded. |
| `podcast-craft-question-architecture` | Plain-language explanation and preservation of source tensions transfer. No fabricated interview, performed ignorance, stance-forcing, or invented quotes. |
| `podcast-craft-rapport` | Warmth and source-grounded specificity transfer. No fake affirmations, claims of research, or guest responses. |
| Conditional interview techniques (cold open, narrative spine, reciprocal vulnerability, tangent threading, closing ritual) | No additional runtime dependencies; the listening/style/review modules provide orientation, continuity, and closure without fabricated tension or guest prompts. |

The product specification and fidelity rules resolve any conflict between
this table and the module text itself; the module text in
`internal/prompts/` is authoritative.
