// Package admin serves the global admin schema shell that the frontend
// island renders; slice-owned admin content (e.g. actoradmin,
// gitrestoreadmin) is mounted elsewhere and only contributes sections to
// this shell. This package is a handler-only module: it composes sections
// contributed by other slices and owns no store.
package admin
