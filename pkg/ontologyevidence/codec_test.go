package ontologyevidence

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestStreamingDecodePreservesExistingJSONDialect(t *testing.T) {
	golden, err := os.ReadFile("testdata/legacy.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"golden evidence":                  string(golden),
		"case and duplicate names":         `{"FORMAT_VERSION":1,"PROJECT_ID":"first","project_id":"second","version":"1"}`,
		"raw authored JSON":                `{"elements":[{"original": {"n":9007199254740993.123456789,"e":1e-99,"s":"<>&\u2028","z":null}}]}`,
		"null and empty":                   `{"inventory":[],"bindings":null,"elements":[],"assessments":[],"sources":[],"source_refs":null,"dependencies":[]}`,
		"unknown field":                    `{"future":{"array":[true,9007199254740993,null]},"project_id":"known"}`,
		"body projection":                  `{"format_version":1,"ontology":{"id":"o"},"version":{"id":"v","ontology_id":"o"},"classes":[{"uri":"雪","prefix":"o","local_name":"x"}],"properties":[],"namespace_bindings":[]}`,
		"invalid UTF-8 compatibility":      "{\"project_id\":\"\xff\"}",
		"unpaired surrogate compatibility": `{"project_id":"\ud800"}`,
		"large skipped field":              `{"unknown":"` + strings.Repeat("雪\\\"", 65536) + `","project_id":"after"}`,
		"large retained field":             `{"sources":[{"json":"` + strings.Repeat("雪\\\"", 65536) + `","digest":"retained"}]}`,
		"trailing whitespace":              string(golden) + " \n\t\r",
		"second document":                  string(golden) + `{}`,
		"trailing garbage":                 string(golden) + `x`,
		"truncated document":               string(golden[:len(golden)/2]),
		"invalid type":                     `{"format_version":true}`,
		"fractional integer":               `{"format_version":1.1}`,
		"empty":                            "",
		"null":                             "null",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			checkDecodeParity[Document](t, raw)
			checkDecodeParity[bodyDocument](t, raw)
		})
	}
}

func checkDecodeParity[T any](t *testing.T, raw string) {
	t.Helper()
	var want T
	wantErr := json.Unmarshal([]byte(raw), &want)
	got, gotErr := decodeJSON[T](raw)
	if (wantErr == nil) != (gotErr == nil) {
		t.Fatalf("decode acceptance changed: original=%v streaming=%v", wantErr, gotErr)
	}
	if wantErr != nil && fmt.Sprintf("%T: %v", wantErr, wantErr) != fmt.Sprintf("%T: %v", gotErr, gotErr) {
		t.Fatalf("decode error changed: original=%v streaming=%v", wantErr, gotErr)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func TestStreamingLocalDigestMatchesOriginalEncoding(t *testing.T) {
	base := valueFixture(t)
	base.Inventory[0].Selection = json.RawMessage(" {\"x\": 9007199254740993.123456789, \"x\": 1e-99, \"s\": \"<>&雪\\u2028\"} \n")
	base.Elements[0].Original = json.RawMessage(`{"s":"\ud800","a":[true,null,false]}`)
	base.ProjectID = "quotes\" \\ \n\x00 雪 <>&\u2028\u2029\xff"
	base.Sources = []Source{{JSON: "embedded sources are excluded", Digest: "original"}}
	for _, tc := range []struct {
		name string
		doc  Document
	}{
		{"all nil", Document{}},
		{"empty collections", Document{Inventory: []Body{}, Elements: []Element{}, Sources: []Source{}, Dependencies: []Dependency{}}},
		{"escaped raw and duplicate names", base},
		{"large body string", Document{Inventory: []Body{{BodyJSON: strings.Repeat("<>&雪\\\"", 200000)}}}},
		{"invalid raw JSON", Document{Elements: []Element{{Original: json.RawMessage(`{`)}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Match the former hash of json.Marshal(local), excluding Sources only.
			local := tc.doc
			local.Sources = nil
			encoded, wantErr := json.Marshal(local)
			got, gotErr := localDigest(tc.doc)
			if (wantErr == nil) != (gotErr == nil) {
				t.Fatalf("encode acceptance changed: original=%v streaming=%v", wantErr, gotErr)
			}
			if wantErr != nil {
				if got != "" {
					t.Fatal("invalid context returned a partial digest")
				}
				return
			}
			if want := fmt.Sprintf("%x", sha256.Sum256(encoded)); got != want {
				t.Fatalf("local digest changed: %s != %s", got, want)
			}
		})
	}
}

func TestMalformedStreamingEvidenceNeverProducesPreparedCapability(t *testing.T) {
	valid := artifact(t, valueFixture(t))
	for _, suffix := range []string{`{}`, `null`, `true`, `"another"`, `garbage`} {
		t.Run(suffix, func(t *testing.T) {
			var p Preparer
			broken := Artifact{JSON: valid.JSON + suffix, Digest: hash(valid.JSON + suffix)}
			for range 2 {
				got, err := p.Prepare(broken)
				if err == nil || got.value != nil {
					t.Fatal("trailing input produced a prepared capability")
				}
			}
			if _, err := p.Prepare(valid); err != nil {
				t.Fatalf("failed parse poisoned the preparer: %v", err)
			}
		})
	}
	brokenBody := valueFixture(t)
	brokenBody.Inventory[0].BodyJSON += "{}"
	brokenBody.Inventory[0].Digest = hash(brokenBody.Inventory[0].BodyJSON)
	brokenSource := valueFixture(t)
	source := artifact(t, restoreContext(t, "source"))
	source.JSON += "{}"
	source.Digest = hash(source.JSON)
	brokenSource.Sources = []Source{Source(source)}
	brokenSource.SourceRefs = []ContextReference{{ProjectID: "source", Version: "1", Digest: source.Digest}}
	for _, d := range []Document{brokenBody, brokenSource} {
		var p Preparer
		for range 2 {
			got, err := p.Prepare(artifact(t, d))
			if err == nil || got.value != nil {
				t.Fatal("nested trailing input produced a prepared capability")
			}
		}
		if _, err := p.Prepare(valid); err != nil {
			t.Fatalf("failed nested parse poisoned the preparer: %v", err)
		}
	}
}

func FuzzStreamingDocumentDecodeParity(f *testing.F) {
	f.Add(`{"format_version":1,"project_id":"p","version":"1"}`)
	f.Add(`{"elements":[{"original":{"n":9007199254740993}}],"inventory":[],"sources":null}`)
	f.Add(`{"FORMAT_VERSION":1,"format_version":2} {}`)
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 1024*1024 {
			t.Skip("bounded codec fuzz input")
		}
		checkDecodeParity[Document](t, raw)
	})
}
