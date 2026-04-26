# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
(with the v0.x.y instability promise — breaking changes are allowed within v0).

## [Unreleased]

### Added
- Initial repository scaffold: license, NOTICE, contributing/security/conduct policies.
- Top-level layout: `schema/`, `domain/`, `gitsync/`, `server/`, `renderer/`, `docs/`.
- Empty Go module at `github.com/pletka-io/pletka`.
- Empty Svelte 5 + Vite renderer wired to embed into the server binary.
- CI: build, test, lint, secret scan, DCO check.
- Release automation via goreleaser.
