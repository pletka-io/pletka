<!--
Thanks for contributing to Pletka.

Before opening, please confirm:
- [ ] Every commit is signed off (`git commit -s`) — required by the DCO check.
- [ ] You have read CONTRIBUTING.md.
- [ ] Tests added or updated where relevant.
- [ ] If you added a widget or link relation, you also added the schema doc and at least one fixture under `schema/examples/`.
-->

## Summary

<!-- One paragraph: what changed and why. -->

## Affected layers

<!-- Tick the layers your change touches, so reviewers know where to focus. -->

- [ ] `schema/` — protocol docs / fixtures
- [ ] `domain/` — pure domain (no HTTP, no DB driver glue)
- [ ] `gitsync/` — git ↔ domain sync
- [ ] `server/` — HTTP, schema builders, link emitters
- [ ] `renderer/` — Svelte renderer
- [ ] `docs/` — architecture, plans, decisions, specs
- [ ] CI / tooling / release

## Testing

<!-- Commands you ran locally; new tests added; manual verification steps. -->

```sh
make test
make lint
```

## Screenshots / output

<!-- For renderer changes, attach before/after screenshots. -->

## Notes for reviewers

<!-- Anything non-obvious: trade-offs, follow-ups, open questions. -->

---

By opening this pull request, I certify that the contribution is made under the project's [Apache 2.0 license](../LICENSE) and that every commit carries a [DCO](https://developercertificate.org/) `Signed-off-by` line.
