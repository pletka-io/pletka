package x3ml

import (
	"archive/zip"
	"bytes"
	"fmt"

	"github.com/pletka-io/pletka/pkg/domain"
)

// BuildZip assembles an in-memory ZIP containing the rendered X3ML
// mapping and one raw schema file per linked ontology that carries
// imported RDF content. Ontologies with empty OriginalFilename or
// RDFContent are skipped; duplicate filenames are deduplicated.
//
// The function is pure with respect to I/O: it consumes the rendered
// mapping bytes and an ontology bundle and returns the zip bytes. The
// caller decides whether to stream to HTTP, hand off to an integration,
// or write to disk.
func BuildZip(mappingFilename string, mapping []byte, bundle []domain.OntologyBundleEntry) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	add := func(name string, data []byte) error {
		fw, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("zip create %q: %w", name, err)
		}
		if _, err := fw.Write(data); err != nil {
			return fmt.Errorf("zip write %q: %w", name, err)
		}
		return nil
	}

	if err := add(mappingFilename, mapping); err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, o := range bundle {
		if o.OriginalFilename == "" || o.RDFContent == "" || seen[o.OriginalFilename] {
			continue
		}
		seen[o.OriginalFilename] = true
		if err := add(o.OriginalFilename, []byte(o.RDFContent)); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("zip close: %w", err)
	}
	return buf.Bytes(), nil
}
