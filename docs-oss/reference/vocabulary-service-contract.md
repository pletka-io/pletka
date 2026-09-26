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

## Vocabularies are project-owned

There is no global tier and a vocabulary is never shared across projects.
Every `weave_vocabularies` row belongs to exactly one project — `project_id`
is `NOT NULL` (migration `013_vocabulary_project_ownership.sql`) — and
`weave_project_vocabularies`, the join table that used to let a project pick
from a shared pool, is dropped. Two projects that both want Getty AAT each
create their own row against the `aat` mount and get their own cached
entries; nothing is resolved or reused across projects.

Owning the row **is** the enablement, so there is no separate "selected"
flag to flip:

- **Enable** — `POST /projects/{projectID}/settings/vocabularies` with
  `{"mount": "<name>", "lang": "<optional>"}` creates a row with
  `connector_type: "vocabservice"` and `config: {"vocab": "<mount>"}` (the
  config shape above). Adding the same mount twice for the same project is
  rejected — a unique index on `(project_id, system_name)` (migration
  `014_vocabulary_project_system_name.sql`) turns a second add into a
  conflict rather than a silent duplicate.
- **Disable** — `DELETE /projects/{projectID}/settings/vocabularies/{vocabularyID}`
  deletes the row. Its cached entries are removed with it by foreign-key
  cascade — they simply re-resolve from the service if the mount is added
  back. A vocabulary a concept list still points at cannot be deleted this
  way: `weave_concept_lists.vocabulary_id` has no `ON DELETE` action, so the
  delete comes back `409` instead of failing at the database (see
  `Handler.DeleteVocabulary` in `pkg/weave/settings`).
- The vocabularies settings screen offers only the mounts the instance's
  configured vocabulary service actually serves (`GET /vocab`, below), with
  each mount's languages shown before a curator picks one, and surfaces an
  error rather than an empty list when the service does not answer (see
  `ServiceVocabularyLister` / `buildServiceVocabularyOptions` in
  `pkg/weave/settings`).

**Frontend gotcha, disclosed here because nothing in the served JSON says
it:** the vocabularies settings form's `add_vocabulary_id` field
(`WidgetSelect`) sits in the same section as `enforce_concept_lists` and
`concept_namespace`, but its submit target is not that section's `PUT`. The
`PUT` (`Handler.UpdateVocabularies`) saves only `enforce_concept_lists` and
`concept_namespace` — picking a mount in `add_vocabulary_id` and hitting the
form's normal Save silently does nothing with it. Its real target is the
`POST` endpoint above. `FieldDef` already has a mechanism for exactly this
shape, a per-field `create_url` (see `pkg/weave/collection/formschema.go`,
`pkg/formschema/composition_sidebar.go` for existing uses) — `add_vocabulary_id`
does not set it, so a generic form renderer has no signal that this field's
action differs from its section's endpoint. This is inherited debt from an
earlier task in the `#3599` vocabulary-ownership work, not something this
document is fixing: treat `add_vocabulary_id` as needing bespoke wiring
(not a plain Save-button field) until it gets a real `create_url` contract.

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
  `VocabularyInfo`. At decode, every one of these fields is equally
  optional — an absent key just leaves the Go zero value, `name` and
  `languages` included, no differently from `label` and `concepts`.
  `label` and `concepts` do carry `omitempty` in the Go struct, but that
  tag only affects *marshaling* `VocabularyInfo` back out (were core to
  serialize it into its own response somewhere); it has no bearing on
  decoding the service's listing, where `encoding/json` never errors on a
  missing key regardless of any tag. `languages` is which languages have
  their own suggest index on that mount (most mounts serve exactly one) —
  a configuration screen needs it to tell an operator that a Dutch project
  pointed at an English-only mount will only ever answer in English.

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
| `limit` | yes | the caller's requested limit; core clamps it itself, client-side, before the request ever leaves: a non-positive value becomes the default `50`, and anything over `100` is capped to `100` |
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
- `scopeNote` — a language-keyed object, `{lang: sentence}`. **This is new
  in this branch: v1 read no `scopeNote` at all.** Narrowed like `prefLabel`
  — display language plus English, at most two keys — when the display
  language has a note of its own; unlike `prefLabel`, it also has a
  fallback chain for when it does not. See **Why the narrowing** below for
  both halves and why they differ.
- `altLabel` — **decoded off the wire, and not mapped anywhere.** `Entry`
  has no field for it; inventing one is out of scope for this work.
- `narrower`, `matches`, `broaderOther` — **not represented in the decode
  type at all.** The connector's Go struct simply has no field for them, so
  they are silently ignored the way any JSON decoder ignores an unknown key.
- `children` — same as `narrower`: not decoded, not consumed. (`children` is
  in fact its own endpoint in the wider contract, `GET
  /vocab/{name}/children/{id}`; core calls neither that endpoint nor reads a
  `children` key were one present on `concept`.)

A `prefLabel` that does not decode as a language-keyed object — the only
shape v2 ever sends — is a real decode error on `Fetch`, for example a JSON
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
`concept/{id}`'s `prefLabel` down to the resolved display language plus
English, exactly as `suggest` already does. That rule is an exact lookup
(`labelFor` in `item.go`) because the contract specifically guarantees
`lang` names a key of `prefLabel` — a miss there would mean the service
broke its own promise, not that a fallback is owed.

`scopeNote` is narrowed with the same at-most-two-keys shape when the
display language has a note of its own: the display language plus English,
present and different, exactly as `prefLabel`'s pairing works and for the
identical reason — the frontend's `tr()` resolves a stored language map as
requested language, then English, then whatever is left, so a note stored
in only the display language falls through to an arbitrary remaining key
for a curator whose next-best language is English, where the pairing lands
them on English instead. (Arguably this matters *more* for a scope note
than a label: a curator can often still recognize a label in an unfamiliar
language, but a whole paragraph either reads or it does not.)

Where `scopeNote` differs from `prefLabel` is what happens on a miss — and
this difference **was gotten wrong once already in an earlier round of
this branch**, by reusing `prefLabel`'s rule (an exact lookup with no
fallback at all) for `scopeNote` too. That rule is correct for `prefLabel`
because the contract guarantees `lang` names one of its keys; it makes no
such promise about `scopeNote`, so treating a miss there as impossible
silently dropped the note whenever the display language had none of its
own — measured against the live service, seven of AAT 300010957's fourteen
`prefLabel` languages have no `scopeNote` at all — and the effect on
`Fetch` was worse than a merely absent field: the stored `Entry` reaches an
upsert (`pkg/database/queries/weave_vocabulary.sql`) with no guard against
replacing an existing scope note with nothing, so resolving the same
concept in one of those languages silently erased a scope note the row
already had. `scopeNote` therefore gets its own fallback chain
(`scopeNoteFor` in `item.go`) for exactly the miss case: when the display
language has no note, fall back to English alone, then whatever `und`
(untagged) value the concept has — the contract calls `scopeNote` out as
the one predicate whose values can still arrive with no language tag at
all, so skipping that fallback would drop an untagged note exactly the way
the bug dropped a tagged one — then nothing. The hit case and the miss case
are both always narrowed: at most the display language plus English on a
hit, at most one key on a fallback, never the whole map.

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
| Row has no `vocab` configured | empty result, `ErrDegraded` (wrapping `errNoVocab`) | error saying the row has no vocabulary configured, not `ErrDegraded` |

`errNoVocab`'s text is "no vocabulary configured for this row" — it names
the *problem*, not a vocabulary, because there isn't one to name: the row's
`vocab` is empty in the first place, which is exactly what triggers this
path.

`Search` has exactly one failure mode from the caller's point of view: it
wraps everything the HTTP round trip can produce — transport error, non-2xx
status, and an undecodable body alike — as `ErrDegraded`, with zero entries
and no other signal. **An undecodable `suggest` body does not surface as an
error** — only `Fetch` ever returns a decode failure as an error — but that
does not make it invisible: `ErrDegraded` exists precisely so a degraded
search can be told apart from a genuine no-match, and `Search`'s wrapping
of a decode failure into it is exactly as deliberate as its wrapping of a
timeout or a non-2xx status. One layer up,
`pkg/weave/vocabulary`'s `classifyConnectorErr` turns any `ErrDegraded`
into a `degraded` flag, and the picker's response carries `"degraded":
true` — so a caller that cares can tell "the service could not be reached
or its answer could not be read" apart from "nothing matched," even though
neither ever reaches the picker as a Go `error` to handle. (A previous
version of this document said suggest's decode failure surfaced as "error"
— that was wrong, `Search`'s wrapping is unconditional; a later revision
overcorrected into calling the result indistinguishable from a no-match,
which contradicts the reason `ErrDegraded` exists at all.)

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
| every other token (`missing_q`, `bad_kind`, `bad_offset`, `bad_limit`, `unknown_concept`, `obsolete`, `vocab_unavailable`, and any token this connector does not name explicitly) | varies | **transient** | none of these mean the *row's configuration* is wrong, which is the only thing `ErrMisconfigured` claims — so none of them get it, even though not all of them will actually resolve themselves. `vocab_unavailable` genuinely can clear on its own (the mount becomes readable again) or on retry. `unknown_concept` and `obsolete` will not: a stored URI that stops resolving, or gets retired, stays that way no matter how many times it is retried — but that is a fact about the *concept id*, not about the row's own `vocab`/`lang` configuration, which is the only thing this classification speaks to |

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
(`resp.StatusCode`), never from the response body: the field is tagged
`json:"-"`, so a body that happens to send its own `"status"` key — no
documented token's does today — cannot overwrite it during decode. That tag
is a defensive choice against a future token, or a misbehaving proxy, doing
exactly that.

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
