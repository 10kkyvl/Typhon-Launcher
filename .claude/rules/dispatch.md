# Who does what: dispatch matrix

The orchestrator applies this table without asking the user. "Alone" means the orchestrator does the
work itself. Agents are listed in `CLAUDE.md`; their briefs follow `typhon-planning`.

| Situation | Who | Why |
|---|---|---|
| Edit under about five tool calls, prose, config, a one-line fix with an obvious test | alone | a brief costs more than the work |
| Any task that needs reading more than three files before the first decision | `typhon-scout` first | Sonnet reads, Opus decides |
| Behavioural fix or feature in one package group | scout → brief → `typhon-test-author` → `typhon-implementer` (worktree) → `typhon-reviewer` → merge | the full pipeline; drop test-author when the test is obvious from the brief |
| Batch of independent items (roadmap, audit findings) | scout per item in parallel, then up to 5 implementers on disjoint packages, one reviewer per branch | parallel where packages do not overlap; shared files at integration by the orchestrator |
| Pure move or split (file, function, component) | implementer with a "pure move" brief, or alone when small | behaviour-free commit before the change that needs it |
| Rebuild or port of a frontend part | `typhon-hunter` on the old code first, then the pipeline above | the brief carries the defect list the rebuild must close |
| Frontend change with visible behaviour | pipeline as above, then the orchestrator runs `typhon-live-smoke` with screenshots (desktop and Big Picture) | agents cannot drive the live app; the orchestrator can |
| Bug report, log excerpt, state files, CI failure, flaky test | `typhon-triage` → then the fix pipeline for the package it names | diagnosis before any edit |
| "Is this code correct?", audit of a package or flow, pre-release sweep | `typhon-hunter` per scope chunk (one package or flow per message) | scope without a diff |
| Review of a branch, PR or commit range | `typhon-reviewer` | one diff against its claim |
| Change touched events, `uierr` codes, exported methods or TS types | `typhon-contract-keeper` on the diff range | Go ↔ TS drift is the most common breakage |
| Change altered a flow, state file, startup recovery or event wiring | `typhon-docs-keeper` on the diff range | the knowledge base must follow the code |
| Change touched `download`, `install`, `updates`, `relocate` or `storage` | `typhon-invariant-keeper` on the diff range | destructive paths and locks |
| Change touched what data leaves the device or which content exists | `typhon-legal-keeper` on the diff range; wording changes need the user's decision | consent versions and legal texts |
| Preparing release notes | `typhon-changelog-keeper` with the previous tag | signrelease parser format |
| Before a tag or a dev → main release merge | `typhon-release-gate` skill: `typhon-deps-keeper` and `typhon-legal-keeper`, then its checklist | the two mandatory pre-release keepers |
| Instructions changed (`CLAUDE.md`, rules, skills, agents) | `typhon-instructions-keeper` on the diff, before the `docs:` commit | dead references and contradictions |
| Windows-only behaviour, real game, CrossOver, elevation | nobody can run it here: name it as unverified in the report | hosted CI runs Windows tests only |

Limits that hold regardless of the row: no more than 5 implementers at once; two agents never edit
the same package; every agent gets a brief with what is already ruled out; every agent's result is
re-run or re-read by the orchestrator before it reaches a commit or the user; a keeper this table
triggers runs on the diff range only, a "full" keeper run is the user's request; a keeper's finding
is re-read in the code before it is applied.
