package conformance

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// These packages are serializer inputs, not binding infrastructure. Keep the
// permitted imports explicit so a future helper cannot reintroduce DB access,
// current request policy, filesystem paths or lazy readers transitively.
func TestPortableReleaseValuesHaveNoRuntimeCapabilities(t *testing.T) {
	allowed := map[string]bool{
		"crypto/sha256": true, "encoding/json": true, "fmt": true, "strings": true, "slices": true,
		"github.com/pletka-io/pletka/pkg/namespace":        true,
		"github.com/pletka-io/pletka/pkg/ontologyevidence": true,
	}
	for _, dir := range []string{"ontologyartifact", "ontologyevidence", "releasecontent", "releasedescriptor", "namespace"} {
		for _, path := range goFilesUnder(t, filepath.Join(repoRoot(t), "pkg", dir)) {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range f.Imports {
				name, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				// The namespace helper uses only pure values and an internal mutex.
				namespaceHelper := dir == "namespace" && (name == "sort" || name == "errors" || name == "sync")
				if !allowed[name] && !namespaceHelper {
					t.Errorf("%s imports %s outside the pure release value boundary", path, name)
				}
			}
		}
	}
}
