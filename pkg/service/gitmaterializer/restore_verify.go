package gitmaterializer

import "fmt"

// verifyVendorChecksums confirms every vendored ontology and vendored
// project directory tree in snapshot matches its pletka.sum entry. It
// mirrors the keying used when the sum was written by
// vendorDependenciesForProject: both vendored projects and vendored
// ontologies are keyed by module and version, recovered from the vendor
// directory layout (see vendoredProjectVersionFromDir). A vendored project
// pinned at the draft version has no version subdirectory, so its lookup
// falls back to a module-only wildcard match — genuinely unambiguous only
// when a single pletka.sum entry exists for that module; see
// findPletkaSumEntry. It returns nil when every entry matches, or a
// descriptive error naming the module (and version, where known) on the
// first missing entry, ambiguous entry, or checksum mismatch.
func verifyVendorChecksums(snapshot *ProjectSnapshot) error {
	if snapshot == nil {
		return nil
	}

	for _, dep := range snapshot.Vendor.Projects {
		if err := verifyVendorTree(snapshot.Sum, dep.Module, dep.Version, dep.Snapshot.RootDir); err != nil {
			return err
		}
	}
	for _, dep := range snapshot.Vendor.Ontologies {
		if err := verifyVendorTree(snapshot.Sum, dep.Module, dep.Version, dep.Snapshot.RootDir); err != nil {
			return err
		}
	}

	return nil
}

func verifyVendorTree(sum []pletkaSumEntry, module, version, rootDir string) error {
	entry, err := findPletkaSumEntry(sum, module, version)
	if err != nil {
		return err
	}

	key := vendorSumKey(module, version)
	hash, err := hashDirectoryTree(rootDir)
	if err != nil {
		return fmt.Errorf("hash vendored tree for %s: %w", key, err)
	}
	if hash != entry.TreeSHA {
		return fmt.Errorf("pletka.sum: checksum mismatch for %s", key)
	}

	return nil
}

// findPletkaSumEntry looks up the pletka.sum entry for module@version. When
// version is "" (the draft case, which has no version subdirectory to
// recover a version from), it matches by module alone. That module-only
// match is only unambiguous when exactly one entry exists for the module —
// pletka.sum can legitimately hold multiple versions of the same module
// (e.g. two projects each vendoring a different release of a shared
// dependency), so a module-only lookup that matches more than one entry
// cannot be resolved by picking the first match and instead returns a
// descriptive error.
func findPletkaSumEntry(sum []pletkaSumEntry, module, version string) (pletkaSumEntry, error) {
	var match pletkaSumEntry
	matched := false
	ambiguous := false
	for _, entry := range sum {
		if entry.Module != module {
			continue
		}
		if version != "" && entry.Version != version {
			continue
		}
		if matched {
			ambiguous = true
			continue
		}
		match = entry
		matched = true
	}
	if !matched {
		return pletkaSumEntry{}, fmt.Errorf("pletka.sum: no entry for %s", vendorSumKey(module, version))
	}
	if version == "" && ambiguous {
		return pletkaSumEntry{}, fmt.Errorf("pletka.sum: ambiguous entries for module %s — version required", module)
	}
	return match, nil
}

func vendorSumKey(module, version string) string {
	if version == "" {
		return module
	}
	return module + "@" + version
}
