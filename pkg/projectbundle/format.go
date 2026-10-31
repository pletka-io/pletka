// Package projectbundle encodes detached complete project values. It has no
// database, filesystem, request, clock, configuration or network capability.
package projectbundle

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pletka-io/pletka/pkg/ontologyartifact"
	"github.com/pletka-io/pletka/pkg/projectsnapshot"
	"github.com/pletka-io/pletka/pkg/releasecontent"
	"github.com/pletka-io/pletka/pkg/releasedescriptor"
)

// SchemaVersion distinguishes this complete format from partial Git formats 1/2.
const SchemaVersion = 3

// ManifestPath is JSON (a YAML subset), like the other canonical bundle files.
const ManifestPath = "project.yaml"

// Files contains relative, slash-separated paths and exact file bytes. A release
// catalog or Git metadata is NOT part of this frozen value. Files are caller-owned.
type Files map[string][]byte

// Limits bounds untrusted decoding before parsing. It does not bound an already
// allocated input map, or peak memory used to validate the detached graph.
// MaxBytes bounds both stored bytes and cumulative referenced bytes (including
// repeated ontology evidence); shared files cannot amplify work without a bound.
type Limits struct {
	MaxBytes    int64
	MaxFiles    int
	MaxProjects int
}

// DefaultLimits matches the assembler's project/byte budget, with room for row
// files and context manifests. Callers must supply explicit positive limits.
func DefaultLimits() Limits {
	return Limits{MaxBytes: 256 << 20, MaxFiles: 1_100_000, MaxProjects: 256}
}

type selection struct {
	Project string `json:"project_id"`
	Version string `json:"version"`
	Draft   bool   `json:"draft"`
}

func selected(id projectsnapshot.Identity) selection {
	return selection{Project: id.ProjectID(), Version: id.Version(), Draft: id.IsDraft()}
}

func (s selection) identity() (projectsnapshot.Identity, error) {
	if s.Draft {
		if s.Version != "" {
			return projectsnapshot.Identity{}, fmt.Errorf("draft has a release version")
		}
		return projectsnapshot.Draft(s.Project)
	}
	return projectsnapshot.Release(s.Project, s.Version)
}

type fileRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type manifest struct {
	SchemaVersion int       `json:"schema_version"`
	Root          selection `json:"root"`
	Projects      []fileRef `json:"projects"`
}

type membership struct {
	Kind   string    `json:"kind"`
	ID     string    `json:"id"`
	Source selection `json:"source"`
}

type contextFile struct {
	Identity     selection            `json:"identity"`
	Dependencies []selection          `json:"dependencies"`
	Membership   []membership         `json:"membership"`
	Evidence     evidenceFile         `json:"evidence"`
	Sections     map[string][]fileRef `json:"sections"`
}

type evidenceFile struct {
	Contract     int                               `json:"contract"`
	Descriptors  releasedescriptor.Document        `json:"descriptors"`
	Receipt      *receiptFile                      `json:"receipt"`
	Publications []publication                     `json:"publications"`
	References   []ontologyartifact.TermResolution `json:"references"`
	Legacy       *fileRef                          `json:"legacy"`
}

type receiptFile struct {
	Format   int                               `json:"format_version"`
	Kind     string                            `json:"evidence_kind"`
	Digest   string                            `json:"ontology_snapshot_digest"`
	Sections map[string]releasecontent.Section `json:"sections"`
}

type publication struct {
	Artifact fileRef `json:"artifact"`
	Linked   bool    `json:"linked"`
	Primary  bool    `json:"primary"`
}

func hash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func canonical(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func contextPath(s selection) string {
	// Fixed field types make encoding infallible. Hashing the full selector keeps
	// path traversal and ambiguous delimiter concatenation out of file names.
	data, _ := json.Marshal(s)
	return "projects/" + hash(data)
}

func validPath(s string) bool {
	if s == "" || strings.ContainsAny(s, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func (f Files) put(path string, data []byte) fileRef {
	f[path] = data
	return fileRef{Path: path, SHA256: hash(data)}
}
