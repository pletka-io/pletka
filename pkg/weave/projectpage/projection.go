package projectpage

import (
	"net/url"

	"github.com/pletka-io/pletka/pkg/browserinput"
	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/formschema"
)

// projectOverview has no request, reader or handler capabilities.
func projectOverview(value browserinput.ProjectSummary, lang string, languages []formschema.LanguageInfo) (*formschema.ProjectOverviewSchema, error) {
	project, err := value.Header.Project()
	if err != nil {
		return nil, err
	}
	ontologies := make([]formschema.OntologyInfo, 0, len(value.Ontologies))
	for _, o := range value.Ontologies {
		source := o.SourceProjectID
		if source == project.ID {
			source = ""
		}
		href := "/projects/" + url.PathEscape(o.SourceProjectID)
		if o.SourceVersion != "" {
			href += "?version=" + url.QueryEscape(o.SourceVersion)
		}
		ontologies = append(ontologies, formschema.OntologyInfo{Name: o.Name, Version: o.Version, URI: o.URI, ClassCount: o.Classes, PropertyCount: o.Properties, ClassesUsed: o.ClassesUsed, PropertiesUsed: o.PropertiesUsed, UsageUnavailable: o.UsageUnavailable, Origin: domain.OriginFromProject(project.ID, source), SourceURL: href})
	}
	c := value.Counts
	return formschema.BuildProjectOverviewSchema(project, c.Models, c.Collections, c.Fields, c.Categories, ontologies, value.Credits, lang, languages), nil
}
