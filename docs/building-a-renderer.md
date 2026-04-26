# Building a Renderer

The Svelte renderer in `renderer/` is the reference implementation. Any
language can produce a conformant alternative — mobile, terminal, voice,
third-party integration. This document is for the implementer.

## What you have to honour

A conformant renderer:

1. **Reads only the JSON schema.** No out-of-band knowledge of users,
   roles, or domain types.
2. **Dispatches on `widget.type`.** Unknown widget types fall through to
   a generic display (e.g. JSON pretty-print). They MUST NOT crash the page.
3. **Treats links as the permission grant.** A link in the response means
   "the user may follow this." Absence means "this is not available to
   the user, in this view, right now." Never decide elsewhere.
4. **Calls a `rel`-keyed handler dispatcher.** One place that maps `rel`
   to action. No widget-private URL construction.
5. **Forwards-compatible.** Unknown fields on a widget are ignored;
   unknown link relations are simply not actioned. Schema minor bumps do
   not break a conformant renderer.

## What you do not have to do

- Run JavaScript. Reproduce the schema → DOM mapping in any technology.
- Use SvelteKit, Vite, Tailwind, or anything else from the reference
  stack. They are implementation choices for `renderer/`, not protocol.
- Implement widgets you do not need. Falling through to a generic
  display is acceptable for any widget your platform does not handle yet.

## Where to look

| You want to know...                          | Look at...                              |
|----------------------------------------------|-----------------------------------------|
| The page envelope shape                       | [`protocol.md`](protocol.md)            |
| Each widget type                              | [`schema/widgets/`](../schema/widgets/) |
| Each link relation                            | [`schema/relations/`](../schema/relations/) |
| Concrete examples that double as test fixtures| [`schema/examples/`](../schema/examples/) |
| The authorization contract                    | [`authorization-model.md`](authorization-model.md) |

## Conformance test

Run your renderer against every fixture under
[`schema/examples/`](../schema/examples/). For each fixture:

- The page must render without error.
- For multi-variant fixtures (e.g. `*-anonymous.json`, `*-editor.json`,
  `*-admin.json`), the rendered output must reveal exactly the actions
  whose links are present in that fixture and hide the rest.

If both hold, the renderer is conformant.

## Pinning a schema version

A renderer pins a baseline schema version (e.g. `0.1.0`). It must accept
any minor or patch version equal to or greater than its baseline,
ignoring fields and widget types it does not understand. Major bumps
require a renderer update.

## Asking for help

Open an issue with the `[renderer]` prefix describing which widget or
relation is unclear. The reference renderer is a guide, not a constraint;
suggestions for clarifying the spec are welcome.
