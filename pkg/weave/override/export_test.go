//go:build integration

package override

// TestLockTimeout lets external integration tests bound real lock contention
// without changing the production API or importing fixtures into this package.
var TestLockTimeout = &lockTimeout
