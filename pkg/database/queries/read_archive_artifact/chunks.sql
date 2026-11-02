-- name: ReadLegacyArtifactChunks :many
-- Keep the original toasted text in the CTE. Materializing convert_to here would
-- spill the whole artifact at small work_mem settings. Convert it once below.
WITH payload AS MATERIALIZED (
    SELECT s.document, s.digest
    FROM weave_legacy_ontology_snapshots s
    JOIN weave_releases r ON r.project_id=s.project_id AND r.version=s.version_number
    WHERE s.project_id=sqlc.arg(project_id)::text AND s.version_number=sqlc.arg(version)::text
      AND r.content_capture_version=2
      AND s.digest=r.content_capture_receipt->>'ontology_snapshot_digest'
), metadata AS MATERIALIZED (
    SELECT digest,
           (CASE WHEN current_setting('server_encoding')='UTF8'
                 THEN octet_length(document)
                 ELSE octet_length(convert_to(document,'UTF8')) END)::integer AS byte_length
    FROM payload
)
SELECT COALESCE((SELECT digest FROM metadata),'')::text AS digest,
       COALESCE((SELECT byte_length FROM metadata),0)::integer AS byte_length,
       offsets.byte_offset::integer AS byte_offset,
       substring((SELECT convert_to(document,'UTF8') FROM payload)
                 FROM offsets.byte_offset FOR sqlc.arg(chunk_bytes)::integer)::bytea AS chunk
FROM (
    -- This barrier sorts only integers before projecting the byte chunks. The
    -- outer ORDER BY retains the ordering guarantee without sorting payloads.
    SELECT pos AS byte_offset
    FROM generate_series(1,(SELECT CASE WHEN byte_length=0 THEN 1 ELSE byte_length END FROM metadata),
                         sqlc.arg(chunk_bytes)::integer) pos
    ORDER BY pos OFFSET 0
) offsets
ORDER BY offsets.byte_offset;
