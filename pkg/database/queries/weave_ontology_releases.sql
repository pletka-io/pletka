-- name: WeaveOntologyPublicationBody :one
SELECT COALESCE(body_text,body::text)::text AS body_json, digest FROM weave_ontology_body_seals
WHERE ontology_id = @ontology_id::text AND version_id = @version_id::text;

-- name: WeaveOntologyPublicationCandidates :many
-- Only the publisher can consult current policy. Include unpublished versions
-- so a selected but unverified active/version IRI fails instead of disappearing.
SELECT o.id AS ontology_id, o.prefix, COALESCE(b.body#>>'{ontology,namespace}', o.namespace)::text AS namespace,
       v.id AS version_id, (p.version_id IS NOT NULL)::boolean AS is_active, v.ontology_uri, v.version_iri,
       r.version_id AS published_version_id, r.digest
FROM weave_ontologies o
LEFT JOIN weave_ontology_versions v ON v.ontology_id = o.id
LEFT JOIN weave_ontology_active_versions p ON p.ontology_id=o.id AND p.version_id=v.id
LEFT JOIN weave_ontology_releases r ON r.version_id = v.id
LEFT JOIN weave_ontology_body_seals b ON b.version_id = v.id
ORDER BY o.id, v.id;

-- name: WeaveOntologyPublicationSource :one
SELECT artifact, digest FROM weave_ontology_releases
WHERE ontology_id = @ontology_id::text AND version_id = @version_id::text;

-- name: WeavePublishOntology :exec
INSERT INTO weave_ontology_releases(ontology_id, version_id, artifact, digest)
VALUES (@ontology_id, @version_id, @artifact, 'computed by trigger');
