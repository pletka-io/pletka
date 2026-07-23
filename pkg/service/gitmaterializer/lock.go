package gitmaterializer

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// acquireProjectLock serializes git work on <baseDir>/<projectID> across
// processes (server drain loop vs CLI) via a blocking exclusive flock on a
// sibling lock file. Callers with baseDir=="" (restore paths that operate on
// caller-supplied dirs) must not call this.
func acquireProjectLock(baseDir, projectID string) (func(), error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create git base dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(baseDir, projectID+".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open project lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flock project %s: %w", projectID, err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
