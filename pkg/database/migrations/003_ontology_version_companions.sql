-- +goose Up
-- Companion RDF sources (e.g. CIDOC-CRM's PC property-classes module) were
-- parsed into class/property rows at import time but their raw bytes were
-- never persisted — only the primary file lands in
-- weave_ontology_versions.rdf_content. A self-contained git snapshot must
-- vendor every source file an ontology version was built from, or a restore
-- silently loses the companion's terms (crm PC14/P14.1 etc.).
CREATE TABLE weave_ontology_version_companions (
    ontology_version_id char(26) NOT NULL REFERENCES weave_ontology_versions(id) ON DELETE CASCADE,
    filename text NOT NULL,
    description text NOT NULL DEFAULT '',
    content text NOT NULL,
    position integer NOT NULL DEFAULT 0,
    PRIMARY KEY (ontology_version_id, filename)
);

-- +goose Down
DROP TABLE weave_ontology_version_companions;
