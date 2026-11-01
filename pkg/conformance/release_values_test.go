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
		"crypto/sha256": true, "encoding/json": true, "fmt": true, "strings": true, "slices": true, "bytes": true,
		"github.com/pletka-io/pletka/pkg/namespace":         true,
		"github.com/pletka-io/pletka/pkg/ontologyevidence":  true,
		"github.com/pletka-io/pletka/pkg/ontologyartifact":  true,
		"github.com/pletka-io/pletka/pkg/releasecontent":    true,
		"github.com/pletka-io/pletka/pkg/releasedescriptor": true,
	}
	for _, dir := range []string{"ontologyartifact", "ontologyevidence", "releasecontent", "releasedescriptor", "namespace", "projectsnapshot", "projectbundle"} {
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
				bundleHelper := dir == "projectbundle" && (name == "unicode/utf8" || name == "github.com/pletka-io/pletka/pkg/projectsnapshot")
				// The row codec visits every typed section against Coverage; patching
				// also compares all fields of opaque retained evidence. Reflection
				// stays private and adds no runtime I/O or lookup capability.
				valueReflection := false
				if dir == "projectsnapshot" && name == "reflect" {
					switch filepath.Base(path) {
					case "rows.go", "project.go", "selection.go", "patch.go":
						valueReflection = true
					}
				}
				// Streaming uses the same v1 options as encoding/json, over owned
				// strings and a hash. The codec exposes no I/O capability to callers.
				evidenceCodec := dir == "ontologyevidence" && filepath.Base(path) == "codec.go" && name == "encoding/json/v2"
				if !allowed[name] && !namespaceHelper && !bundleHelper && !valueReflection && !evidenceCodec {
					t.Errorf("%s imports %s outside the pure release value boundary", path, name)
				}
			}
		}
	}
}
