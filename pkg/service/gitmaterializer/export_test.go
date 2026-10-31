//go:build integration

package gitmaterializer

import "context"

// BuildReleaseTree exposes buildReleaseTree to the external integration test
// package so the determinism assertion can build a release snapshot tree twice
// and diff the two directories byte-for-byte.
func (m *Materializer) BuildReleaseTree(ctx context.Context, dstDir, projectID, version string) error {
	return m.buildReleaseTree(ctx, dstDir, projectID, version)
}

// BuildLegacyReleaseTree exercises the stored schema1/2 verification adapter.
func (m *Materializer) BuildLegacyReleaseTree(ctx context.Context, dstDir, projectID, version string) error {
	return m.buildLegacyReleaseTree(ctx, dstDir, projectID, version)
}
