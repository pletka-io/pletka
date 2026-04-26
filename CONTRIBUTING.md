# Contributing to Pletka

Thanks for your interest. Pletka is an open-source project; outside contributions land here first, then flow downstream into private deployments.

## Quick start

```sh
git clone git@github.com:pletka-io/pletka.git
cd pletka
make build       # builds renderer + Go binary
make test        # runs Go + renderer tests
make lint        # golangci-lint + svelte-check
```

Requires Go 1.26+, Node 22+, npm 10+.

## How to contribute

1. **Open an issue first** for non-trivial changes, so we can agree on direction before you spend time implementing. Bug fixes and small improvements can go straight to a PR.
2. **Fork**, create a branch off `main`, do the work.
3. **Follow existing conventions.** See [`CLAUDE.md`](CLAUDE.md) and [`docs/`](docs/) for architectural invariants. The most important ones:
   - The schema is the contract. Domain knowledge lives in Go schema builders, not in the renderer.
   - Authorization is hypermedia-driven — links a user is permitted to act on are emitted by the server; the renderer never decides.
   - The renderer is a pure function of the schema. No business logic, no permission checks, no data fetching outside link relations.
4. **Tests required.** New widgets, new link relations, new domain logic must come with tests. Schema additions must come with example fixtures under `schema/examples/` that both server and renderer test against.
5. **Open a PR.** Use the template. Describe what changed and how you tested it.

## DCO sign-off (required)

Every commit must be signed off under the [Developer Certificate of Origin](https://developercertificate.org/). This is a lightweight statement that you have the right to submit the work under the project's license. You sign off by adding a `Signed-off-by` line to your commit message:

```
Signed-off-by: Random J Developer <random@developer.example.org>
```

`git commit -s` adds this automatically. Configure git once with the email matching your GitHub account:

```sh
git config user.name "Random J Developer"
git config user.email "random@developer.example.org"
```

Pull requests without a DCO sign-off on every commit will be blocked by CI.

## Coding standards

**Go** — see [`.golangci.yml`](.golangci.yml) for the enforced linter set. Format with `gofmt` / `goimports`. Tests use `go test` with `github.com/google/go-cmp` for structural comparisons. See [`CLAUDE.md`](CLAUDE.md) for naming, error handling, logging conventions.

**Svelte / TypeScript** — Svelte 5 runes. `npm run check` (`svelte-check`) must pass. Tailwind 4 with the in-repo design tokens. The renderer is a pure consumer of the JSON schema — if a component contains a hardcoded entity name, an `if (user.role === ...)` check, or constructs an API URL, it does not belong.

## Reporting security vulnerabilities

See [`SECURITY.md`](SECURITY.md). Do not open public issues for vulnerabilities.

## Code of Conduct

This project follows the [Contributor Covenant 2.1](CODE_OF_CONDUCT.md). By participating you agree to abide by it.
