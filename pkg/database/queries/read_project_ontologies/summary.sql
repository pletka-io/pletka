-- name: BrowserOwnOntologySummaries :many
WITH retained AS (
 SELECT p.ontology_version_id,(o.artifact::jsonb->>'body_json')::jsonb AS body,o.artifact::jsonb->>'body_digest' AS digest
 FROM weave_release_ontology_dependencies p
 JOIN weave_ontology_releases o ON (o.ontology_id,o.version_id,o.digest)=(p.ontology_id,p.ontology_version_id,p.digest)
 JOIN weave_releases r ON r.project_id=p.project_id AND r.version=p.version_number AND r.content_capture_version=1
 WHERE p.project_id = @project_id::text AND p.version_number = @version::text
 UNION ALL
 SELECT b->>'version_id',(b->>'body_json')::jsonb,b->>'digest' FROM weave_legacy_ontology_snapshots s
 JOIN weave_releases r ON r.project_id=s.project_id AND r.version=s.version_number AND r.content_capture_version=2
   AND s.digest=r.content_capture_receipt->>'ontology_snapshot_digest',
 LATERAL jsonb_array_elements(s.document::jsonb->'inventory') b
 WHERE s.project_id = @project_id::text AND s.version_number = @version::text
), bodies AS (
 SELECT l.ontology_version_id,r.body,r.digest FROM weave_project_ontology_versions_archive l
 LEFT JOIN retained r ON r.ontology_version_id=l.ontology_version_id
 WHERE l.project_id = @project_id::text AND l.version_number = @version::text
)
SELECT ontology_version_id,
 jsonb_build_object('digest',digest,'name',body->'ontology'->>'name','label',body->'version'->'ontology_label',
 'version',body->'version'->>'version_string','uri',body->'version'->>'ontology_uri',
 'classes',jsonb_array_length(body->'classes'),'properties',jsonb_array_length(body->'properties')) AS metadata,
 COALESCE(body->'version'->>'id'=ontology_version_id AND body->>'format_version'='1',false)::boolean AS present
FROM bodies ORDER BY ontology_version_id LIMIT sqlc.arg(row_limit)::integer;

-- name: BrowserOntologyUsage :many
WITH refs AS (
 SELECT e->>'version_id' AS version_id,e->>'uri' AS uri,e->'reference'->>'kind' AS kind
 FROM weave_release_ontology_references s JOIN weave_releases r ON r.project_id=s.project_id AND r.version=s.version_number AND r.content_capture_version=1,
 LATERAL jsonb_array_elements(s.resolutions) e
 WHERE s.project_id = @project_id::text AND s.version_number = @version::text
 UNION ALL
 SELECT e->'candidates'->0->>'version_id',e->'candidates'->0->>'uri',e->'reference'->>'kind'
 FROM weave_legacy_ontology_snapshots s JOIN weave_releases r ON r.project_id=s.project_id AND r.version=s.version_number AND r.content_capture_version=2
   AND s.digest=r.content_capture_receipt->>'ontology_snapshot_digest',
 LATERAL jsonb_array_elements(s.document::jsonb->'assessments') e
 WHERE s.project_id = @project_id::text AND s.version_number = @version::text AND e->>'status'='resolved'
)
SELECT selected.version_id::text AS version_id,count(DISTINCT uri) FILTER(WHERE kind='class')::integer AS classes,
       count(DISTINCT uri) FILTER(WHERE kind='property')::integer AS properties,
       NOT EXISTS(SELECT 1 FROM weave_legacy_ontology_snapshots s,LATERAL jsonb_array_elements(s.document::jsonb->'assessments') e
         WHERE s.project_id = @project_id::text AND s.version_number = @version::text AND e->>'status'<>'resolved') AS known
FROM unnest(sqlc.arg(ontology_versions)::text[]) selected(version_id) LEFT JOIN refs ON refs.version_id=selected.version_id
GROUP BY selected.version_id ORDER BY selected.version_id;

-- name: BrowserSummaryEvidence :one
SELECT CASE r.content_capture_version
 WHEN 1 THEN r.ontology_dependencies_version=1 AND EXISTS(SELECT 1 FROM weave_release_ontology_references e WHERE e.project_id=r.project_id AND e.version_number=r.version)
 WHEN 2 THEN EXISTS(SELECT 1 FROM weave_legacy_ontology_snapshots e WHERE e.project_id=r.project_id AND e.version_number=r.version AND e.digest=r.content_capture_receipt->>'ontology_snapshot_digest')
 ELSE false END::boolean AS present
FROM weave_releases r WHERE r.project_id = @project_id::text AND r.version = @version::text;

-- name: BrowserMatchOntologyReferences :many
WITH retained AS (
 SELECT p.ontology_version_id,(o.artifact::jsonb->>'body_json')::jsonb AS body,o.artifact::jsonb->>'body_digest' AS digest
 FROM weave_release_ontology_dependencies p
 JOIN weave_ontology_releases o ON (o.ontology_id,o.version_id,o.digest)=(p.ontology_id,p.ontology_version_id,p.digest)
 JOIN weave_releases r ON r.project_id=p.project_id AND r.version=p.version_number AND r.content_capture_version=1
 WHERE p.project_id = @project_id::text AND p.version_number = @version::text
 UNION ALL
 SELECT b->>'version_id',(b->>'body_json')::jsonb,b->>'digest' FROM weave_legacy_ontology_snapshots s
 JOIN weave_releases r ON r.project_id=s.project_id AND r.version=s.version_number AND r.content_capture_version=2
   AND s.digest=r.content_capture_receipt->>'ontology_snapshot_digest',
 LATERAL jsonb_array_elements(s.document::jsonb->'inventory') b
 WHERE s.project_id = @project_id::text AND s.version_number = @version::text
), body AS (SELECT ontology_version_id AS version_id,body AS b FROM retained WHERE ontology_version_id=ANY(sqlc.arg(ontology_versions)::text[])), terms AS (
 SELECT version_id,'class'::text AS kind,t FROM body,LATERAL jsonb_array_elements(b->'classes') t
 UNION ALL SELECT version_id,'property',t FROM body,LATERAL jsonb_array_elements(b->'properties') t
), matches AS (
 SELECT DISTINCT version_id,kind,t->>'uri' AS uri FROM terms,LATERAL jsonb_array_elements(sqlc.arg(references_json)::jsonb) AS refs(reference)
 WHERE kind=reference->>'kind' AND (COALESCE(reference->>'version_id','')='' OR reference->>'version_id'=terms.version_id)
 AND (CASE WHEN COALESCE(reference->>'uri','')<>'' THEN t->>'uri'=reference->>'uri'
 ELSE t->>'prefix'=reference->>'prefix' AND t->>'local_name'=reference->>'local_name' END)
)
SELECT selected.version_id::text AS version_id,count(DISTINCT uri) FILTER(WHERE kind='class')::integer AS classes,
       count(DISTINCT uri) FILTER(WHERE kind='property')::integer AS properties
FROM unnest(sqlc.arg(ontology_versions)::text[]) selected(version_id) LEFT JOIN matches ON matches.version_id=selected.version_id
GROUP BY selected.version_id ORDER BY selected.version_id;
