<!-- pletka-pages-edit v1
slug: about
lang: en
-->

# About (en)

Edit prose between the `<!--key:...-->` markers. Do not rename, reorder, or remove the markers. Run `pletka pages hydrate` to write changes back to the language JSON.

<!--key:pages.about.title-->
About Pletka

<!--key:pages.about.subtitle-->
A tool for building, sharing and reusing semantic data patterns in support of LOUDER semantic data — **L**inked, **O**pen, **U**nderstandable **D**ata that is **E**xtendable and **R**eusable.

<!--key:pages.about.service_title-->
The Pletka Service

<!--key:pages.about.service_text-->
The [Pletka.io](https://pletka.io) service offers a library of open semantic data projects (weaves) for reuse and learning by the community.

It also allows registered users to adopt and adapt these patterns in their own project or to build and share their own weaves.

Pletka is a full life-cycle solution to creating semantic data modelling contracts and applications, offering many tools for generating and using derivatives — serializations, queries, mappings, system templates — for the implementation of these contracts in and across systems.

<!--key:pages.about.who_title-->
Who Are We

<!--key:pages.about.who_text-->
[Pletka.io](https://pletka.io) is a service:

- **Designed and Managed by:** [Takin.solutions](https://takin.solutions)
- **Hosted and Powered by:** [Delving](https://delving.io)
- **Inspired and Driven by:** our Community
- **Built on:** the Pletka® open-source software package

<!--key:pages.about.method_title-->
The Pletka Method

<!--key:pages.about.method_text-->
**Adopt Solid Foundations · Create Reusable Patterns · Weave together a Data Whole.**

<!--key:pages.about.tools_title-->
Our Tools

<!--key:pages.about.ontologies_title-->
Ontologies

<!--key:pages.about.ontologies_text-->
Extant ontologies give us the basic language — human- and computer-readable — to build patterns out of.

<!--key:pages.about.fields_title-->
Fields

<!--key:pages.about.fields_text-->
The field is the base pattern. It is the definition of a repeating kind of information documented in your information space, defined in human terms and as an ontological path that starts with a scope class and ends where a user would enter real content — a name, a date, a reference to another entity. Each field has an ontology path, an expected value type, and multilingual titles and descriptions.

<!--key:pages.about.collections_title-->
Collections

<!--key:pages.about.collections_text-->
Collections group related fields that share a common ontological context — for example, a "Birth Event" collection gathers all the fields related to birth: the date, the place, the participants. Inside a collection, you can override field titles and descriptions to match the context.

<!--key:pages.about.models_title-->
Models

<!--key:pages.about.models_text-->
A model represents a real-world entity type you want to describe — a Person, an Object, a Place, an Event. A model has a scope class and bundles collections and fields together with its own layer of overrides that can rename, constrain, or hide fields.

<!--key:pages.about.categories_title-->
Categories

<!--key:pages.about.categories_text-->
Categories are the organisational layer that makes your field library navigable. They group fields into human-readable sections — "Existence", "Description", "Names and Identifiers" — so users can find what they need. Categories carry no ontological meaning; they are how you organise fields for human consumption.

<!--key:pages.about.weave_title-->
The Weave

<!--key:pages.about.weave_text-->
The weave is the complete picture: all your models, their collections and fields, the relationships between models, and the overrides that make everything fit your specific use case. If a model describes one entity type, the weave describes how those entity types relate to each other.

<!--key:pages.about.overrides_title-->
The Power of Overrides

<!--key:pages.about.overrides_text-->
Overrides are the mechanism that makes composition and reuse work. You build one field and adapt it everywhere. The same field — same ontology path, same formal semantics — can appear as "Title" in an Object model, "Name" in a Person model, and "Label" in a Concept model. The interoperability is preserved at the ontology level. The human understanding is preserved at the override level.

<!--key:pages.about.overrides_layers-->
The override chain has three layers: base field (canonical definition), collection override (context-specific naming), and model override (further specialisation with cardinality constraints). The most specific override wins.

<!--key:pages.about.reuse_title-->
Composition and Reuse

<!--key:pages.about.reuse_text-->
Instead of building from scratch, you compose from a growing library of patterns that the community has already validated. Every time a field path is adopted by another project, the groove gets deeper. The most-adopted patterns float to the top, making reuse the path of least resistance.

<!--key:pages.about.generators_title-->
Generators

<!--key:pages.about.generators_text-->
Once you have a well-described weave, Pletka offers generators that produce reusable output formats: serializations (RDF/RDFS, JSON-LD), visualisations (Mermaid), mapping software files (X3ML), semantic data management platform files (ResearchSpace), and more. Change your weave, regenerate, and your implementation formats stay in sync.

<!--key:pages.about.explore_patterns-->
Explore Patterns

<!--key:pages.about.explore_ontologies-->
Explore Ontologies
