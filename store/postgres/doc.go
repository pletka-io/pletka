// Package postgres holds the shared PostgreSQL connection plumbing
// used by every store/<subsystem> implementation: pgx pool
// configuration, transaction helpers, and migration runner wiring.
//
// store/postgres depends on domain/ for shared types but knows
// nothing about which subsystem (weave, ontology, ...) it is serving.
package postgres
