// Package formatschema embeds the JSON Schemas of the project repository
// format (docs-oss/format) and validates tree files against them. It lives
// beside the schemas so the spec folder holds the only copy.
package formatschema

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"
)

//go:embed *.schema.json
var schemaFiles embed.FS

const baseURL = "https://pletka.io/schema/format/2/"

// File kinds, each named after its schema file.
const (
	KindProject         = "project"
	KindMod             = "pletka-mod"
	KindCategory        = "category"
	KindConceptList     = "concept-list"
	KindField           = "field"
	KindModel           = "model"
	KindCollection      = "collection"
	KindOverride        = "override"
	KindReceiptsIndex   = "receipts-index"
	KindAdoptionReceipt = "adoption-receipt"
	KindForkReceipt     = "fork-receipt"
	KindOntology        = "ontology"
	KindReleasesIndex   = "releases-index"
)

var allKinds = []string{
	KindProject, KindMod, KindCategory, KindConceptList, KindField, KindModel, KindCollection,
	KindOverride, KindReceiptsIndex, KindAdoptionReceipt, KindForkReceipt, KindOntology, KindReleasesIndex,
}

// KindForPath returns the file kind of rel, a slash-separated path relative
// to a module root, or false when rel is not a format file.
func KindForPath(rel string) (string, bool) {
	p := strings.Split(rel, "/")
	yamlFile := strings.HasSuffix(rel, ".yaml")
	switch {
	case rel == "project.yaml":
		return KindProject, true
	case rel == "pletka.mod":
		return KindMod, true
	case rel == "ontology.yaml":
		return KindOntology, true
	case rel == "releases/index.yaml":
		return KindReleasesIndex, true
	case len(p) == 2 && (p[0] == "adoptions" || p[0] == "forks") && p[1] == "index.yaml":
		return KindReceiptsIndex, true
	case len(p) == 2 && p[0] == "adoptions" && yamlFile:
		return KindAdoptionReceipt, true
	case len(p) == 2 && p[0] == "forks" && yamlFile:
		return KindForkReceipt, true
	case len(p) == 2 && p[0] == "categories" && yamlFile:
		return KindCategory, true
	case len(p) == 2 && p[0] == "concept-lists" && yamlFile:
		return KindConceptList, true
	case len(p) == 3 && p[0] == "fields" && p[2] == "field.yaml":
		return KindField, true
	case len(p) == 3 && p[0] == "fields" && p[2] == "base-override.yaml":
		return KindOverride, true
	case len(p) == 3 && p[0] == "models" && p[2] == "model.yaml":
		return KindModel, true
	case len(p) == 3 && p[0] == "collections" && p[2] == "collection.yaml":
		return KindCollection, true
	case len(p) == 4 && (p[0] == "models" || p[0] == "collections") && p[2] == "overrides" && yamlFile:
		return KindOverride, true
	}
	return "", false
}

// Violation is one schema rule a document breaks.
type Violation struct {
	Pointer string // JSON pointer into the document; "" is the root
	Keyword string // the failing schema keyword, e.g. "additionalProperties"
	Message string
}

// Validator holds the compiled schemas, one per file kind.
type Validator struct {
	schemas map[string]*jsonschema.Schema
}

// New compiles the embedded schemas.
func New() (*Validator, error) {
	c := jsonschema.NewCompiler()
	entries, err := schemaFiles.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read embedded schemas: %w", err)
	}
	for _, e := range entries {
		raw, err := schemaFiles.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		if err := c.AddResource(baseURL+e.Name(), doc); err != nil {
			return nil, fmt.Errorf("add %s: %w", e.Name(), err)
		}
	}
	v := &Validator{schemas: map[string]*jsonschema.Schema{}}
	for _, k := range allKinds {
		s, err := c.Compile(baseURL + k + ".schema.json")
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", k, err)
		}
		v.schemas[k] = s
	}
	return v, nil
}

var (
	defaultOnce      sync.Once
	defaultValidator *Validator
)

// Default returns a Validator over the embedded schemas, compiled once. It
// panics if they do not compile: static data, fail-fast (TestSchemasCompile
// guards it).
func Default() *Validator {
	defaultOnce.Do(func() {
		v, err := New()
		if err != nil {
			panic(fmt.Sprintf("formatschema: %v", err))
		}
		defaultValidator = v
	})
	return defaultValidator
}

var printer = message.NewPrinter(language.English)

// Validate checks payload, a YAML document of the given kind. The error is
// for an unknown kind or YAML that does not parse; schema problems are
// violations, at most one per document location.
func (v *Validator) Validate(fileKind string, payload []byte) ([]Violation, error) {
	sch, ok := v.schemas[fileKind]
	if !ok {
		return nil, fmt.Errorf("unknown file kind %q", fileKind)
	}
	var doc any
	if err := yaml.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("to json: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("to json: %w", err)
	}
	err = sch.Validate(inst)
	if err == nil {
		return nil, nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return nil, err
	}
	var out []Violation
	seen := map[string]bool{}
	collect(ve, &out, seen)
	return out, nil
}

// collect appends the leaves of e's cause tree, one per document location;
// an additionalProperties leaf becomes one violation per extra key.
func collect(e *jsonschema.ValidationError, out *[]Violation, seen map[string]bool) {
	if len(e.Causes) > 0 {
		for _, c := range e.Causes {
			collect(c, out, seen)
		}
		return
	}
	pointer := ""
	if len(e.InstanceLocation) > 0 {
		pointer = "/" + strings.Join(e.InstanceLocation, "/")
	}
	keywordPath := e.ErrorKind.KeywordPath()
	keyword := ""
	if len(keywordPath) > 0 {
		keyword = keywordPath[len(keywordPath)-1]
	}
	if ap, ok := e.ErrorKind.(*kind.AdditionalProperties); ok {
		for _, prop := range ap.Properties {
			p := pointer + "/" + prop
			if !seen[p] {
				seen[p] = true
				*out = append(*out, Violation{Pointer: p, Keyword: keyword, Message: fmt.Sprintf("%q is not a key of this file", prop)})
			}
		}
		return
	}
	if seen[pointer] {
		return
	}
	seen[pointer] = true
	*out = append(*out, Violation{Pointer: pointer, Keyword: keyword, Message: e.ErrorKind.LocalizedString(printer)})
}

// Line is the 1-based line of the node pointer names in payload, or of its
// nearest existing ancestor; 0 when payload does not parse.
func Line(payload []byte, pointer string) int {
	var root yaml.Node
	if err := yaml.Unmarshal(payload, &root); err != nil || len(root.Content) == 0 {
		return 0
	}
	n := root.Content[0]
	if pointer == "" {
		return n.Line
	}
	for _, tok := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		switch n.Kind {
		case yaml.MappingNode:
			var next *yaml.Node
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == tok {
					next = n.Content[i] // the key's line, where a reader looks
					if i+1 < len(n.Content) && n.Content[i+1].Kind != yaml.ScalarNode {
						next = n.Content[i+1]
					}
					break
				}
			}
			if next == nil {
				return n.Line
			}
			n = next
		case yaml.SequenceNode:
			i, err := strconv.Atoi(tok)
			if err != nil || i >= len(n.Content) {
				return n.Line
			}
			n = n.Content[i]
		default:
			return n.Line
		}
	}
	return n.Line
}
