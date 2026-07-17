# Expert Reference: CIDOC-CRM Cultural Heritage Ontology

**CIDOC-CRM version 7.1.3 (February 2024) is the current stable release**, corresponding to ISO 21127:2023, with 81 classes and 160 unique properties. The ecosystem spans Linked Art 1.0, LRMoo 1.0, EDM 5.2.8, and multiple CRM extensions.

**Load this document when:** working on ontology paths, field scope classes, CRM class/property mapping, ontology import, or Linked Art integration in Pletka.

---

## 1. CIDOC-CRM Core Structure

### Current version, namespace, and ISO status

The **official stable version is 7.1.3**, released February 2024. Draft versions 7.2.4, 7.3, and 7.3.1 exist but are not recommended for implementation. The ISO standard is **ISO 21127:2023**.

**Official namespace IRI:** `http://www.cidoc-crm.org/cidoc-crm/`

### Event-centric modelling philosophy

CRM's foundational insight is that the world consists of **persistent items (endurants)** whose states and relationships change through **events (perdurants)**. Events serve as the temporal/causal nexus connecting things, people, and ideas in specific spatiotemporal contexts.

**E5_Event** is the hub connecting **who** (P11 had participant, P14 carried out by), **what** (P12 occurred in the presence of, P16 used specific object), **where** (P7 took place at), **when** (P4 has time-span), **how** (P2 has type), and **why** (P17 was motivated by).

Direct property assertions between persistent items (e.g., "Object X P52 has current owner Person Y") are **shortcuts** of fuller event-mediated paths. They are ontologically inferior because they conflate temporally distinct states.

### The Open World Assumption

CRM explicitly adopts the **Open World Assumption (OWA)**: absence of a statement does not imply the statement is false. Key implications:

- Incomplete knowledge is the norm
- A property not used is either not applicable or unknown
- Adding new correct knowledge should never invalidate existing correct statements
- Contradictory factual data is permitted (historical data cannot be verified in an absolute sense)

### Entity hierarchy

The class hierarchy splits between **E2_Temporal_Entity** (perdurants) and **E77_Persistent_Item** (endurants), both under E1_CRM_Entity.

**Temporal entity branch (events and periods):**

```
E1_CRM_Entity
  E2_Temporal_Entity
    E3_Condition_State
    E4_Period
      E5_Event
        E7_Activity
          E8_Acquisition
          E9_Move
          E10_Transfer_of_Custody
          E11_Modification
            E12_Production (also IsA E63)
            E79_Part_Addition
            E80_Part_Removal
          E13_Attribute_Assignment
            E14_Condition_Assessment
            E15_Identifier_Assignment
            E16_Measurement
            E17_Type_Assignment
          E65_Creation (also IsA E63)
          E66_Formation (also IsA E63)
          E85_Joining
          E86_Leaving
          E87_Curation_Activity
          E83_Type_Creation (also IsA E65)
          E96_Purchase (IsA E8)
        E63_Beginning_of_Existence
          E12_Production, E65_Creation, E66_Formation
          E67_Birth
          E81_Transformation (also IsA E64)
        E64_End_of_Existence
          E6_Destruction (also IsA E5)
          E68_Dissolution
          E69_Death
          E81_Transformation (also IsA E63)
```

**Persistent item branch (things, actors, conceptual objects):**

```
E1_CRM_Entity
  E77_Persistent_Item
    E39_Actor
      E21_Person
      E74_Group
    E70_Thing
      E18_Physical_Thing (disjoint from E28)
        E19_Physical_Object
          E20_Biological_Object
          E22_Human-Made_Object (also IsA E24)
          E78_Curated_Holding
        E24_Physical_Human-Made_Thing (also IsA E71)
          E22_Human-Made_Object
          E25_Human-Made_Feature
        E26_Physical_Feature
          E25_Human-Made_Feature
          E27_Site
      E71_Human-Made_Thing
        E24_Physical_Human-Made_Thing
        E28_Conceptual_Object (disjoint from E18)
          E55_Type
            E56_Language, E57_Material, E58_Measurement_Unit
            E98_Currency, E99_Product_Type
          E89_Propositional_Object
            E29_Design_or_Procedure
            E73_Information_Object
              E31_Document, E32_Authority_Document
              E33_Linguistic_Object, E34_Inscription, E35_Title
          E90_Symbolic_Object
            E41_Appellation, E42_Identifier
            E73_Information_Object (also IsA E89)
            E94_Space_Primitive, E95_Spacetime_Primitive
          E36_Visual_Item, E37_Mark, E38_Image
          E30_Right
    E72_Legal_Object (superclass of E18 and E90)
```

**Key standalone classes:** E41_Appellation, E42_Identifier, E52_Time-Span, E53_Place, E54_Dimension, E55_Type.

**Key disjoint pairs:** E2_Temporal_Entity / E77_Persistent_Item; E18_Physical_Thing / E28_Conceptual_Object. CRM uses **poly-hierarchy** (multiple inheritance).

### Property naming conventions

Properties are numbered P1 through P198+. Each has a forward reading (domain to range) and an inverse marked with suffix "i". Every property specifies domain, range, quantifiers, scope note, and superproperty/subproperty relations.

CRM is a **property-centric ontology** where classes exist primarily to serve as domain and range of meaningful properties.

### Ontological correctness

**Three levels of validity:**

1. **Syntactic validity**: well-formed RDF
2. **Schema conformance**: triples use predicates declared in the CRM schema with consistent domain/range
3. **Semantic/ontological correctness**: the real-world situation matches the scope note

**The scope note test**: if a real-world situation does not match the scope note of a class or property, the modelling is incorrect regardless of passing syntactic or schema validation.

### CRM shortcuts vs full paths

| Shortcut | Full Path |
|---|---|
| P52 has current owner (E18 to E39) | E18 P24i changed ownership through E8_Acquisition P22 transferred title to E39 |
| P55 has current location (E19 to E53) | E19 P25i moved by E9_Move P26 moved to E53 |
| P50 has current keeper (E18 to E39) | E18 P30i custody transferred through E10_Transfer_of_Custody P29 custody received by E39 |

**Shortcuts are acceptable** when event details are unknown. **Full paths are needed** when temporal context, provenance chains, or multiple participants matter.

### CRM extensions

| Extension | Version | Scope |
|---|---|---|
| **CRMsci** | 2.0 | Scientific observation, measurements |
| **CRMarchaeo** | 2.0 | Archaeological excavation |
| **CRMdig** | 3.2.1 | Digital provenance |
| **CRMba** | 1.4 | Buildings archaeology |
| **CRMtex** | 2.0 | Inscriptions, manuscripts |
| **CRMgeo** | 1.2 | Spatiotemporal integration, GeoSPARQL |
| **LRMoo** | 1.0 | Bibliographic (replaces FRBRoo) |
| **CRMinf** | -- | Argumentation/belief |

---

## 2. Linked Art Application Profile

### Current version and adoption

**Linked Art Model 1.0 and API 1.0**. Built on **CIDOC-CRM 7.1.3**, **JSON-LD 1.1**, and **Getty Vocabularies** (AAT, ULAN, TGN).

Major adopters: Getty, Rijksmuseum, Louvre, Met, Smithsonian, MoMA, V&A, NGA, Yale, Europeana.

**Context URL:** `https://linked.art/ns/v1/linked-art.json`

The context transforms CRM: removes namespaces (`HumanMadeObject` not `crm:E22_Human-Made_Object`), strips class/property numbers, uses **CamelCase for classes** and **snake_case for properties**.

### Core record types (13 entity endpoints)

| JSON-LD type | CRM Class | API Endpoint |
|---|---|---|
| HumanMadeObject | crm:E22_Human-Made_Object | /physical_object/ |
| DigitalObject | dig:D1_Digital_Object | /digital_object/ |
| Person | crm:E21_Person | /person/ |
| Group | crm:E74_Group | /group/ |
| Place | crm:E53_Place | /place/ |
| Activity | crm:E7_Activity | /event/ |
| Set | la:Set | /set/ |
| Type (Concept) | crm:E55_Type | /concept/ |
| VisualItem | crm:E36_Visual_Item | /visual_work/ |
| LinguisticObject | crm:E33_Linguistic_Object | /textual_work/ |
| PropositionalObject | crm:E89_Propositional_Object | /abstract_work/ |

### Baseline patterns

Every entity MUST have: `@context`, `id` (URI), `type` (exactly one). Every entity SHOULD have `_label`.

**Name pattern:** `identified_by` with `Name` type (crm:E33_E41_Linguistic_Appellation). Properties: `content`, `classified_as`, `language`, `part`.

**Identifier pattern:** `identified_by` with `Identifier` type (crm:E42_Identifier). Properties: `content`, `classified_as`.

**Classification pattern:** `classified_as` references E55_Type instances, typically Getty AAT URIs.

**Time-Span pattern:** Four timestamp properties for uncertainty: `begin_of_the_begin`, `end_of_the_begin`, `begin_of_the_end`, `end_of_the_end`.

### Common Linked Art modelling mistakes

- Confusing physical objects (HumanMadeObject) with visual content (VisualItem) or digital surrogates (DigitalObject)
- Using `produced_by` for digital objects (should be `created_by` with `Creation`)
- Directly linking artists to objects without the intermediate Production event
- Assigning multiple `type` values (Linked Art requires exactly one)
- Using raw CRM property names instead of JSON-LD keys
- Treating names as simple strings instead of structured `Name` objects within `identified_by`

---

## 3. LRMoo (Replaces FRBRoo)

**LRMoo version 1.0**, aligned with CIDOC CRM v7.1.3. 16 classes and 37 properties.

**Namespace:** `http://iflastandards.info/ns/lrm/lrmoo/`

### WEMI entities mapped to CRM

| IFLA LRM Entity | LRMoo Class | CRM Superclass |
|---|---|---|
| Work | F1_Work | E89_Propositional_Object |
| Expression | F2_Expression | E73_Information_Object |
| Manifestation | F3_Manifestation | E73_Information_Object |
| Item | F5_Item | E24_Physical_Human-Made_Thing |

Key properties: R3 is realised in (Work to Expression), R4 embodies (Manifestation to Expression), R7 exemplifies (Item to Manifestation).

---

## 4. Europeana Data Model (EDM)

**Current EDM Definition version: 5.2.8.** Namespace: `http://www.europeana.eu/schemas/edm/`

### Core pattern: Aggregation + ProvidedCHO + WebResource

**ore:Aggregation** groups a cultural heritage object with its digital representations. Mandatory: `edm:aggregatedCHO`, `edm:dataProvider`, `edm:provider`, `edm:rights`.

**edm:ProvidedCHO** is the real-world cultural heritage object. Carries Dublin Core descriptive properties plus `edm:type` (mandatory: TEXT, IMAGE, SOUND, VIDEO, or 3D).

**edm:WebResource** is the digital surrogate.

EDM is **CRM-inspired but not fully CRM-compliant**. EDM validation uses XML Schema + Schematron rules, not SHACL.

---

## 5. SHACL Validation for CRM-Based Profiles

### The OWA/CWA tension

CRM operates under the Open World Assumption; SHACL operates under the Closed World Assumption. SHACL cardinality constraints effectively impose application profile requirements, not ontological truths.

### Best practices

- Validate application profiles, not CRM itself
- Use open shapes (`sh:closed false`)
- Load CRM class hierarchy triples alongside shapes
- Reserve `sh:Violation` for hard profile requirements; use `sh:Warning` for soft expectations
- Use `sh:class` for range validation
- Document that cardinality constraints are profile requirements, not CRM constraints

---

## 6. Tools and Infrastructure

### Getty Vocabularies

| Vocabulary | Namespace | Use |
|---|---|---|
| AAT | `http://vocab.getty.edu/aat/` | E55_Type instances (classification) |
| TGN | `http://vocab.getty.edu/tgn/` | E53_Place instances |
| ULAN | `http://vocab.getty.edu/ulan/` | E39_Actor instances |

**SPARQL endpoint:** `http://vocab.getty.edu/sparql`

### Key AAT IRIs

- aat:300033618 "paintings"
- aat:300015045 "oil painting (technique)"
- aat:300404670 "preferred terms"
- aat:300312355 "accession number"
- aat:300435443 "type of work"
- aat:300055863 "provenance activity"
- aat:300435416 "description"
- aat:300055644 "height"
- aat:300379098 "centimeters"

### Pletka ontology tools

- Ontology import uses the weave ontology service; browser uploads and platform
  ops tools call the same parsing/persistence path.
- `frontend/src/lib/components/form/widgets/path-builder/` (RichPathBuilder.svelte, SimplePathBuilder.svelte) — Interactive CRM class/property path builders
- `POST /api/v1/ontology/autocomplete` — Autocomplete for CRM paths (project or version scoped)
- PathBuilder supports `scopeMode: true` (single class) and `scopeMode: false` (full path)

### External tools

- **X3ML Toolkit** (ICS-FORTH) — CRM mapping environment
- **ResearchSpace** (British Museum) — Virtual Research Environment on CRM
- **PeriodO** (https://perio.do/) — Linked Data gazetteer of scholarly period definitions
