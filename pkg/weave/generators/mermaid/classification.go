package mermaid

import (
	"embed"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/pletka-io/pletka/pkg/domain"
	"gopkg.in/yaml.v3"
)

//go:embed classifications/default.yaml
var classificationFiles embed.FS

type ClassificationConfig struct {
	Version  string                `yaml:"version"`
	Groups   []ClassificationGroup `yaml:"groups"`
	Classes  []ClassClassification `yaml:"classes"`
	Metadata ClassificationMeta    `yaml:"metadata,omitempty"`
}

type ClassificationMeta struct {
	OntologyID      string `yaml:"ontology_id,omitempty"`
	OntologyPrefix  string `yaml:"ontology_prefix,omitempty"`
	OntologyVersion string `yaml:"ontology_version,omitempty"`
}

type ClassificationGroup struct {
	ID     string `yaml:"id"`
	Label  string `yaml:"label"`
	Fill   string `yaml:"fill"`
	Stroke string `yaml:"stroke"`
}

type ClassClassification struct {
	URI       string `yaml:"uri"`
	Qname     string `yaml:"qname,omitempty"`
	LocalName string `yaml:"local_name,omitempty"`
	Label     string `yaml:"label,omitempty"`
	Group     string `yaml:"group"`
}

type MissingClassification struct {
	URI       string `yaml:"uri"`
	Qname     string `yaml:"qname,omitempty"`
	LocalName string `yaml:"local_name,omitempty"`
	Label     string `yaml:"label,omitempty"`
	Reason    string `yaml:"reason"`
}

type MissingClassificationReport struct {
	OntologyID      string                  `yaml:"ontology_id,omitempty"`
	OntologyPrefix  string                  `yaml:"ontology_prefix,omitempty"`
	OntologyVersion string                  `yaml:"ontology_version,omitempty"`
	MissingClasses  []MissingClassification `yaml:"missing_classes"`
}

func NewClassificationConfig(ontology *domain.Ontology, version *domain.OntologyVersion, classes []*domain.OntologyClass, groupsByURI map[string]string) ClassificationConfig {
	out := ClassificationConfig{
		Version: "1",
		Groups:  DefaultClassificationGroups(),
		Metadata: ClassificationMeta{
			OntologyID:     ontologyID(ontology),
			OntologyPrefix: ontologyPrefix(ontology),
		},
	}
	if version != nil {
		out.Metadata.OntologyVersion = version.VersionString
	}

	sorted := append([]*domain.OntologyClass(nil), classes...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].URI < sorted[j].URI
	})

	out.Classes = make([]ClassClassification, 0, len(sorted))
	for _, class := range sorted {
		if class == nil {
			continue
		}
		out.Classes = append(out.Classes, ClassClassification{
			URI:       class.URI,
			Qname:     class.Qname,
			LocalName: class.LocalName,
			Label:     class.Label.Get("en"),
			Group:     groupsByURI[class.URI],
		})
	}
	return out
}

func DefaultClassificationGroups() []ClassificationGroup {
	cfg, err := DefaultClassificationConfig()
	if err != nil {
		return nil
	}
	return append([]ClassificationGroup(nil), cfg.Groups...)
}

func BuiltinClassificationGroup(uri string) string {
	cfg, err := DefaultClassificationConfig()
	if err != nil {
		return ""
	}
	return cfg.ClassGroupsByURI()[uri]
}

type Classifier struct {
	byURI        map[string]string
	byPrefixed   map[string]string
	byLocalName  map[string]string
	defaultGroup string
}

func NewClassifier(cfg ClassificationConfig) Classifier {
	c := Classifier{
		byURI:        make(map[string]string, len(cfg.Classes)),
		byPrefixed:   make(map[string]string, len(cfg.Classes)),
		byLocalName:  make(map[string]string, len(cfg.Classes)),
		defaultGroup: "Default",
	}
	for _, class := range cfg.Classes {
		if class.Group == "" {
			continue
		}
		if class.URI != "" {
			c.byURI[class.URI] = class.Group
		}
		if class.Qname != "" {
			c.byPrefixed[class.Qname] = class.Group
		}
		if class.LocalName != "" {
			c.byLocalName[class.LocalName] = class.Group
		}
	}
	return c
}

func DefaultClassifier() (Classifier, error) {
	cfg, err := DefaultClassificationConfig()
	if err != nil {
		return Classifier{}, err
	}
	return NewClassifier(cfg), nil
}

func (c Classifier) Group(element domain.PathElement) string {
	if group := c.byURI[element.URI]; group != "" {
		return group
	}
	if group := c.byPrefixed[element.PrefixedName()]; group != "" {
		return group
	}
	if group := c.byLocalName[element.LocalName]; group != "" {
		return group
	}
	if element.ClassCode != "" {
		if group := c.byLocalName[element.ClassCode]; group != "" {
			return group
		}
	}
	if c.defaultGroup == "" {
		return "Default"
	}
	return c.defaultGroup
}

func DefaultClassificationConfig() (ClassificationConfig, error) {
	data, err := classificationFiles.ReadFile("classifications/default.yaml")
	if err != nil {
		return ClassificationConfig{}, fmt.Errorf("read embedded mermaid classification config: %w", err)
	}
	var cfg ClassificationConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ClassificationConfig{}, fmt.Errorf("parse embedded mermaid classification config: %w", err)
	}
	return cfg, nil
}

func (c ClassificationConfig) ClassGroupsByURI() map[string]string {
	out := make(map[string]string, len(c.Classes))
	for _, class := range c.Classes {
		if class.URI == "" {
			continue
		}
		out[class.URI] = class.Group
	}
	return out
}

func MissingClassifications(ontology *domain.Ontology, version *domain.OntologyVersion, classes []*domain.OntologyClass, cfg ClassificationConfig) MissingClassificationReport {
	report := MissingClassificationReport{
		OntologyID:     ontologyID(ontology),
		OntologyPrefix: ontologyPrefix(ontology),
	}
	if version != nil {
		report.OntologyVersion = version.VersionString
	}

	byURI := make(map[string]ClassClassification, len(cfg.Classes))
	for _, class := range cfg.Classes {
		byURI[class.URI] = class
	}

	sorted := append([]*domain.OntologyClass(nil), classes...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].URI < sorted[j].URI
	})

	for _, class := range sorted {
		if class == nil {
			continue
		}
		configured, ok := byURI[class.URI]
		reason := ""
		switch {
		case !ok:
			reason = "missing"
		case configured.Group == "":
			reason = "empty_group"
		}
		if reason == "" {
			continue
		}
		report.MissingClasses = append(report.MissingClasses, MissingClassification{
			URI:       class.URI,
			Qname:     class.Qname,
			LocalName: class.LocalName,
			Label:     class.Label.Get("en"),
			Reason:    reason,
		})
	}
	return report
}

func ReadClassificationConfig(path string) (ClassificationConfig, error) {
	if path == "" {
		return DefaultClassificationConfig()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ClassificationConfig{}, fmt.Errorf("read mermaid classification config: %w", err)
	}
	var cfg ClassificationConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ClassificationConfig{}, fmt.Errorf("parse mermaid classification config: %w", err)
	}
	return cfg, nil
}

func WriteClassificationConfig(w io.Writer, cfg ClassificationConfig) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer enc.Close()
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("write mermaid classification config: %w", err)
	}
	return nil
}

func WriteMissingClassificationReport(w io.Writer, report MissingClassificationReport) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer enc.Close()
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("write missing mermaid classifications: %w", err)
	}
	return nil
}

func ontologyID(ontology *domain.Ontology) string {
	if ontology == nil {
		return ""
	}
	return ontology.ID
}

func ontologyPrefix(ontology *domain.Ontology) string {
	if ontology == nil {
		return ""
	}
	return ontology.Prefix
}
