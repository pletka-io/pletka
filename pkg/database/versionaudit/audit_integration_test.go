//go:build integration

package versionaudit_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/pletka-io/pletka/internal/testdb"
	"github.com/pletka-io/pletka/pkg/database/sqlcgen"
	"github.com/pletka-io/pletka/pkg/database/versionaudit"
)

// TestRun_ReportsEachRule seeds one violation per check and asserts the audit
// names it, and that a healthy neighbour is not reported.
func TestRun_ReportsEachRule(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}
	t.Cleanup(func() {
		bg := context.Background()
		for _, sql := range []string{
			`DELETE FROM weave_adoptions_archive WHERE project_id LIKE 'ZVA%'`,
			`DELETE FROM weave_adoptions WHERE project_id LIKE 'ZVA%'`,
			`DELETE FROM weave_categories WHERE project_id LIKE 'ZVA%'`,
			`DELETE FROM weave_project_ontology_versions WHERE project_id LIKE 'ZVA%'`,
			`DELETE FROM weave_ontology_versions WHERE id LIKE 'ZVAONT%'`,
			`DELETE FROM weave_ontologies WHERE id = 'ZVAONT'`,
			`DELETE FROM weave_project_inheritance_archive WHERE project_id LIKE 'ZVA%'`,
			`DELETE FROM weave_project_inheritance WHERE project_id LIKE 'ZVA%'`,
			`DELETE FROM weave_releases WHERE project_id LIKE 'ZVA%'`,
			`UPDATE weave_projects SET parent_project_id = NULL WHERE id LIKE 'ZVA%'`,
			`DELETE FROM weave_projects WHERE id LIKE 'ZVA%'`,
			`DELETE FROM weave_actors WHERE id = 'ZVA_OWNER'`,
		} {
			_, _ = pool.Exec(bg, sql)
		}
	})

	exec(`INSERT INTO weave_actors (id, display_name, slug) VALUES ('ZVA_OWNER', 'ZVA_OWNER', 'ZVA_OWNER')`)
	for _, id := range []string{
		"ZVAP", "ZVAQ", "ZVAG", // parents
		"ZVAL8", "ZVAL7", "ZVAL1", "ZVAL3", // link checks
		"ZVACA", "ZVACB", // cycle through the legacy column
		"ZVAD", "ZVAA", "ZVAB", // diamond on ZVAG, ontology conflict
		"ZVAR", "ZVAS", // releases
		"ZVAX",         // receipts without a link
		"ZVAPUB",       // public, never released
		"ZVAK", "ZVAJ", // cycle through a pinned release
	} {
		exec(`INSERT INTO weave_projects (id, owner_id) VALUES ($1, 'ZVA_OWNER')`, id)
	}
	exec(`INSERT INTO weave_releases (project_id, version, created_by_id) VALUES
		('ZVAG', '1.0.0', 'ZVA_OWNER'), ('ZVAG', '2.0.0', 'ZVA_OWNER'),
		('ZVAR', '1.0.0', 'ZVA_OWNER'), ('ZVAS', '1.0.0', 'ZVA_OWNER'),
		('ZVAJ', '1.0.0', 'ZVA_OWNER')`)
	exec(`INSERT INTO weave_releases (project_id, version, created_by_id, archived_at) VALUES ('ZVAP', '0.9.0', 'ZVA_OWNER', now())`)

	// L8: legacy column, no link row. L7: legacy column disagrees with the primary link.
	exec(`UPDATE weave_projects SET parent_project_id = 'ZVAP' WHERE id IN ('ZVAL8', 'ZVAL7')`)
	exec(`INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, source_mode) VALUES ('ZVAL7', 'ZVAQ', true, 'draft')`)
	// L1: pinned to a release that does not exist. L3: pinned to a withdrawn one.
	exec(`INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, source_mode, source_version) VALUES
		('ZVAL1', 'ZVAP', true, 'release', '9.9.9'), ('ZVAL3', 'ZVAP', true, 'release', '0.9.0')`)
	// L4: a cycle the link-table trigger cannot see, through the legacy column.
	exec(`UPDATE weave_projects SET parent_project_id = 'ZVACB' WHERE id = 'ZVACA'`)
	exec(`UPDATE weave_projects SET parent_project_id = 'ZVACA' WHERE id = 'ZVACB'`)
	// L5: ZVAD follows ZVAA and ZVAB, which pin ZVAG at different releases.
	exec(`INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, source_mode, source_version) VALUES
		('ZVAD', 'ZVAA', true, 'draft', NULL), ('ZVAD', 'ZVAB', false, 'draft', NULL),
		('ZVAA', 'ZVAG', true, 'release', '1.0.0'), ('ZVAB', 'ZVAG', true, 'release', '2.0.0')`)
	// R5: a release whose recorded chain follows a draft.
	exec(`INSERT INTO weave_project_inheritance_archive (project_id, parent_project_id, is_primary, version_number, source_mode) VALUES ('ZVAR', 'ZVAP', true, '1.0.0', 'draft')`)
	// R3: ZVAS 1.0.0 reaches ZVAG, which has its own 1.0.0: both wrote ZVAG's rows under "1.0.0".
	exec(`INSERT INTO weave_project_inheritance_archive (project_id, parent_project_id, is_primary, version_number, source_mode, source_version) VALUES ('ZVAS', 'ZVAG', true, '1.0.0', 'release', '1.0.0')`)
	// A2: receipts that disagree with the closure, in a draft and in a release.
	exec(`INSERT INTO weave_adoptions (project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id, source_version) VALUES
		('ZVAA', 'project', 'ZVAA', 'field', 'ZVAG', 'ZVAGF.1', '2.0.0'),
		('ZVAA', 'collection', 'ZVAAC.1', 'field', 'ZVAG', 'ZVAGF.1', '2.0.0'),
		('ZVAB', 'project', 'ZVAB', 'field', 'ZVAG', 'ZVAGF.1', '2.0.0')`)
	exec(`INSERT INTO weave_adoptions_archive (project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id, source_version, adopted_at, version_number) VALUES
		('ZVAS', 'project', 'ZVAS', 'field', 'ZVAG', 'ZVAGF.1', '2.0.0', now(), '1.0.0')`)
	// A4 and the duplicate: ZVAX has no link to ZVAG and holds one field twice.
	exec(`INSERT INTO weave_adoptions (project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id, source_version) VALUES
		('ZVAX', 'project', 'ZVAX', 'field', 'ZVAG', 'ZVAGF.2', '1.0.0'),
		('ZVAX', 'project', 'ZVAX', 'field', 'ZVAG', 'ZVAGF.2', '2.0.0')`)
	// O2: ZVAA and ZVAB link different versions of one ontology; ZVAD links none.
	exec(`INSERT INTO weave_ontologies (id, prefix, namespace, name) VALUES ('ZVAONT', 'zvaont', 'https://example.org/zvaont/', 'ZVA test ontology')`)
	exec(`INSERT INTO weave_ontology_versions (id, ontology_id, version_string) VALUES ('ZVAONT_1', 'ZVAONT', '1'), ('ZVAONT_2', 'ZVAONT', '2')`)
	exec(`INSERT INTO weave_project_ontology_versions (project_id, ontology_version_id, added_at) VALUES ('ZVAA', 'ZVAONT_1', now()), ('ZVAB', 'ZVAONT_2', now())`)
	// S1: public, never released.
	exec(`UPDATE weave_projects SET visibility = 'public' WHERE id = 'ZVAPUB'`)
	// A8: two categories copied from ZVAP into ZVAL7, one edited since.
	exec(`INSERT INTO weave_categories (id, semantic_id, system_name, ui_name, project_id) VALUES
		('ZVAP.CAT.1', 'ZVAP.CAT.1', 'dating', '{"en":"Dating"}', 'ZVAP'),
		('ZVAP.CAT.2', 'ZVAP.CAT.2', 'place', '{"en":"Place"}', 'ZVAP'),
		('ZVAL7.CAT.1', 'ZVAL7.CAT.1', 'dating', '{"en":"Dating"}', 'ZVAL7'),
		('ZVAL7.CAT.2', 'ZVAL7.CAT.2', 'place', '{"en":"Location"}', 'ZVAL7')`)
	exec(`INSERT INTO weave_adoptions (project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id) VALUES
		('ZVAL7', 'project', 'ZVAL7', 'category', 'ZVAP', 'ZVAP.CAT.1'),
		('ZVAL7', 'project', 'ZVAL7', 'category', 'ZVAP', 'ZVAP.CAT.2')`)

	// L4 through a pin: ZVAK pins ZVAJ 1.0.0, whose recorded chain leads back to ZVAK.
	exec(`INSERT INTO weave_project_inheritance (project_id, parent_project_id, is_primary, source_mode, source_version) VALUES ('ZVAK', 'ZVAJ', true, 'release', '1.0.0')`)
	exec(`INSERT INTO weave_project_inheritance_archive (project_id, parent_project_id, is_primary, version_number, source_mode) VALUES ('ZVAJ', 'ZVAK', true, '1.0.0', 'draft')`)
	// A4 healthy neighbour: ZVAL8's only parent is the legacy column, and it adopts from that parent.
	exec(`INSERT INTO weave_adoptions (project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id) VALUES
		('ZVAL8', 'project', 'ZVAL8', 'field', 'ZVAP', 'ZVAPF.1')`)
	// A8: two local categories carry the name of one copied category.
	exec(`INSERT INTO weave_categories (id, semantic_id, system_name, ui_name, project_id) VALUES
		('ZVAP.CAT.3', 'ZVAP.CAT.3', 'find', '{"en":"Find"}', 'ZVAP'),
		('ZVAL7.CAT.3', 'ZVAL7.CAT.3', 'find', '{"en":"Find"}', 'ZVAL7'),
		('ZVAL7.CAT.4', 'ZVAL7.CAT.4', 'find', '{"en":"Finds"}', 'ZVAL7')`)
	exec(`INSERT INTO weave_adoptions (project_id, context_entity_type, context_entity_id, entity_type, source_project_id, source_entity_id) VALUES
		('ZVAL7', 'project', 'ZVAL7', 'category', 'ZVAP', 'ZVAP.CAT.3')`)

	// As the command runs it: one read-only transaction.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	all, err := versionaudit.Run(ctx, sqlcgen.New(tx))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got []versionaudit.Finding
	for _, f := range all {
		if strings.HasPrefix(f.ProjectID, "ZVA") {
			got = append(got, f)
		}
	}
	has := func(rule, project, subject, detail string) bool {
		for _, f := range got {
			if f.Rule == rule && f.ProjectID == project && strings.Contains(f.Subject, subject) && strings.Contains(f.Detail, detail) {
				return true
			}
		}
		return false
	}

	for _, want := range []struct{ rule, project, subject, detail string }{
		{"L8", "ZVAL8", "ZVAP", "legacy parent column without a link row"},
		{"L7", "ZVAL7", "ZVAQ", "legacy parent column says ZVAP"},
		{"L1", "ZVAL1", "ZVAP@9.9.9", "does not exist"},
		{"L3", "ZVAL3", "ZVAP@0.9.0", "withdrawn"},
		{"L4", "ZVACA", "ZVACA > ZVACB > ZVACA", "cycle"},
		{"L5", "ZVAD", "ZVAG", "1.0.0, 2.0.0"},
		{"R5", "ZVAR", "1.0.0: ZVAR > ZVAP", "chain-unpinned"},
		{"R3", "ZVAG", "1.0.0", "ZVAG, ZVAS"},
		{"A2", "ZVAA", "field ZVAGF.1", "receipt says 2.0.0, the closure selects ZVAG at 1.0.0"},
		{"A2", "ZVAS", "field ZVAGF.1 in release 1.0.0", "receipt-off-selector"},
		{"A2", "ZVAX", "field ZVAGF.2", "adopted 2 times"},
		{"A4", "ZVAX", "field ZVAGF.2", "not in the closure"},
		{"O2", "ZVAD", "ZVAONT", "ZVAONT_1 via ZVAA, ZVAONT_2 via ZVAB"},
		{"S1", "ZVAPUB", "", "public project without a release"},
		{"A8", "ZVAL7", "ZVAL7.CAT.1", "unchanged copy of ZVAP.CAT.1"},
		{"A8", "ZVAL7", "ZVAL7.CAT.2", "changed copy of ZVAP.CAT.2"},
		{"A8", "ZVAL7", "ZVAL7.CAT.3", "2 categories share the name of ZVAP.CAT.3"},
		{"L4", "ZVAK", "ZVAK > ZVAJ > ZVAK", "cycle"},
	} {
		if !has(want.rule, want.project, want.subject, want.detail) {
			t.Errorf("missing finding %+v", want)
		}
	}

	// Every rule the package says it can report is seeded above, and nothing
	// is reported under an id the package does not list.
	var seen []string
	for _, f := range got {
		seen = append(seen, f.Rule)
	}
	for _, r := range versionaudit.Rules {
		if !slices.Contains(seen, r) {
			t.Errorf("rule %s is listed in Rules but no seeded finding has it", r)
		}
	}
	if unknown := versionaudit.UnknownRules(seen); len(unknown) > 0 {
		t.Errorf("findings carry rule ids missing from Rules: %v", unknown)
	}

	// Healthy neighbours: ZVAB's receipt matches its pin, and neither ZVAA nor
	// ZVAB has a diamond or an ontology conflict of its own.
	cat3, twice := 0, 0
	for _, f := range got {
		if f.Rule == "A2" && f.ProjectID == "ZVAA" {
			twice++
		}
		if f.Rule == "A8" && strings.HasSuffix(f.Detail, "ZVAP.CAT.3") {
			cat3++
		}
		switch {
		case f.Rule == "A4" && f.ProjectID == "ZVAL8",
			f.Rule == "A2" && f.ProjectID == "ZVAB",
			(f.Rule == "L5" || f.Rule == "O2") && (f.ProjectID == "ZVAA" || f.ProjectID == "ZVAB"):
			t.Errorf("unexpected finding %+v", f)
		}
	}
	if cat3 != 1 {
		t.Errorf("receipt of ZVAP.CAT.3 reported %d times, want once", cat3)
	}
	// One finding per receipt: the same entity adopted in two places is two.
	if twice != 2 {
		t.Errorf("ZVAA holds two receipts off the selector, reported %d", twice)
	}
	if t.Failed() {
		for _, f := range got {
			t.Logf("%s\t%s\t%s\t%s", f.Rule, f.ProjectID, f.Subject, f.Detail)
		}
	}
}
