package gitmaterializer

import "testing"

func TestVendoredProjectDir(t *testing.T) {
	draftDir := vendoredProjectDir("/tmp/root", pletkaProjectRequirement{
		Module:  "pletka.io/orgs/test/projects/LA",
		Version: parentDependencyVersionDraft,
	})
	if draftDir != "/tmp/root/vendor/projects/pletka.io/orgs/test/projects/LA" {
		t.Fatalf("unexpected draft vendor dir: %s", draftDir)
	}

	releaseDir := vendoredProjectDir("/tmp/root", pletkaProjectRequirement{
		Module:  "pletka.io/orgs/test/projects/LA",
		Version: "1.2.0",
	})
	if releaseDir != "/tmp/root/vendor/projects/pletka.io/orgs/test/projects/LA/1.2.0" {
		t.Fatalf("unexpected release vendor dir: %s", releaseDir)
	}
}
