<!-- pletka-pages-edit v1
slug: about
lang: nl
-->

# About (nl)

Edit prose between the `<!--key:...-->` markers. Do not rename, reorder, or remove the markers. Run `pletka pages hydrate` to write changes back to the language JSON.

<!--key:pages.about.title-->
Over Pletka

<!--key:pages.about.subtitle-->
Composeerbare semantische modellering voor cultureel erfgoed

<!--key:pages.about.problem_title-->
Het Probleem

<!--key:pages.about.problem_text-->
Cultureel-erfgoedinstellingen — musea, archieven, bibliotheken — moeten hun collecties op een gestructureerde, interoperabele manier beschrijven. Ze gebruiken ontologieën: formele vocabulaires die definiëren hoe dingen met elkaar samenhangen. Maar elke instelling heeft van oudsher haar datamodellen vanaf nul opgebouwd, met verschillende keuzes voor dezelfde concepten. Wanneer het tijd werd om data te delen of te integreren, betekenden de inconsistenties maanden aan reconciliatiewerk.

<!--key:pages.about.problem_text2-->
De drempel was hoog. De kosten van fouten waren onzichtbaar tot het moment van integratie. En de collectieve kennis over wat werkte, bleef opgesloten in individuele hoofden en institutionele wiki's.

<!--key:pages.about.building_blocks_title-->
Onze Bouwstenen

<!--key:pages.about.fields_title-->
Velden

<!--key:pages.about.fields_text-->
Een veld is de atomaire eenheid. Het is een ontologisch pad dat begint bij een scope class en eindigt waar een gebruiker daadwerkelijk inhoud invoert — een naam, een datum, een verwijzing naar een andere entiteit. Elk veld heeft een ontologiepad, een verwacht waardetype, en meertalige titels en beschrijvingen.

<!--key:pages.about.categories_title-->
Categorieën

<!--key:pages.about.categories_text-->
Categorieën zijn de organisatielaag die je veldenbibliotheek navigeerbaar maakt. Ze groeperen velden in leesbare secties — "Bestaan", "Beschrijving", "Namen en Identificaties" — zodat gebruikers kunnen vinden wat ze nodig hebben. Categorieën dragen geen ontologische betekenis; ze zijn hoe je velden organiseert voor menselijk gebruik.

<!--key:pages.about.collections_title-->
Collecties

<!--key:pages.about.collections_text-->
Collecties zijn semantische blokken van betekenis. Ze groeperen gerelateerde velden die een gemeenschappelijke ontologische context delen — bijvoorbeeld een "Geboortegebeurtenis"-collectie verzamelt alle velden gerelateerd aan geboorte: de datum, de plaats, de deelnemers. Binnen een collectie kun je veldtitels en -beschrijvingen overschrijven om bij de context te passen.

<!--key:pages.about.models_title-->
Modellen

<!--key:pages.about.models_text-->
Een model vertegenwoordigt een entiteitstype uit de werkelijkheid dat je wilt beschrijven — een Persoon, een Object, een Plaats, een Gebeurtenis. Een model heeft een scope class en bundelt collecties en velden samen met een eigen laag van overrides die velden kunnen hernoemen, beperken of verbergen.

<!--key:pages.about.weave_title-->
Het Weefsel

<!--key:pages.about.weave_text-->
Het weefsel is het complete plaatje: al je modellen, hun collecties en velden, de relaties tussen modellen, en de overrides die alles passend maken voor jouw specifieke use case. Als een model één entiteitstype beschrijft, dan beschrijft het weefsel hoe die entiteitstypes zich tot elkaar verhouden.

<!--key:pages.about.overrides_title-->
De Kracht van Overrides

<!--key:pages.about.overrides_text-->
Overrides zijn het mechanisme dat compositie en hergebruik mogelijk maakt. Je bouwt één veld en past het overal aan. Hetzelfde veld — hetzelfde ontologiepad, dezelfde formele semantiek — kan verschijnen als "Titel" in een Object-model, "Naam" in een Persoon-model, en "Label" in een Concept-model. De interoperabiliteit blijft behouden op ontologieniveau. Het menselijk begrip blijft behouden op override-niveau.

<!--key:pages.about.overrides_layers-->
De override-keten heeft drie lagen: basisveld (canonieke definitie), collectie-override (contextspecifieke naamgeving) en model-override (verdere specialisatie met cardinaliteitsbeperkingen). De meest specifieke override wint.

<!--key:pages.about.reuse_title-->
Compositie en Hergebruik

<!--key:pages.about.reuse_text-->
In plaats van vanaf nul te bouwen, componeer je uit een groeiende bibliotheek van patronen die de gemeenschap al heeft gevalideerd. Elke keer dat een veldpad wordt overgenomen door een ander project, wordt het spoor dieper. De meest overgenomen patronen stijgen naar boven, waardoor hergebruik de weg van de minste weerstand wordt.

<!--key:pages.about.generators_title-->
Generators

<!--key:pages.about.generators_text-->
Zodra je een goed beschreven weefsel hebt, biedt Pletka generators die herbruikbare uitvoerformaten produceren: RDF/RDFS, SHACL-validatieshapes, Linked Art JSON-LD, Arches-resourcemodellen, en meer. Wijzig je weefsel, genereer opnieuw, en je implementatieformaten blijven in sync.

<!--key:pages.about.read_zen-->
Lees de Zen van Pletka

<!--key:pages.about.explore_ontologies-->
Ontologieën Verkennen

