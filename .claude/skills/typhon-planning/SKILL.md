---
name: typhon-planning
description: Use when breaking a Typhon Launcher task into a plan or writing a brief for an implementer — a feature, a behavioural fix spanning packages, a refactor phase, or any work where "done" must be defined before coding starts.
---

# Planning a Typhon task

A plan fixes behaviour, boundaries and evidence, not a file list. The implementer chooses the
implementation; the plan sets the acceptance criteria.

## Order

1. **User scenario**: from click to result, including the error and the retry. UI work covers both
   modes (desktop and Big Picture). Background work states what the user sees on failure
   (`*:degraded`, status, toast).
2. **Reading map**: the `docs/INDEX.md` row for the task class → owning files and tests. Read those,
   not all of `docs/`. Verify document facts against code (the documents are a dated snapshot).
3. **Impact map**: `grep` usages of the symbols, events, codes and JSON fields being changed. Write
   "touched → how we will know it still works".
4. **Invariants**: from the table in `.claude/rules/go-internal.md`, name the I-numbers and, for each,
   the test or check that proves it.
5. **Stages**: each stage is an independent commit with a working intermediate state. Pure moves are
   separate commits made before the change that needs them.
6. **Result check**: table scenario → evidence (a test that fails without the change; a live run; a
   screenshot). Unverifiable items are named up front.

## Brief for `typhon-implementer`

One stage per brief, one package group per implementer. Sections, in order, each a few lines:
scenario (state → action → result, including the failure path); contract (signatures, `uierr`
codes, events, state files that may change, and those that must not); package(s) and the files the
scout pointed at; invariants (I-numbers) with the proof each needs; acceptance criteria as
observable checks; the test text from `typhon-test-author` when it exists; what is already ruled out
and why; shared files the implementer must not touch. Do not include the orchestrator's guesses at
the implementation.

## Mandatory tail of every plan

> After implementing, return to this plan. For each item: done / not done / how it was verified. For
> every scenario in "Result check", name the existing test that covers it or add one and show that it
> fails without the change. An unverified item does not count as done.

## Do not

- Dictate exact function names and files where the implementer can choose.
- Write "test more thoroughly": name concrete scenarios and boundaries instead.
- Inflate a five-line fix with documents and checklists: scale to the change.
