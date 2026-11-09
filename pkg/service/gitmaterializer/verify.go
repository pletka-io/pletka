package gitmaterializer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Finding is one thing verify-git found wrong with a tree.
type Finding struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// String renders file:line: code: message, without the line when unknown.
func (f Finding) String() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d: %s: %s", f.File, f.Line, f.Code, f.Message)
	}
	return fmt.Sprintf("%s: %s: %s", f.File, f.Code, f.Message)
}

// VerifyOptions steer Verify.
type VerifyOptions struct {
	// ParentDirs are trees of declared parent projects.
	ParentDirs []string
	// OntologiesDir is a directory in vendor/ontologies layout to take the
	// ontologies from; empty means the tree's own vendor/ontologies.
	OntologiesDir string
	// SkipOntology runs the structure checks only.
	SkipOntology bool
}

// VerifyReport is what Verify returns: counts and findings sorted by file.
type VerifyReport struct {
	ProjectID string    `json:"project_id"`
	Fields    int       `json:"fields"`
	Steps     int       `json:"steps"`
	Findings  []Finding `json:"findings"`
}

type verifyContext struct {
	root         string
	snap         *ProjectSnapshot
	parents      map[string]*ProjectSnapshot
	vendored     []VendoredOntologySnapshot // the ontologies the offline index is built from
	declared     map[string]bool
	receipts     map[string]bool // field ids backed by an adoption receipt
	fields       map[string]bool
	categories   map[string]bool
	collections  map[string]bool
	conceptLists map[string]bool
	schemaFailed map[string]bool // tree-relative paths the schema pass flagged
	findings     []Finding
}

func (v *verifyContext) add(file string, line int, code, format string, args ...any) {
	v.findings = append(v.findings, Finding{File: filepath.ToSlash(file), Line: line, Code: code, Message: fmt.Sprintf(format, args...)})
}

// Verify checks a project tree without a database. It returns an error only
// when the directory cannot be read; everything else is a finding.
func Verify(ctx context.Context, rootDir string, opts VerifyOptions) (*VerifyReport, error) {
	if st, err := os.Stat(rootDir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("verify-git: %s is not a directory", rootDir)
	}
	v := &verifyContext{
		root:         rootDir,
		parents:      map[string]*ProjectSnapshot{},
		declared:     map[string]bool{},
		receipts:     map[string]bool{},
		schemaFailed: map[string]bool{},
		fields:       map[string]bool{}, categories: map[string]bool{}, collections: map[string]bool{}, conceptLists: map[string]bool{},
	}
	v.checkSchemas()
	snap, err := LoadProjectSnapshot(rootDir)
	if err != nil {
		// A loader error about a file the schema pass flagged says nothing
		// new; those findings already say why, with file and line.
		if !v.schemaReported(err) {
			v.add(loadErrorFile(err), 0, "load", "%v", err)
		}
		return v.report(""), nil
	}
	v.snap = snap
	v.loadParents(opts.ParentDirs)
	v.checkVendorChecksums()
	v.checkUnknownFiles()
	v.collectIdentities()
	v.checkEntities()
	v.checkOverrides()
	v.checkPlacements()
	v.checkAdoptions()
	rep := v.report(snap.Manifest.Project.ID)
	if !opts.SkipOntology {
		v.checkOntology(ctx, opts, rep)
		rep.Findings = v.sorted()
	}
	return rep, nil
}

func (v *verifyContext) sorted() []Finding {
	sort.SliceStable(v.findings, func(i, j int) bool {
		if v.findings[i].File != v.findings[j].File {
			return v.findings[i].File < v.findings[j].File
		}
		return v.findings[i].Line < v.findings[j].Line
	})
	return v.findings
}

func (v *verifyContext) report(projectID string) *VerifyReport {
	return &VerifyReport{ProjectID: projectID, Findings: v.sorted()}
}

// schemaReported reports whether err names a file the schema pass flagged.
func (v *verifyContext) schemaReported(err error) bool {
	for rel := range v.schemaFailed {
		if strings.Contains(err.Error(), rel) {
			return true
		}
	}
	return false
}

// loadErrorFile pulls the relative path out of a loader error when it names
// one ("project.yaml: ...", "fields/X/field.yaml: ..."); otherwise "project.yaml".
func loadErrorFile(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i > 0 && !strings.ContainsAny(msg[:i], " \t") {
		return msg[:i]
	}
	return "project.yaml"
}

// readRel returns the bytes of a file under the tree root, or nil.
func (v *verifyContext) readRel(rel string) []byte {
	b, err := os.ReadFile(filepath.Join(v.root, rel)) //nolint:gosec // rel is a tree path the loader already read
	if err != nil {
		return nil
	}
	return b
}

// yamlKeyLine is the 1-based line of the first top-level "key:"; 0 if absent.
// ponytail: a regex over lines, no YAML positions; enough to point at a key.
func yamlKeyLine(payload []byte, key string) int {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `\s*:`)
	loc := re.FindIndex(payload)
	if loc == nil {
		return 0
	}
	return bytes.Count(payload[:loc[0]], []byte("\n")) + 1
}
