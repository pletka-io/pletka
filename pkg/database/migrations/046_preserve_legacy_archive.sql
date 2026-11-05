-- +goose Up
-- The legacy rebuild's preserve mode keeps a release's archived rows and only
-- rebuilds its evidence. It records a migration 041 import-ledger row BEFORE
-- finalization, and that row must carry the receipt the 036/041 finalizer will
-- compute. These read-only functions are that computation over archive rows
-- alone: the same section registry, the same per-section row hash, the same
-- session settings (to_jsonb renders timestamps under them).
-- +goose StatementBegin
CREATE FUNCTION weave_legacy_archive_sections(project text, release_version text) RETURNS jsonb LANGUAGE plpgsql STABLE
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE s record; total bigint; digest text; sections jsonb:='{}';
BEGIN
    FOR s IN SELECT * FROM weave_content_sections_v1() ORDER BY archive_table LOOP
        EXECUTE format('SELECT count(*),encode(sha256(convert_to(COALESCE(string_agg(body::text,E''\n'' ORDER BY body::text COLLATE "C"),''''),''UTF8'')),''hex'') FROM (%s) r',
            weave_content_archive_sql_v1(s.archive_table,s.owner_column,s.parent_table,s.parent_column))
            INTO total,digest USING project,release_version;
        sections:=sections||jsonb_build_object(s.archive_table,jsonb_build_object('rows',total,'sha256',digest));
    END LOOP;
    RETURN sections;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION weave_legacy_archive_receipt(project text, release_version text) RETURNS jsonb LANGUAGE plpgsql STABLE
SET search_path=public,pg_temp SET timezone='UTC' SET datestyle='ISO, YMD' AS $$
DECLARE snapshot_digest text;
BEGIN
    SELECT digest INTO snapshot_digest FROM weave_legacy_ontology_snapshots
        WHERE project_id=project AND version_number=release_version;
    IF snapshot_digest IS NULL THEN
        RAISE EXCEPTION 'legacy archive receipt requires the captured ontology snapshot of %@%',project,release_version USING ERRCODE='23514';
    END IF;
    RETURN jsonb_build_object('format_version',2,'sections',weave_legacy_archive_sections(project,release_version),
        'evidence_kind','legacy_ontology_snapshot','ontology_snapshot_digest',snapshot_digest);
END $$;
-- +goose StatementEnd

-- Archived project rows cut before migrations 031/032 never received the
-- concept-policy columns those migrations added. Only unsealed releases
-- (archive ownership 0) can still have NULLs, so the content guard permits the
-- update. The effective namespace is the function's default for an absent
-- concept namespace and lists were not enforced before the feature existed:
-- the only values a release cut then could have carried.
UPDATE weave_projects_archive SET effective_concept_namespace = weave_effective_concept_namespace_v1(concept_namespace)
WHERE effective_concept_namespace IS NULL;
UPDATE weave_projects_archive SET enforce_concept_lists = false
WHERE enforce_concept_lists IS NULL;

-- +goose Down
DROP FUNCTION weave_legacy_archive_receipt(text,text),weave_legacy_archive_sections(text,text);
