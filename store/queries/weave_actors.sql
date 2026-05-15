-- name: WeaveCreateActor :one
INSERT INTO weave_actors (id, type, display_name, system_name, slug, first_name, last_name, acronym, email, country, website, orcid, role, parent_id, staging_id, visibility, created_by_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (id) DO UPDATE SET
    type = EXCLUDED.type,
    display_name = EXCLUDED.display_name,
    system_name = EXCLUDED.system_name,
    slug = EXCLUDED.slug,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    acronym = EXCLUDED.acronym,
    email = COALESCE(EXCLUDED.email, weave_actors.email),
    country = EXCLUDED.country,
    website = EXCLUDED.website,
    orcid = COALESCE(EXCLUDED.orcid, weave_actors.orcid),
    role = EXCLUDED.role,
    parent_id = EXCLUDED.parent_id,
    visibility = EXCLUDED.visibility,
    updated_at = NOW()
RETURNING *;

-- name: WeaveGetActorByID :one
SELECT * FROM weave_actors WHERE id = $1;

-- name: WeaveGetActorBySystemName :one
SELECT * FROM weave_actors WHERE system_name = $1;

-- name: WeaveGetActorBySlug :one
SELECT * FROM weave_actors WHERE slug = $1;

-- name: WeaveListActors :many
SELECT * FROM weave_actors ORDER BY type, display_name;

-- name: WeaveListActorsByType :many
SELECT * FROM weave_actors WHERE type = $1 ORDER BY display_name;

-- name: WeaveListOrganizations :many
SELECT wa.*,
       COALESCE(p.project_count, 0)::bigint AS project_count
FROM weave_actors wa
LEFT JOIN (
    SELECT owner_id, COUNT(*) AS project_count
    FROM weave_projects
    GROUP BY owner_id
) p ON p.owner_id = wa.id
WHERE wa.type = 'organization'
  AND (@search::text = ''
       OR wa.display_name ILIKE '%' || @search::text || '%'
       OR wa.slug ILIKE '%' || @search::text || '%'
       OR COALESCE(wa.acronym, '') ILIKE '%' || @search::text || '%')
  AND (
       COALESCE(cardinality(@country_codes::text[]), 0) = 0
       OR wa.country = ANY(@country_codes::text[])
  )
  AND (
       @include_private::boolean
       OR wa.visibility = 'public'
       OR wa.id = ANY(@readable_org_ids::text[])
  )
ORDER BY
    CASE WHEN @sort_by::text = 'display_name' AND NOT @sort_desc::boolean THEN wa.display_name END ASC NULLS LAST,
    CASE WHEN @sort_by::text = 'display_name' AND @sort_desc::boolean THEN wa.display_name END DESC NULLS LAST,
    CASE WHEN @sort_by::text = 'slug' AND NOT @sort_desc::boolean THEN wa.slug END ASC NULLS LAST,
    CASE WHEN @sort_by::text = 'slug' AND @sort_desc::boolean THEN wa.slug END DESC NULLS LAST,
    wa.display_name,
    wa.id
LIMIT @result_limit::integer OFFSET @result_offset::integer;

-- name: WeaveCountOrganizations :one
SELECT COUNT(*)
FROM weave_actors wa
WHERE wa.type = 'organization'
  AND (@search::text = ''
       OR wa.display_name ILIKE '%' || @search::text || '%'
       OR wa.slug ILIKE '%' || @search::text || '%'
       OR COALESCE(wa.acronym, '') ILIKE '%' || @search::text || '%')
  AND (
       COALESCE(cardinality(@country_codes::text[]), 0) = 0
       OR wa.country = ANY(@country_codes::text[])
  )
  AND (
       @include_private::boolean
       OR wa.visibility = 'public'
       OR wa.id = ANY(@readable_org_ids::text[])
  );

-- name: WeaveListOrganizationCountries :many
-- Returns distinct country codes that have at least one organization
-- visible under the caller's auth scope. Used to populate the country
-- filter dropdown — only countries with actual org rows show up so
-- the dropdown stays meaningful.
SELECT DISTINCT wa.country AS country
FROM weave_actors wa
WHERE wa.type = 'organization'
  AND wa.country IS NOT NULL
  AND wa.country <> ''
  AND (
       @include_private::boolean
       OR wa.visibility = 'public'
       OR wa.id = ANY(@readable_org_ids::text[])
  )
ORDER BY country ASC;

-- name: WeaveLinkProjectActor :exec
INSERT INTO weave_project_actors (project_id, actor_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (project_id, actor_id, role) DO NOTHING;

-- name: WeaveListProjectActors :many
SELECT wa.*, wpa.role AS project_role
FROM weave_actors wa
JOIN weave_project_actors wpa ON wpa.actor_id = wa.id
WHERE wpa.project_id = $1
ORDER BY wpa.role, wa.display_name;
