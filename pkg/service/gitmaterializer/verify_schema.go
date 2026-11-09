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
		mrel := moduleRel(v.root, p)
		kind, ok := formatschema.KindForPath(mrel)
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
			v.flagSchema(rel, mrel)
			v.add(rel, 0, "decode", "%v", err)
			return nil
		}
		for _, viol := range viols {
			v.flagSchema(rel, mrel)
			v.add(rel, formatschema.Line(payload, viol.Pointer), schemaFindingCode(viol), "%s: %s", pointerLabel(viol.Pointer), viol.Message)
		}
		return nil
	})
}

// flagSchema remembers a flagged file by its tree path and, for matching
// loader errors (which name module-relative paths), by its module path.
func (v *verifyContext) flagSchema(rel, mrel string) {
	v.schemaFailed[rel] = true
	v.schemaFailedModule[mrel] = true
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
// directory holding project.yaml or ontology.yaml, or root itself. It walks
// the segments of the root-relative path, so root may be "." or relative.
func moduleRel(root, p string) string {
	rel, _ := filepath.Rel(root, p)
	segs := strings.Split(filepath.ToSlash(rel), "/")
	for i := len(segs) - 1; i > 0; i-- {
		d := filepath.Join(root, filepath.FromSlash(strings.Join(segs[:i], "/")))
		if isFile(filepath.Join(d, "project.yaml")) || isFile(filepath.Join(d, "ontology.yaml")) {
			return strings.Join(segs[i:], "/")
		}
	}
	return filepath.ToSlash(rel)
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
