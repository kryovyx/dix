# dix — a dependency-injection container

`github.com/kryovyx/dix`. Part of the REX framework: developed in a `go.work` workspace
alongside its siblings, released as its own module.

## Boundaries

Stdlib only, and it depends on **no sibling module** — nothing in this repo may
import `rextension` or `rex`. It provides `Container`, `Resolver` and `Scope`;
a scope has a lifetime and must be closed.

Concurrency-safety and leak-freedom are contractual here (D5–D8), not
incidental: a change that drops a lock, or that leaves a scope unclosed on an
error path, is a regression rather than a refactor. Resolution order must stay
deterministic — two runs with the same registrations resolve the same way.

## Working here

- **Never `go build`.** Syntax-check with `go vet ./...`.
- **`go test -race ./...` always.**
- **Tests are per branch, not per coverage number.** Every branch of every
  function gets its own case; the README's coverage figure is recomputed from
  a measurement, never hand-edited.
- **No `replace` directives** in `go.mod`.
- **Commits:** [COMMIT-CONVENTIONS.md](COMMIT-CONVENTIONS.md) — gitmoji, a
  space, an imperative summary; no `type(scope)` prefix. At most one trailer,
  and no generated footers.
- `make check` here runs fmt, vet and race tests for this module alone.
- Default branch is `main`. **Never push without asking** — github
  authenticates with a hardware key that needs a physical tap, so an
  unattended push hangs and then fails.

Design decisions are numbered (D…/O…/W…) and recorded in the workspace this
module is developed in, not in this repo. If a rule here looks arbitrary, it is
load-bearing — ask before removing it.
