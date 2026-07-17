package ontology

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pletka-io/pletka/pkg/domain"
)

// EdgeProvenanceVersion is one version that declares the requested edge.
type EdgeProvenanceVersion struct {
	VersionID     string `json:"version_id"`
	VersionString string `json:"version_string"`
	Prefix        string `json:"prefix"`
}

// EdgeProvenance handles GET /api/v1/ontology/edge-provenance.
//
// Required query parameters:
//
//	project_id — the project whose linked ontology versions are searched
//	source     — source node qname (e.g. crm:E13_Attribute_Assignment)
//	target     — target qname (e.g. crm:E11_Modification)
//	rel        — relation type (e.g. subclass_of, domain, range)
//
// Returns 200 JSON {"versions":[{version_id, version_string, prefix}]}
// listing which of the project's linked ontology versions declare the
// requested edge. Returns an empty versions array when no version
// declares the edge. Returns 400 when any required parameter is missing.
func (h *Handler) EdgeProvenance(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	projectID := q.Get("project_id")
	source := q.Get("source")
	target := q.Get("target")
	rel := q.Get("rel")

	if projectID == "" || source == "" || target == "" || rel == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "project_id, source, target, and rel are required"}) // best-effort: headers already sent, write failure isn't actionable
		return
	}

	result, err := h.svc.EdgeProvenance(r.Context(), projectID, source, target, rel)
	if err != nil {
		h.log.Error("edge provenance", "err", err, "project_id", projectID, "source", source, "target", target, "rel", rel)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"versions": result})
}

// EdgeProvenance resolves which of projectID's linked ontology versions
// declare the edge source -[rel]-> target. It iterates versions one by
// one via per-version Store calls; this is intentionally off the hot
// path and makes no use of the index cache.
func (s *Service) EdgeProvenance(ctx context.Context, projectID, source, target, rel string) ([]EdgeProvenanceVersion, error) {
	if s.projects == nil {
		return nil, fmt.Errorf("edge provenance: projects reader not configured")
	}
	resolved, err := s.projects.ResolvedOntologyVersions(ctx, projectID, domain.ResolvedOntologyVersionOpts{})
	if err != nil {
		return nil, fmt.Errorf("edge provenance resolve versions: %w", err)
	}

	out := make([]EdgeProvenanceVersion, 0)
	for _, r := range resolved {
		versionID := r.Link.OntologyVersionID
		found, versionString, prefix, err := s.edgePresentInVersion(ctx, versionID, source, target, rel)
		if err != nil {
			return nil, err
		}
		if found {
			out = append(out, EdgeProvenanceVersion{
				VersionID:     versionID,
				VersionString: versionString,
				Prefix:        prefix,
			})
		}
	}
	return out, nil
}

// edgePresentInVersion checks whether the edge source -[rel]-> target
// exists within versionID. Returns (true, versionString, ontologyPrefix, nil)
// on match.
func (s *Service) edgePresentInVersion(ctx context.Context, versionID, sourceQname, targetQname, rel string) (bool, string, string, error) {
	versionString, prefix, err := s.versionMeta(ctx, versionID)
	if err != nil {
		return false, "", "", err
	}

	sourceKind, sourceID, err := s.resolveSourceNode(ctx, versionID, sourceQname, rel)
	if err != nil {
		return false, "", "", err
	}
	if sourceID == "" {
		// source not present in this version — edge cannot exist here
		return false, "", "", nil
	}

	rels, err := s.store.ListRelationsForSourceByType(ctx, sourceID, sourceKind, rel)
	if err != nil {
		return false, "", "", fmt.Errorf("edge provenance list relations (version %s): %w", versionID, err)
	}
	for _, r := range rels {
		if r.TargetQname == targetQname {
			return true, versionString, prefix, nil
		}
	}
	return false, "", "", nil
}

// versionMeta returns (versionString, ontologyPrefix) for versionID.
func (s *Service) versionMeta(ctx context.Context, versionID string) (string, string, error) {
	v, err := s.store.GetVersion(ctx, versionID)
	if err != nil {
		return "", "", fmt.Errorf("edge provenance get version %s: %w", versionID, err)
	}
	if v == nil {
		return "", "", nil
	}
	prefix := ""
	o, err := s.store.GetOntology(ctx, v.OntologyID)
	if err != nil {
		return "", "", fmt.Errorf("edge provenance get ontology for version %s: %w", versionID, err)
	}
	if o != nil {
		prefix = o.Prefix
	}
	return v.VersionString, prefix, nil
}

// resolveSourceNode resolves the qname to a (sourceKind, sourceID) pair
// within versionID. For class-type relations the source must be a class;
// for property-type relations it must be a property; for unknown rel
// types we try class first, then property.
func (s *Service) resolveSourceNode(ctx context.Context, versionID, qname, rel string) (string, string, error) {
	switch rel {
	case "subclass_of", "equivalent_class", "disjoint_class":
		c, err := s.store.GetClassByQname(ctx, versionID, qname)
		if err != nil || c == nil {
			return "class", "", nil
		}
		return "class", c.ID, nil
	case "domain", "range", "subproperty_of", "equivalent_property", "disjoint_property", "inverse_property":
		p, err := s.store.GetPropertyByQname(ctx, versionID, qname)
		if err != nil || p == nil {
			return "property", "", nil
		}
		return "property", p.ID, nil
	default:
		c, err := s.store.GetClassByQname(ctx, versionID, qname)
		if err == nil && c != nil {
			return "class", c.ID, nil
		}
		p, err := s.store.GetPropertyByQname(ctx, versionID, qname)
		if err == nil && p != nil {
			return "property", p.ID, nil
		}
		return "", "", nil
	}
}
