# Worktree setup

Feature work happens in git worktrees so a branch is isolated from the main
checkout. Each worktree gets its own database clone and its own port, so sibling
worktrees never collide.

## Per-worktree database

A worktree's database name comes from `PLETKA_DB_NAME` in its `.env` (gitignored).
The Makefile reads it:

```make
WEAVE_DB ?= $(if $(PLETKA_DB_NAME),$(PLETKA_DB_NAME),pletka_weave)
```

So the main checkout uses the canonical `pletka_weave`; a worktree uses whatever
its `.env` sets. Every data command honours this.

## Setting up a new worktree

```bash
git worktree add ../pletka-myfeature -b myfeature
cd ../pletka-myfeature
cp ../main/.env .env        # start from a known-good .env
make worktree-init          # clones pletka_weave → per-worktree DB, writes PLETKA_DB_NAME
make air                    # build + run against the clone
```

`make worktree-init` (Makefile, target near the database section) clones the
canonical database to a per-worktree copy and writes `PLETKA_DB_NAME` into `.env`.
On the main worktree it is a no-op — main uses the canonical database directly.
Pass `NAME=foo` to override the derived name.

Related targets:

| Target | Purpose |
|---|---|
| `make worktree-init` | clone the canonical DB and stamp `PLETKA_DB_NAME` into `.env` |
| `make db-clone` | clone `$(SRC_DB)` → `$(WEAVE_DB)` manually |
| `make db-drop` | drop the per-worktree DB on cleanup |
| `make db-status` | stats for the current worktree's DB |

Also set a distinct `PLETKA_PORT` in `.env` if you want to run two worktrees'
servers at once.

## AI scaffolding in a worktree

If you use an AI assistant, its per-directory hook config (`.claude/settings.json`)
is worktree-local, so each new worktree is initialized on its own. Project-wide
memory — learnings and bug history — is best shared across worktrees rather than
duplicated, e.g. by symlinking it back to a canonical worktree so every branch
sees the same accumulated context. The maintainers automate this with their
internal tooling; see [`ai-workflow.md`](ai-workflow.md) for how the project
approaches AI-assisted work. None of this is required to contribute.

## Cleanup

```bash
make db-drop                # drop the clone
cd ../main
git worktree remove ../pletka-myfeature
```
