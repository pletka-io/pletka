# Pletka: Design Philosophy and User Story

## The Problem

Cultural heritage institutions — museums, archives, libraries — need to describe their
collections in structured, interoperable ways. They use ontologies: formal vocabularies that
define how things relate to each other. CIDOC-CRM alone has 81 classes and 160 properties.
Add extensions like Linked Art, LRMoo, and EDM, and you face a universe of possible paths
with little guidance on best practices.

Historically, each institution built their data models from scratch. A curator would study
the ontology specification, choose classes and properties, define constraints, and document
everything manually. The next institution would do the same work again, often making
different choices for the same concepts. When it came time to share or integrate data, the
inconsistencies meant months of reconciliation work.

The barrier to entry was high. The cost of mistakes was invisible until integration time.
And the collective knowledge of what worked stayed locked in individual heads and
institutional wikis.

---

## Our Building Blocks

Pletka breaks ontology modelling into composable building blocks. You build bottom-up,
from small reusable pieces to complete descriptions of your domain.

### Fields

A **field** is the atomic unit. It is an ontological path that starts with a scope class
and ends where a user would enter real content — a name, a date, a reference to another
entity.

Each field has:
- An **ontology path**: the chain of classes and properties from the CRM (or other ontology)
  that gives this field its formal meaning
- An **expected value type**: what kind of content goes at the end of the path — plain text,
  a date, a URI, a reference to another model, or a term from a controlled vocabulary
- A **multilingual title and description**: human-readable context for what this field means
  and how it should be used

The expected value type is what defines the field's semantic relation. A field that expects
a "Model" reference creates a link to another entity. A field that expects a "Concept"
connects to a controlled vocabulary. A field that expects a "String" holds literal content.
The same ontology path can end with different value types depending on how you want to use
it.

You don't build field paths by hand. Pletka's **path builder** guides you step by step:
you select a class, the platform shows you which properties are available on that class,
you pick one, and it shows the next set of classes. At each step, the suggestions are
filtered by your project's ontologies and ranked by community adoption — commonly used
paths appear first. The path builder ensures ontological correctness as you go: you
cannot create an invalid class-property combination.

By itself, a single field is not very useful. Its power comes from composition.

### Categories

**Categories** are the organisational layer that makes your field library navigable. Without
them, you face a flat list of dozens or hundreds of fields. Categories group fields into
human-readable sections — "Existence", "Description", "Names and Identifiers", "Technical
Metadata" — so users can find what they need.

Categories are not semantic units. They carry no ontological meaning. They are how you
organise fields for human consumption: tabs in a form, sections in documentation, chapters
in a guide. Each project can organise its categories differently, and when you adopt fields
from another project, you can remap their categories to match your own structure.

### Collections

**Collections** are semantic blocks of meaning. They group related fields that share a
common ontological context — for example, a "Birth Event" collection gathers all the fields
related to E67_Birth: the date, the place, the participants.

Inside a collection, you often want to **override** the field's title and description.
The base field might be called "P7 took place at" (its formal ontology name), but inside
a "Birth Event" collection, you would override it to "Place of birth" with a description
like "Where this person was born." These overrides make the intent clear without changing
the underlying ontological path.

Overrides are what make reuse practical. The same field — same ontology path, same formal
meaning — can appear in multiple collections with different names and descriptions that
match each context. The ontological rigour stays intact underneath.

### Models

A **model** is a semantic unit of meaning. It represents a real-world entity type you want
to describe — a Person, an Object, a Place, an Event. A model has a scope class (the CRM
class it represents) and bundles collections and fields together with its own layer of
overrides.

Model-level overrides can do more than rename. They can change the expected value type of a
field (narrowing what content is accepted), mark fields as required or optional, set
minimum and maximum occurrences, or hide fields that don't apply in this context.

### The Weave

The **weave** is the complete picture: all your models, their collections and fields, the
relationships between models, and the overrides that make everything fit your specific use
case. A weave is a slice of the ontological universe — the part you have chosen to describe,
with the precision you need.

If a model describes one entity type, the weave describes how those entity types relate to
each other. A "Person" model references a "Place" model through a birth event. An "Object"
model references a "Person" model through a production event. The weave captures these
connections.

---

## Relations and Expected Value Types

Fields don't exist in isolation. They create connections — between entities, between
concepts, between data and meaning. The **expected value type** at the end of a field's
ontology path determines what kind of connection it makes.

| Expected Value Type | What It Connects To | Example |
|---|---|---|
| **String** | Literal text content | A title, a description, a note |
| **Date** | A temporal value | A birth date, a creation date |
| **URI** | An external web resource | A link to a Wikipedia page |
| **Model** | Another entity in your weave | A Person linked from a Production event |
| **Collection** | A structured block of fields | An Address block within a Place |
| **Concept** | A controlled vocabulary term | A material type from the AAT thesaurus |
| **GeoJSON** | Geographic coordinates | A location on a map |

When a field's expected value type is "Model", it creates a semantic relation between two
entity types. This is how the weave connects: an Object's "produced by" field points to a
Person model. A Person's "born at" field points to a Place model. The expected value types
are the joints that hold the weave together.

These value types can be **overridden** at the model level. A base field might accept any
Model reference, but in a specific model you can narrow it to require a particular model
type — for example, the "created by" field on an Object must reference a Person or Group,
not a Place. This layered constraint system keeps the base fields reusable while letting
each model enforce its own rules.

---

## The Power of Overrides

Overrides are the mechanism that makes composition and reuse work. Without overrides, you
would need a separate field for every context — "Person Name", "Object Title",
"Place Name", "Event Label" — even though they all use the same ontology path
(P1_is_identified_by). With overrides, you build one field and adapt it everywhere.

The override chain has three layers:

1. **Base field**: the canonical definition with its ontology path, default name, and
   default expected value type
2. **Collection override**: rename and redescribe the field for the collection's context
3. **Model override**: further rename, change the expected value type, set cardinality
   constraints, or mark as required

The most specific override wins. If a model sets a field name, that takes precedence over
the collection name, which takes precedence over the base field name. If no override exists
at a level, the system falls back to the next layer.

This means the same field — the same ontological path, the same formal semantics — can
appear as "Title" in an Object model, "Name" in a Person model, and "Label" in a Concept
model. The interoperability is preserved at the ontology level. The human understanding is
preserved at the override level.

See **Appendix A** for a concrete example of how this works with Linked Art's Names and
Identifiers.

---

## How Pletka Changes This

Our platform focuses on composition and reuse. Instead of building from scratch, you
compose from a growing library of patterns that the community has already validated.

Think of it like neural pathways. Every time a field path is adopted by another project,
the groove gets deeper. The more projects that use a particular way of describing
"production events" or "object dimensions", the more confident you can be that this path
is well-established and interoperable. We surface these patterns through weighted
suggestions — similar to how Google's PageRank algorithm ranked web pages by how many
other pages linked to them. The most-adopted patterns float to the top.

This makes reuse the path of least resistance. You *can* create something entirely new,
but the platform gently guides you toward patterns that others have already proven to work.

Pletka also **validates** your models as you build them. Ontology paths are checked for
correct class-property alternation. Required fields are enforced. Expected value types are
verified against the ontology's domain and range constraints. When something is
inconsistent, the platform tells you what is wrong and why — before you publish, not after
your data fails to integrate.

### Starting a Project

You start by creating a project and selecting which ontologies you want to work with.
Ontologies come in **families** — CIDOC-CRM is the parent, and extensions like Linked Art,
LRMoo, and CRMarchaeo inherit from it. When you select an extension, its parent ontology
is included automatically. This filters your suggestions: choose CIDOC-CRM with the Linked
Art profile, and you see fields, collections, and models relevant to art museum data. Add
LRMoo, and bibliographic patterns become available too. The family structure means you
never accidentally use a property from an extension you haven't loaded.

You can select a **parent project** as your foundation. All the parent's published fields,
collections, and models become available in your library. You don't copy them — you
reference them. When you adopt a pattern, you can override names and descriptions for your
context. If the parent updates their patterns and publishes a new version, you can choose
to incorporate those changes.

Everything you adopt or override becomes part of your weave. The library holds all the
available patterns. Your weave is what you have actively chosen to use and how you have
adapted it.

### Composing Your Weave

With your library populated, you compose what you want to describe. Browse fields by
category. Adopt collections that match your needs. Assemble models from collections and
individual fields. The field paths under the overrides drive the interoperability — no
matter how you rename things for your users, the ontological paths stay consistent.

The suggestion engine learns from the community. Fields that are commonly used together
appear as recommendations. Collections that have been adopted by many projects rank higher.
Patterns that work well together are surfaced. You benefit from every project that came
before yours.

### The Weave Loom

The list-based interfaces work well for precise editing, but sometimes you need to see the
big picture. The **Weave Loom** is a graphical canvas where you can visually compose and
explore your weave. Models appear as nodes, fields and collections as connectors, and the
relationships between models as edges. You can drag models into place, connect them through
fields, and see at a glance how your entity types relate to each other.

The Loom is not a replacement for the structured editors — it is a different lens on the
same data. Changes on the canvas update the underlying models and fields. Changes in the
editors appear on the canvas. The Loom is where you design the shape of your weave; the
editors are where you refine the details.

---

## Making It Accessible

All of this is still technical. How does it help the non-technical users who actually
enter data, review records, or manage collections?

Overrides support **examples**. For each field in a collection or model, you can provide
sample values that show users what correct data looks like. "For the Title field, enter
the object's primary display name, e.g., 'The Night Watch' or 'Portrait of a Lady'."

These examples, combined with the multilingual descriptions on overrides, become the basis
for **generated documentation**. Pletka can produce human-readable guides that explain
each model: what fields it has, what each field means, what kind of data goes there, and
what good examples look like. This is how end-users understand what you have built to
describe your information — without ever seeing an ontology path or a CRM class number.

Beyond documentation, Pletka can **visualise** your weave as interactive diagrams. See
how models connect to each other, which fields link entity types, and where ontology paths
converge. These visualisations help during design — spotting missing connections or
redundant paths — and serve as shareable overviews for stakeholders who need to understand
the data model without reading every field definition.

---

## Generators

Once you have a well-described weave, you might ask: and then what? Hand-translating your
models into implementation formats is cumbersome and error-prone. So Pletka offers
**generators** that take a weave and produce reusable output formats:

- **RDF/RDFS** — the full weave as a formal ontology
- **SHACL** — validation shapes for your application profile
- **Linked Art JSON-LD** — records conforming to the Linked Art API
- **Arches** — resource models for the Arches heritage platform
- **ResearchSpace** — configuration for the ResearchSpace environment
- **SIP-Creator** — record definitions for metadata ingestion
- **3M/X3ML** — mapping definitions for the X3ML transformation engine

By implementing the generator interface, you can add new output formats for your specific
needs. The generator reads your weave — models, collections, fields, overrides, expected
value types — and produces the output. Change your weave, regenerate, and your
implementation formats stay in sync.

---

## Users and Organisations

Individual users can create projects, build models, and publish their work. This is
enough for researchers, freelance consultants, or small teams.

**Organisations** — museums, archives, library networks — are users with additional
capabilities. They can manage members with different roles, control which projects are
visible internally versus publicly, and coordinate work across teams. The model follows
what developers know from GitHub: an organisation owns projects, members have roles, and
visibility is controlled per project.

Roles determine what you can do within a project. **Viewers** can browse models and
generated documentation. **Contributors** can create and edit fields, collections, and
models. **Managers** handle project settings, categories, and member access. **Owners**
have full control, including publishing releases and managing organisation-level settings.
These roles apply consistently whether you are an individual collaborator or an
organisation member.

For the platform, an organisation is a user with more features. The same composition and
reuse mechanisms work at both levels. An organisation's projects can serve as parent
projects for their member institutions, creating a shared foundation that individual
projects extend and adapt.

---

## Versioning

Things change. Fields get refined, new collections are added, models evolve as
understanding deepens. How do you keep track of what changed and why?

Pletka adopts version control concepts from software development — specifically, the
distributed model behind Git and GitHub:

- **Branches** are where you actively work on changes. Your draft modifications don't
  affect anyone else until you are ready.
- **Merge requests** let collaborators review proposed changes before they take effect.
  Reviewers can comment, suggest modifications, and approve or request revisions.
- **Releases** mark specific versions as stable and published. Other projects can only
  adopt from published releases — they never see your work-in-progress.
- **History** records every change with who made it, when, and why. You can compare any
  two versions and see exactly what changed.

Under the hood, Pletka stores data in both a database (for fast querying and the web
interface) and a Git-backed filesystem (for full history and distributed operation). The
Git layer means your work is never locked into a single platform. If Pletka the service
disappears, your ontology models, version history, and collaboration records remain intact
and portable.

---

## The Future

We are building toward full collaborative workflows on the platform itself: issues for
tracking decisions and open questions, comments on specific fields or models, task
assignment for team coordination. This mirrors how development teams work on GitHub — making
the work, the decisions, and the reasoning visible and transparent.

The goal is that a new team member can look at a project's history and understand not just
*what* was modelled, but *why* each choice was made and *who* made it.

---

## Appendix A: Building "Names and Identifiers" from Scratch

This walkthrough shows how Pletka's building blocks compose in practice. We will build a
"Names and Identifiers" category following the Linked Art pattern, then show how the same
fields work in two different models with different overrides.

### Step 1: Create the Fields

We start with the atomic units — individual ontology paths that end where users enter
content.

**Field: "Appellation Content"**
- Ontology path: `P1_is_identified_by → E33_E41_Linguistic_Appellation → P190_has_symbolic_content`
- Expected value type: **String**
- Description: "The textual content of a name or title"

**Field: "Appellation Type"**
- Ontology path: `P1_is_identified_by → E33_E41_Linguistic_Appellation → P2_has_type → E55_Type`
- Expected value type: **Concept** (from Getty AAT)
- Description: "The classification of this name (primary, alternative, former, etc.)"

**Field: "Appellation Language"**
- Ontology path: `P1_is_identified_by → E33_E41_Linguistic_Appellation → P72_has_language → E56_Language`
- Expected value type: **Concept**
- Description: "The language this name is expressed in"

**Field: "Identifier Content"**
- Ontology path: `P1_is_identified_by → E42_Identifier → P190_has_symbolic_content`
- Expected value type: **String**
- Description: "The textual content of a formal identifier (accession number, catalogue number, etc.)"

**Field: "Identifier Type"**
- Ontology path: `P1_is_identified_by → E42_Identifier → P2_has_type → E55_Type`
- Expected value type: **Concept** (from Getty AAT)
- Description: "The classification of this identifier (accession number, inventory number, etc.)"

Notice that these fields are generic. They say nothing about objects, people, or any
specific entity. They describe how names and identifiers work in the ontology. The
specificity comes from overrides.

### Step 2: Create the Collections

We group related fields into semantic blocks.

**Collection: "Name" (context: E33_E41_Linguistic_Appellation)**

| Field | Override Name | Override Description |
|---|---|---|
| Appellation Content | **Name** | "The display name" |
| Appellation Type | **Name Type** | "Primary, alternative, or former name" |
| Appellation Language | **Language** | "The language of this name" |

**Collection: "Identifier" (context: E42_Identifier)**

| Field | Override Name | Override Description |
|---|---|---|
| Identifier Content | **Identifier** | "The identifier value (e.g., '1990.45.2')" |
| Identifier Type | **Identifier Type** | "What kind of identifier this is" |

Each collection takes generic fields and gives them purpose through overrides. "Appellation
Content" becomes "Name" — immediately understandable.

### Step 3: Create the Category

**Category: "Names and Identifiers"**
- Canonical order: 1 (appears first in forms)
- Description: "How this entity is named and formally identified"

Both the "Name" and "Identifier" collections are assigned to this category. When a user
sees a form, the first section they encounter is "Names and Identifiers", containing name
fields and identifier fields in clearly separated blocks.

### Step 4: Use in an Object Model

**Model: "Object" (scope: E22_Human-Made_Object)**

The Object model adopts both collections but overrides further:

| Collection | Field | Model Override Name | Notes |
|---|---|---|---|
| Name | Name | **Title** | Objects have titles, not names |
| Name | Name Type | **Title Type** | "Primary title, alternative title" |
| Name | Language | *(no override)* | Keeps collection name |
| Identifier | Identifier | **Accession Number** | Narrows the description |
| Identifier | Identifier Type | *(hidden)* | Only one type applies, so hide the chooser |

The model also sets constraints:
- Title is **required** (min occurs: 1)
- Accession Number is **required** (min occurs: 1)
- Title Type defaults to AAT:300404670 ("preferred terms")

A curator filling out an Object record sees: "Title" (required), "Title Type", "Language",
"Accession Number" (required). Clean, focused, purpose-built for objects.

### Step 5: Use in a Person Model

**Model: "Person" (scope: E21_Person)**

The same collections, different overrides:

| Collection | Field | Model Override Name | Notes |
|---|---|---|---|
| Name | Name | *(no override)* | "Name" works for people |
| Name | Name Type | *(no override)* | "Primary name, birth name, married name" |
| Name | Language | *(no override)* | |
| Identifier | Identifier | **ULAN ID** | Narrows to artist identifiers |
| Identifier | Identifier Type | *(hidden)* | Fixed to ULAN type |

Constraints:
- Name is **required** (min occurs: 1)
- ULAN ID is **optional** (not all people have ULAN records)

A registrar filling out a Person record sees: "Name" (required), "Name Type", "Language",
"ULAN ID" (optional). Same underlying fields, completely different user experience.

### What We Built

```
Fields (5 generic)
  └── Collection: "Name" (3 fields, overridden)
  │     └── Category: "Names and Identifiers"
  │     └── Used in: Object (as "Title"), Person (as "Name")
  └── Collection: "Identifier" (2 fields, overridden)
        └── Category: "Names and Identifiers"
        └── Used in: Object (as "Accession Number"), Person (as "ULAN ID")
```

Five fields. Two collections. One category. Used in two models with completely different
user-facing names and constraints. The ontological paths are identical — data from both
models is interoperable. The user experience is tailored to each context.

This is what composition and reuse look like in practice. Every project that adopts these
fields deepens the grooves. Every override adds clarity without breaking compatibility.
