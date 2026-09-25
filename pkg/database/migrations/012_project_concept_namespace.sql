-- +goose Up
-- F4 (#3599): a project may set the IRI namespace that its local concept
-- curies (pletka:concept/{ULID}) expand to on export (SKOS, future RDF).
-- NULL/empty falls back to the platform default at emit time.
ALTER TABLE weave_projects ADD COLUMN IF NOT EXISTS concept_namespace text;

-- +goose Down
ALTER TABLE weave_projects DROP COLUMN IF EXISTS concept_namespace;
