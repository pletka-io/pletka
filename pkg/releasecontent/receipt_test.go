package releasecontent_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/pletka-io/pletka/pkg/releasecontent"
)

func document() map[string]any {
	sections := map[string]any{}
	for _, name := range releasecontent.RequiredSections() {
		sections[name] = map[string]any{"rows": 0, "sha256": strings.Repeat("a", 64)}
	}
	return map[string]any{"format_version": 1, "sections": sections}
}

func encode(t *testing.T, d map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReceiptsKeepContractIdentityAndExplicitEmptyCoverage(t *testing.T) {
	d := document()
	raw := encode(t, d)
	r, err := releasecontent.DecodeVerified(raw, "project", "1")
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = '!'
	if r.Origin().ProjectID() != "project" || r.Origin().Version() != "1" {
		t.Fatal("receipt lost its source binding")
	}
	counts := map[string]int64{}
	for _, name := range releasecontent.RequiredSections() {
		s, ok := r.Section(name)
		if !ok || cmp.Diff(releasecontent.Section{Rows: 0, SHA256: strings.Repeat("a", 64)}, s) != "" {
			t.Fatalf("empty section lost: %s %+v", name, s)
		}
		counts[name] = 0
	}
	if err := r.CheckCounts(counts); err != nil {
		t.Fatal(err)
	}
	if err := (releasecontent.Verified{}).CheckCounts(nil); err == nil {
		t.Fatal("zero receipt usable")
	}
	name := releasecontent.RequiredSections()[0]
	counts[name] = 1
	if err := r.CheckCounts(counts); err == nil {
		t.Fatal("different row count accepted")
	}
	delete(counts, name)
	if err := r.CheckCounts(counts); err == nil {
		t.Fatal("unloaded section appeared empty")
	}
	if _, err := releasecontent.DecodeLegacy(encode(t, d), "project", "1"); err == nil {
		t.Fatal("verified contract accepted as legacy")
	}
	d["format_version"], d["evidence_kind"], d["ontology_snapshot_digest"] = 2, "legacy_ontology_snapshot", strings.Repeat("b", 64)
	legacy, err := releasecontent.DecodeLegacy(encode(t, d), "project", "1")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Ontology().Origin() != legacy.Origin() || legacy.Ontology().Digest() != strings.Repeat("b", 64) {
		t.Fatal("legacy binding changed")
	}
	if _, err := releasecontent.DecodeVerified(encode(t, d), "project", "1"); err == nil {
		t.Fatal("legacy contract promoted to verified")
	}
}

func TestReceiptRejectsInventoriesThatOnlyHaveTheRightLength(t *testing.T) {
	name := releasecontent.RequiredSections()[0]
	for _, mode := range []string{"unknown section", "missing rows", "null section", "negative rows", "bad hash", "missing hash", "null hash", "short inventory"} {
		t.Run(mode, func(t *testing.T) {
			d := document()
			sections := d["sections"].(map[string]any)
			row := sections[name].(map[string]any)
			switch mode {
			case "unknown section":
				delete(sections, name)
				sections["invented"] = row
			case "missing rows":
				delete(row, "rows")
			case "null section":
				sections[name] = nil
			case "negative rows":
				row["rows"] = -1
			case "bad hash":
				row["sha256"] = strings.Repeat("z", 64)
			case "missing hash":
				delete(row, "sha256")
			case "null hash":
				row["sha256"] = nil
			case "short inventory":
				delete(sections, name)
			}
			if _, err := releasecontent.DecodeVerified(encode(t, d), "project", "1"); err == nil {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
	if _, err := releasecontent.DecodeVerified(encode(t, document()), "project", " "); err == nil {
		t.Fatal("invalid release binding accepted")
	}
}
