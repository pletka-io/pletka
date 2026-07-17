# AI-assisted development

Pletka is built with AI pair-programming as a first-class workflow. The repo ships
scaffolding that keeps an AI assistant grounded in *this* codebase's decisions
instead of generic patterns — and keeps its work reviewable. None of it is
required to contribute as a human, but understanding it explains a lot of the
files you'll see.

The pieces, in order of how often they matter:

## 1. Design contracts — `.claude/rules/`

Short, enforceable rule files, one per concern: `api-patterns.md`,
`database-patterns.md`, `service-layer.md`, `domain-model.md`,
`naming-conventions.md`, `go-style.md`, `ui-patterns.md`. Each states the core
principle, the red flags that signal a violation, and points to the fuller
reference.

These are the same rules documented in this `docs-oss/` set — the `.claude/rules/`
versions are the terse, always-loaded contract an assistant reads before
generating code. Treat them and the docs here as two views of one truth.

## 2. Decision records — `docs-oss/decisions/`

ADRs are the memory across sessions. Before proposing an architectural change, an
assistant reads `docs-oss/decisions/` so it doesn't re-suggest something already
rejected. The same discipline serves human contributors: the
[decisions index](../decisions/README.md) tells you *why* the system is shaped the
way it is. ADRs are never edited — only superseded.

## 3. Project memory and session hooks (maintainers' tooling)

The maintainers run an optional Claude Code middleware that gives an assistant a
persistent, project-scoped memory and deterministic pre/post-session hooks. It
keeps two kinds of state:

- A *shared, tracked* operating protocol and hook engine — the discipline that
  produces the memory, versioned with the repo.
- *Per-developer, gitignored* state — learnings and user-preference notes, a
  chronological session log, structured bug history, and per-session token
  accounting. This is personal and machine-local, never committed.

This middleware is internal to the maintainers' workflow and is **not required to
contribute** — external contributors won't have it, and nothing in the public
tree depends on it. The idea it embodies is portable, though: give your assistant
a durable place to record decisions and past bugs so it stops re-suggesting things
this project already rejected. Any equivalent memory mechanism works.

## 4. CodeGraph — code intelligence index

`.codegraph/` holds a SQLite knowledge graph of every symbol and edge in the
workspace, used for sub-millisecond symbol lookups instead of grepping. The whole
directory — config, generated index, and daemon socket — is gitignored and local
to each checkout. Regenerate per checkout.

## 5. Skills

The maintainers also keep task-specific Claude Code skills alongside their
tooling — for example a CIDOC-CRM / RDFS / OWL / SHACL ontology expert that backs
the ontology work described in
[`../../.claude/rules/ontology.md`](../../.claude/rules/ontology.md). These skills
are part of the maintainers' internal setup rather than the public tree; the rule
files in `.claude/rules/` capture the guidance that matters for contribution.

## What's tracked vs personal

| Tracked (shared scaffolding) | Personal / machine-local (gitignored) |
|---|---|
| `.claude/rules/*` | `.claude/settings.local.json`, `.claude/settings.json` |
| project-memory protocol + hooks (maintainers' tooling) | project-memory session log, learnings, bug history, token accounting |
| — | all of `.codegraph/` (config + index + daemon state) |

If you don't use an AI assistant, none of this affects you — build and test
through the Makefile as normal. If you do, start a session by letting it read the
rules and decisions before it writes code.
