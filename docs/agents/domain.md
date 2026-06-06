# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Layout

This is a single-context repo.

Read these when present:

* `CONTEXT.md` at the repo root
* `docs/adr/`
* Existing supporting docs under `docs/`, especially `docs/BEAD_MODEL.md`, `docs/spec.md`, and `docs/UI_PRINCIPLES.md`

If `CONTEXT.md` or `docs/adr/` do not exist, proceed silently. Do not suggest creating them upfront; producer skills create them lazily when terms or decisions actually get resolved.

## Use the Project Vocabulary

When output names a domain concept in an issue title, refactor proposal, hypothesis, or test name, use the term as defined in `CONTEXT.md` when present. If the concept is not in the glossary yet, note the gap only when it matters to the current task.

## Flag ADR Conflicts

If output contradicts an existing ADR, surface it explicitly rather than silently overriding it.
