package ontology

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pletka-io/pletka/pkg/domain"
)

type OntologyPageLinkModel struct {
	Label string
	Href  string
}

type FamilyLandingPageModel struct {
	Families        []FamilyPageCardModel
	FamilyCount     int
	OntologyCount   int
	RootFamilyCount int
}

type FamilyPageCardModel struct {
	ID            string
	Name          string
	Slug          string
	Description   string
	HomepageURL   string
	DisplayOrder  int64
	OntologyCount int64
	ChildCount    int
	URL           string
	AdminURL      string
}

type AllOntologiesPageModel struct {
	Ontologies []OntologyPageCardModel
}

type FamilyDetailPageModel struct {
	Family         *domain.OntologyFamily
	Name           string
	Description    string
	HomepageURL    string
	Breadcrumbs    []OntologyPageLinkModel
	ChildFamilies  []FamilyPageCardModel
	BaseOntologies []OntologyPageCardModel
	Extensions     []OntologyPageCardModel
	ActiveTab      string
}

type OntologyPageCardModel struct {
	ID            string
	Name          string
	Prefix        string
	Namespace     string
	Description   string
	FamilyName    string
	FamilyURL     string
	OntologyType  string
	ExtendsName   string
	ExtendsURL    string
	DetailURL     string
	AdminURL      string
	HomepageURL   string
	SourceURL     string
	IsExtension   bool
	VersionCount  int
	ActiveVersion string
}

type OntologyDetailPageModel struct {
	Ontology        *domain.Ontology
	Name            string
	Prefix          string
	Namespace       string
	Description     string
	OntologyType    string
	FamilyName      string
	FamilyURL       string
	FamilyAdminURL  string
	HomepageURL     string
	SourceURL       string
	Breadcrumbs     []OntologyPageLinkModel
	ExtendsOntology *OntologyPageCardModel
	Extensions      []OntologyPageCardModel
	Versions        []VersionPageCardModel
}

type VersionPageCardModel struct {
	ID                      string
	OntologyID              string
	VersionString           string
	IsActive                bool
	LifecycleState          string
	LifecycleTone           string
	UsageSummary            string
	CompatibilitySummary    string
	ImportSummary           string
	ClassCount              int64
	PropertyCount           int64
	ProjectCount            int64
	CompatibleBaseVersions  []string
	ImportedOntologies      []string
	OntologyURI             string
	VersionIRI              string
	VersionInfo             string
	OntologyComment         string
	OriginalFilename        string
	FileSize                int64
	FileMD5                 string
	ParsedAt                string
	CreatedAt               string
	UpdatedAt               string
	OntologyMetadataSummary string
	VersionURL              string
	AdminVersionURL         string
	AdminOntologyURL        string
	ClassesURL              string
	PropertiesURL           string
}

type VersionDetailPageModel struct {
	Ontology            *domain.Ontology
	VersionRow          *domain.OntologyVersion
	Name                string
	Prefix              string
	Namespace           string
	VersionString       string
	Breadcrumbs         []OntologyPageLinkModel
	ActiveTab           string
	ActiveVersionString string
	ProjectUsages       []VersionProjectUsageModel
	Version             VersionPageCardModel
}

type VersionProjectUsageModel struct {
	ProjectID   string
	ProjectName string
	ProjectURL  string
	AddedAt     string
	IsPrimary   bool
}

type ClassDetailPageModel struct {
	Ontology      *domain.Ontology
	Version       *domain.OntologyVersion
	Class         *domain.OntologyClass
	Prefix        string
	VersionString string
	Label         string
	Comment       string
	Breadcrumbs   []OntologyPageLinkModel
	SuperClasses  []OntologyRelationLinkModel
	SubClasses    []OntologyRelationLinkModel
	Properties    []OntologyRelationLinkModel
}

type PropertyDetailPageModel struct {
	Ontology        *domain.Ontology
	Version         *domain.OntologyVersion
	Property        *domain.OntologyProperty
	Prefix          string
	VersionString   string
	Label           string
	Comment         string
	Breadcrumbs     []OntologyPageLinkModel
	Domain          []OntologyRelationLinkModel
	Range           []OntologyRelationLinkModel
	SuperProperties []OntologyRelationLinkModel
	SubProperties   []OntologyRelationLinkModel
	Inverse         *OntologyRelationLinkModel
}

type OntologyRelationLinkModel struct {
	Qname string
	Label string
	Href  string
	Kind  string
}

func (s *Service) FamilyLandingPageModel(ctx context.Context, lang string) (*FamilyLandingPageModel, error) {
	allFamilies, err := s.ListFamilies(ctx)
	if err != nil {
		return nil, err
	}
	allOntologies, err := s.ListOntologies(ctx)
	if err != nil {
		return nil, err
	}
	childCounts := make(map[string]int)
	ontologyCounts := make(map[string]int64)
	for _, family := range allFamilies {
		if family.ParentFamilyID != nil && *family.ParentFamilyID != "" {
			childCounts[*family.ParentFamilyID]++
		}
	}
	for _, ontology := range allOntologies {
		if ontology.FamilyID != nil && *ontology.FamilyID != "" {
			ontologyCounts[*ontology.FamilyID]++
		}
	}
	cards := make([]FamilyPageCardModel, 0, len(allFamilies))
	for _, family := range allFamilies {
		if family.ParentFamilyID != nil && *family.ParentFamilyID != "" {
			continue
		}
		cards = append(cards, familyCardModel(family, localizedText(family.Description, lang), ontologyCounts[family.ID], childCounts[family.ID]))
	}
	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].DisplayOrder == cards[j].DisplayOrder {
			return cards[i].Name < cards[j].Name
		}
		return cards[i].DisplayOrder < cards[j].DisplayOrder
	})
	return &FamilyLandingPageModel{
		Families:        cards,
		FamilyCount:     len(allFamilies),
		OntologyCount:   len(allOntologies),
		RootFamilyCount: len(cards),
	}, nil
}

func (s *Service) AllOntologiesPageModel(ctx context.Context, lang string) (*AllOntologiesPageModel, error) {
	ontologies, err := s.ListOntologies(ctx)
	if err != nil {
		return nil, err
	}
	families, err := s.ListFamilies(ctx)
	if err != nil {
		return nil, err
	}
	familyByID := make(map[string]*domain.OntologyFamily, len(families))
	ontologyByID := make(map[string]*domain.Ontology, len(ontologies))
	for _, family := range families {
		familyByID[family.ID] = family
	}
	for _, ontology := range ontologies {
		ontologyByID[ontology.ID] = ontology
	}
	cards := make([]OntologyPageCardModel, 0, len(ontologies))
	for _, ontology := range ontologies {
		cards = append(cards, ontologyCardModel(ontology, familyByID, ontologyByID, lang))
	}
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].Prefix < cards[j].Prefix })
	return &AllOntologiesPageModel{Ontologies: cards}, nil
}

func (s *Service) FamilyDetailPageModel(ctx context.Context, slug, activeTab, lang string) (*FamilyDetailPageModel, error) {
	family, err := s.GetFamilyBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if activeTab = strings.TrimSpace(activeTab); activeTab == "" {
		activeTab = "overview"
	}
	ontologies, err := s.ListOntologiesByFamily(ctx, family.ID)
	if err != nil {
		return nil, err
	}
	childFamilies, err := s.ListChildFamilies(ctx, family.ID)
	if err != nil {
		return nil, err
	}
	allOntologies, err := s.ListOntologies(ctx)
	if err != nil {
		return nil, err
	}
	ontologyByID := make(map[string]*domain.Ontology, len(allOntologies))
	for _, ontology := range allOntologies {
		ontologyByID[ontology.ID] = ontology
	}
	var baseCards, extensionCards []OntologyPageCardModel
	for _, ontology := range ontologies {
		card := ontologyCardModel(ontology, nil, ontologyByID, lang)
		if ontology.IsExtension() {
			extensionCards = append(extensionCards, card)
		} else {
			baseCards = append(baseCards, card)
		}
	}
	sort.SliceStable(baseCards, func(i, j int) bool { return baseCards[i].Prefix < baseCards[j].Prefix })
	sort.SliceStable(extensionCards, func(i, j int) bool { return extensionCards[i].Prefix < extensionCards[j].Prefix })

	allFamilies, err := s.ListFamilies(ctx)
	if err != nil {
		return nil, err
	}
	// Per-child counts so child-family cards on a detail page match the
	// counts shown on the landing page (cards used to
	// hardcode 0/0). Mirrors FamilyLandingPageModel's aggregation.
	childOntologyCounts := make(map[string]int64)
	for _, ontology := range allOntologies {
		if ontology.FamilyID != nil && *ontology.FamilyID != "" {
			childOntologyCounts[*ontology.FamilyID]++
		}
	}
	childChildCounts := make(map[string]int)
	for _, fam := range allFamilies {
		if fam.ParentFamilyID != nil && *fam.ParentFamilyID != "" {
			childChildCounts[*fam.ParentFamilyID]++
		}
	}

	childCards := make([]FamilyPageCardModel, 0, len(childFamilies))
	for _, child := range childFamilies {
		childCards = append(childCards, familyCardModel(child, localizedText(child.Description, lang), childOntologyCounts[child.ID], childChildCounts[child.ID]))
	}
	sort.SliceStable(childCards, func(i, j int) bool { return childCards[i].Name < childCards[j].Name })

	return &FamilyDetailPageModel{
		Family:         family,
		Name:           family.Name,
		Description:    localizedText(family.Description, lang),
		HomepageURL:    family.HomepageURL,
		Breadcrumbs:    familyPageBreadcrumbs(ctx, s, family),
		ChildFamilies:  childCards,
		BaseOntologies: baseCards,
		Extensions:     extensionCards,
		ActiveTab:      activeTab,
	}, nil
}

func (s *Service) OntologyDetailPageModel(ctx context.Context, prefix, lang string) (*OntologyDetailPageModel, error) {
	ontology, err := s.GetOntologyByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	return s.ontologyDetailPageModelForOntology(ctx, ontology, lang)
}

func (s *Service) OntologyDetailPageModelByID(ctx context.Context, id, lang string) (*OntologyDetailPageModel, error) {
	ontology, err := s.GetOntology(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.ontologyDetailPageModelForOntology(ctx, ontology, lang)
}

func (s *Service) ontologyDetailPageModelForOntology(ctx context.Context, ontology *domain.Ontology, lang string) (*OntologyDetailPageModel, error) {
	var family *domain.OntologyFamily
	if ontology.FamilyID != nil && *ontology.FamilyID != "" {
		family, _ = s.GetFamily(ctx, *ontology.FamilyID)
	}
	var extendsOntology *domain.Ontology
	if ontology.ExtendsOntologyID != nil && *ontology.ExtendsOntologyID != "" {
		extendsOntology, _ = s.GetOntology(ctx, *ontology.ExtendsOntologyID)
	}
	extensions, err := s.ListOntologyExtensions(ctx, ontology.ID)
	if err != nil {
		return nil, err
	}
	versionRows, err := s.VersionListWithUsage(ctx, ontology.ID)
	if err != nil {
		return nil, err
	}
	extensionCards := make([]OntologyPageCardModel, 0, len(extensions))
	for _, ext := range extensions {
		extensionCards = append(extensionCards, ontologyCardModel(ext, nil, nil, lang))
	}
	sort.SliceStable(extensionCards, func(i, j int) bool { return extensionCards[i].Prefix < extensionCards[j].Prefix })

	versionCards := make([]VersionPageCardModel, 0, len(versionRows))
	for _, row := range versionRows {
		if row.Version == nil {
			continue
		}
		versionCards = append(versionCards, versionCardModel(ontology, row.Version, row.ProjectCount, lang))
	}
	sort.SliceStable(versionCards, func(i, j int) bool {
		if versionCards[i].IsActive != versionCards[j].IsActive {
			return versionCards[i].IsActive
		}
		return versionCards[i].VersionString > versionCards[j].VersionString
	})

	var extendsCard *OntologyPageCardModel
	if extendsOntology != nil {
		card := ontologyCardModel(extendsOntology, nil, nil, lang)
		extendsCard = &card
	}

	crumbs := []OntologyPageLinkModel{{Label: "Ontologies", Href: "/ontologies"}}
	if family != nil {
		crumbs = append(crumbs, familyPageBreadcrumbs(ctx, s, family)...)
		crumbs = append(crumbs, OntologyPageLinkModel{Label: family.Name, Href: "/ontologies/families/" + family.Slug})
	}
	return &OntologyDetailPageModel{
		Ontology:        ontology,
		Name:            ontology.Name,
		Prefix:          ontology.Prefix,
		Namespace:       ontology.Namespace,
		Description:     localizedText(ontology.Description, lang),
		OntologyType:    ontologyTypeLabel(ontology.OntologyType),
		FamilyName:      familyName(family),
		FamilyURL:       familyURL(family),
		FamilyAdminURL:  familyAdminURL(family),
		HomepageURL:     ontology.HomepageURL,
		SourceURL:       ontology.SourceURL,
		Breadcrumbs:     append(crumbs, OntologyPageLinkModel{Label: ontology.Prefix}),
		ExtendsOntology: extendsCard,
		Extensions:      extensionCards,
		Versions:        versionCards,
	}, nil
}

func (s *Service) VersionDetailPageModel(ctx context.Context, prefix, versionString, activeTab, lang string) (*VersionDetailPageModel, error) {
	ontology, err := s.GetOntologyByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	version, err := s.store.GetVersionByOntologyAndString(ctx, ontology.ID, versionString)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, ErrNotFound
	}
	projectCount, err := s.store.VersionUsageCount(ctx, version.ID)
	if err != nil {
		return nil, err
	}
	activeVersionString, err := s.activeVersionString(ctx, ontology.ID, version.ID)
	if err != nil {
		return nil, err
	}
	projectUsages, err := s.versionProjectUsages(ctx, version.ID)
	if err != nil {
		return nil, err
	}
	return versionDetailModel(ontology, version, activeTab, lang, projectCount, activeVersionString, projectUsages), nil
}

func (s *Service) VersionDetailPageModelByID(ctx context.Context, id, activeTab, lang string) (*VersionDetailPageModel, error) {
	version, err := s.GetVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	ontology, err := s.GetOntology(ctx, version.OntologyID)
	if err != nil {
		return nil, err
	}
	projectCount, err := s.store.VersionUsageCount(ctx, version.ID)
	if err != nil {
		return nil, err
	}
	activeVersionString, err := s.activeVersionString(ctx, ontology.ID, version.ID)
	if err != nil {
		return nil, err
	}
	projectUsages, err := s.versionProjectUsages(ctx, version.ID)
	if err != nil {
		return nil, err
	}
	return versionDetailModel(ontology, version, activeTab, lang, projectCount, activeVersionString, projectUsages), nil
}

func (s *Service) activeVersionString(ctx context.Context, ontologyID, currentVersionID string) (string, error) {
	active, err := s.store.GetActiveVersion(ctx, ontologyID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	if active == nil || active.ID == currentVersionID {
		return "", nil
	}
	return active.VersionString, nil
}

func (s *Service) versionProjectUsages(ctx context.Context, versionID string) ([]VersionProjectUsageModel, error) {
	usages, err := s.store.ListProjectsUsingVersion(ctx, versionID, 12)
	if err != nil {
		return nil, err
	}
	out := make([]VersionProjectUsageModel, 0, len(usages))
	for _, usage := range usages {
		addedAt := ""
		if !usage.AddedAt.IsZero() {
			addedAt = usage.AddedAt.Format("2006-01-02")
		}
		out = append(out, VersionProjectUsageModel{
			ProjectID:   usage.ProjectID,
			ProjectName: firstNonEmpty(usage.ProjectName, usage.ProjectID),
			ProjectURL:  "/projects/" + usage.ProjectID,
			AddedAt:     addedAt,
			IsPrimary:   usage.IsPrimary,
		})
	}
	return out, nil
}

func versionDetailModel(ontology *domain.Ontology, version *domain.OntologyVersion, activeTab, lang string, projectCount int64, activeVersionString string, projectUsages []VersionProjectUsageModel) *VersionDetailPageModel {
	if activeTab = strings.TrimSpace(activeTab); activeTab == "" {
		activeTab = "overview"
	}
	card := versionCardModel(ontology, version, projectCount, lang)
	return &VersionDetailPageModel{
		Ontology:            ontology,
		VersionRow:          version,
		Name:                ontology.Name,
		Prefix:              ontology.Prefix,
		Namespace:           ontology.Namespace,
		VersionString:       version.VersionString,
		ActiveTab:           activeTab,
		ActiveVersionString: activeVersionString,
		ProjectUsages:       projectUsages,
		Breadcrumbs: []OntologyPageLinkModel{
			{Label: "Ontologies", Href: "/ontologies"},
			{Label: ontology.Prefix, Href: "/ontologies/" + ontology.Prefix},
			{Label: version.VersionString},
		},
		Version: card,
	}
}

func (s *Service) ClassDetailPageModel(ctx context.Context, prefix, versionString, classID, lang string) (*ClassDetailPageModel, error) {
	ontology, version, err := s.resolveOntologyVersion(ctx, prefix, versionString)
	if err != nil {
		return nil, err
	}
	class, err := s.store.GetClass(ctx, classID)
	if err != nil {
		return nil, err
	}
	if class == nil || class.OntologyVersionID != version.ID {
		return nil, ErrNotFound
	}
	baseURL := "/ontologies/" + ontology.Prefix + "/" + version.VersionString
	properties, err := s.store.PropertiesForDomainQname(ctx, version.ID, class.Qname)
	if err != nil {
		return nil, err
	}
	return &ClassDetailPageModel{
		Ontology:      ontology,
		Version:       version,
		Class:         class,
		Prefix:        ontology.Prefix,
		VersionString: version.VersionString,
		Label:         localizedText(class.Label, lang),
		Comment:       localizedText(class.Comment, lang),
		Breadcrumbs: []OntologyPageLinkModel{
			{Label: "Ontologies", Href: "/ontologies"},
			{Label: ontology.Prefix, Href: "/ontologies/" + ontology.Prefix},
			{Label: version.VersionString, Href: baseURL},
			{Label: "Classes", Href: baseURL + "?tab=classes"},
			{Label: class.Qname},
		},
		SuperClasses: s.outgoingRelationLinks(ctx, version.ID, class.ID, "class", "subclass_of", "classes", baseURL),
		SubClasses:   s.incomingRelationLinks(ctx, version.ID, class.Qname, "subclass_of", "class", "classes", baseURL),
		Properties:   propertyRelationLinks(properties, baseURL),
	}, nil
}

func (s *Service) PropertyDetailPageModel(ctx context.Context, prefix, versionString, propertyID, lang string) (*PropertyDetailPageModel, error) {
	ontology, version, err := s.resolveOntologyVersion(ctx, prefix, versionString)
	if err != nil {
		return nil, err
	}
	property, err := s.store.GetProperty(ctx, propertyID)
	if err != nil {
		return nil, err
	}
	if property == nil || property.OntologyVersionID != version.ID {
		return nil, ErrNotFound
	}
	baseURL := "/ontologies/" + ontology.Prefix + "/" + version.VersionString
	inverse := s.outgoingRelationLinks(ctx, version.ID, property.ID, "property", "inverse_property", "properties", baseURL)
	model := &PropertyDetailPageModel{
		Ontology:      ontology,
		Version:       version,
		Property:      property,
		Prefix:        ontology.Prefix,
		VersionString: version.VersionString,
		Label:         localizedText(property.Label, lang),
		Comment:       localizedText(property.Comment, lang),
		Breadcrumbs: []OntologyPageLinkModel{
			{Label: "Ontologies", Href: "/ontologies"},
			{Label: ontology.Prefix, Href: "/ontologies/" + ontology.Prefix},
			{Label: version.VersionString, Href: baseURL},
			{Label: "Properties", Href: baseURL + "/properties"},
			{Label: property.Qname},
		},
		Domain:          s.outgoingRelationLinks(ctx, version.ID, property.ID, "property", "domain", "classes", baseURL),
		Range:           s.outgoingRelationLinks(ctx, version.ID, property.ID, "property", "range", "classes", baseURL),
		SuperProperties: s.outgoingRelationLinks(ctx, version.ID, property.ID, "property", "subproperty_of", "properties", baseURL),
		SubProperties:   s.incomingRelationLinks(ctx, version.ID, property.Qname, "subproperty_of", "property", "properties", baseURL),
	}
	if len(inverse) > 0 {
		model.Inverse = &inverse[0]
	}
	return model, nil
}

func (s *Service) resolveOntologyVersion(ctx context.Context, prefix, versionString string) (*domain.Ontology, *domain.OntologyVersion, error) {
	ontology, err := s.GetOntologyByPrefix(ctx, prefix)
	if err != nil {
		return nil, nil, err
	}
	if ontology == nil {
		return nil, nil, ErrNotFound
	}
	version, err := s.store.GetVersionByOntologyAndString(ctx, ontology.ID, versionString)
	if err != nil {
		return nil, nil, err
	}
	if version == nil {
		return nil, nil, ErrNotFound
	}
	return ontology, version, nil
}

func (s *Service) outgoingRelationLinks(ctx context.Context, versionID, sourceID, sourceKind, relType, targetKind, baseURL string) []OntologyRelationLinkModel {
	rels, err := s.store.ListRelationsForSourceByType(ctx, sourceID, sourceKind, relType)
	if err != nil {
		return nil
	}
	out := make([]OntologyRelationLinkModel, 0, len(rels))
	for _, rel := range rels {
		out = append(out, s.relationLinkForQname(ctx, versionID, rel.TargetQname, targetKind, baseURL))
	}
	return out
}

func (s *Service) incomingRelationLinks(ctx context.Context, versionID, targetQname, relType, sourceKind, baseKind, baseURL string) []OntologyRelationLinkModel {
	rels, err := s.store.ListRelationsForTarget(ctx, targetQname, relType)
	if err != nil {
		return nil
	}
	out := make([]OntologyRelationLinkModel, 0, len(rels))
	for _, rel := range rels {
		switch sourceKind {
		case "class":
			if class, err := s.store.GetClass(ctx, rel.SourceID); err == nil && class != nil {
				out = append(out, classRelationLink(class, baseURL))
			}
		case "property":
			if property, err := s.store.GetProperty(ctx, rel.SourceID); err == nil && property != nil {
				out = append(out, propertyRelationLink(property, baseURL))
			}
		default:
			out = append(out, OntologyRelationLinkModel{Qname: rel.SourceID, Kind: baseKind})
		}
	}
	return out
}

func (s *Service) relationLinkForQname(ctx context.Context, versionID, qname, kind, baseURL string) OntologyRelationLinkModel {
	switch kind {
	case "classes":
		if class, err := s.store.GetClassByQname(ctx, versionID, qname); err == nil && class != nil {
			return classRelationLink(class, baseURL)
		}
	case "properties":
		if property, err := s.store.GetPropertyByQname(ctx, versionID, qname); err == nil && property != nil {
			return propertyRelationLink(property, baseURL)
		}
	}
	return OntologyRelationLinkModel{Qname: qname, Kind: kind}
}

func classRelationLink(class *domain.OntologyClass, baseURL string) OntologyRelationLinkModel {
	return OntologyRelationLinkModel{
		Qname: class.Qname,
		Label: localizedText(class.Label, "en"),
		Href:  baseURL + "/classes/" + class.ID,
		Kind:  "class",
	}
}

func propertyRelationLinks(properties []*domain.OntologyProperty, baseURL string) []OntologyRelationLinkModel {
	out := make([]OntologyRelationLinkModel, 0, len(properties))
	for _, property := range properties {
		out = append(out, propertyRelationLink(property, baseURL))
	}
	return out
}

func propertyRelationLink(property *domain.OntologyProperty, baseURL string) OntologyRelationLinkModel {
	return OntologyRelationLinkModel{
		Qname: property.Qname,
		Label: localizedText(property.Label, "en"),
		Href:  baseURL + "/properties/" + property.ID,
		Kind:  "property",
	}
}

func familyCardModel(family *domain.OntologyFamily, description string, ontologyCount int64, childCount int) FamilyPageCardModel {
	return FamilyPageCardModel{
		ID:            family.ID,
		Name:          family.Name,
		Slug:          family.Slug,
		Description:   description,
		HomepageURL:   family.HomepageURL,
		DisplayOrder:  family.DisplayOrder,
		OntologyCount: ontologyCount,
		ChildCount:    childCount,
		URL:           "/ontologies/families/" + family.Slug,
		AdminURL:      "/admin/ontologies/families/" + family.ID + "/page",
	}
}

func ontologyCardModel(ontology *domain.Ontology, families map[string]*domain.OntologyFamily, ontologies map[string]*domain.Ontology, lang string) OntologyPageCardModel {
	card := OntologyPageCardModel{
		ID:           ontology.ID,
		Name:         ontology.Name,
		Prefix:       ontology.Prefix,
		Namespace:    ontology.Namespace,
		Description:  localizedText(ontology.Description, lang),
		OntologyType: ontologyTypeLabel(ontology.OntologyType),
		DetailURL:    "/ontologies/" + ontology.Prefix,
		AdminURL:     "/admin/ontologies/" + ontology.ID + "/page",
		HomepageURL:  ontology.HomepageURL,
		SourceURL:    ontology.SourceURL,
		IsExtension:  ontology.IsExtension(),
	}
	if ontology.FamilyID != nil && *ontology.FamilyID != "" && families != nil {
		if family := families[*ontology.FamilyID]; family != nil {
			card.FamilyName = family.Name
			card.FamilyURL = "/ontologies/families/" + family.Slug
		}
	}
	if ontology.ExtendsOntologyID != nil && *ontology.ExtendsOntologyID != "" && ontologies != nil {
		if parent := ontologies[*ontology.ExtendsOntologyID]; parent != nil {
			card.ExtendsName = parent.Name
			card.ExtendsURL = "/ontologies/" + parent.Prefix
		}
	}
	return card
}

func versionCardModel(ontology *domain.Ontology, version *domain.OntologyVersion, projectCount int64, lang string) VersionPageCardModel {
	parsedAt := ""
	if version.ParsedAt != nil {
		parsedAt = version.ParsedAt.Format("2006-01-02 15:04")
	}
	lifecycleState := "Inactive"
	lifecycleTone := "slate"
	if version.IsActive {
		lifecycleState = "Active"
		lifecycleTone = "green"
	}
	return VersionPageCardModel{
		ID:                      version.ID,
		OntologyID:              version.OntologyID,
		VersionString:           version.VersionString,
		IsActive:                version.IsActive,
		LifecycleState:          lifecycleState,
		LifecycleTone:           lifecycleTone,
		UsageSummary:            usageSummary(projectCount),
		CompatibilitySummary:    compatibilitySummary(version.CompatibleBaseVersions),
		ImportSummary:           importSummary(version.OriginalFilename, parsedAt),
		ClassCount:              version.ClassCount,
		PropertyCount:           version.PropertyCount,
		ProjectCount:            projectCount,
		CompatibleBaseVersions:  version.CompatibleBaseVersions,
		ImportedOntologies:      version.ImportedOntologies,
		OntologyURI:             version.OntologyURI,
		VersionIRI:              version.VersionIRI,
		VersionInfo:             localizedText(version.VersionInfo, lang),
		OntologyComment:         localizedText(version.OntologyComment, lang),
		OriginalFilename:        version.OriginalFilename,
		FileSize:                version.FileSize,
		FileMD5:                 version.FileMD5,
		ParsedAt:                parsedAt,
		CreatedAt:               version.CreatedAt.Format("2006-01-02 15:04"),
		UpdatedAt:               version.UpdatedAt.Format("2006-01-02 15:04"),
		OntologyMetadataSummary: ontologyMetadataSummary(version.OntologyMetadata),
		VersionURL:              "/ontologies/" + ontology.Prefix + "/" + version.VersionString,
		AdminVersionURL:         "/admin/ontologies/versions/" + version.ID + "/page",
		AdminOntologyURL:        "/admin/ontologies/" + ontology.ID + "/page",
		ClassesURL:              "/ontologies/" + ontology.Prefix + "/" + version.VersionString + "?tab=classes",
		PropertiesURL:           "/ontologies/" + ontology.Prefix + "/" + version.VersionString + "/properties",
	}
}

func ontologyMetadataSummary(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || len(obj) == 0 {
		return ""
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, min(len(keys), 6))
	for _, key := range keys {
		value := obj[key]
		switch v := value.(type) {
		case string:
			if v != "" {
				parts = append(parts, key+"="+v)
			}
		case float64, bool:
			parts = append(parts, fmt.Sprintf("%s=%v", key, v))
		case []any:
			parts = append(parts, fmt.Sprintf("%s=%d items", key, len(v)))
		case map[string]any:
			parts = append(parts, fmt.Sprintf("%s=%d fields", key, len(v)))
		}
		if len(parts) == 6 {
			break
		}
	}
	return strings.Join(parts, ", ")
}

func usageSummary(projectCount int64) string {
	if projectCount == 1 {
		return "Linked to 1 project"
	}
	if projectCount > 1 {
		return fmt.Sprintf("Linked to %d projects", projectCount)
	}
	return "No linked projects"
}

func compatibilitySummary(versions []string) string {
	if len(versions) == 0 {
		return "No compatibility constraints recorded"
	}
	return "Compatible with " + strings.Join(versions, ", ")
}

func importSummary(filename, parsedAt string) string {
	if filename != "" && parsedAt != "" {
		return "Imported from " + filename + " on " + parsedAt
	}
	if filename != "" {
		return "Imported from " + filename
	}
	if parsedAt != "" {
		return "Imported on " + parsedAt
	}
	return "No import metadata recorded"
}

func familyAdminURL(family *domain.OntologyFamily) string {
	if family == nil {
		return ""
	}
	return "/admin/ontologies/families/" + family.ID + "/page"
}

func familyPageBreadcrumbs(ctx context.Context, svc *Service, family *domain.OntologyFamily) []OntologyPageLinkModel {
	if family == nil {
		return nil
	}
	var chain []OntologyPageLinkModel
	parentID := family.ParentFamilyID
	for parentID != nil && *parentID != "" {
		parent, err := svc.GetFamily(ctx, *parentID)
		if err != nil || parent == nil {
			break
		}
		chain = append(chain, OntologyPageLinkModel{
			Label: parent.Name,
			Href:  "/ontologies/families/" + parent.Slug,
		})
		parentID = parent.ParentFamilyID
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}
