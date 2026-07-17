// Package researchspace renders a weave generator snapshot as the YAML
// configuration ResearchSpace consumes for field discovery / faceting.
//
// Output shape (matches Python ResearchSpaceTransformer):
//
//	prefix: ""
//	container: ""
//	namespaces:
//	  crm: http://www.cidoc-crm.org/cidoc-crm/
//	  …
//	fields:
//	  - id: system_name
//	    label: Display Name
//	    description: …
//	    datatype: xsd:string
//	    queries:
//	      - select: |
//	          PREFIX crm: <…>
//	          SELECT DISTINCT ?value WHERE { … }
//
// Each field's SELECT is built by the snapshot SPARQL renderer so the
// query stays consistent with the standalone /sparql output. ?subject
// type / property chain / OPTIONAL label / Set_Value filter all flow
// through unchanged.
package researchspace

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/pletka-io/pletka/pkg/weave/generators"
	"github.com/pletka-io/pletka/pkg/weave/generators/sparql"
)

// Renderer writes ResearchSpace YAML configs from a weave snapshot.
type Renderer struct {
	sparql *sparql.Renderer
}

// NewRenderer constructs the ResearchSpace renderer.
func NewRenderer() *Renderer {
	return &Renderer{sparql: sparql.NewRenderer()}
}

// Spec implements generators.Renderer.
func (r *Renderer) Spec() generators.FormatSpec {
	return generators.FormatSpec{
		Format:        generators.FormatResearchSpace,
		ContentType:   "application/yaml; charset=utf-8",
		FileExtension: ".yml",
		RequiresTree:  false,
	}
}

// Render implements generators.Renderer.
func (r *Renderer) Render(ctx context.Context, snap *generators.Snapshot, w io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap == nil {
		return fmt.Errorf("researchspace renderer: snapshot is nil")
	}
	if len(snap.Report.Errors) > 0 {
		return fmt.Errorf("researchspace renderer: snapshot has %d error(s)", len(snap.Report.Errors))
	}

	doc := rsDocument{
		Prefix:     "",
		Container:  "",
		Namespaces: buildNamespaces(snap),
	}

	for _, field := range snap.Fields {
		fd, err := r.fieldEntry(ctx, snap, field)
		if err != nil {
			return err
		}
		doc.Fields = append(doc.Fields, fd)
	}

	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer enc.Close()
	return enc.Encode(doc)
}

// rsDocument is the top-level YAML shape consumed by ResearchSpace.
type rsDocument struct {
	Prefix     string            `yaml:"prefix"`
	Container  string            `yaml:"container"`
	Namespaces map[string]string `yaml:"namespaces"`
	Fields     []fieldEntry      `yaml:"fields"`
}

type fieldEntry struct {
	ID          string  `yaml:"id"`
	Label       string  `yaml:"label"`
	Description string  `yaml:"description,omitempty"`
	Datatype    string  `yaml:"datatype"`
	Queries     []query `yaml:"queries"`
}

type query struct {
	Select literalString `yaml:"select"`
}

// literalString marshals into YAML using the literal block style ("|"),
// preserving newlines verbatim. Plain string would emit a single-line
// quoted scalar that swallows the SPARQL formatting.
type literalString string

func (s literalString) MarshalYAML() (any, error) {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Style: yaml.LiteralStyle,
		Value: string(s),
	}, nil
}

// fieldEntry builds one YAML entry for the resolved field, embedding a
// single-field SPARQL SELECT so consumers can run it directly.
func (r *Renderer) fieldEntry(ctx context.Context, snap *generators.Snapshot, field generators.FieldNode) (fieldEntry, error) {
	subSnap := *snap
	subSnap.Fields = []generators.FieldNode{field}
	subSnap.Tree = nil

	var buf bytes.Buffer
	if err := r.sparql.Render(ctx, &subSnap, &buf); err != nil {
		return fieldEntry{}, fmt.Errorf("researchspace: render sparql for %s: %w", field.Field.SystemName, err)
	}

	lang := snap.Options.Lang
	if lang == "" {
		lang = "en"
	}
	label := field.Field.DisplayName.Get(lang, field.Field.SystemName)
	description := strings.TrimSpace(field.Field.Description.Get(lang, ""))

	return fieldEntry{
		ID:          field.Field.SystemName,
		Label:       label,
		Description: description,
		Datatype:    datatypeFor(field.Field.ExpectedValueType),
		Queries:     []query{{Select: literalString(strings.TrimSpace(buf.String()))}},
	}, nil
}

// buildNamespaces flattens the snapshot's namespace bindings into a
// prefix → URI map. Only prefixes that actually appear in any field
// path are emitted, so the YAML stays focused.
func buildNamespaces(snap *generators.Snapshot) map[string]string {
	used := map[string]struct{}{}
	for _, field := range snap.Fields {
		if field.Scope.Prefix != "" {
			used[field.Scope.Prefix] = struct{}{}
		}
		for _, node := range field.Path {
			if node.Element.Prefix != "" {
				used[node.Element.Prefix] = struct{}{}
			}
		}
	}
	out := make(map[string]string, len(used))
	for prefix := range used {
		if namespace, ok := snap.Namespaces.ResolvePrefix(prefix); ok {
			out[prefix] = namespace
		}
	}
	return out
}

// datatypeFor maps the field's expected value type onto the XSD
// vocabulary ResearchSpace expects. Mirrors the Python mapping; URI /
// Reference / Concept / Collection collapse to xsd:anyURI; numeric
// fields land on xsd:integer; everything unknown falls back to
// xsd:anyURI so the consumer treats it as a resource handle.
func datatypeFor(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "reference model", "reference", "uri", "concept", "collection":
		return "xsd:anyURI"
	case "integer":
		return "xsd:integer"
	case "date":
		return "xsd:date"
	case "datetime":
		return "xsd:dateTime"
	case "string":
		return "xsd:string"
	case "boolean":
		return "xsd:boolean"
	case "decimal":
		return "xsd:decimal"
	}
	return "xsd:anyURI"
}
