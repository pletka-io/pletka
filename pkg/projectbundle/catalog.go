package projectbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pletka-io/pletka/pkg/projectsnapshot"
)

// CatalogPath is a separate overlay. It is never included in Encode's frozen
// content or Decode's file inventory; tag verification supplies its stored bytes.
const CatalogPath = "releases/index.yaml"

// ReleaseRecord preserves a release's catalog metadata, including its original
// creator/time. These records do not supply missing release content. A catalog may
// mention older root releases whose bodies are not in this particular bundle.
type ReleaseRecord struct {
	ProjectID       string                     `json:"project_id"`
	Version         string                     `json:"version"`
	Title           string                     `json:"title"`
	Description     string                     `json:"description"`
	CreatedAt       projectsnapshot.Timestamp  `json:"created_at"`
	CreatedByID     string                     `json:"created_by_id"`
	ArchivedAt      *projectsnapshot.Timestamp `json:"archived_at"`
	ArchivedMessage string                     `json:"archived_message"`
}

// Catalog is explicit, detached metadata for one complete snapshot's selected
// projects. It is not release evidence. Its zero value cannot supply history.
type Catalog struct{ document string }

type catalogDocument struct {
	SchemaVersion int             `json:"schema_version"`
	RootProjectID string          `json:"root_project_id"`
	Releases      []ReleaseRecord `json:"releases"`
}

// NewCatalog requires the original record for every concrete release in value,
// including source releases. Extra history entries are allowed only for the root
// project and never authorize restoring nonexistent release bodies.
func NewCatalog(value projectsnapshot.CompleteProject, records []ReleaseRecord) (Catalog, error) {
	root, err := value.Root()
	if err != nil {
		return Catalog{}, err
	}
	ids, err := value.Projects()
	if err != nil {
		return Catalog{}, err
	}
	if records == nil {
		return Catalog{}, fmt.Errorf("release catalog must be explicitly loaded")
	}
	selected := map[projectsnapshot.Identity]bool{}
	for _, id := range ids {
		if !id.IsDraft() {
			selected[id] = false
		}
	}
	seen := map[projectsnapshot.Identity]bool{}
	for _, record := range records {
		id, err := projectsnapshot.Release(record.ProjectID, record.Version)
		if err != nil {
			return Catalog{}, err
		}
		if seen[id] {
			return Catalog{}, fmt.Errorf("duplicate catalog release %s@%s", record.ProjectID, record.Version)
		}
		seen[id] = true
		if err := record.checkMetadata(); err != nil {
			return Catalog{}, err
		}
		if _, ok := selected[id]; ok {
			evidence, err := value.Evidence(id)
			if err != nil {
				return Catalog{}, err
			}
			if _, err := evidence.Descriptors().Actor(&record.CreatedByID); err != nil {
				return Catalog{}, fmt.Errorf("catalog creator differs from captured descriptors: %w", err)
			}
			selected[id] = true
		} else if record.ProjectID != root.ProjectID() {
			return Catalog{}, fmt.Errorf("catalog contains an unselected source release")
		}
	}
	for id, found := range selected {
		if !found {
			return Catalog{}, fmt.Errorf("catalog lacks selected release %s@%s", id.ProjectID(), id.Version())
		}
	}
	ordered := append([]ReleaseRecord{}, records...)
	slices.SortFunc(ordered, func(a, b ReleaseRecord) int {
		if n := strings.Compare(a.ProjectID, b.ProjectID); n != 0 {
			return n
		}
		return strings.Compare(a.Version, b.Version)
	})
	data, err := canonical(catalogDocument{SchemaVersion: 2, RootProjectID: root.ProjectID(), Releases: ordered})
	if err != nil {
		return Catalog{}, err
	}
	return Catalog{document: string(data)}, nil
}

// EncodeCatalog returns canonical overlay bytes. No timestamp or policy is added.
func EncodeCatalog(c Catalog) ([]byte, error) {
	if c.document == "" {
		return nil, fmt.Errorf("release catalog is uninitialized")
	}
	return []byte(c.document), nil
}

// DecodeCatalog validates the new overlay against its associated complete value.
// Legacy schema1 remains on the legacy materializer; it lacks source metadata.
func DecodeCatalog(value projectsnapshot.CompleteProject, data []byte, maxBytes int64) (Catalog, error) {
	if maxBytes <= 0 || int64(len(data)) > maxBytes {
		return Catalog{}, fmt.Errorf("catalog byte budget exceeded or invalid")
	}
	var d catalogDocument
	if err := json.Unmarshal(data, &d); err != nil {
		return Catalog{}, err
	}
	root, err := value.Root()
	if err != nil {
		return Catalog{}, err
	}
	if d.SchemaVersion != 2 || d.RootProjectID != root.ProjectID() {
		return Catalog{}, fmt.Errorf("unsupported catalog format or wrong root")
	}
	catalog, err := NewCatalog(value, d.Releases)
	if err != nil {
		return Catalog{}, err
	}
	if !bytes.Equal(data, []byte(catalog.document)) {
		return Catalog{}, fmt.Errorf("noncanonical or incomplete release catalog")
	}
	return catalog, nil
}

// SelectedReleases returns metadata only for the concrete releases whose bodies
// are present. Other root history entries remain catalog evidence, not empty cuts.
func (c Catalog) SelectedReleases(value projectsnapshot.CompleteProject) ([]ReleaseRecord, error) {
	if c.document == "" {
		return nil, fmt.Errorf("release catalog is uninitialized")
	}
	var d catalogDocument
	if err := json.Unmarshal([]byte(c.document), &d); err != nil {
		return nil, err
	}
	validated, err := NewCatalog(value, d.Releases)
	if err != nil {
		return nil, err
	}
	if validated.document != c.document {
		return nil, fmt.Errorf("catalog is bound to another root")
	}
	ids, err := value.Projects()
	if err != nil {
		return nil, err
	}
	selected := map[projectsnapshot.Identity]bool{}
	for _, id := range ids {
		if !id.IsDraft() {
			selected[id] = true
		}
	}
	out := make([]ReleaseRecord, 0, len(selected))
	for _, record := range d.Releases {
		id, err := projectsnapshot.Release(record.ProjectID, record.Version)
		if err != nil {
			return nil, err
		}
		if selected[id] {
			out = append(out, record)
		}
	}
	return out, nil
}

func (r ReleaseRecord) checkMetadata() error {
	if strings.TrimSpace(string(r.CreatedAt)) == "" || strings.TrimSpace(r.CreatedByID) == "" || r.ArchivedAt != nil && strings.TrimSpace(string(*r.ArchivedAt)) == "" {
		return fmt.Errorf("release catalog lacks original creator or timestamp")
	}
	return nil
}
