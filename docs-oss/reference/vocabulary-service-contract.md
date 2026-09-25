# Vocabulary Service Contract

The subset of the vocabulary-service HTTP contract that Pletka core actually
depends on. `pkg/weave/vocabconnector/vocabservice` is the connector that
speaks it. `https://vocab.pletka.io` is the reference deployment; kakugo is
its reference implementation.

**Load this document when:** implementing a new vocabulary service, changing
the vocabservice connector, or auditing what a `connector_type: "vocabservice"`
row can rely on.

This is not a copy of the service's own documentation. The service's contract
is wider than this — it also documents `children` and other fields core does
not read. Only what core sends and reads is written down here; if the service
adds or changes something outside this list, core does not notice either way.

---

## One service per instance

An instance talks to exactly one vocabulary service, configured once in the
instance config as `vocabulary_service.base_url`:

```yaml
vocabulary_service:
  base_url: "https://vocab.pletka.io"
```

Unset means no service is configured, and every row asking for this connector
falls back to `local` — its stored entries only, no remote suggestions and no
degraded flag, because no call is made.

A `weave_vocabularies` row using this connector (`connector_type =
"vocabservice"`) names only *which vocabulary* to use on that service, via
its `config` JSON:

```json
{"vocab": "aat", "lang": "en"}
```

`vocab` is the service's vocabulary name (`aat`, `tgn`, `ulan`, or any future
mount). `lang` is an optional per-row default language; the connector's own
`base_url` field, if a row happens to set one, is overwritten with the
instance's configured service and never read from the row — one service per
instance is the point.

## Endpoints core calls

### `GET /vocab`

Lists what the service serves. Called only when configuring a row (deciding
which `vocab` name to use), never from autocomplete.

Response — what `https://vocab.pletka.io/vocab` actually returns, per
vocabulary (`languages` abridged here; the real array runs to dozens of tags):

```json
{
  "vocabs": [
    {
      "name": "aat",
      "profile": "gvp",
      "dump": "aat",
      "scheme": "http://vocab.getty.edu/aat/",
      "concepts": 58996,
      "kinds": {"concept": 57085, "guideTerm": 1785, "hierarchy": 118, "facet": 8},
      "obsolete": 1332,
      "languages": ["en", "nl", "de", "fr"],
      "languages_skipped": 139,
      "roots": 8,
      "built": "2026-09-25T15:40:33Z",
      "endpoints": {
        "suggest": "/vocab/aat/suggest",
        "concept": "/vocab/aat/concept/{id}",
        "children": "/vocab/aat/children/{id}"
      }
    }
  ]
}
```

Core reads only `name` (used as `vocab` in a row's config), `label`, and
`concepts`, and ignores every other field. Both `label` and `concepts` are
optional on the core side (`omitempty`) — an implementer may omit either, and
the reference deployment in fact sends no `label` at all, so a configuration
UI has only `name` to show.

### `GET /vocab/{name}/suggest`

Autocomplete. Core sends:

| Parameter | Always sent? | Notes |
|---|---|---|
| `q` | yes | the search term, trimmed; a blank query is never sent — core returns an empty result locally instead |
| `lang` | yes | resolved as: the caller's requested language, then the row's configured `lang`, then `"en"` |
| `limit` | yes | the caller's requested limit, clamped to `1..100` server-side by core (default `50`) before sending |
| `under` | only when narrowing to a parent | the parent concept's identifier (last path segment of its URI), when the caller scopes the search under a broader concept |

Response:

```json
{
  "results": [
    {
      "uri": "http://vocab.getty.edu/aat/300010957",
      "id": "300010957",
      "prefLabel": "brons",
      "lang": "nl",
      "parentString": "koperlegering,non-ferrometaal,metaal,...",
      "broader": "http://vocab.getty.edu/aat/300011068"
    }
  ]
}
```

Core reads, per hit:

- `uri` — required in practice; core normalizes it (see below) and treats it
  as the entry's identity.
- `id` — the service's own identifier for the concept.
- `prefLabel` — **a single string, already resolved to one language, on this
  endpoint.** May be absent or empty; core stores no label rather than an
  empty one in that case.
- `lang` — the language `prefLabel` actually came back in; may differ from
  the requested `lang`. Core falls back to the requested `lang` when this is
  absent.
- `parentString` — **a single comma-joined chain, nearest ancestor first, on
  this endpoint.** May be absent (a facet root, or a hit with no chain at
  all) or empty; either way core stores no ancestor path.
- `broader` — the immediate broader concept's URI, or `null`/absent for none.

A search that fails — timeout, non-2xx, transport failure, or an undecodable
body — **degrades to an empty result, never an error.** Autocomplete-while-
typing has no error state; a struggling service must look like "no matches
yet," not break the picker. (Internally the connector still distinguishes
this from a genuine no-match — see below — but nothing in this endpoint's
public behavior depends on that distinction.)

### `GET /vocab/{name}/concept/{id}`

Resolves one concept by identifier (the last path segment of its URI). Core
calls this as an explicit lookup — when a picker resolves a stored value, not
while the curator is typing — so unlike `suggest`, **a failure here is a real
error**, not a degrade-to-empty.

Core sends no query parameters to this endpoint. (An earlier version of the
connector sent `lang`, matching `suggest`; this endpoint's parameter table
does not document `lang`, so core stopped sending it — an undocumented
parameter is one a future service version may reject.)

Response, shaped differently from `suggest`:

```json
{
  "uri": "http://vocab.getty.edu/aat/300010957",
  "id": "300010957",
  "prefLabel": {"en": "bronze", "nl": "brons"},
  "parentString": {"en": "copper alloy, metal", "nl": "koperlegering, metaal"},
  "broader": "http://vocab.getty.edu/aat/300011068",
  "children": ["http://vocab.getty.edu/aat/300250435"]
}
```

Core reads:

- `uri`, `id`, `broader` — same as `suggest`.
- `prefLabel` — **a language-keyed object on this endpoint**, covering every
  language the concept has. May be absent.
- `parentString` — **also a language-keyed object here**, one chain per
  language. May be absent.
- `children` — **exists in the wider service contract. Core does not consume
  it yet.** The connector does not decode this field at all.

`prefLabel` and `parentString` tolerate both shapes — a single string
(`suggest`'s shape) or a language-keyed object (`concept`'s shape) — on
*either* endpoint, because the connector's decoder sniffs the value's first
byte rather than assuming which endpoint it came from. In practice the
service sends the plain-string shape on `suggest` and the object shape on
`concept`, and an implementer only has to serve that pairing correctly — but
an implementer serving only one shape everywhere still decodes on the core
side. A value that is neither a string nor an object (for example, a number)
is a decode error. On `concept` (`Fetch`) that error reaches the caller. On
`suggest` (`Search`) it does not: `Search` wraps everything the HTTP call
returns — transport, status, and decode alike — as a degrade, so the caller
sees an empty result flagged degraded, never an error.

Language resolution on `concept`, when `prefLabel`/`parentString` are
language-keyed objects: the requested language, then English, then whichever
language the object actually has (chosen deterministically — the
lexicographically smallest key — not by map iteration order). A key present
with an empty string counts as absent, not as a value, for this resolution.
The ancestor chain resolves independently of the label — a concept can have
its label resolved to one language and its chain to another (e.g. the label
falls back to English while the chain is only present in Dutch) — so a
picker sees the ancestor path in whichever language it actually exists in,
rather than an empty path just because the label landed elsewhere. The label
itself is narrowed to the resolved language plus English (when different)
before being stored — not the whole language map the service returned.

`scopeNote` is part of the wider service contract (served on the concept
response). **Core reads no `scopeNote` today.**

Language-key matching against the returned object is case-insensitive: the
service matches `lang` case-insensitively and echoes back its own spelling,
so core resolves keys the same way rather than requiring an exact match.

## URI normalization

Any concept URI on host `vocab.getty.edu` — regardless of vocabulary name in
its path — is normalized to `https` and has a leading `/page` path segment
stripped. This is host-based, not vocabulary-name-based, so a fourth Getty
vocabulary mounted on the service tomorrow normalizes correctly with no
connector change. A URI on any other host is passed through unchanged.

## Failure behavior, summarized

| Failure | `suggest` (`Search`) | `concept/{id}` (`Fetch`) |
|---|---|---|
| Timeout | empty result, no error | error |
| Non-2xx status | empty result, no error | error |
| Transport failure (connection refused, etc.) | empty result, no error | error |
| Body that will not decode as JSON | empty result, no error | error |
| Decoded body has a field of neither tolerated shape (e.g. `prefLabel` as a number) | empty result, no error | error |
| Row has no `vocab` configured | empty result, no error, reported as degraded | error naming the problem |

`Search` has exactly one failure mode: it wraps everything the HTTP call
returns — transport, status, and decode alike — as a degrade. Only `Fetch`
surfaces any of it as an error. A degraded search is reported to the caller
(the search response carries `"degraded": true`, and the server logs a
throttled warning naming the vocabulary and the underlying cause), but it is
never an error the picker has to handle.

There is one failure the service can produce that core cannot see at all:
core keys on the response's `results` array, so a 200 whose body is
well-formed JSON *without* it — `{}`, or a differently-named envelope —
decodes cleanly into zero results and is indistinguishable from a genuine
no-match. That vocabulary is permanently empty, with no degrade and nothing
in the log.

Every timeout is bounded — a single call to the service (either endpoint)
is canceled at 3 seconds by default, so a slow or hung service cannot block
the picker indefinitely.

The per-row override is the `config` JSON's `timeout`, and it is a Go
`time.Duration` decoded from a JSON number, so **its unit is nanoseconds**,
not seconds. `{"timeout": 5}` is a 5-nanosecond deadline: every call is
canceled before it leaves, and that vocabulary degrades permanently. Five
seconds is written out in full:

```json
{"vocab": "aat", "lang": "en", "timeout": 5000000000}
```

A value of `0` or less (or an absent `timeout`) takes the 3-second default.

## Out of scope for this contract

`children`, `Entry.Matched`, response caching, and project-level enablement
of the service are all things the service may support that core does not
consume as of this document. They are not wrong to serve; core simply has
no code path reading them yet.
