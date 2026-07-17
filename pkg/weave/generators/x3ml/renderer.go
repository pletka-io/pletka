// Package x3ml renders weave generator snapshots as 3M Editor X3ML
// mapping documents. Two forms are supported, matching the customer's
// existing X3ML pipeline:
//
//   - Form A (FormatX3ML, .a.x3ml): a single <mapping> per snapshot
//     whose <domain> is the root entity (model or collection); every
//     resolved field becomes one <link> carrying the full ontology
//     path from the domain class to the value.
//
//   - Form B (FormatX3MLB, .b.x3ml): the same path, split by collection.
//     The primary <mapping> on the model carries one <link> per collection
//     the model uses (path = the model→collection join, range = the
//     collection's scope class) plus full-path links for standalone
//     fields. One secondary <mapping> per collection placement, domain-
//     anchored on the collection's scope class, carries that collection's
//     fields. The primary link and its secondary mapping share a template
//     string — that string is the join.
//
// Behaviour preserved from the Python X3MLTransformer reference:
//
//   - Standard X3ML envelope: <x3ml editor=... source_type="xpath"
//     xsi:noNamespaceSchemaLocation="x3ml_v1.5.xsd"> with <info>,
//     <namespaces>, <mappings>.
//   - Each mapping has <domain template="..."> with a UUID instance
//     generator on the entity type. Templates carry the documented
//     pattern name: semantic id joined to system name, dot kept
//     (LAC.1_name, LAF.6_name_content) — 3M renders them as the
//     mapping's human label.
//   - Each link has <path><source_relation><relation/> +
//     <target_relation>(<relationship>+<entity>)*</target_relation> +
//     <range><target_node><entity>...</target_node></range>.
//   - Literal terminals get a Literal instance generator with text()
//     xpath + en language constant.
//   - Class terminals/intermediates get UUID instance generators and a
//     `variable=` attribute. Intermediate nodes are *shared*: the
//     variable is keyed off the mapping root plus the ordered path
//     prefix that reaches the node, so every field passing through the
//     same node (e.g. one appellation carrying content + type +
//     language) coreferences it. The terminal node is per-field, keyed
//     off the field template. Class-local-name variables (the previous
//     scheme) collided whenever a class recurred along a path and lost
//     the connection back to the pattern.
//
// What we drop on purpose vs. the Python version:
//
//   - The MySQL scraper-definition table lookup. Source-info now reads
//     from the snapshot project alone; downstream tooling can wrap.
//   - Hardcoded URI substrings (xsd:date / rdf:literal). The renderer
//     resolves the literal terminal element directly from the field's
//     PathElements, so any literal type the snapshot carries works.
package x3ml

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/views"
)

const (
	editorAttribute    = "3MEditor v3.3"
	x3mlVersionAttr    = "1.0"
	sourceTypeXPath    = "xpath"
	schemaLocation     = "x3ml_v1.5.xsd"
	xsiNamespace       = "http://www.w3.org/2001/XMLSchema-instance"
	literalLangDefault = "en"
	crmPrefix          = "crm"
	crmNamespaceURI    = "http://www.cidoc-crm.org/cidoc-crm/"
)

// Renderer writes X3ML mapping documents from a weave snapshot.
type Renderer struct {
	form Form
}

// Form selects between the two X3ML output flavours.
type Form string

const (
	// FormA emits one mapping with full-path links per field.
	FormA Form = "a"
	// FormB emits a primary mapping with one link per collection the
	// model uses, plus one secondary mapping per collection placement
	// carrying that collection's fields.
	FormB Form = "b"
)

// NewRendererA constructs the form-A X3ML renderer.
func NewRendererA() *Renderer { return &Renderer{form: FormA} }

// NewRendererB constructs the form-B X3ML renderer.
func NewRendererB() *Renderer { return &Renderer{form: FormB} }

// Spec implements generators.Renderer.
func (r *Renderer) Spec() generators.FormatSpec {
	if r.form == FormB {
		return generators.FormatSpec{
			Format:        generators.FormatX3MLB,
			ContentType:   "application/xml; charset=utf-8",
			FileExtension: ".b.x3ml",
			RequiresTree:  true,
		}
	}
	return generators.FormatSpec{
		Format:        generators.FormatX3ML,
		ContentType:   "application/xml; charset=utf-8",
		FileExtension: ".a.x3ml",
		RequiresTree:  false,
	}
}

// Render implements generators.Renderer.
func (r *Renderer) Render(ctx context.Context, snap *generators.Snapshot, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap == nil {
		return fmt.Errorf("x3ml renderer: snapshot is nil")
	}
	if len(snap.Report.Errors) > 0 {
		return fmt.Errorf("x3ml renderer: snapshot has %d error(s)", len(snap.Report.Errors))
	}

	doc := buildDocument(snap, r.form)
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Flush(); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	return nil
}

// ----------------------------------------------------------------------------
// Document model
// ----------------------------------------------------------------------------

type x3mlDoc struct {
	XMLName        xml.Name    `xml:"x3ml"`
	XSINamespace   string      `xml:"xmlns:xsi,attr"`
	Editor         string      `xml:"editor,attr"`
	SourceType     string      `xml:"source_type,attr"`
	Version        string      `xml:"version,attr"`
	SchemaLocation string      `xml:"xsi:noNamespaceSchemaLocation,attr"`
	Info           *infoBlock  `xml:"info,omitempty"`
	Namespaces     *namespaces `xml:"namespaces,omitempty"`
	Mappings       mappings    `xml:"mappings"`
}

// infoBlock mirrors the full <info> skeleton 3M's importer expects.
// Every child element must be present even when empty, otherwise 3M
// rejects the document on load. The empty sub-elements
// are modelled as struct{} fields so they always serialise.
type infoBlock struct {
	Title              string           `xml:"title"`
	GeneralDescription string           `xml:"general_description"`
	Source             sourceBlock      `xml:"source"`
	Target             targetBlock      `xml:"target"`
	MappingInfo        mappingInfoBlock `xml:"mapping_info"`
	ExampleDataInfo    exampleDataBlock `xml:"example_data_info"`
}

type sourceBlock struct {
	SourceInfo       sourceInfoBlock `xml:"source_info"`
	SourceCollection struct{}        `xml:"source_collection"`
}

// sourceInfoBlock carries the <namespaces> block 3M's importer requires
// even when empty — omitting it rejects the document on load.
type sourceInfoBlock struct {
	SourceSchema schemaRef   `xml:"source_schema"`
	Namespaces   *namespaces `xml:"namespaces"`
}

// schemaRef is a <source_schema>/<target_schema> entry: an optional
// schema_file plus type + version attributes, with the schema label as
// element text. schema_file names the RDFS file shipped in the ZIP
// bundle and is omitted when no raw schema file exists.
type schemaRef struct {
	SchemaFile string `xml:"schema_file,attr,omitempty"`
	Type       string `xml:"type,attr"`
	Version    string `xml:"version,attr"`
	Label      string `xml:",chardata"`
}

// targetBlock is the single <target> container: 3M expects every
// ontology as a <target_info> child of one <target>, not one <target>
// per ontology.
type targetBlock struct {
	Infos            []targetInfoBlock `xml:"target_info"`
	TargetCollection struct{}          `xml:"target_collection"`
}

type targetInfoBlock struct {
	TargetSchema schemaRef   `xml:"target_schema"`
	Namespaces   *namespaces `xml:"namespaces,omitempty"`
}

type mappingInfoBlock struct {
	CreatedByOrg    struct{} `xml:"mapping_created_by_org"`
	CreatedByPerson struct{} `xml:"mapping_created_by_person"`
	InCollaboration struct{} `xml:"in_collaboration_with"`
}

type exampleDataBlock struct {
	From          struct{} `xml:"example_data_from"`
	ContactPerson struct{} `xml:"example_data_contact_person"`
	SourceRecord  struct{} `xml:"example_data_source_record"`
	GeneratorInfo struct{} `xml:"generator_policy_info"`
	TargetRecord  struct{} `xml:"example_data_target_record"`
	ThesaurusInfo struct{} `xml:"thesaurus_info"`
}

type namespaces struct {
	Entries []nsEntry `xml:"namespace"`
}

type nsEntry struct {
	Prefix string `xml:"prefix,attr"`
	URI    string `xml:"uri,attr"`
}

type mappings struct {
	Items []mappingNode `xml:"mapping"`
}

type mappingNode struct {
	Domain domainNode `xml:"domain"`
	Links  []linkNode `xml:"link"`
}

type domainNode struct {
	Template   string     `xml:"template,attr,omitempty"`
	SourceNode string     `xml:"source_node"`
	TargetNode targetNode `xml:"target_node"`
}

type linkNode struct {
	Template string    `xml:"template,attr,omitempty"`
	Path     pathNode  `xml:"path"`
	Range    rangeNode `xml:"range"`
}

type pathNode struct {
	SourceRelation sourceRelation `xml:"source_relation"`
	TargetRelation targetRelation `xml:"target_relation"`
}

type sourceRelation struct {
	Relation struct{} `xml:"relation"`
}

type targetRelation struct {
	Steps []targetStep `xml:",any"`
}

// targetStep is one element of the alternating relationship/entity sequence
// inside <target_relation>. We render via the encoding/xml `,any` slot so
// the order is preserved across mixed element names.
type targetStep struct {
	XMLName xml.Name
	Text    string `xml:",chardata"`
	// Variable applies to <entity variable="…"> rows.
	Variable string `xml:"variable,attr,omitempty"`
	// Name applies to <instance_generator name="UUID"> rows. 3M's
	// X3ML schema rejects `variable` there.
	Name     string       `xml:"name,attr,omitempty"`
	Children []targetStep `xml:",any"`
}

type rangeNode struct {
	SourceNode string     `xml:"source_node"`
	TargetNode targetNode `xml:"target_node"`
}

type targetNode struct {
	Entity entityNode `xml:"entity"`
}

type entityNode struct {
	Variable          string            `xml:"variable,attr,omitempty"`
	Type              string            `xml:"type"`
	InstanceGenerator instanceGenerator `xml:"instance_generator"`
}

type instanceGenerator struct {
	Name string           `xml:"name,attr"`
	Args []instanceGenArg `xml:"arg,omitempty"`
}

type instanceGenArg struct {
	Name    string `xml:"name,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// ----------------------------------------------------------------------------
// Build pipeline
// ----------------------------------------------------------------------------

func buildDocument(snap *generators.Snapshot, form Form) *x3mlDoc {
	doc := &x3mlDoc{
		XSINamespace:   xsiNamespace,
		Editor:         editorAttribute,
		SourceType:     sourceTypeXPath,
		Version:        x3mlVersionAttr,
		SchemaLocation: schemaLocation,
		Info:           buildInfo(snap),
		Namespaces:     buildNamespaces(snap),
		Mappings:       mappings{Items: buildMappings(snap, form)},
	}
	return doc
}

// buildInfo emits the full <info> skeleton 3M requires.
// Title is "<SemanticID> - <Name>"; the general description carries the
// entity's English description.
func buildInfo(snap *generators.Snapshot) *infoBlock {
	semID, name, desc := rootIdentity(snap)
	title := name
	if semID != "" {
		title = semID + " - " + name
	}
	return &infoBlock{
		Title:              title,
		GeneralDescription: desc,
		Source: sourceBlock{
			SourceInfo: sourceInfoBlock{
				// 3M requires the <namespaces> block even when empty.
				Namespaces: &namespaces{Entries: []nsEntry{{}}},
			},
		},
		Target: buildTarget(snap),
	}
}

// buildTarget emits the single <target> container: one <target_info>
// child per linked ontology, then one <target_collection/>. 3M rejects
// a document with multiple <target> blocks. With no
// ontologies it falls back to a single CIDOC-CRM target_info.
func buildTarget(snap *generators.Snapshot) targetBlock {
	infos := make([]targetInfoBlock, 0, len(snap.Options.X3MLTargets))
	for _, t := range snap.Options.X3MLTargets {
		// TODO: ontologies authored in Pletka carry no raw
		// schema file. Once we can generate — and re-version — an RDFS
		// serialisation for a Pletka-edited ontology, set SchemaFile here
		// and add the generated file to the ZIP bundle.
		infos = append(infos, targetInfoBlock{
			TargetSchema: schemaRef{
				SchemaFile: t.SchemaFile,
				Type:       "rdfs",
				Version:    t.Version,
				Label:      t.Label,
			},
			Namespaces: &namespaces{Entries: []nsEntry{
				{Prefix: t.Prefix, URI: t.Namespace},
			}},
		})
	}
	if len(infos) == 0 {
		infos = append(infos, targetInfoBlock{
			TargetSchema: schemaRef{Type: "rdfs", Version: "7.1.1", Label: "CIDOC CRM"},
			Namespaces: &namespaces{Entries: []nsEntry{
				{Prefix: crmPrefix, URI: crmNamespaceURI},
			}},
		})
	}
	return targetBlock{Infos: infos}
}

// rootIdentity returns the semantic id, English display name and English
// description of the entity the document is generated for — the model,
// collection or field, not the project.
func rootIdentity(snap *generators.Snapshot) (semID, name, description string) {
	const lang = "en"
	switch snap.RootKind {
	case generators.EntityModel:
		if snap.Model != nil {
			return snap.Model.SemanticID,
				snap.Model.UIName.Get(lang, snap.Model.SystemName),
				snap.Model.Description.Get(lang, "")
		}
	case generators.EntityCollection:
		if snap.Collection != nil {
			return snap.Collection.SemanticID,
				snap.Collection.UIName.Get(lang, snap.Collection.SystemName),
				snap.Collection.Description.Get(lang, "")
		}
	case generators.EntityField:
		if snap.Field != nil {
			return snap.Field.SemanticID,
				snap.Field.UIName.Get(lang, snap.Field.SystemName),
				snap.Field.Description.Get(lang, "")
		}
	}
	return "",
		snap.Project.UIName.Get(lang, snap.Project.SystemName),
		snap.Project.Description.Get(lang, "")
}

func buildNamespaces(snap *generators.Snapshot) *namespaces {
	used := collectUsedPrefixes(snap)
	prefixes := make([]string, 0, len(used))
	for p := range used {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	out := &namespaces{}
	for _, p := range prefixes {
		uri, ok := snap.Namespaces.ResolvePrefix(p)
		if !ok {
			continue
		}
		out.Entries = append(out.Entries, nsEntry{Prefix: p, URI: uri})
	}
	return out
}

func collectUsedPrefixes(snap *generators.Snapshot) map[string]struct{} {
	used := map[string]struct{}{}
	if snap.Tree != nil && snap.Tree.Root.Scope != nil && snap.Tree.Root.Scope.Prefix != "" {
		used[snap.Tree.Root.Scope.Prefix] = struct{}{}
	}
	for _, field := range snap.Fields {
		if field.Scope.Prefix != "" {
			used[field.Scope.Prefix] = struct{}{}
		}
		paths := append([][]generators.PathNode{field.Path}, field.SubPaths...)
		for _, p := range paths {
			for _, node := range p {
				if node.Element.Prefix != "" {
					used[node.Element.Prefix] = struct{}{}
				}
			}
		}
	}
	return used
}

// ----------------------------------------------------------------------------
// Mappings
// ----------------------------------------------------------------------------

func buildMappings(snap *generators.Snapshot, form Form) []mappingNode {
	if len(snap.Fields) == 0 {
		return nil
	}
	domainElem, domainTemplate := rootDomainElement(snap)
	if form == FormA {
		return []mappingNode{{
			Domain: buildDomain(domainElem, domainTemplate, ""),
			Links:  buildFormALinks(snap, domainTemplate),
		}}
	}
	return buildFormBMappings(snap, domainElem, domainTemplate)
}

// rootDomainElement returns the entity that anchors the primary
// mapping's <domain>. Model + collection roots use the root scope;
// field roots fall back to the field's own scope.
func rootDomainElement(snap *generators.Snapshot) (domain.PathElement, string) {
	switch snap.RootKind {
	case generators.EntityModel:
		if snap.Model != nil {
			return snap.Model.OntologyScope, templateFor(snap.Model.SemanticID, snap.Model.SystemName, snap.Model.ID)
		}
	case generators.EntityCollection:
		if snap.Collection != nil {
			return snap.Collection.OntologyScope, templateFor(snap.Collection.SemanticID, snap.Collection.SystemName, snap.Collection.ID)
		}
	case generators.EntityField:
		if snap.Field != nil {
			return snap.Field.OntologyScope, templateFor(snap.Field.SemanticID, snap.Field.SystemName, snap.Field.ID)
		}
	}
	if len(snap.Fields) > 0 {
		return snap.Fields[0].Scope, ""
	}
	return domain.PathElement{}, ""
}

// templateFor builds an X3ML template attribute — the semantic id joined
// to the system name (LAF.6_name_content). The dot is kept on purpose:
// 3M renders the template as the mapping's human-readable label and the
// customer's existing exports use the dotted form.
func templateFor(semanticID, systemName, fallbackID string) string {
	switch {
	case semanticID != "" && systemName != "":
		return semanticID + "_" + systemName
	case semanticID != "":
		return semanticID
	case systemName != "":
		return systemName
	default:
		return fallbackID
	}
}

// fieldTemplate is the X3ML template attribute for a resolved field.
func fieldTemplate(field generators.FieldNode) string {
	return templateFor(field.Field.SemanticID, field.Field.SystemName, field.Field.ID)
}

// slugifyVar normalises a string into an X3ML variable token: lowercased,
// with dots, colons, slashes, hashes and spaces collapsed to underscores.
// Variables are internal coreference keys, never shown to users, so they
// may be freely flattened (unlike templates, which keep their dots).
func slugifyVar(s string) string {
	r := strings.NewReplacer(".", "_", ":", "_", "/", "_", "#", "_", " ", "_")
	return strings.ToLower(r.Replace(strings.TrimSpace(s)))
}

func buildDomain(elem domain.PathElement, template, variable string) domainNode {
	return domainNode{
		Template:   template,
		SourceNode: "/",
		TargetNode: targetNode{Entity: classEntity(elem, variable)},
	}
}

func classEntity(elem domain.PathElement, variable string) entityNode {
	return entityNode{
		Variable:          variable,
		Type:              elementQName(elem),
		InstanceGenerator: instanceGenerator{Name: "UUID"},
	}
}

func literalEntity(elem domain.PathElement) entityNode {
	return entityNode{
		Type: literalTypeURI(elem),
		InstanceGenerator: instanceGenerator{
			Name: "Literal",
			Args: []instanceGenArg{
				{Name: "text", Type: "xpath", Content: "text()"},
				{Name: "language", Type: "constant", Content: literalLangDefault},
			},
		},
	}
}

func elementQName(elem domain.PathElement) string {
	if elem.URI != "" {
		return elem.URI
	}
	if elem.Prefix != "" && elem.LocalName != "" {
		return elem.Prefix + ":" + elem.LocalName
	}
	return elem.LocalName
}

// literalTypeURI maps the literal element to an XSD datatype, falling
// back to xsd:string for the generic rdf:literal case.
func literalTypeURI(elem domain.PathElement) string {
	switch strings.ToLower(elementQName(elem)) {
	case "xsd:date", "xsd:datetime", "xsd:time":
		return "http://www.w3.org/2001/XMLSchema#dateTime"
	case "rdf:literal", "rdf:langstring":
		return "http://www.w3.org/2001/XMLSchema#string"
	}
	if elem.URI != "" {
		return elem.URI
	}
	return "http://www.w3.org/2001/XMLSchema#string"
}

// ----------------------------------------------------------------------------
// Form A: every field is a full-path link in a single mapping.
// ----------------------------------------------------------------------------

func buildFormALinks(snap *generators.Snapshot, rootIdent string) []linkNode {
	links := make([]linkNode, 0, len(snap.Fields))
	for _, field := range snap.Fields {
		// Primary path link, then one link per legacy subfield path. Each is a
		// full-path link sharing the field's domain; the subfield template gets
		// a "_pN" suffix so its terminal node is distinct, not coreferenced
		// with the primary's.
		if link, ok := buildLinkFromPath(fullPathSteps(field.Path), fieldTemplate(field), rootIdent); ok {
			links = append(links, link)
		}
		for i, sub := range field.SubPaths {
			tmpl := fmt.Sprintf("%s_p%d", fieldTemplate(field), i+1)
			if link, ok := buildLinkFromPath(fullPathSteps(sub), tmpl, rootIdent); ok {
				links = append(links, link)
			}
		}
	}
	return links
}

// fullPathSteps returns the property/class steps that follow the
// scope. The scope itself anchors the domain so it does not appear
// inside the link's <path>.
func fullPathSteps(pathNodes []generators.PathNode) []generators.PathNode {
	if len(pathNodes) == 0 {
		return nil
	}
	if pathNodes[0].Role == generators.PathRoleScope {
		return pathNodes[1:]
	}
	return pathNodes
}

// buildLinkFromPath turns a sequence of path nodes (alternating
// property → class/literal → property → …) into a <link> element. The
// last element drops to the <range>; intermediate properties + classes
// stack inside <target_relation>.
//
// Variable assignment: intermediate class nodes are
// *shared* — keyed by the ordered path prefix that reaches them — so
// every field passing through, e.g., the same appellation node
// coreferences it. The terminal class node is per-field, keyed off the
// field template. rootIdent scopes the shared keys to this mapping.
func buildLinkFromPath(steps []generators.PathNode, template, rootIdent string) (linkNode, bool) {
	if len(steps) == 0 {
		return linkNode{}, false
	}
	link := linkNode{
		Template: template,
		Path: pathNode{
			SourceRelation: sourceRelation{},
		},
	}
	terminal := steps[len(steps)-1]
	mid := steps[:len(steps)-1]

	// A class node's variable is its generated path_node_id — the
	// structural node identity the snapshot stamps onto every path
	// element. Nodes reached through the same prefix
	// share a path_node_id, so coreference falls out. The terminalVar /
	// prefixVar scheme below is the fallback for elements that carry no
	// path_node_id (legacy/degenerate snapshots).
	terminalVar := slugifyVar(template)
	prefixVar := func(upto int) string {
		parts := []string{rootIdent}
		for _, n := range steps[:upto+1] {
			parts = append(parts, elementQName(n.Element))
		}
		return slugifyVar(strings.Join(parts, "_"))
	}
	classVar := func(n generators.PathNode, fallback string) string {
		if n.Element.PathNodeID != "" {
			return n.Element.PathNodeID
		}
		return fallback
	}

	// Walk middle steps; properties → <relationship>, classes →
	// <entity variable=...>, literal → break (only valid at terminal).
	for i, node := range mid {
		switch node.Role {
		case generators.PathRoleProperty:
			link.Path.TargetRelation.Steps = append(link.Path.TargetRelation.Steps, targetStep{
				XMLName: xml.Name{Local: "relationship"},
				Text:    elementQName(node.Element),
			})
		case generators.PathRoleClass:
			link.Path.TargetRelation.Steps = append(link.Path.TargetRelation.Steps, targetStep{
				XMLName:  xml.Name{Local: "entity"},
				Variable: classVar(node, prefixVar(i)),
				Children: []targetStep{
					{XMLName: xml.Name{Local: "type"}, Text: elementQName(node.Element)},
					{XMLName: xml.Name{Local: "instance_generator"}, Name: "UUID"},
				},
			})
		case generators.PathRoleLiteral:
			// Literal mid-path is malformed; stop emitting and leave
			// terminal handling to render whatever the snapshot carries.
			break
		}
	}

	// Terminal: relationship + range entity. Property→class hops are
	// the common case; bare-literal terminals (final node literal with
	// preceding property in mid) are handled by treating the final mid
	// property as the range's anchor.
	switch terminal.Role {
	case generators.PathRoleProperty:
		// Trailing property with implicit literal range.
		link.Path.TargetRelation.Steps = append(link.Path.TargetRelation.Steps, targetStep{
			XMLName: xml.Name{Local: "relationship"},
			Text:    elementQName(terminal.Element),
		})
		link.Range = rangeNode{
			SourceNode: "",
			TargetNode: targetNode{Entity: literalEntity(domain.PathElement{Prefix: "rdf", LocalName: "literal"})},
		}
	case generators.PathRoleLiteral, generators.PathRoleTerminal:
		// Walk back to find the trailing property; emit it then the literal/class entity.
		// Convention here matches the Python pattern: the last property
		// has already been pushed in `mid` since terminal is a class or
		// literal. If terminal is the last element with a property
		// directly preceding it, mid carries the property already.
		if terminal.Element.Type == "literal" {
			link.Range = rangeNode{TargetNode: targetNode{Entity: literalEntity(terminal.Element)}}
		} else {
			link.Range = rangeNode{TargetNode: targetNode{Entity: classEntity(terminal.Element, classVar(terminal, terminalVar))}}
		}
	case generators.PathRoleClass:
		link.Range = rangeNode{TargetNode: targetNode{Entity: classEntity(terminal.Element, classVar(terminal, terminalVar))}}
	default:
		return linkNode{}, false
	}
	return link, true
}

// ----------------------------------------------------------------------------
// Form B: one primary mapping pointing at each collection placement, plus
// one secondary mapping per placement carrying that collection's fields.
// Standalone fields (not in any collection) are full-path links in the
// primary mapping.
// ----------------------------------------------------------------------------

func buildFormBMappings(snap *generators.Snapshot, rootElem domain.PathElement, rootTemplate string) []mappingNode {
	primary := mappingNode{Domain: buildDomain(rootElem, rootTemplate, "")}
	if snap.Tree == nil {
		return []mappingNode{primary}
	}

	fieldsByID := indexFieldsByID(snap.Fields)
	occurrence := map[string]int{}
	var secondaries []mappingNode
	// Standalone-field links are collected separately so they can be
	// appended after every collection link — the primary mapping reads
	// collections first, then the model's own direct fields.
	var standaloneLinks []linkNode

	for _, child := range snap.Tree.Root.Children {
		switch child.Kind {
		case views.NodeField:
			// Standalone field — full-path link in the primary mapping.
			fn, ok := fieldsByID[child.ID]
			if !ok {
				continue
			}
			if link, ok := buildLinkFromPath(fullPathSteps(fn.Path), fieldTemplate(fn), rootTemplate); ok {
				standaloneLinks = append(standaloneLinks, link)
			}
			for i, sub := range fn.SubPaths {
				tmpl := fmt.Sprintf("%s_p%d", fieldTemplate(fn), i+1)
				if link, ok := buildLinkFromPath(fullPathSteps(sub), tmpl, rootTemplate); ok {
					standaloneLinks = append(standaloneLinks, link)
				}
			}
		case views.NodeCategory:
			for _, collNode := range child.Children {
				if collNode.Kind != views.NodeCollection {
					continue
				}
				occurrence[collNode.ID]++
				link, secondary, ok := buildCollectionMappings(collNode, occurrence[collNode.ID], fieldsByID, rootTemplate)
				if !ok {
					continue
				}
				primary.Links = append(primary.Links, link)
				if secondary != nil {
					secondaries = append(secondaries, *secondary)
				}
			}
		}
	}
	primary.Links = append(primary.Links, standaloneLinks...)

	return append([]mappingNode{primary}, secondaries...)
}

// buildCollectionMappings produces the primary-mapping <link> that points
// at one collection placement plus the secondary <mapping> carrying that
// placement's fields. ok is false when the placement has no usable join
// path; secondary is nil when the placement contributes no field links.
func buildCollectionMappings(
	collNode views.TreeNode,
	occurrence int,
	fieldsByID map[string]generators.FieldNode,
	rootTemplate string,
) (linkNode, *mappingNode, bool) {
	prefix := pathElementsToNodes(collNode.PathPrefix)
	if len(prefix) == 0 {
		return linkNode{}, nil, false
	}
	template := collectionPlacementTemplate(collNode, occurrence)

	link, ok := buildLinkFromPath(prefix, template, rootTemplate)
	if !ok {
		return linkNode{}, nil, false
	}

	// Secondary domain = the collection's anchor class (collNode.Anchor)
	// — the class its fields converge on, NOT the declared Scope (a
	// generic placeholder). Both the <type> and the variable come from
	// it, coreferencing the primary link's range entity. Anchor is
	// non-nil here: the len(prefix)==0 guard above already returned.
	anchor := domain.PathElement{}
	if collNode.Anchor != nil {
		anchor = *collNode.Anchor
	}
	secondary := mappingNode{Domain: buildDomain(anchor, template, anchor.PathNodeID)}
	prefixLen := len(collNode.PathPrefix)
	for _, fieldNode := range collNode.Children {
		if fieldNode.Kind != views.NodeField {
			continue
		}
		fn, ok := fieldsByID[fieldNode.ID]
		if !ok {
			continue
		}
		// PathPrefix elements map one-for-one onto the leading steps of
		// fullPathSteps(fn.Path) — both derive from the same PathElements
		// sequence — so the collection-relative path is steps[prefixLen:].
		// ponytail: legacy subfield paths in collection context are not
		// emitted here (prefix alignment is undefined); they surface via
		// Form A and standalone-field links. Upgrade if a collection-placed
		// multi-path field appears.
		steps := fullPathSteps(fn.Path)
		if len(steps) <= prefixLen {
			continue
		}
		if fieldLink, ok := buildLinkFromPath(steps[prefixLen:], fieldTemplate(fn), template); ok {
			secondary.Links = append(secondary.Links, fieldLink)
		}
	}
	if len(secondary.Links) == 0 {
		return link, nil, true
	}
	return link, &secondary, true
}

// collectionPlacementTemplate is the shared join string between a
// primary-mapping link and its secondary mapping: the collection's
// semantic id + system name + a per-model occurrence number, e.g.
// LAC.1_name_1. The occurrence number disambiguates a collection placed
// more than once in the same model.
func collectionPlacementTemplate(collNode views.TreeNode, occurrence int) string {
	base := templateFor(collNode.SemanticID, collNode.SystemName, collNode.ID)
	return fmt.Sprintf("%s_%d", base, occurrence)
}

// pathElementsToNodes adapts a raw ontology path prefix into the
// PathNode slice buildLinkFromPath consumes, deriving each step's role
// from the element type.
func pathElementsToNodes(elems []domain.PathElement) []generators.PathNode {
	out := make([]generators.PathNode, 0, len(elems))
	for i, e := range elems {
		role := generators.PathRoleClass
		switch e.Type {
		case "property":
			role = generators.PathRoleProperty
		case "literal":
			role = generators.PathRoleLiteral
		}
		out = append(out, generators.PathNode{Element: e, Index: i, Role: role})
	}
	return out
}

// indexFieldsByID keys the snapshot's flat field list by field id so the
// tree walk can recover each field's resolved ontology path. A field
// reused across collection placements appears once per placement in
// snap.Fields; last-wins is safe because every occurrence carries the
// same ontology Path. If per-placement paths ever diverge this must key
// by (fieldID, collectionID) instead.
func indexFieldsByID(fields []generators.FieldNode) map[string]generators.FieldNode {
	out := make(map[string]generators.FieldNode, len(fields))
	for _, f := range fields {
		out[f.Field.ID] = f
	}
	return out
}
