// Package domain holds shared primitives used by every subsystem of
// Pletka — Translations, ULID generation, identity types, common
// errors, and the cross-cutting BaseModel / VersionedEntity
// foundations.
//
// Subsystems live in their own subpackages (domain/weave,
// domain/ontology). Each subpackage imports github.com/pletka-io/pletka/domain
// for these shared types but never imports another subsystem.
//
// This package depends on nothing else inside Pletka. It is the
// pure-types foundation: no HTTP, no DB driver glue, no JSON shape.
//
// Subpackages may declare Store interfaces (e.g.
// domain/weave/store.go) that the store package then implements.
// Keeping the interface here lets handlers, CLI tools, importers,
// and tests depend only on domain — never on a concrete persistence
// implementation.
package domain
