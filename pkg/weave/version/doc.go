// Package version serves the platform's build-identity probe at
// GET /version. This package is a handler-only module: it owns no store,
// only reading pkg/buildinfo, goose_db_version, and the frontend asset
// manifest at request time. The footer's "copy build info" button reads
// this endpoint so user-test bug reports can include a bisect-friendly
// fingerprint:
//
//   git commit · build time · migration version · frontend manifest hash · instance
//
// Public, no auth — the same way /healthz is public. The blast radius
// of leaking the commit hash is zero (already visible in any deployed
// binary's section data).
//
// Sources:
//
//   pkg/buildinfo  — Version, GitCommit, GitBranch, GitDirty, BuildTime
//                    (populated at build via `-ldflags -X`).
//   goose_db_version — current schema version, queried at boot and
//                    refreshed on `?refresh=1`.
//   pkg/assets/static/dist/.vite/manifest.json — frontend bundle
//                    fingerprint, read at boot.
//   version.Host.Instance — deployment instance name from app wiring.
package version
