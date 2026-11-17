package versionaudit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var sample = []Finding{
	{Rule: "L5", ProjectID: "AA", Subject: "GG", Detail: "reached at more than one selector: 1.0.0, 2.0.0"},
	{Rule: "L8", ProjectID: "BB", Subject: "PP", Detail: "legacy parent column without a link row"},
	{Rule: "L8", ProjectID: "CC", Subject: "PP", Detail: "legacy parent column without a link row"},
}

func TestFilter(t *testing.T) {
	if diff := cmp.Diff(sample, Filter(sample, nil)); diff != "" {
		t.Errorf("no rules must keep everything (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(sample[1:], Filter(sample, []string{"L8"})); diff != "" {
		t.Errorf("Filter L8 (-want +got):\n%s", diff)
	}
	if got := Filter(sample, []string{"S1"}); len(got) != 0 {
		t.Errorf("Filter S1 = %v, want none", got)
	}
}

func TestUnknownRules(t *testing.T) {
	if got := UnknownRules([]string{"L5", "A2"}); len(got) != 0 {
		t.Errorf("UnknownRules(known) = %v, want none", got)
	}
	if diff := cmp.Diff([]string{"Z9", "l5"}, UnknownRules([]string{"L5", "Z9", "l5"})); diff != "" {
		t.Errorf("UnknownRules (-want +got):\n%s", diff)
	}
}

func TestWriteJSON_EmptyIsAnArray(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Errorf("WriteJSON(nil) = %q, want []", got)
	}
}

func TestWriteText(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteText(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "no findings" {
		t.Errorf("WriteText(nil) = %q, want %q", got, "no findings")
	}

	buf.Reset()
	if err := WriteText(&buf, sample); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"L5    1", "L8    2", "BB", "legacy parent column without a link row"} {
		if !strings.Contains(out, want) {
			t.Errorf("WriteText output lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "L5    1") > strings.Index(out, "RULE  PROJECT") {
		t.Errorf("counts must come before the findings:\n%s", out)
	}
}

func TestWrite_Grandfathered(t *testing.T) {
	findings := []Finding{
		{Rule: "O2", ProjectID: "AA", Subject: "crm", Detail: "links more than one version itself: 6.2, 7.1.3", Grandfathered: "ontology-mixed"},
		{Rule: "O2", ProjectID: "BB", Subject: "crm", Detail: "links more than one version itself: 6.2, 7.1.3"},
	}
	var buf bytes.Buffer
	if err := WriteText(&buf, findings); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"O2    2         1", "7.1.3 [grandfathered: ontology-mixed]"} {
		if !strings.Contains(out, want) {
			t.Errorf("WriteText output lacks %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "[grandfathered"); n != 1 {
		t.Errorf("%d lines marked grandfathered, want 1:\n%s", n, out)
	}

	buf.Reset()
	if err := WriteJSON(&buf, findings); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), `"grandfathered"`); n != 1 {
		t.Errorf("JSON names grandfathered %d times, want 1:\n%s", n, buf.String())
	}
}
