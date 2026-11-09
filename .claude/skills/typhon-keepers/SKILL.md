---
name: typhon-keepers
description: Use when the user asks to check that something in Typhon Launcher is still up to date — legal texts and consents, release notes, docs/ knowledge base, agent instructions, the Go↔frontend contract, dependencies and CI, invariant hygiene — or when preparing a release.
---

# Keepers

Seven read-only Sonnet agents. Each returns mismatches with file:line and ready patches; the
orchestrator applies, verifies and commits. One agent = one area.

| Agent | Keeps current | Runs |
|---|---|---|
| `typhon-deps-keeper` | go.mod, npm, the anacrolix fork, Wails, `ci.yml`/`release.yml` supply chain | before every release |
| `typhon-legal-keeper` | PRIVACY/TERMS/COPYRIGHT/THIRD_PARTY ru+en, `legal.Version`, `CurrentTelemetryConsent`, site copies | before every release; after a feature that sends data out |
| `typhon-changelog-keeper` | CHANGELOG.md / .en.md in the `signrelease` format, version in all sources | on the user's request |
| `typhon-docs-keeper` | `docs/*` against the code, audit and memory statuses | on the user's request |
| `typhon-instructions-keeper` | CLAUDE.md, AGENTS.md, rules, skills, agents, hooks | on the user's request |
| `typhon-contract-keeper` | events, `uierr` codes ↔ i18n, bindings ↔ TS types, enum labels | on the user's request |
| `typhon-invariant-keeper` | code sweep against I-1…I-17 and test hygiene; lint candidates | on the user's request |

No keeper runs on a schedule or automatically. The two pre-release keepers are part of
`typhon-release-gate`; everything else waits for an explicit request.

## How to run one

1. Give the agent a narrow input: a `git diff` range, a file list, or "full" with a bounded area. Run
   in parallel only agents of different areas.
2. Re-check every finding by reading the code before applying it (an agent can be wrong about its own
   refutation).
3. Apply fixes in small commits per area. Legal texts and `CurrentTelemetryConsent` change only after
   the user's decision (product and legal matter).
4. Instructions and docs shrink, not grow: a fix replaces the stale text instead of adding a paragraph.
5. Report to the user: what changed, what needs their decision, what was not checked.
