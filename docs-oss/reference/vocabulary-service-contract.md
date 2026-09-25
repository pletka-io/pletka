# Vocabulary Service Contract

The subset of the vocabulary-service HTTP contract that Pletka core actually
depends on. `pkg/weave/vocabconnector/vocabservice` is the connector that
speaks it. `https://vocab.pletka.io` is the reference deployment; kakugo is
its reference implementation.

**Load this document when:** implementing a new vocabulary service, changing
the vocabservice connector, or auditing what a `connector_type: "vocabservice"`
row can rely on.

This is not a copy of the service's own documentation. The service's contract
is wider than this — it also documents `children`, `narrower`, `matches`,
`broaderOther`, `altLabel` and other fields core does not read. Only what
core sends and reads is written down here; if the service adds or changes
something outside this list, core does not notice either way. This document
is derived from the connector's own code (`connector.go`, `item.go`,
`errors.go`), not from the service's documentation — the two should agree,
and where they were checked against each other and the live service while
writing this, that is noted below.

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
which `vocab` name to use), never from autocomplete. The connector also uses
this call's top-level `version` to warn — never to fail — when the service
answers a discovery contract this connector was not written against (see
**Discovery version** below).

Response — what `https://vocab.pletka.io/vocab` actually returns, per
vocabulary (abridged: the real array carries many more fields per mount,
listed in full further down for `aat`):

```json
{
  "version": 2,
  "vocabs": [
    {
      "name": "aat",
      "label": "Art & Architecture Thesaurus",
      "concepts": 58996,
      "languages": ["ar", "en", "nl", "..."]
    }
  ]
}
```

Core reads:

- `version` (top level) — compared against the connector's own
  `expectedListingVersion` (`2`); a mismatch logs a warning naming both, and
  the listing is still returned. This is the owner's deliberate call: `GET
  /vocab` is read at configuration time, not on a curator's keystroke, so a
  wrong or upgraded service is something an operator can be warned about
  rather than something that should ever break a configuration screen.
- `name`, `label`, `concepts`, `languages` per vocabulary — mapped onto
  `VocabularyInfo`. `label` and `concepts` are optional on the core side
  (`omitempty`); an implementer may omit either. `languages` is which
  languages have their own suggest index on that mount (most mounts serve
  exactly one) — a configuration screen needs it to tell an operator that a
  Dutch project pointed at an English-only mount will only ever answer in
  English.

Every other field the service may send on this endpoint (`profile`, `dump`,
`scheme`, `kinds`, `obsolete`, `languages_skipped`, `roots`, `built`,
`source`, `license`, `endpoints`, `extras`, and a per-mount `version`) is
decoded by nothing here — an implementer sending them is not wrong, core
simply never reads them.

### `GET /vocab/{name}/suggest`

Autocomplete. Core sends:

| Parameter | Always sent? | Notes |
|---|---|---|
| `q` | yes | the search term, trimmed; a blank query is never sent — core returns an empty result locally instead |
| `lang` | yes | resolved as: the caller's requested language, then the row's configured `lang`, then `"en"` |
| `limit` | yes | the caller's requested limit, clamped to `1..100` server-side by core (default `50`) before sending |
| `under` | only when narrowing to a parent | the parent concept's identifier (last path segment of its URI), when the caller scopes the search under a broader concept |

Response — one **item** per hit (see **The one item shape** below), plus the
fields that explain why it matched and where it sits in the hierarchy:

```json
{
  "results": [
    {
      "uri": "http://vocab.getty.edu/aat/300250435",
      "id": "300250435",
      "kind": "concept",
      "class": "Concept",
      "prefLabel": {"en": "ethnicity"},
      "lang": "en",
      "matched": "prefLabel",
      "matchedLabel": "ethnicity",
      "broader": "http://vocab.getty.edu/aat/300162135",
      "parents": [
        {"uri": "http://vocab.getty.edu/aat/300162135", "id": "300162135",
         "kind": "concept", "class": "Concept",
         "prefLabel": {"en": "culture-related concepts"}, "lang": "en"}
      ],
      "score": 9.8095
    }
  ]
}
```

Core reads, per hit:

- `uri` — the entry's identity, sent by the service exactly as the publisher
  minted it. **v2 promises this is the canonical IRI, never rewritten** —
  there is no URI-normalization step on the core side any more (v1 rewrote
  `vocab.getty.edu` hosts; that code is gone in v2).
- `id` — the service's own identifier for the concept.
- `kind`, `class` — decoded, currently unused for anything core reads back
  out (kept on the wire shape because they are part of the one item shape
  every reference decodes into).
- `prefLabel` — **always a language-keyed object**, never a bare string, on
  every endpoint. On `suggest` it carries at most two keys: the language
  named by `lang`, plus `en` when the concept has one too and `lang` is not
  already `"en"`. May be `{}` (with `lang` `null`) for a subject with no
  `skos:prefLabel` at all — core stores no label in that case, not an empty
  one.
- `lang` — which `prefLabel` key is the display language; `null` only
  alongside an empty `prefLabel`. Core indexes the map with this key
  directly. It does **not** scan case-insensitively for `"en"`, and it does
  **not** fall back to a near-miss such as `"en-GB"` — a concept whose only
  English-ish key is `"en-GB"` and whose `lang` is something else gets one
  stored language, deterministically, not two.
- `broader` — the immediate broader concept's URI, or absent/`null` for none.
- `parents` — the ancestor chain, nearest first, each entry a full **item**
  in its own right (own URI, own id, own `lang`). This is what replaced v1's
  `parentString`: v1 stamped the entire chain with the requested language,
  which was sometimes a fabrication — see **Ancestor languages** below.
- `matched`, `matchedLabel`, `score` — **decoded and not consumed.** They
  explain a suggest hit (which label matched, and the engine's relevance
  number) but nothing on the core side reads them back out today.

A search that fails — timeout, non-2xx status, transport failure, or a body
that will not decode — **degrades to an empty result, never an error**.
`Search` wraps every failure mode from the HTTP call the same way: transport
error, non-2xx status, and an undecodable body all become `ErrDegraded`
alike. There is no special case for a body that comes back malformed — it is
just one more way the call can fail, and it degrades exactly like every
other one. See **Failure behavior** below for the full table.

### `GET /vocab/{name}/concept/{id}`

Resolves one concept by identifier (the last path segment of its URI). Core
calls this as an explicit lookup — when a picker resolves a stored value, not
while the curator is typing — so unlike `suggest`, **a failure here is a real
error**, not a degrade-to-empty.

Core sends one query parameter:

| Parameter | Always sent? | Notes |
|---|---|---|
| `lang` | yes | picks the display language for the concept **and every nested item** (each ancestor resolves its own `lang` independently — see below) |

This is a correction from v1's connector, which sent no `lang` to this
endpoint at all. Under v2 the parameter is documented and load-bearing —
omitting it would leave the display language to the service's own default —
so the connector sends it on every `Fetch`, unconditionally.

Response — the same **item** shape as a suggest hit, with `prefLabel`
carrying every language the concept has (not just up to two) plus a couple
of fields only a concept fetch carries:

```json
{
  "uri": "http://vocab.getty.edu/aat/300010957",
  "id": "300010957",
  "kind": "concept",
  "class": "Concept",
  "prefLabel": {"en": "bronze (metal)", "nl": "brons", "de": "Bronze", "...": "..."},
  "lang": "en",
  "altLabel": {"en": ["copper-tin alloy"]},
  "scopeNote": {"en": "Refers to a broad range of alloys of copper...", "nl": "..."},
  "broader": "http://vocab.getty.edu/aat/300010942",
  "parents": [
    {"uri": "http://vocab.getty.edu/aat/300010942", "id": "300010942",
     "kind": "concept", "class": "Concept",
     "prefLabel": {"en": "copper alloy"}, "lang": "en"},
    {"uri": "http://vocab.getty.edu/aat/300241441", "id": "300241441",
     "kind": "guideTerm", "class": "GuideTerm",
     "prefLabel": {"en": "<copper and copper alloy>"}, "lang": "en"},
    {"uri": "http://vocab.getty.edu/aat/300011014", "id": "300011014",
     "kind": "concept", "class": "Concept",
     "prefLabel": {"es": "metal no ferroso"}, "lang": "es"}
  ],
  "narrower": [ "...decoded by neither the service's other clients' contract nor this one..." ]
}
```

Core reads:

- `uri`, `id`, `broader`, `parents` — same meaning as `suggest`.
- `prefLabel` — a language-keyed object covering every language the concept
  has. Core narrows this down to the same at-most-two-keys shape `suggest`
  already sends (the resolved `lang`, plus `en` when different and present)
  **before storing it** — see **Why the narrowing** below.
- `scopeNote` — a language-keyed object, `{lang: sentence}`. Narrowed by the
  exact same rule as `prefLabel` (display language plus English), for the
  same reason. **This is new in this branch: v1 read no `scopeNote` at
  all.**
- `altLabel` — **decoded off the wire, and not mapped anywhere.** `Entry`
  has no field for it; inventing one is out of scope for this work.
- `narrower`, `matches`, `broaderOther` — **not represented in the decode
  type at all.** The connector's Go struct simply has no field for them, so
  they are silently ignored the way any JSON decoder ignores an unknown key.
- `children` — same as `narrower`: not decoded, not consumed. (`children` is
  in fact its own endpoint in the wider contract, `GET
  /vocab/{name}/children/{id}`; core calls neither that endpoint nor reads a
  `children` key were one present on `concept`.)

A `prefLabel` that decodes to neither a language-keyed object (the only
shape v2 ever sends) is a real decode error on `Fetch` — for example a JSON
number where an object was expected.

#### Ancestor languages

**Each ancestor in `parents` resolves its own `lang` independently of the
concept being fetched, and independently of every other ancestor.** A
Dutch-language `Fetch` can come back with a chain whose third link only has
a Spanish `prefLabel`, because that particular AAT subject was never given a
Dutch (or English) preferred label. Core keeps the ancestor's own key rather
than substituting the requested language or dropping the entry — see the
example above (AAT 300010957, lang `en`): ancestor `300011014` is stored as
`{"es": "metal no ferroso"}`, never stamped `en`. This is the whole reason
`parents` (a list of full items) replaced v1's `parentString` (one
comma-joined string nominally "in" the requested language, which was
sometimes simply untrue). Verified live against `https://vocab.pletka.io` —
see the hand-off for the exact command and output.

An ancestor with no `skos:prefLabel` at all (`prefLabel: {}`, `lang: null`)
still contributes an entry to the breadcrumb, with its URI and id but no
label — it is not dropped, because dropping it would silently shorten the
chain.

#### Why the narrowing

`concept/{id}`'s `prefLabel` is the one place in the contract where a
language-keyed object can carry more than two keys — every language the
concept has, which for a well-translated AAT subject easily runs past ten.
Measured against the live service (2026-09-25): AAT 300010957 ("bronze
(metal)") returns 14 `prefLabel` keys, TGN 1000063 ("Belgium") returns 9,
and TGN 7006952 ("Amsterdam") returns 2. These numbers vary by concept and
by vocabulary — they are not a property of any one mount, and in particular
**they are not the mount-level `languages` count from `GET /vocab`**, which
counts something else entirely (which languages have their own *suggest
index* on that mount, not any single concept's label count).

Storing all of a concept's languages in a `weave_vocabularies` entry row
would mean that one field, on one endpoint, carries an order of magnitude
more data than the same field carries everywhere else `Entry.Label` is
populated (a suggest hit, an ancestor item — both already narrowed to at
most two keys by the service itself). That breaks row-shape consistency for
no benefit core currently has a use for, so `Fetch` narrows
`concept/{id}`'s `prefLabel` (and, by the same reasoning, its `scopeNote`)
down to the resolved display language plus English, using the identical
rule `suggest` already applies — one rule, not two invented separately for
the same shape of problem.

## The one item shape

Every concept reference the connector decodes — a suggest hit, an entry of
`parents`, and the concept fetch itself — shares one underlying shape in the
Go code (`item` in `item.go`):

| Field | Meaning |
|---|---|
| `uri` | the publisher's canonical IRI, sent and stored verbatim — never rewritten |
| `id` | the last path segment; what `concept/{id}` and `under=` take |
| `kind`, `class` | decoded, not read back out anywhere core currently acts on |
| `prefLabel` | a language-keyed object (never a bare string) |
| `lang` | which `prefLabel` key to display; `null` only when `prefLabel` is empty |

`Entry.Label` (and an ancestor's `Label`) is `nil` when `prefLabel` is empty
or `lang` is `null` — a subject with no `skos:prefLabel` at all. Core does
not iterate `prefLabel` looking for something to show, does not match
language tags case-insensitively, and does not fall back to a near-miss tag:
the service's own `lang` is the one authoritative answer for "which key is
the display language."

## Discovery version

`GET /vocab`'s top-level `version` is the discovery contract version this
connector was written against (`2`, `expectedListingVersion` in the code). A
service reporting a different version still has its listing decoded and
returned — the owner's explicit choice is **warn, not fail**: this call
happens once, while an operator is configuring a vocabulary row, not on
every keystroke, so it is a safe place to surface "this service speaks a
contract version I wasn't built for" without breaking configuration for a
service that is otherwise compatible enough to keep working.

## URI normalization

**There is none, in v2.** v1 normalized any `vocab.getty.edu` URI's scheme
and stripped a leading `/page/` segment before storing it. v2's contract
promises the `uri` field is already the publisher's canonical IRI on every
response, so the connector stores whatever the service sends, unchanged.

## Failure behavior, summarized

| Failure | `suggest` (`Search`) | `concept/{id}` (`Fetch`) |
|---|---|---|
| Timeout | empty result, `ErrDegraded` | error |
| Non-2xx status | empty result, `ErrDegraded` | error |
| Transport failure (connection refused, etc.) | empty result, `ErrDegraded` | error |
| Body that will not decode as JSON | empty result, `ErrDegraded` — **no error surfaces to the caller** | error |
| Row has no `vocab` configured | empty result, `ErrDegraded` (wrapping `errNoVocab`) | error naming the missing vocabulary, not `ErrDegraded` |

`Search` has exactly one failure mode from the caller's point of view: it
wraps everything the HTTP round trip can produce — transport error, non-2xx
status, and an undecodable body alike — as `ErrDegraded`, with zero entries
and no other signal. **An undecodable `suggest` body is not an error the
picker ever sees** — it is indistinguishable, to the caller, from a timeout
or a genuine no-match; only `Fetch` ever returns a decode failure as an
error. (A previous version of this document said suggest's decode failure
surfaced as "error" — that was wrong. `Search`'s wrapping is unconditional.)

Only `Fetch` surfaces any of this as an error a caller has to handle —
because `Fetch` is an explicit lookup (resolving a stored value), not a
keystroke, so there is no "just show nothing" fallback that makes sense.

## Error tokens

A non-2xx response body decodes into `ServiceError`:
`{"error":"<token>","message":"<sentence>"}`, plus whatever extra keys that
token carries (today: `vocab`, sent only by `unknown_vocabulary`). The
connector switches on the token; `message` is prose for a log and may be
reworded between service releases without notice. A non-2xx body that is
not this shape at all — no JSON, or JSON with no `error` key — still
produces an error naming the HTTP status; it is never treated as success.

| Token | Status (typical) | Core's classification | Why |
|---|---|---|---|
| `unknown_vocabulary` | 404 | **permanent** (`ErrMisconfigured`) | the row names a vocabulary this service does not mount at all; retrying changes nothing until the row's config changes |
| `bad_lang` | 400 | **permanent** (`ErrMisconfigured`) | the row's configured language tag cannot name an index directory at all (malformed, not merely unindexed) — under v2 an unindexed-but-well-formed tag is answered from a fallback, so `bad_lang` narrowed to mean only "this tag is not usable at all," which is as permanent a fault as an unknown vocabulary |
| every other token (`missing_q`, `bad_kind`, `bad_offset`, `bad_limit`, `unknown_concept`, `obsolete`, `vocab_unavailable`, and any token this connector does not name explicitly) | varies | **transient** | the mount exists and the row's configuration is not inherently wrong; the failure is the service's problem right now (a bad request the connector should never actually send, an id that moved, a mount that could not be read) and may clear on its own or on retry |

The permanent/transient split exists because `Search` and `Fetch` need to
tell an operator two different kinds of "this failed": a row that is
*configured wrong* (fix the row) versus a service that is *currently having
trouble* (nothing to fix locally, watch the logs). `ServiceError.Unwrap`
returns `ErrMisconfigured` for exactly the two permanent tokens above, so
`errors.Is(err, ErrMisconfigured)` finds a permanent fault without any
caller having to switch on `Token` itself; every other token's `Unwrap`
returns `nil`. This travels *alongside* `ErrDegraded` on `Search`, not
instead of it — autocomplete still degrades to an empty list for a
misconfigured row exactly as it does for an outage, but a throttled server
log can say which one it actually was.

`ServiceError.Status` is always set from the HTTP transport
(`resp.StatusCode`), never trusted from the response body, even though the
body has no `json:"-"` protection against a same-named key — a defensive
choice against a future token, or a misbehaving proxy, that happens to send
its own `"status"` field.

## URI resolution

`Fetch`'s `uri` argument is reduced to a bare concept id (the URI's last
path segment) before it is sent — `concept/{id}` and `under=` on `suggest`
both take a bare id, never a full IRI. This holds regardless of which host
the URI names; there is no vocabulary-name-specific or Getty-specific
special case.

## Every call is bounded

A single call to either endpoint is canceled at a per-row timeout — default
**3 seconds** — so a slow or hung service cannot block the picker
indefinitely.

The per-row override is the row's `config` JSON's `timeout` key, decoded
straight into a Go `time.Duration` field. **A JSON number in that position
is nanoseconds**, because that is what `time.Duration`'s default JSON
decoding does — there is no unit conversion in the connector. `{"timeout":
5}` is therefore a five-*nanosecond* deadline: every call to that
vocabulary is canceled before it can leave the process, and that row
degrades permanently. A five-second timeout has to be written out in full:

```json
{"vocab": "aat", "lang": "en", "timeout": 5000000000}
```

A value of `0` or less (or an absent `timeout`) takes the 3-second default.

## Out of scope for this contract

`children`, `Entry.Matched`/`matchedLabel`, `altLabel`, `narrower`,
`matches`, `broaderOther`, `langs=`, a JSON-LD representation, response
caching, and any language pre-check or retry logic are all things the wider
service contract supports (or documents) that core does not consume as of
this document. They are not wrong for an implementer to serve; core simply
has no code path reading any of them yet.
