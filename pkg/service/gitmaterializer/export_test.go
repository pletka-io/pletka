//go:build integration

package gitmaterializer

import (
	"context"
	"fmt"
)

// BuildReleaseTree exposes buildReleaseTree to the external integration test
// package so the determinism assertion can build a release snapshot tree twice
// and diff the two directories byte-for-byte.
func (m *Materializer) BuildReleaseTree(ctx context.Context, dstDir, projectID, version string) error {
	return m.buildReleaseTree(ctx, dstDir, projectID, version)
}

// BuildLegacyReleaseTree preserves the historical schema1/2 tree and catalog
// recipe for compatibility fixtures, for integration tests only. Production release writes use complete
// values; verification calls only the snapshot adapter with a frozen catalog.
func (m *Materializer) BuildLegacyReleaseTree(ctx context.Context, dstDir, projectID, version string) error {
	if err := m.writeLegacyReleaseSnapshot(ctx, dstDir, projectID, version); err != nil {
		return err
	}
	idx, err := m.loadReleasesIndex(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load releases index %s: %w", projectID, err)
	}
	data, err := encodeReleasesIndex(idx)
	if err != nil {
		return fmt.Errorf("encode releases index %s: %w", projectID, err)
	}
	return writeEntityFile(dstDir, releasesIndexPath, data)
}

// PrepareCapturedDraftForTest stops at the durable prepare boundary to simulate
// process loss before publishing the Git ref.
func (m *Materializer) PrepareCapturedDraftForTest(ctx context.Context, project string) error {
	_, err := m.withCapturedRepo(ctx, project, func(git *gitRunner) (int, error) {
		_, err := m.prepareDraft(ctx, git, project, nil)
		return 0, err
	})
	return err
}
