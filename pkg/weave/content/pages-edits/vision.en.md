<!-- pletka-pages-edit v1
slug: vision
lang: en
-->

# Vision (en)

Edit prose between the `<!--key:...-->` markers. Do not rename, reorder, or remove the markers. Run `pletka pages hydrate` to write changes back to the language JSON.

<!--key:pages.vision.nav_label-->
Vision

<!--key:pages.vision.seo_title-->
Pletka — The Approach

<!--key:pages.vision.seo_description-->
Composable semantic data patterns. Build bottom-up, from small reusable pieces to complete descriptions of your domain.

<!--key:pages.vision.blocks_0_eyebrow-->
The Approach

<!--key:pages.vision.blocks_0_title-->
Pletka

<!--key:pages.vision.blocks_0_subtitle-->
Composable semantic data patterns. Build bottom-up, from small reusable pieces to complete descriptions of your domain.

<!--key:pages.vision.blocks_1_heading-->
Contents

<!--key:pages.vision.blocks_2_eyebrow-->
Section I

<!--key:pages.vision.blocks_2_heading-->
The Problem

<!--key:pages.vision.blocks_2_body-->
In an information society, data is our primary source of information; it provides us the picture of the world from which we act. When it is fragmented, opaque or uninterpretable, we are blocked from having the best information to guide our understanding and our action. Formal ontologies and semantic data are historically the solution offered to combat this problem. Explicitly adopting ontologies — formal descriptions of data in a domain — should enable the integration and conscious description of data. But there's a catch. Ontologies can do the job, but very few people are trained into how to use them.

Pletka offers a whole new approach to building a conscious, usable data contract, for a project, an institution, or even a global network of actors. It makes the application of ontologies practical by allowing them to be organized according to understandable data patterns: fields, collections, models. These data patterns don't have the high learning curve that ontologies do; instead they enable the ontologies to be implemented based on this easy to understand encoding as patterns.

For example, in the world of cultural heritage, museums, archives, and libraries aim to describe their collections in structured and interoperable ways. Semantic data integration would allow them to share knowledge about works, exhibitions, loans, and more. This is all data that is described in different information silos, built with different data structures and different data curation methodologies and held in different institutions but which, when brought together, can tell the whole story of our cultural past.

There is even a well known and maintained top level ontology to do so: the CIDOC CRM. But with over 80 classes and 160 properties, alongside the many choices of how to implement these, the task of building a common data picture of our past across systems — even in one institution — is daunting. Add extensions like LRMoo and you face a universe of possible paths with little guidance on best practices.

Historically, ontologies have been adopted individually, by projects and institutions, each learning, understanding and implementing the ontology from scratch. A curator, for example, would study the ontology specification, choose classes and properties, define constraints, and document everything manually. The next institution would do the same work again, often making different choices for the same concepts. When it came time to share or integrate data, the inconsistencies meant months of reconciliation work.

When you think of it, the old approach was wrong from the start. If ontologies are built collectively to represent a shared understanding, then the decision of how to apply them must also be a collective act. Until now there was no method or tool to make that possible.

Pletka changes this. It enables experts and expert groups to create and document reusable semantic data patterns that say what they are for in plain language for all stakeholders: the users, the developers, the ontologists and the computer systems. Even better, these can be published, shared and reused and consciously versioned, enabling teams to manage their long term investment in their semantic contract.

<!--key:pages.vision.blocks_3_text-->
The idea of expecting domain users and separate institutions to each learn and identically apply ontologies to solve data fragmentation is a dead end. Building and sharing good semantic data patterns that guide individuals and institutions toward their data integration goals is the future — which is now.

<!--key:pages.vision.blocks_4_eyebrow-->
Section II

<!--key:pages.vision.blocks_4_heading-->
Our Building Blocks

<!--key:pages.vision.blocks_4_body-->
Pletka enables the documentation and representation of semantic modelling patterns as composable building blocks that everyone can understand. You build bottom-up, from small reusable pieces to complete descriptions of your domain.

### Fields

The **field** is our atomic unit. Fields are named data points that would typically be registered and queried in a domain: birth date, name, production place, and so on. A field gives an identifier, a name and a description to an ontology pattern that starts with a class and follows an ontological path (classes and properties) to an end value where a user would enter or search real content — a name, a date, a reference to another entity.

Each field has:

- **Identifier** — a unique code by which to name and retrieve this pattern
- **Name** — a human-readable label that indicates what the field is for generally
- **Description** — a human-readable text that describes how the field should be interpreted and used
- **Ontology Scope** — the class in an ontology from which this field can be used
- **Ontology Path** — the chain of classes and properties from the CRM (or other ontology) that gives this field its formal meaning
- **Expected Value Type** — what kind of content goes at the end of the path: plain text, a date, a URI, a reference to another model, or a term from a controlled vocabulary

*By itself, a single field is not very useful. Its power comes from composition.*

### Collections

**Collections** group multiple fields that share the same documentation subject but can be reused in multiple contexts. Take for example the idea of a description. A description may have its content, a type, and its language. A description is something that needs modelling and documentation but whose semantic content is defined by several fields. The collection serves the function of naming and identifying this unit — e.g. *Description: a free text description of an entity potentially in a language with a source* — and formally defining how it is constituted by composing it with two or more fields (e.g. `description_content`, `description_language`, `description_source`).

Each collection has:

- **Identifier** — a unique code by which to name and retrieve this pattern
- **Name** — a human-readable label that indicates what the collection is for generally
- **Description** — a human-readable text that describes how the collection should be interpreted and used
- **Ontology Scope** — the class in an ontology from which this collection can be used

And is composed from a list of fields with a compatible scope and subject.

Inside a collection, you often want to override the field's name and description. The base field might be called "had location" and define a high-level property like `crm:P7 took place at` (its formal ontology name), but inside a "Birth Event" collection you would override it to "Place of Birth" with a description like "Where this person was born." These overrides make the intent clear without changing the underlying ontological path strategy. Better yet, in your semantic contract you don't need to explain `crm:P7_took_place_at` to all stakeholders, just "Place of Birth".

Overrides are what make reuse practical. The same field — same ontology path, same formal meaning — can appear in multiple collections with different names and descriptions that match each context. The ontological rigour stays intact underneath.

### Models

A **model** represents the semantic contract of your project on how and what you want to represent about a main entity of concern for your domain of interest. It represents a real-world entity type you want to describe — a Person, an Object, a Place, an Event. One model describes a class of things as you want to document and retrieve them semantically. A collection of models defines your domain of interest and makes sure the dataspace about them integrates seamlessly.

Each model has:

- **Identifier** — a unique code by which to name and retrieve this pattern
- **Name** — a human-readable label that indicates what the model is for generally
- **Description** — a human-readable text that describes how the model should be interpreted and used
- **Ontology Scope** — the class in an ontology from which this model can be used

And is composed from a list of collections and fields with a compatible scope.

With a full library of patterns at hand, composing a model is not a laborious time-consuming process — it is just a matter of clicking patterns into place.

The magic of creating a common data contract among stakeholders comes alive with models and using overrides. Ontologies define classes and properties in high abstract terms by necessity — this generality gives us the integration layer we need. But data projects work at the level of particular communities with particular interests. Model overrides allow us to rename fields and collections and provide local descriptions that make sense to domain users while the underlying semantics stay robust and correct for a robust semantic strategy. Meanwhile, the ability to narrow and specify the particular required values for a field or collection in a project context creates a strict data definition for the implementers of the data model. Everyone looks at the same data object and gets something they can understand, agree or disagree about, and finally put into action for researching data, building a semantic net, or rolling out data or an application.

### Categories

**Categories** are an additional organisational layer offered in Pletka projects. They serve a couple of fundamental functions. A robust Pletka project will have potentially hundreds of patterns encoding rich semantic documentation for different topics. Sorting through this requires a mechanism. Categories group fields and collections into areas of concern — "Existence", "Description", "Names and Identifiers", "Technical Metadata". They are like buckets where you can go to find the right pattern for the right job. Categories are also deployed within models in order to organize information relative to an overall entity. The documentation of a person by a prosopographer or a provenance event by a provenance researcher might entail dozens of fields or more. Categories help organize models by grouping information into logical units where people can consistently find them. These same groupings can be used to organize actual implementation and use of the model in production systems.

Categories are not semantic units. They carry no ontological meaning. They are how you organise fields for human consumption: tabs in a form, sections in documentation, chapters in a guide. Each project can organise its categories differently, and when you adopt fields from another project, you can remap their categories to match your own structure.

### The Weave

The **weave** is the complete picture, a total semantic project: all your models, their collections and fields, the relationships between models, and the overrides that make everything fit your specific use case. A weave is a slice of the ontological universe — the part you have chosen to describe, with the precision you need.

If a model describes one entity type, the weave describes how those entity types relate to each other. A "Person" model references a "Place" model through a birth event. An "Object" model references a "Person" model through a production event. The weave captures these connections.

<!--key:pages.vision.blocks_5_eyebrow-->
Section III

<!--key:pages.vision.blocks_5_heading-->
Relations and Expected Value Types

<!--key:pages.vision.blocks_5_body-->
Fields don't exist in isolation. They create connections — between entities, between concepts, between data and meaning. The **expected value type** at the end of a field's ontology path determines what kind of connection it makes.

| Expected Value Type | What It Connects To | Example |
|---|---|---|
| **String** | Literal text content | A title, a description, a note |
| **Date** | A temporal value | A birth date, a creation date |
| **URI** | An external web resource | A link to a Wikipedia page |
| **Model** | Another entity in your weave | A Person linked from a Production event |
| **Collection** | A structured block of fields | An Address block within a Place |
| **Concept** | A controlled vocabulary term | A material type from the AAT thesaurus |
| **GeoJSON** | Geographic coordinates | A location on a map |

When a field's expected value type is "Model", it creates a semantic relation between two entity types. This is how the weave connects: an Object's "produced by" field points to a Person model. A Person's "born at" field points to a Place model. The expected value types are the joints that hold the weave together.

These value types can be **overridden** at the model level. A base field might accept any Model reference, but in a specific model you can narrow it to require a particular model type — for example, the "created by" field on an Object must reference a Person or Group, not a Place. This layered constraint system keeps the base fields reusable while letting each model enforce its own rules.

<!--key:pages.vision.blocks_6_eyebrow-->
Section IV

<!--key:pages.vision.blocks_6_heading-->
The Power of Overrides

<!--key:pages.vision.blocks_6_body-->
Overrides are the mechanism that makes composition and reuse work. Without overrides, you would need a separate field for every context — "Person Name", "Object Title", "Place Name", "Event Label" — even though they all use the same ontology path (`P1_is_identified_by`). With overrides, you build one field and adapt it everywhere.

<div class="bg-white border border-gray-200 rounded-xl p-6 my-8 not-prose">
  <p class="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-6">The Override Chain</p>
  <div class="space-y-6">
    <div class="border-l-4 border-gray-300 pl-4">
      <p class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-1">Layer 1</p>
      <p class="text-lg font-semibold text-gray-900">Base Field</p>
      <p class="text-gray-500">The canonical definition with its ontology path, default name, and default expected value type</p>
    </div>
    <div class="border-l-4 border-pletka-primary pl-4">
      <p class="text-xs font-semibold text-pletka-primary uppercase tracking-wider mb-1">Layer 2</p>
      <p class="text-lg font-semibold text-gray-900">Collection Override</p>
      <p class="text-gray-500">Rename and redescribe the field for the collection's context</p>
    </div>
    <div class="border-l-4 border-orange-500 pl-4">
      <p class="text-xs font-semibold text-orange-500 uppercase tracking-wider mb-1">Layer 3</p>
      <p class="text-lg font-semibold text-gray-900">Model Override</p>
      <p class="text-gray-500">Further rename, change the expected value type, set cardinality constraints, or mark as required</p>
    </div>
  </div>
  <div class="mt-6 pt-6 border-t border-gray-100">
    <p class="text-sm text-gray-500 italic">The most specific override wins. If no override exists at a level, the system falls back to the next layer.</p>
  </div>
</div>

This means the same field — the same ontological path, the same formal semantics — can appear as "Title" in an Object model, "Name" in a Person model, and "Label" in a Concept model. The interoperability is preserved at the ontology level. The human understanding is preserved at the override level.

<!--key:pages.vision.blocks_7_eyebrow-->
Section V

<!--key:pages.vision.blocks_7_heading-->
Composition and Reuse

<!--key:pages.vision.blocks_7_body-->
Our platform focuses on composition and reuse. Instead of building from scratch, you compose from a growing library of patterns that the community has already validated.

<!--key:pages.vision.blocks_8_text-->
Think of it like neural pathways. Every time a field path is adopted by another project, the groove gets deeper. The more projects that use a particular way of describing "production events" or "object dimensions", the more confident you can be that this path is well-established and interoperable.

<!--key:pages.vision.blocks_9_body-->
We surface these patterns through weighted suggestions — similar to how Google's PageRank algorithm ranked web pages by how many other pages linked to them. The most-adopted patterns float to the top. This makes reuse the path of least resistance. You *can* create something entirely new, but the platform gently guides you toward patterns that others have already proven to work.

### Starting a Project / Weave

You start by creating a project and selecting which ontologies and extensions you want to work with. This filters your suggestions — if you choose CIDOC-CRM with the Linked Art profile, you see fields, collections, and models that are relevant to art museum data. If you add LRMoo, bibliographic patterns become available too.

You can select a **parent project** as your foundation. All the parent's published fields, collections, and models become available in your library. You don't copy them — you reference them. When you adopt a pattern, you can override names and descriptions for your context. If the parent updates their patterns and publishes a new version, you can choose to incorporate those changes.

Everything you adopt or override becomes part of your weave. The library holds all the available patterns. Your weave is what you have actively chosen to use and how you have adapted it.

### Composing Your Weave

With your library populated, you compose what you want to describe. Browse fields by category. Adopt collections that match your needs. Assemble models from collections and individual fields. The field paths under the overrides drive the interoperability — no matter how you rename things for your users, the ontological paths stay consistent.

The suggestion engine learns from the community. Fields that are commonly used together appear as recommendations. Collections that have been adopted by many projects rank higher. Patterns that work well together are surfaced. You benefit from every project that came before yours.

<!--key:pages.vision.blocks_10_eyebrow-->
Section VI

<!--key:pages.vision.blocks_10_heading-->
Making It Accessible

<!--key:pages.vision.blocks_10_body-->
All of this is still technical. How does it help the non-technical users who actually enter data, review records, or manage collections?

Overrides support **examples**. For each field in a collection or model, you can provide sample values that show users what correct data looks like. *"For the Title field, enter the object's primary display name, e.g., 'The Night Watch' or 'Portrait of a Lady'."*

These examples, combined with the multilingual descriptions on overrides, become the basis for **generated documentation**. Pletka can produce human-readable guides that explain each model: what fields it has, what each field means, what kind of data goes there, and what good examples look like. This is how end-users understand what you have built to describe your information — without ever seeing an ontology path or a CRM class number.

<!--key:pages.vision.blocks_11_eyebrow-->
Section VII

<!--key:pages.vision.blocks_11_heading-->
Generators

<!--key:pages.vision.blocks_11_body-->
Once you have a well-described weave, you might ask: and then what? Hand-translating your models into implementation formats is cumbersome and error-prone. So Pletka offers **generators** that take a weave and produce reusable output formats:

<!--key:pages.vision.blocks_12_items_0_title-->
RDF/RDFS

<!--key:pages.vision.blocks_12_items_0_body-->
formal ontology

<!--key:pages.vision.blocks_12_items_1_title-->
SHACL

<!--key:pages.vision.blocks_12_items_1_body-->
validation shapes

<!--key:pages.vision.blocks_12_items_2_title-->
Linked Art JSON-LD

<!--key:pages.vision.blocks_12_items_2_body-->
museum records

<!--key:pages.vision.blocks_12_items_3_title-->
Arches

<!--key:pages.vision.blocks_12_items_3_body-->
heritage platform

<!--key:pages.vision.blocks_12_items_4_title-->
ResearchSpace

<!--key:pages.vision.blocks_12_items_4_body-->
environment config

<!--key:pages.vision.blocks_12_items_5_title-->
SIP-Creator

<!--key:pages.vision.blocks_12_items_5_body-->
metadata ingestion

<!--key:pages.vision.blocks_12_items_6_title-->
3M/X3ML

<!--key:pages.vision.blocks_12_items_6_body-->
mapping definitions for transformation

<!--key:pages.vision.blocks_13_body-->
By implementing the generator interface, you can add new output formats for your specific needs. The generator reads your weave — models, collections, fields, overrides, expected value types — and produces the output. Change your weave, regenerate, and your implementation formats stay in sync.

<!--key:pages.vision.blocks_14_eyebrow-->
Section VIII

<!--key:pages.vision.blocks_14_heading-->
Users and Organisations

<!--key:pages.vision.blocks_14_body-->
Individual users can create projects, build models, and publish their work. This is enough for researchers, freelance consultants, or small teams.

**Organisations** — museums, archives, library networks — are users with additional capabilities. They can manage members with different roles (curators, reviewers, editors), control which projects are visible internally versus publicly, and coordinate work across teams. The model follows what developers know from GitHub: an organisation owns projects, members have roles, and visibility is controlled per project.

For the platform, an organisation is a user with more features. The same composition and reuse mechanisms work at both levels. An organisation's projects can serve as parent projects for their member institutions, creating a shared foundation that individual projects extend and adapt.

<!--key:pages.vision.blocks_15_eyebrow-->
Section IX

<!--key:pages.vision.blocks_15_heading-->
Versioning

<!--key:pages.vision.blocks_15_body-->
Things change. Fields get refined, new collections are added, models evolve as understanding deepens. How do you keep track of what changed and why?

Pletka adopts version control concepts from software development — specifically, the distributed model behind Git and GitHub:

- **Branches** — Where you actively work on changes. Your draft modifications don't affect anyone else until you are ready.
- **Merge requests** — Let collaborators review proposed changes before they take effect. Reviewers can comment, suggest modifications, and approve or request revisions.
- **Releases** — Mark specific versions as stable and published. Other projects can only adopt from published releases — they never see your work-in-progress.
- **History** — Records every change with who made it, when, and why. You can compare any two versions and see exactly what changed.

Under the hood, Pletka stores data in both a database (for fast querying and the web interface) and a Git-backed filesystem (for full history and distributed operation). The Git layer means your work is never locked into a single platform. If Pletka the service disappears, your ontology models, version history, and collaboration records remain intact and portable.

<!--key:pages.vision.blocks_16_eyebrow-->
Section X

<!--key:pages.vision.blocks_16_heading-->
The Future

<!--key:pages.vision.blocks_16_body-->
We are building toward full collaborative workflows on the platform itself: issues for tracking decisions and open questions, comments on specific fields or models, task assignment for team coordination. This mirrors how development teams work on Git — making the work, the decisions, and the reasoning visible and transparent.

<!--key:pages.vision.blocks_17_text-->
The goal is that a new team member can look at a project's history and understand not just *what* was modelled, but *why* each choice was made and *who* made it.
