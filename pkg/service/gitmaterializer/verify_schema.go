package gitmaterializer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	formatschema "github.com/pletka-io/pletka/docs-oss/format/schema"
)

const gitDirName = ".git"

// checkSchemas validates every format file of the tree against the format's
// JSON Schemas. Files of a vendored module are classified relative to that
// module's root. Files with a finding are remembered so later decoders do
// not report the same problem again.
func (v *verifyContext) checkSchemas() {
	val := formatschema.Default()
	_ = filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is the loader's finding
		}
		if d.IsDir() {
			if d.Name() == gitDirName {
				return filepath.SkipDir
			}
			return nil
		}
		kind, ok := formatschema.KindForPath(moduleRel(v.root, p))
		if !ok {
			return nil
		}
		rel, _ := filepath.Rel(v.root, p)
		rel = filepath.ToSlash(rel)
		payload, err := os.ReadFile(p) //nolint:gosec // p is under the tree root
		if err != nil {
			return nil //nolint:nilerr // reported by the loader
		}
		viols, err := val.Validate(kind, payload)
		if err != nil {
			v.schemaFailed[rel] = true
			v.add(rel, 0, "decode", "%v", err)
			return nil
		}
		for _, viol := range viols {
			v.schemaFailed[rel] = true
			v.add(rel, formatschema.Line(payload, viol.Pointer), schemaFindingCode(viol), "%s: %s", pointerLabel(viol.Pointer), viol.Message)
		}
		return nil
	})
}

func schemaFindingCode(viol formatschema.Violation) string {
	switch {
	case viol.Keyword == "additionalProperties":
		return "unknown-key"
	case viol.Pointer == "/schema_version":
		return "schema-version"
	case viol.Keyword == "type":
		return "decode"
	default:
		return "schema"
	}
}

func pointerLabel(pointer string) string {
	if pointer == "" {
		return "document"
	}
	return strings.TrimPrefix(pointer, "/")
}

// moduleRel is p relative to the nearest enclosing module root under root: a
// directory holding project.yaml or ontology.yaml, or root itself.
func moduleRel(root, p string) string {
	for d := filepath.Dir(p); d != root && strings.HasPrefix(d, root); d = filepath.Dir(d) {
		if fileExists(filepath.Join(d, "project.yaml")) || fileExists(filepath.Join(d, "ontology.yaml")) {
			r, _ := filepath.Rel(d, p)
			return filepath.ToSlash(r)
		}
	}
	r, _ := filepath.Rel(root, p)
	return filepath.ToSlash(r)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
