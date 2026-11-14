// Package versionaudit lists where an instance's data breaks the versioning
// and inheritance rules (docs-oss/architecture/versioning-and-inheritance.md).
// It only reads. It is the first step of enforcing a rule: count what exists
// before refusing it.
package versionaudit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
)

// Finding is one place where the data breaks a rule.
type Finding struct {
	// Rule is the rule id, e.g. "L5".
	Rule string `json:"rule"`
	// ProjectID is the project the finding belongs to.
	ProjectID string `json:"project_id"`
	// Subject names what the finding is about inside that project.
	Subject string `json:"subject"`
	// Detail is one sentence for the reader.
	Detail string `json:"detail"`
}

// Run executes every check and returns the findings ordered by rule, project
// and subject. A query that fails stops the run: a partial report would read
// as "nothing found" for the checks that did not run.
func Run(ctx context.Context, q *sqlcgen.Queries) ([]Finding, error) {
	var out []Finding
	add := func(rule, project, subject, detail string) {
		out = append(out, Finding{Rule: rule, ProjectID: project, Subject: subject, Detail: detail})
	}

	links, err := q.WeaveVersionAuditLinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit parent links: %w", err)
	}
	for _, r := range links {
		add(r.Rule, r.ProjectID, r.Subject, r.Detail)
	}
	closures, err := q.WeaveVersionAuditClosures(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit closures: %w", err)
	}
	for _, r := range closures {
		add(r.Rule, r.ProjectID, r.Subject, r.Detail)
	}
	shared, err := q.WeaveVersionAuditSharedClosureRows(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit shared closure rows: %w", err)
	}
	for _, r := range shared {
		add(r.Rule, r.ProjectID, r.Subject, r.Detail)
	}
	content, err := q.WeaveVersionAuditContent(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit content: %w", err)
	}
	for _, r := range content {
		add(r.Rule, r.ProjectID, r.Subject, r.Detail)
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.ProjectID != b.ProjectID {
			return a.ProjectID < b.ProjectID
		}
		return a.Subject < b.Subject
	})
	return out, nil
}

// Filter keeps the findings of the given rules; no rules keeps everything.
func Filter(findings []Finding, rules []string) []Finding {
	if len(rules) == 0 {
		return findings
	}
	keep := make(map[string]bool, len(rules))
	for _, r := range rules {
		keep[r] = true
	}
	out := make([]Finding, 0, len(findings))
	for _, f := range findings {
		if keep[f.Rule] {
			out = append(out, f)
		}
	}
	return out
}

// WriteJSON writes the findings as a JSON array; an empty report is "[]".
func WriteJSON(w io.Writer, findings []Finding) error {
	if findings == nil {
		findings = []Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(findings)
}

// WriteText writes a count per rule, then one line per finding.
func WriteText(w io.Writer, findings []Finding) error {
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "no findings")
		return err
	}
	counts := map[string]int{}
	var rules []string
	for _, f := range findings {
		if counts[f.Rule] == 0 {
			rules = append(rules, f.Rule)
		}
		counts[f.Rule]++
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "RULE\tFINDINGS")
	for _, r := range rules {
		fmt.Fprintf(tw, "%s\t%d\n", r, counts[r])
	}
	fmt.Fprintln(tw)
	fmt.Fprintln(tw, "RULE\tPROJECT\tSUBJECT\tDETAIL")
	for _, f := range findings {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", f.Rule, f.ProjectID, f.Subject, f.Detail)
	}
	return tw.Flush()
}
