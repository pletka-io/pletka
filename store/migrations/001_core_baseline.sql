-- +goose Up

-- Clean public Pletka core baseline.
--
-- This is intentionally not a replay of the legacy private migration chain.
-- Airtable staging is included for now because current core rows still carry
-- staging provenance. It can be decoupled later behind a neutral provenance
-- model once the public/platform split is further along.

CREATE TABLE weave_import_staging (
    id             bigserial PRIMARY KEY,
    import_run     text NOT NULL,
    source_type    text NOT NULL,
    source_path    text NOT NULL,
    project_name   text NOT NULL,
    semantic_id    text,
    airtable_ref   text,
    raw_data       jsonb NOT NULL,
    status         text NOT NULL DEFAULT 'pending',
    error_msg      text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    normalized_id  text,
    path_elements  jsonb,
    ontology_scope jsonb,
    project_id     text
);

CREATE INDEX idx_wis_run ON weave_import_staging (import_run);
CREATE INDEX idx_wis_type ON weave_import_staging (source_type, project_name);
CREATE INDEX idx_wis_ref ON weave_import_staging (airtable_ref) WHERE airtable_ref IS NOT NULL;
CREATE INDEX idx_wis_semantic ON weave_import_staging (semantic_id) WHERE semantic_id IS NOT NULL;
CREATE INDEX idx_wis_status ON weave_import_staging (import_run, status);
CREATE INDEX idx_wis_source_path ON weave_import_staging (source_path);
CREATE INDEX idx_wis_normalized ON weave_import_staging (normalized_id) WHERE normalized_id IS NOT NULL;
CREATE INDEX idx_wis_project_id ON weave_import_staging (project_id) WHERE project_id IS NOT NULL;

CREATE TABLE weave_actors (
    id            text PRIMARY KEY,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    type          text NOT NULL DEFAULT 'person',
    display_name  text NOT NULL,
    system_name   text,
    first_name    text,
    last_name     text,
    acronym       text,
    email         text,
    country       text,
    website       text,
    orcid         text,
    role          text NOT NULL DEFAULT 'contributor',
    parent_id     text REFERENCES weave_actors(id),
    staging_id    bigint REFERENCES weave_import_staging(id),
    slug          text NOT NULL,
    visibility    text NOT NULL DEFAULT 'private',
    created_by_id text REFERENCES weave_actors(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX idx_wa_slug ON weave_actors (slug);
CREATE UNIQUE INDEX idx_wa_system_name ON weave_actors (system_name)
    WHERE system_name IS NOT NULL AND system_name != '';
CREATE INDEX idx_wa_type ON weave_actors (type);
CREATE INDEX idx_wa_parent ON weave_actors (parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_wa_created_by ON weave_actors (created_by_id);
CREATE INDEX idx_wa_type_visibility ON weave_actors (type, visibility);

CREATE TABLE weave_projects (
    id                    text PRIMARY KEY,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    system_name           text,
    ui_name               jsonb,
    description           jsonb,
    status                text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    namespace             text DEFAULT '',
    parent_project_id     text REFERENCES weave_projects(id),
    staging_id            bigint REFERENCES weave_import_staging(id),
    owner_id              text NOT NULL REFERENCES weave_actors(id),
    visibility            text NOT NULL DEFAULT 'private' CHECK (visibility IN ('public', 'internal', 'private')),
    deprecated            boolean NOT NULL DEFAULT false,
    license               text NOT NULL DEFAULT '',
    readme                jsonb NOT NULL DEFAULT '{}'::jsonb,
    topics                text[] NOT NULL DEFAULT '{}',
    base_url              text NOT NULL DEFAULT '',
    created_by_id         text REFERENCES weave_actors(id) ON DELETE SET NULL,
    version_number        text NOT NULL DEFAULT '',
    enforce_concept_lists boolean NOT NULL DEFAULT false,
    is_master_weave       boolean NOT NULL DEFAULT false
);

CREATE INDEX idx_wp_parent ON weave_projects (parent_project_id) WHERE parent_project_id IS NOT NULL;
CREATE INDEX idx_wp_owner ON weave_projects (owner_id);
CREATE INDEX idx_wp_created_by ON weave_projects (created_by_id);
CREATE INDEX idx_wp_release_versions ON weave_projects (id, version_number) WHERE version_number != '';
CREATE INDEX idx_wp_is_master_weave ON weave_projects (id) WHERE is_master_weave = true;

CREATE TABLE weave_project_actors (
    project_id text NOT NULL REFERENCES weave_projects(id) ON DELETE CASCADE,
    actor_id   text NOT NULL REFERENCES weave_actors(id) ON DELETE CASCADE,
    role       text NOT NULL DEFAULT 'owner',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, actor_id, role)
);

CREATE INDEX idx_wpa_actor ON weave_project_actors (actor_id);

CREATE TABLE weave_auth (
    actor_id                  text PRIMARY KEY REFERENCES weave_actors(id) ON DELETE CASCADE,
    password_hash             text NOT NULL,
    email_verified_at         timestamptz,
    password_reset_token      text,
    password_reset_expires_at timestamptz,
    last_login_at             timestamptz,
    perms_version             integer NOT NULL DEFAULT 0,
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_wauth_reset_token ON weave_auth (password_reset_token)
    WHERE password_reset_token IS NOT NULL;

CREATE TABLE weave_memberships (
    actor_id   text NOT NULL REFERENCES weave_actors(id) ON DELETE CASCADE,
    scope_type text NOT NULL CHECK (scope_type IN ('org', 'project')),
    scope_id   text NOT NULL,
    role       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_id, scope_type, scope_id)
);

CREATE INDEX idx_wm_actor ON weave_memberships (actor_id);
CREATE INDEX idx_wm_scope ON weave_memberships (scope_type, scope_id);

CREATE TABLE sessions (
    token  text PRIMARY KEY,
    data   bytea NOT NULL,
    expiry timestamptz NOT NULL
);

CREATE INDEX sessions_expiry_idx ON sessions (expiry);

CREATE TABLE weave_categories (
    id              text PRIMARY KEY,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    semantic_id     text,
    system_name     text,
    ui_name         jsonb,
    description     jsonb,
    status          text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    project_id      text NOT NULL,
    canonical_order integer NOT NULL DEFAULT 0,
    deprecated      boolean NOT NULL DEFAULT false,
    version_number  text NOT NULL DEFAULT ''
);

CREATE INDEX idx_weave_categories_project ON weave_categories (project_id);
CREATE INDEX idx_weave_categories_active ON weave_categories (project_id) WHERE deprecated = false;
CREATE INDEX idx_wcat_release_versions ON weave_categories (project_id, version_number) WHERE version_number != '';

CREATE TABLE weave_vocabularies (
    id               text PRIMARY KEY,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    semantic_id      text,
    system_name      text,
    ui_name          jsonb,
    description      jsonb,
    status           text NOT NULL DEFAULT 'draft',
    project_id       text,
    connector_type   text NOT NULL,
    base_uri         text,
    config           jsonb,
    config_encrypted bytea
);

CREATE UNIQUE INDEX idx_wv_global_system_name ON weave_vocabularies (system_name)
    WHERE project_id IS NULL AND system_name IS NOT NULL;

CREATE TABLE weave_vocabulary_entries (
    id                 text PRIMARY KEY,
    vocabulary_id      text NOT NULL REFERENCES weave_vocabularies(id) ON DELETE CASCADE,
    uri                text NOT NULL,
    label              jsonb NOT NULL,
    scope_note         jsonb,
    broader_uri        text,
    broader_path       jsonb NOT NULL DEFAULT '[]'::jsonb,
    broader_path_items jsonb NOT NULL DEFAULT '[]'::jsonb,
    external_id        text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_wve_vocabulary ON weave_vocabulary_entries (vocabulary_id);
CREATE INDEX idx_wve_uri ON weave_vocabulary_entries (uri);
CREATE INDEX idx_wve_external_id ON weave_vocabulary_entries (external_id);
CREATE UNIQUE INDEX idx_wve_vocabulary_uri_unique ON weave_vocabulary_entries (vocabulary_id, uri);
CREATE INDEX idx_wve_label_search ON weave_vocabulary_entries
    USING gin (to_tsvector('simple'::regconfig, COALESCE(label::text, '')));

CREATE TABLE weave_project_vocabularies (
    project_id       text NOT NULL REFERENCES weave_projects(id) ON DELETE CASCADE,
    vocabulary_id    text NOT NULL REFERENCES weave_vocabularies(id) ON DELETE CASCADE,
    selected_version text,
    status           text NOT NULL DEFAULT 'active',
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, vocabulary_id)
);

CREATE INDEX idx_wpv_vocabulary ON weave_project_vocabularies (vocabulary_id);

CREATE TABLE weave_concept_lists (
    id            text PRIMARY KEY,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    semantic_id   text,
    system_name   text,
    ui_name       jsonb,
    description   jsonb,
    status        text NOT NULL DEFAULT 'draft',
    project_id    text NOT NULL,
    list_type     text REFERENCES weave_vocabulary_entries(id),
    vocabulary_id text REFERENCES weave_vocabularies(id)
);

CREATE INDEX idx_wcl_project ON weave_concept_lists (project_id);
CREATE UNIQUE INDEX idx_wcl_project_semantic_unique ON weave_concept_lists (project_id, semantic_id)
    WHERE semantic_id IS NOT NULL AND semantic_id <> '';

CREATE TABLE weave_concept_list_entries (
    id                  text PRIMARY KEY,
    concept_list_id     text NOT NULL REFERENCES weave_concept_lists(id) ON DELETE CASCADE,
    vocabulary_entry_id text NOT NULL REFERENCES weave_vocabulary_entries(id),
    position            integer NOT NULL DEFAULT 0,
    custom_label        jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_wcle_list ON weave_concept_list_entries (concept_list_id);
CREATE INDEX idx_wcle_entry ON weave_concept_list_entries (vocabulary_entry_id);

CREATE TABLE weave_fields (
    id                  text PRIMARY KEY,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    semantic_id         text,
    system_name         text,
    ui_name             jsonb,
    description         jsonb,
    status              text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    project_id          text NOT NULL,
    ontology_scope      jsonb,
    ontology_path       text,
    path_elements       jsonb,
    expected_value_type text,
    examples            jsonb,
    staging_id          bigint REFERENCES weave_import_staging(id),
    deprecated          boolean NOT NULL DEFAULT false,
    version_number      text NOT NULL DEFAULT ''
);

CREATE INDEX idx_weave_fields_project ON weave_fields (project_id);
CREATE INDEX idx_weave_fields_scope ON weave_fields USING gin (ontology_scope jsonb_path_ops);
CREATE INDEX idx_weave_fields_path_elements ON weave_fields USING gin (path_elements jsonb_path_ops);
CREATE INDEX idx_weave_fields_active ON weave_fields (project_id) WHERE deprecated = false;
CREATE INDEX idx_wf_release_versions ON weave_fields (project_id, version_number) WHERE version_number != '';

CREATE TABLE weave_path_elements (
    field_id            text NOT NULL REFERENCES weave_fields(id) ON DELETE CASCADE,
    position            integer NOT NULL,
    local_name          text NOT NULL,
    type                text NOT NULL,
    prefix              text,
    class_code          text,
    ontology_version_id text,
    PRIMARY KEY (field_id, position)
);

CREATE INDEX idx_wpe_local_name ON weave_path_elements (local_name);
CREATE INDEX idx_wpe_class_code ON weave_path_elements (class_code);
CREATE INDEX idx_wpe_type_name ON weave_path_elements (type, local_name);
CREATE INDEX idx_wpe_ontology ON weave_path_elements (ontology_version_id);
CREATE INDEX idx_wpe_position_local_name ON weave_path_elements (position, local_name);

CREATE TABLE weave_models (
    id             text PRIMARY KEY,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    system_name    text,
    ui_name        jsonb,
    description    jsonb,
    status         text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    project_id     text NOT NULL,
    ontology_scope jsonb,
    staging_id     bigint REFERENCES weave_import_staging(id),
    deprecated     boolean NOT NULL DEFAULT false,
    version_number text NOT NULL DEFAULT '',
    model_type     text NOT NULL DEFAULT 'auxiliary'
);

CREATE INDEX idx_wm_project ON weave_models (project_id);
CREATE INDEX idx_weave_models_active ON weave_models (project_id) WHERE deprecated = false;
CREATE INDEX idx_wm_release_versions ON weave_models (project_id, version_number) WHERE version_number != '';
CREATE INDEX idx_wm_model_type_core ON weave_models (project_id) WHERE model_type = 'core';

CREATE TABLE weave_collections (
    id                         text PRIMARY KEY,
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now(),
    system_name                text,
    ui_name                    jsonb,
    description                jsonb,
    status                     text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    project_id                 text NOT NULL,
    ontology_scope             jsonb,
    collection_number          integer DEFAULT 0,
    canonical_collection_order integer DEFAULT 0,
    staging_id                 bigint REFERENCES weave_import_staging(id),
    deprecated                 boolean NOT NULL DEFAULT false,
    default_category_id        text REFERENCES weave_categories(id) ON DELETE SET NULL,
    version_number             text NOT NULL DEFAULT ''
);

CREATE INDEX idx_wc_project ON weave_collections (project_id);
CREATE INDEX idx_weave_collections_active ON weave_collections (project_id) WHERE deprecated = false;
CREATE INDEX idx_wc_release_versions ON weave_collections (project_id, version_number) WHERE version_number != '';
CREATE INDEX idx_weave_collections_default_category ON weave_collections (default_category_id)
    WHERE default_category_id IS NOT NULL;

CREATE TABLE weave_field_overrides (
    id                    bigserial PRIMARY KEY,
    field_id              text NOT NULL,
    project_id            text NOT NULL,
    entity_type           text NOT NULL DEFAULT '',
    entity_id             text NOT NULL DEFAULT '',
    position              integer NOT NULL DEFAULT 0,
    collection_order      integer NOT NULL DEFAULT 0,
    display_name          jsonb,
    description           jsonb,
    collection_name       jsonb,
    category_id           text DEFAULT '',
    part_of_collection_id text DEFAULT '',
    expected_value_type   text DEFAULT '',
    set_value             text DEFAULT '',
    is_required           boolean DEFAULT false,
    min_occurs            integer DEFAULT 0,
    max_occurs            integer,
    is_hidden             boolean DEFAULT false,
    visibility            text DEFAULT '',
    staging_id            bigint REFERENCES weave_import_staging(id),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    content_hash          text,
    version_number        text NOT NULL DEFAULT '',
    set_value_entry_id    text
);

CREATE INDEX idx_wfo_entity ON weave_field_overrides (entity_type, entity_id);
CREATE INDEX idx_wfo_field ON weave_field_overrides (field_id);
CREATE INDEX idx_wfo_project_base ON weave_field_overrides (project_id) WHERE entity_type = '';
CREATE INDEX idx_wfo_project_entity ON weave_field_overrides (project_id, entity_type, entity_id);
CREATE INDEX idx_wfo_content_hash ON weave_field_overrides (content_hash);
CREATE INDEX idx_wfo_release_versions ON weave_field_overrides (project_id, version_number) WHERE version_number != '';
CREATE UNIQUE INDEX uniq_wfo_base_field_project ON weave_field_overrides (field_id, project_id)
    WHERE entity_type = '';

CREATE TABLE weave_override_refs (
    override_id bigint NOT NULL REFERENCES weave_field_overrides(id) ON DELETE CASCADE,
    ref_type    text NOT NULL,
    target_id   text NOT NULL DEFAULT '',
    semantic_id text NOT NULL,
    position    integer DEFAULT 0,
    PRIMARY KEY (override_id, ref_type, position)
);

CREATE INDEX idx_wor_target ON weave_override_refs (target_id) WHERE target_id != '';
CREATE INDEX idx_wor_semantic ON weave_override_refs (semantic_id);

CREATE TABLE weave_entity_counters (
    project_id text NOT NULL,
    kind       text NOT NULL,
    next_n     bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, kind)
);

CREATE TABLE weave_ontology_families (
    id               text PRIMARY KEY,
    slug             text NOT NULL UNIQUE,
    name             text NOT NULL,
    description      jsonb,
    parent_family_id text REFERENCES weave_ontology_families(id) ON DELETE SET NULL,
    homepage_url     text,
    icon             text,
    display_order    bigint NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_weave_ontology_families_parent ON weave_ontology_families (parent_family_id);

CREATE TABLE weave_ontologies (
    id                  text PRIMARY KEY,
    prefix              text NOT NULL UNIQUE,
    namespace           text NOT NULL,
    name                text NOT NULL,
    description         jsonb,
    family_id           text REFERENCES weave_ontology_families(id) ON DELETE SET NULL,
    ontology_type       text NOT NULL DEFAULT 'base' CHECK (ontology_type IN ('base', 'extension')),
    extends_ontology_id text REFERENCES weave_ontologies(id) ON DELETE SET NULL,
    homepage_url        text,
    source_url          text,
    created_by_id       text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_weave_ontologies_family ON weave_ontologies (family_id);
CREATE INDEX idx_weave_ontologies_extends ON weave_ontologies (extends_ontology_id);

CREATE TABLE weave_ontology_versions (
    id                       text PRIMARY KEY,
    ontology_id              text NOT NULL REFERENCES weave_ontologies(id) ON DELETE CASCADE,
    version_string           text NOT NULL,
    is_active                boolean NOT NULL DEFAULT false,
    compatible_base_versions text[],
    rdf_content              text,
    parsed_at                timestamptz,
    original_filename        text,
    file_size                bigint,
    file_md5                 text,
    ontology_uri             text,
    version_iri              text,
    version_info             jsonb,
    imported_ontologies      text[],
    ontology_label           jsonb,
    ontology_comment         jsonb,
    ontology_metadata        jsonb,
    class_count              bigint NOT NULL DEFAULT 0,
    property_count           bigint NOT NULL DEFAULT 0,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    UNIQUE (ontology_id, version_string)
);

CREATE UNIQUE INDEX idx_weave_ontology_versions_active_per_ontology
    ON weave_ontology_versions (ontology_id) WHERE is_active = true;

CREATE TABLE weave_ontology_classes (
    id                  text PRIMARY KEY,
    ontology_version_id text NOT NULL REFERENCES weave_ontology_versions(id) ON DELETE CASCADE,
    prefix              text NOT NULL,
    local_name          text NOT NULL,
    uri                 text NOT NULL,
    qname               text GENERATED ALWAYS AS (prefix || ':' || local_name) STORED,
    label               jsonb,
    comment             jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    natural_sort_key    text NOT NULL DEFAULT '',
    UNIQUE (ontology_version_id, uri),
    UNIQUE (ontology_version_id, qname)
);

CREATE INDEX idx_weave_ontology_classes_qname ON weave_ontology_classes (qname);
CREATE INDEX idx_weave_ontology_classes_version ON weave_ontology_classes (ontology_version_id);
CREATE INDEX idx_woc_version_natural_sort ON weave_ontology_classes (ontology_version_id, natural_sort_key);

CREATE TABLE weave_ontology_properties (
    id                    text PRIMARY KEY,
    ontology_version_id   text NOT NULL REFERENCES weave_ontology_versions(id) ON DELETE CASCADE,
    prefix                text NOT NULL,
    local_name            text NOT NULL,
    uri                   text NOT NULL,
    qname                 text GENERATED ALWAYS AS (prefix || ':' || local_name) STORED,
    label                 jsonb,
    comment               jsonb,
    property_type         text,
    is_functional         boolean NOT NULL DEFAULT false,
    is_inverse_functional boolean NOT NULL DEFAULT false,
    is_transitive         boolean NOT NULL DEFAULT false,
    is_symmetric          boolean NOT NULL DEFAULT false,
    is_asymmetric         boolean NOT NULL DEFAULT false,
    is_reflexive          boolean NOT NULL DEFAULT false,
    is_irreflexive        boolean NOT NULL DEFAULT false,
    inverse_property_uri  text,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    natural_sort_key      text NOT NULL DEFAULT '',
    UNIQUE (ontology_version_id, uri),
    UNIQUE (ontology_version_id, qname)
);

CREATE INDEX idx_weave_ontology_properties_qname ON weave_ontology_properties (qname);
CREATE INDEX idx_weave_ontology_properties_version ON weave_ontology_properties (ontology_version_id);
CREATE INDEX idx_wop_version_natural_sort ON weave_ontology_properties (ontology_version_id, natural_sort_key);

CREATE TABLE weave_ontology_relations (
    source_id    text NOT NULL,
    source_kind  text NOT NULL CHECK (source_kind IN ('class', 'property')),
    rel_type     text NOT NULL,
    target_qname text NOT NULL,
    position     integer NOT NULL DEFAULT 0,
    PRIMARY KEY (source_id, source_kind, rel_type, target_qname)
);

CREATE INDEX idx_weave_ontology_relations_source
    ON weave_ontology_relations (source_id, source_kind, rel_type);
CREATE INDEX idx_weave_ontology_relations_target
    ON weave_ontology_relations (target_qname, rel_type);

CREATE TABLE weave_field_ontology_refs (
    field_id   text NOT NULL REFERENCES weave_fields(id) ON DELETE CASCADE,
    project_id text NOT NULL,
    prefix     text NOT NULL,
    local_name text NOT NULL,
    qname      text GENERATED ALWAYS AS (prefix || ':' || local_name) STORED,
    position   integer NOT NULL,
    ref_kind   text NOT NULL CHECK (ref_kind IN ('class', 'property', 'literal')),
    PRIMARY KEY (field_id, position)
);

CREATE INDEX idx_weave_field_ontology_refs_qname ON weave_field_ontology_refs (project_id, qname);
CREATE INDEX idx_weave_field_ontology_refs_field ON weave_field_ontology_refs (field_id);

CREATE TABLE weave_project_ontology_versions (
    project_id          text NOT NULL,
    ontology_version_id text NOT NULL,
    added_at            timestamptz NOT NULL DEFAULT now(),
    added_by_id         text,
    is_primary          boolean DEFAULT false,
    usage_notes         text,
    version_number      text NOT NULL DEFAULT '',
    PRIMARY KEY (project_id, ontology_version_id)
);

CREATE INDEX idx_wpov_project ON weave_project_ontology_versions (project_id);
CREATE INDEX idx_wpov_release_versions ON weave_project_ontology_versions (project_id, version_number)
    WHERE version_number != '';

CREATE TABLE weave_namespace_bindings (
    id             text PRIMARY KEY,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    system_name    text,
    ui_name        jsonb,
    description    jsonb,
    project_id     text,
    prefix         text NOT NULL,
    namespace      text NOT NULL,
    weight         bigint NOT NULL DEFAULT 10,
    source         text NOT NULL DEFAULT 'user',
    ontology_id    text,
    version_number text NOT NULL DEFAULT ''
);

CREATE INDEX idx_wnb_project ON weave_namespace_bindings (project_id);
CREATE INDEX idx_wnb_release_versions ON weave_namespace_bindings (project_id, version_number)
    WHERE version_number != '';

CREATE TABLE weave_releases (
    project_id    text NOT NULL REFERENCES weave_projects(id) ON DELETE CASCADE,
    version       text NOT NULL,
    title         text NOT NULL DEFAULT '',
    description   text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    created_by_id text NOT NULL,
    PRIMARY KEY (project_id, version)
);

CREATE INDEX idx_wr_project_created ON weave_releases (project_id, created_at DESC);

CREATE TABLE weave_projects_archive (
    id             text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    system_name    text,
    ui_name        jsonb,
    description    jsonb,
    status         text NOT NULL DEFAULT 'draft',
    namespace      text DEFAULT '',
    parent_project_id text,
    staging_id     bigint,
    owner_id       text NOT NULL,
    visibility     text NOT NULL DEFAULT 'private',
    deprecated     boolean NOT NULL DEFAULT false,
    license        text NOT NULL DEFAULT '',
    readme         jsonb NOT NULL DEFAULT '{}'::jsonb,
    topics         text[] NOT NULL DEFAULT '{}',
    base_url       text NOT NULL DEFAULT '',
    created_by_id  text,
    version_number text NOT NULL,
    PRIMARY KEY (id, version_number)
);

CREATE INDEX idx_wpa_version ON weave_projects_archive (id, version_number);

CREATE TABLE weave_categories_archive (
    id              text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    semantic_id     text,
    system_name     text,
    ui_name         jsonb,
    description     jsonb,
    status          text NOT NULL DEFAULT 'draft',
    project_id      text NOT NULL,
    canonical_order integer NOT NULL DEFAULT 0,
    deprecated      boolean NOT NULL DEFAULT false,
    version_number  text NOT NULL,
    PRIMARY KEY (id, version_number)
);

CREATE INDEX idx_wca_project_version ON weave_categories_archive (project_id, version_number);

CREATE TABLE weave_fields_archive (
    id                  text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    semantic_id         text,
    system_name         text,
    ui_name             jsonb,
    description         jsonb,
    status              text NOT NULL DEFAULT 'draft',
    project_id          text NOT NULL,
    ontology_scope      jsonb,
    ontology_path       text,
    path_elements       jsonb,
    expected_value_type text,
    examples            jsonb,
    staging_id          bigint,
    deprecated          boolean NOT NULL DEFAULT false,
    version_number      text NOT NULL,
    PRIMARY KEY (id, version_number)
);

CREATE INDEX idx_wfa_project_version ON weave_fields_archive (project_id, version_number);

CREATE TABLE weave_models_archive (
    id             text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    system_name    text,
    ui_name        jsonb,
    description    jsonb,
    status         text NOT NULL DEFAULT 'draft',
    project_id     text NOT NULL,
    ontology_scope jsonb,
    staging_id     bigint,
    deprecated     boolean NOT NULL DEFAULT false,
    version_number text NOT NULL,
    model_type     text NOT NULL DEFAULT 'auxiliary',
    PRIMARY KEY (id, version_number)
);

CREATE INDEX idx_wma_project_version ON weave_models_archive (project_id, version_number);

CREATE TABLE weave_collections_archive (
    id                         text NOT NULL,
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now(),
    system_name                text,
    ui_name                    jsonb,
    description                jsonb,
    status                     text NOT NULL DEFAULT 'draft',
    project_id                 text NOT NULL,
    ontology_scope             jsonb,
    collection_number          integer DEFAULT 0,
    canonical_collection_order integer DEFAULT 0,
    staging_id                 bigint,
    deprecated                 boolean NOT NULL DEFAULT false,
    default_category_id        text,
    version_number             text NOT NULL,
    PRIMARY KEY (id, version_number)
);

CREATE INDEX idx_wcoa_project_version ON weave_collections_archive (project_id, version_number);

CREATE TABLE weave_field_overrides_archive (
    id                    bigint NOT NULL,
    field_id              text NOT NULL,
    project_id            text NOT NULL,
    entity_type           text NOT NULL DEFAULT '',
    entity_id             text NOT NULL DEFAULT '',
    position              integer NOT NULL DEFAULT 0,
    collection_order      integer NOT NULL DEFAULT 0,
    display_name          jsonb,
    description           jsonb,
    collection_name       jsonb,
    category_id           text DEFAULT '',
    part_of_collection_id text DEFAULT '',
    expected_value_type   text DEFAULT '',
    set_value             text DEFAULT '',
    is_required           boolean DEFAULT false,
    min_occurs            integer DEFAULT 0,
    max_occurs            integer,
    is_hidden             boolean DEFAULT false,
    visibility            text DEFAULT '',
    staging_id            bigint,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    content_hash          text,
    version_number        text NOT NULL,
    PRIMARY KEY (id, version_number)
);

CREATE INDEX idx_wfoa_project_version ON weave_field_overrides_archive (project_id, version_number);
CREATE INDEX idx_wfoa_entity_version ON weave_field_overrides_archive (entity_type, entity_id, version_number);

CREATE TABLE weave_project_ontology_versions_archive (
    project_id          text NOT NULL,
    ontology_version_id text NOT NULL,
    added_at            timestamptz NOT NULL DEFAULT now(),
    added_by_id         text,
    is_primary          boolean DEFAULT false,
    usage_notes         text,
    version_number      text NOT NULL,
    PRIMARY KEY (project_id, ontology_version_id, version_number)
);

CREATE INDEX idx_wpova_project_version ON weave_project_ontology_versions_archive (project_id, version_number);

CREATE TABLE weave_namespace_bindings_archive (
    id             text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    system_name    text,
    ui_name        jsonb,
    description    jsonb,
    project_id     text,
    prefix         text NOT NULL,
    namespace      text NOT NULL,
    weight         bigint NOT NULL DEFAULT 10,
    source         text NOT NULL DEFAULT 'user',
    ontology_id    text,
    version_number text NOT NULL,
    PRIMARY KEY (id, version_number)
);

CREATE INDEX idx_wnba_project_version ON weave_namespace_bindings_archive (project_id, version_number);

CREATE TABLE weave_override_refs_archive (
    override_id    bigint NOT NULL,
    ref_type       text NOT NULL,
    target_id      text NOT NULL DEFAULT '',
    semantic_id    text NOT NULL,
    position       integer DEFAULT 0,
    project_id     text NOT NULL DEFAULT '',
    version_number text NOT NULL,
    PRIMARY KEY (override_id, ref_type, position, version_number)
);

CREATE INDEX idx_wora_project_version ON weave_override_refs_archive (project_id, version_number);
CREATE INDEX idx_wora_target_version ON weave_override_refs_archive (target_id, version_number)
    WHERE target_id != '';

CREATE TABLE weave_concept_lists_archive (
    id             text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    semantic_id    text,
    system_name    text,
    ui_name        jsonb,
    description    jsonb,
    status         text NOT NULL DEFAULT 'draft',
    project_id     text NOT NULL,
    list_type      text,
    vocabulary_id  text,
    version_number text NOT NULL DEFAULT '',
    PRIMARY KEY (id, version_number)
);

CREATE INDEX weave_concept_lists_archive_project_idx ON weave_concept_lists_archive (project_id, version_number);
CREATE INDEX weave_concept_lists_archive_semantic_idx ON weave_concept_lists_archive (semantic_id, version_number);

CREATE TABLE weave_concept_list_entries_archive (
    id                  text NOT NULL,
    concept_list_id     text NOT NULL,
    vocabulary_entry_id text NOT NULL,
    position            integer NOT NULL DEFAULT 0,
    custom_label        jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    version_number      text NOT NULL DEFAULT '',
    PRIMARY KEY (id, version_number)
);

CREATE INDEX weave_concept_list_entries_archive_list_idx
    ON weave_concept_list_entries_archive (concept_list_id, version_number);

CREATE TABLE weave_project_inheritance (
    project_id        text NOT NULL REFERENCES weave_projects(id) ON DELETE CASCADE,
    parent_project_id text NOT NULL REFERENCES weave_projects(id) ON DELETE RESTRICT,
    is_primary        boolean NOT NULL DEFAULT false,
    canonical_order   integer NOT NULL DEFAULT 0,
    adopted_at        timestamptz NOT NULL DEFAULT now(),
    source_mode       text NOT NULL DEFAULT 'draft',
    source_version    text,
    PRIMARY KEY (project_id, parent_project_id),
    CHECK (project_id <> parent_project_id),
    CHECK (
        (source_mode = 'draft' AND source_version IS NULL) OR
        (source_mode = 'release' AND source_version IS NOT NULL)
    )
);

CREATE UNIQUE INDEX idx_wpi_primary ON weave_project_inheritance (project_id) WHERE is_primary;
CREATE INDEX idx_wpi_parent ON weave_project_inheritance (parent_project_id);
CREATE INDEX idx_wpi_project_order
    ON weave_project_inheritance (project_id, is_primary DESC, canonical_order ASC, parent_project_id ASC);

CREATE TABLE weave_project_inheritance_archive (
    project_id        text NOT NULL,
    parent_project_id text NOT NULL,
    is_primary        boolean NOT NULL DEFAULT false,
    canonical_order   integer NOT NULL DEFAULT 0,
    adopted_at        timestamptz NOT NULL DEFAULT now(),
    version_number    text NOT NULL DEFAULT '',
    source_mode       text NOT NULL DEFAULT 'draft',
    source_version    text,
    PRIMARY KEY (project_id, parent_project_id, version_number),
    CHECK (
        (source_mode = 'draft' AND source_version IS NULL) OR
        (source_mode = 'release' AND source_version IS NOT NULL)
    )
);

CREATE UNIQUE INDEX idx_wpia_primary ON weave_project_inheritance_archive (project_id, version_number)
    WHERE is_primary;
CREATE INDEX idx_wpia_project_version
    ON weave_project_inheritance_archive (project_id, version_number, is_primary DESC, canonical_order ASC, parent_project_id ASC);
CREATE INDEX idx_wpia_parent_version ON weave_project_inheritance_archive (parent_project_id, version_number);

CREATE TABLE weave_adoptions (
    id                  bigserial PRIMARY KEY,
    project_id          text NOT NULL REFERENCES weave_projects(id) ON DELETE CASCADE,
    context_entity_type text NOT NULL CHECK (context_entity_type IN ('project', 'model', 'collection')),
    context_entity_id   text NOT NULL,
    entity_type         text NOT NULL CHECK (entity_type IN ('field', 'collection', 'model', 'category')),
    source_project_id   text NOT NULL REFERENCES weave_projects(id) ON DELETE CASCADE,
    source_entity_id    text NOT NULL,
    source_version      text NOT NULL DEFAULT '',
    adopted_at          timestamptz NOT NULL DEFAULT now(),
    created_by_id       text REFERENCES weave_actors(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX weave_adoptions_context_source_key ON weave_adoptions (
    project_id,
    context_entity_type,
    context_entity_id,
    entity_type,
    source_project_id,
    source_entity_id,
    source_version
);
CREATE INDEX weave_adoptions_project_idx
    ON weave_adoptions (project_id, entity_type, source_project_id, source_entity_id);

CREATE TABLE weave_adoptions_archive (
    project_id          text NOT NULL,
    context_entity_type text NOT NULL,
    context_entity_id   text NOT NULL,
    entity_type         text NOT NULL,
    source_project_id   text NOT NULL,
    source_entity_id    text NOT NULL,
    source_version      text NOT NULL DEFAULT '',
    adopted_at          timestamptz NOT NULL,
    created_by_id       text,
    version_number      text NOT NULL,
    PRIMARY KEY (
        project_id,
        context_entity_type,
        context_entity_id,
        entity_type,
        source_project_id,
        source_entity_id,
        source_version,
        version_number
    )
);

CREATE INDEX weave_adoptions_archive_project_idx
    ON weave_adoptions_archive (project_id, version_number, entity_type, source_project_id, source_entity_id);

CREATE TABLE weave_entity_forks (
    id                bigserial PRIMARY KEY,
    project_id        text NOT NULL REFERENCES weave_projects(id) ON DELETE CASCADE,
    entity_type       text NOT NULL CHECK (entity_type IN ('field', 'collection', 'model')),
    fork_entity_id    text NOT NULL,
    source_project_id text NOT NULL REFERENCES weave_projects(id) ON DELETE CASCADE,
    source_entity_id  text NOT NULL,
    source_version    text NOT NULL DEFAULT '',
    forked_at         timestamptz NOT NULL DEFAULT now(),
    created_by_id     text REFERENCES weave_actors(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX weave_entity_forks_project_entity_key
    ON weave_entity_forks (project_id, entity_type, fork_entity_id);
CREATE INDEX weave_entity_forks_source_idx
    ON weave_entity_forks (project_id, entity_type, source_project_id, source_entity_id);

CREATE TABLE weave_entity_forks_archive (
    project_id        text NOT NULL,
    entity_type       text NOT NULL,
    fork_entity_id    text NOT NULL,
    source_project_id text NOT NULL,
    source_entity_id  text NOT NULL,
    source_version    text NOT NULL DEFAULT '',
    forked_at         timestamptz NOT NULL,
    created_by_id     text,
    version_number    text NOT NULL,
    PRIMARY KEY (project_id, entity_type, fork_entity_id, version_number)
);

CREATE INDEX weave_entity_forks_archive_source_idx
    ON weave_entity_forks_archive (project_id, version_number, entity_type, source_project_id, source_entity_id);

CREATE TABLE weave_examples (
    id             text PRIMARY KEY,
    project_id     text NOT NULL REFERENCES weave_projects(id) ON DELETE CASCADE,
    entity_type    text NOT NULL CHECK (entity_type IN ('model', 'collection')),
    entity_id      text NOT NULL,
    title          jsonb,
    description    jsonb,
    status         text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'valid', 'has_issues')),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    version_number text NOT NULL DEFAULT ''
);

CREATE INDEX idx_weave_examples_target ON weave_examples (project_id, entity_type, entity_id);
CREATE INDEX idx_weave_examples_status ON weave_examples (project_id, status);

CREATE TABLE weave_example_values (
    id                    bigserial PRIMARY KEY,
    example_id            text NOT NULL REFERENCES weave_examples(id) ON DELETE CASCADE,
    override_id           bigint NOT NULL REFERENCES weave_field_overrides(id) ON DELETE CASCADE,
    field_id              text NOT NULL REFERENCES weave_fields(id) ON DELETE CASCADE,
    part_of_collection_id text,
    occurrence_index      integer NOT NULL DEFAULT 0,
    value_kind            text NOT NULL CHECK (value_kind IN ('string', 'integer', 'date', 'uri', 'concept', 'example_ref')),
    value_payload         jsonb NOT NULL,
    text_value            text,
    number_value          numeric,
    date_value            date,
    uri_value             text,
    concept_uri           text,
    linked_example_id     text REFERENCES weave_examples(id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    version_number        text NOT NULL DEFAULT '',
    UNIQUE (example_id, override_id, occurrence_index)
);

CREATE INDEX idx_weave_example_values_example ON weave_example_values (example_id);
CREATE INDEX idx_weave_example_values_field ON weave_example_values (field_id);
CREATE INDEX idx_weave_example_values_override ON weave_example_values (override_id);
CREATE INDEX idx_weave_example_values_kind ON weave_example_values (value_kind);
CREATE INDEX idx_weave_example_values_linked_example ON weave_example_values (linked_example_id);
CREATE INDEX idx_weave_example_values_concept_uri ON weave_example_values (concept_uri);

CREATE TABLE weave_change_set (
    id             bigserial PRIMARY KEY,
    project_id     text NOT NULL,
    actor_id       text,
    actor_name     text NOT NULL,
    actor_email    text NOT NULL,
    commit_message text NOT NULL,
    started_at     timestamptz NOT NULL DEFAULT now(),
    closed_at      timestamptz,
    processed_at   timestamptz,
    git_commit_sha text
);

CREATE INDEX idx_wcs_project_time ON weave_change_set (project_id, started_at DESC);
CREATE INDEX idx_wcs_actor ON weave_change_set (actor_id, started_at DESC);
CREATE INDEX idx_wcs_unprocessed ON weave_change_set (closed_at) WHERE processed_at IS NULL;

CREATE TABLE weave_change_log (
    id               bigserial PRIMARY KEY,
    change_set_id    bigint NOT NULL REFERENCES weave_change_set(id),
    entity_type      text NOT NULL,
    entity_id        text NOT NULL,
    operation        text NOT NULL,
    project_id       text NOT NULL,
    file_path        text NOT NULL,
    payload          bytea,
    previous_payload bytea,
    created_at       timestamptz NOT NULL DEFAULT now(),
    processed_at     timestamptz
);

CREATE INDEX idx_wcl_change_set ON weave_change_log (change_set_id);
CREATE INDEX idx_wcl_entity ON weave_change_log (entity_type, entity_id, id DESC);
CREATE INDEX idx_wcl_unprocessed ON weave_change_log (change_set_id) WHERE processed_at IS NULL;

CREATE TABLE weave_change_set_archive (
    id             bigint PRIMARY KEY,
    project_id     text NOT NULL,
    actor_id       text,
    actor_name     text NOT NULL,
    actor_email    text NOT NULL,
    commit_message text NOT NULL,
    started_at     timestamptz NOT NULL,
    closed_at      timestamptz,
    processed_at   timestamptz,
    git_commit_sha text
);

CREATE INDEX idx_wcsa_project_time ON weave_change_set_archive (project_id, started_at DESC);
CREATE INDEX idx_wcsa_actor ON weave_change_set_archive (actor_id, started_at DESC);

CREATE TABLE weave_change_log_archive (
    id               bigint PRIMARY KEY,
    change_set_id    bigint NOT NULL,
    entity_type      text NOT NULL,
    entity_id        text NOT NULL,
    operation        text NOT NULL,
    project_id       text NOT NULL,
    file_path        text NOT NULL,
    payload          bytea,
    previous_payload bytea,
    created_at       timestamptz NOT NULL,
    processed_at     timestamptz
);

CREATE INDEX idx_wcla_change_set ON weave_change_log_archive (change_set_id);
CREATE INDEX idx_wcla_entity ON weave_change_log_archive (entity_type, entity_id, id DESC);

CREATE VIEW weave_change_set_all AS
SELECT id, project_id, actor_id, actor_name, actor_email,
       commit_message, started_at, closed_at, processed_at, git_commit_sha
FROM weave_change_set
UNION ALL
SELECT id, project_id, actor_id, actor_name, actor_email,
       commit_message, started_at, closed_at, processed_at, git_commit_sha
FROM weave_change_set_archive;

CREATE VIEW weave_change_log_all AS
SELECT id, change_set_id, entity_type, entity_id, operation, project_id,
       file_path, payload, previous_payload, created_at, processed_at
FROM weave_change_log
UNION ALL
SELECT id, change_set_id, entity_type, entity_id, operation, project_id,
       file_path, payload, previous_payload, created_at, processed_at
FROM weave_change_log_archive;

CREATE TABLE weave_error_events (
    id          bigserial PRIMARY KEY,
    request_id  text,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    source      text NOT NULL DEFAULT 'server' CHECK (source IN ('server', 'client')),
    route       text NOT NULL,
    method      text,
    status      integer,
    actor_id    text,
    duration_ms integer,
    user_agent  text,
    request     jsonb NOT NULL DEFAULT '{}'::jsonb,
    response    jsonb NOT NULL DEFAULT '{}'::jsonb,
    error       jsonb NOT NULL DEFAULT '{}'::jsonb,
    category    text NOT NULL DEFAULT 'handler_decision'
        CHECK (category IN ('no_route', 'handler_decision', 'server_error', 'client'))
);

CREATE INDEX idx_wee_status_time ON weave_error_events (status, occurred_at DESC) WHERE status IS NOT NULL;
CREATE INDEX idx_wee_route_time ON weave_error_events (route, occurred_at DESC);
CREATE INDEX idx_wee_source_time ON weave_error_events (source, occurred_at DESC);
CREATE INDEX idx_wee_category_time ON weave_error_events (category, occurred_at DESC);

CREATE TABLE admin_git_restore_jobs (
    id                text PRIMARY KEY,
    snapshot_path     text NOT NULL,
    source_project_id text NOT NULL,
    target_project_id text NOT NULL,
    requested_by_id   text NOT NULL REFERENCES weave_actors(id),
    status            text NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    current_phase     text NOT NULL DEFAULT '',
    error_message     text NOT NULL DEFAULT '',
    preview_json      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at        timestamptz NOT NULL DEFAULT now(),
    started_at        timestamptz,
    finished_at       timestamptz
);

CREATE INDEX idx_admin_git_restore_jobs_created_at ON admin_git_restore_jobs (created_at DESC);
CREATE INDEX idx_admin_git_restore_jobs_status ON admin_git_restore_jobs (status);
CREATE INDEX idx_admin_git_restore_jobs_target_project ON admin_git_restore_jobs (target_project_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION sync_path_elements() RETURNS trigger AS $$
BEGIN
    DELETE FROM weave_path_elements WHERE field_id = NEW.id;

    IF NEW.path_elements IS NOT NULL THEN
        INSERT INTO weave_path_elements (field_id, position, local_name, type, prefix, class_code, ontology_version_id)
        SELECT NEW.id,
               (elem->>'position')::integer,
               elem->>'local_name',
               elem->>'type',
               elem->>'prefix',
               elem->>'class_code',
               elem->>'ontology_version_id'
        FROM jsonb_array_elements(NEW.path_elements) AS t(elem);
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_sync_path_elements
    AFTER INSERT OR UPDATE OF path_elements ON weave_fields
    FOR EACH ROW EXECUTE FUNCTION sync_path_elements();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION match_path_prefix(
    p_project_ids text[],
    p_elements text[],
    p_prefixes text[]
) RETURNS TABLE(field_id text) AS $$
DECLARE
    i integer;
    joins_clause text;
    where_clause text;
    q text;
    n integer;
BEGIN
    n := array_length(p_elements, 1);
    IF n IS NULL OR n = 0 THEN
        RETURN QUERY
            SELECT f.id FROM weave_fields f WHERE f.project_id = ANY(p_project_ids);
        RETURN;
    END IF;

    joins_clause := 'FROM weave_path_elements pe0 '
                 || 'JOIN weave_fields f ON f.id = pe0.field_id ';

    where_clause := 'WHERE f.project_id = ANY($1) '
                 || 'AND pe0.position = 0 '
                 || 'AND starts_with(pe0.local_name, ' || quote_literal(p_elements[1]) || ')';

    IF p_prefixes[1] IS NOT NULL AND p_prefixes[1] != '' THEN
        where_clause := where_clause || ' AND pe0.prefix = ' || quote_literal(p_prefixes[1]);
    END IF;

    FOR i IN 2..n LOOP
        joins_clause := joins_clause || format(
            'JOIN weave_path_elements pe%s ON pe%s.field_id = pe0.field_id AND pe%s.position = %s ',
            i-1, i-1, i-1, i-1
        );
        where_clause := where_clause || ' AND starts_with(pe' || (i-1) || '.local_name, ' || quote_literal(p_elements[i]) || ')';
        IF p_prefixes[i] IS NOT NULL AND p_prefixes[i] != '' THEN
            where_clause := where_clause || ' AND pe' || (i-1) || '.prefix = ' || quote_literal(p_prefixes[i]);
        END IF;
    END LOOP;

    q := 'SELECT DISTINCT pe0.field_id ' || joins_clause || where_clause;

    RETURN QUERY EXECUTE q USING p_project_ids;
END;
$$ LANGUAGE plpgsql STABLE;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION path_next_suggestions(
    p_project_ids text[],
    p_elements text[],
    p_prefixes text[],
    p_query text DEFAULT ''
) RETURNS TABLE(local_name text, prefix text, elem_type text, field_count bigint) AS $$
DECLARE
    i integer;
    n integer;
    next_pos integer;
    q text;
BEGIN
    n := COALESCE(array_length(p_elements, 1), 0);
    next_pos := n;

    IF n = 0 THEN
        q := 'SELECT pe.local_name, pe.prefix, pe.type AS elem_type, '
             || 'COUNT(DISTINCT pe.field_id) AS field_count '
             || 'FROM weave_path_elements pe '
             || 'JOIN weave_fields f ON f.id = pe.field_id '
             || 'WHERE f.project_id = ANY($1) AND pe.position = 0';

        IF p_query != '' THEN
            q := q || ' AND pe.local_name ILIKE '
                   || quote_literal('%' || replace(replace(replace(p_query, '\', '\\'), '_', '\_'), '%', '\%') || '%')
                   || ' ESCAPE ' || quote_literal('\');
        END IF;

        q := q || ' GROUP BY pe.local_name, pe.prefix, pe.type ORDER BY field_count DESC';
        RETURN QUERY EXECUTE q USING p_project_ids;
        RETURN;
    END IF;

    q := 'SELECT pe_next.local_name, pe_next.prefix, pe_next.type AS elem_type, '
         || 'COUNT(DISTINCT pe_next.field_id) AS field_count '
         || 'FROM weave_path_elements pe0 '
         || 'JOIN weave_fields f ON f.id = pe0.field_id ';

    FOR i IN 2..n LOOP
        q := q || format(
            'JOIN weave_path_elements pe%s ON pe%s.field_id = pe0.field_id AND pe%s.position = %s ',
            i-1, i-1, i-1, i-1
        );
    END LOOP;

    q := q || format(
        'JOIN weave_path_elements pe_next ON pe_next.field_id = pe0.field_id AND pe_next.position = %s ',
        next_pos
    );

    q := q || 'WHERE f.project_id = ANY($1) '
           || 'AND pe0.position = 0 '
           || 'AND starts_with(pe0.local_name, ' || quote_literal(p_elements[1]) || ')';

    IF p_prefixes[1] IS NOT NULL AND p_prefixes[1] != '' THEN
        q := q || ' AND pe0.prefix = ' || quote_literal(p_prefixes[1]);
    END IF;

    FOR i IN 2..n LOOP
        q := q || ' AND starts_with(pe' || (i-1) || '.local_name, ' || quote_literal(p_elements[i]) || ')';
        IF p_prefixes[i] IS NOT NULL AND p_prefixes[i] != '' THEN
            q := q || ' AND pe' || (i-1) || '.prefix = ' || quote_literal(p_prefixes[i]);
        END IF;
    END LOOP;

    IF p_query != '' THEN
        q := q || ' AND pe_next.local_name ILIKE '
               || quote_literal('%' || replace(replace(replace(p_query, '\', '\\'), '_', '\_'), '%', '\%') || '%')
               || ' ESCAPE ' || quote_literal('\');
    END IF;

    q := q || ' GROUP BY pe_next.local_name, pe_next.prefix, pe_next.type ORDER BY field_count DESC';

    RETURN QUERY EXECUTE q USING p_project_ids;
END;
$$ LANGUAGE plpgsql STABLE;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION weave_reconcile_entity_counters() RETURNS void AS $$
BEGIN
    INSERT INTO weave_entity_counters (project_id, kind, next_n)
    SELECT
        project_id,
        'model',
        COALESCE(MAX(NULLIF(regexp_replace(id, '^.*\.', ''), '')::bigint), 0) + 1
    FROM weave_models
    GROUP BY project_id
    ON CONFLICT (project_id, kind)
    DO UPDATE SET
        next_n = GREATEST(weave_entity_counters.next_n, EXCLUDED.next_n),
        updated_at = now();

    INSERT INTO weave_entity_counters (project_id, kind, next_n)
    SELECT
        project_id,
        'collection',
        COALESCE(MAX(NULLIF(regexp_replace(id, '^.*\.', ''), '')::bigint), 0) + 1
    FROM weave_collections
    GROUP BY project_id
    ON CONFLICT (project_id, kind)
    DO UPDATE SET
        next_n = GREATEST(weave_entity_counters.next_n, EXCLUDED.next_n),
        updated_at = now();

    INSERT INTO weave_entity_counters (project_id, kind, next_n)
    SELECT
        project_id,
        'field',
        COALESCE(MAX(NULLIF(regexp_replace(id, '^.*\.', ''), '')::bigint), 0) + 1
    FROM weave_fields
    GROUP BY project_id
    ON CONFLICT (project_id, kind)
    DO UPDATE SET
        next_n = GREATEST(weave_entity_counters.next_n, EXCLUDED.next_n),
        updated_at = now();

    INSERT INTO weave_entity_counters (project_id, kind, next_n)
    SELECT
        project_id,
        'category',
        COALESCE(MAX(NULLIF(regexp_replace(COALESCE(semantic_id, id), '^.*\.', ''), '')::bigint), 0) + 1
    FROM weave_categories
    GROUP BY project_id
    ON CONFLICT (project_id, kind)
    DO UPDATE SET
        next_n = GREATEST(weave_entity_counters.next_n, EXCLUDED.next_n),
        updated_at = now();
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down

DROP VIEW IF EXISTS weave_change_log_all;
DROP VIEW IF EXISTS weave_change_set_all;

DROP TRIGGER IF EXISTS trg_sync_path_elements ON weave_fields;
DROP FUNCTION IF EXISTS sync_path_elements();
DROP FUNCTION IF EXISTS match_path_prefix(text[], text[], text[]);
DROP FUNCTION IF EXISTS path_next_suggestions(text[], text[], text[], text);
DROP FUNCTION IF EXISTS weave_reconcile_entity_counters();

DROP TABLE IF EXISTS admin_git_restore_jobs;
DROP TABLE IF EXISTS weave_error_events;
DROP TABLE IF EXISTS weave_change_log_archive;
DROP TABLE IF EXISTS weave_change_set_archive;
DROP TABLE IF EXISTS weave_change_log;
DROP TABLE IF EXISTS weave_change_set;
DROP TABLE IF EXISTS weave_example_values;
DROP TABLE IF EXISTS weave_examples;
DROP TABLE IF EXISTS weave_entity_forks_archive;
DROP TABLE IF EXISTS weave_entity_forks;
DROP TABLE IF EXISTS weave_adoptions_archive;
DROP TABLE IF EXISTS weave_adoptions;
DROP TABLE IF EXISTS weave_project_inheritance_archive;
DROP TABLE IF EXISTS weave_project_inheritance;
DROP TABLE IF EXISTS weave_concept_list_entries_archive;
DROP TABLE IF EXISTS weave_concept_lists_archive;
DROP TABLE IF EXISTS weave_override_refs_archive;
DROP TABLE IF EXISTS weave_namespace_bindings_archive;
DROP TABLE IF EXISTS weave_project_ontology_versions_archive;
DROP TABLE IF EXISTS weave_field_overrides_archive;
DROP TABLE IF EXISTS weave_collections_archive;
DROP TABLE IF EXISTS weave_models_archive;
DROP TABLE IF EXISTS weave_fields_archive;
DROP TABLE IF EXISTS weave_categories_archive;
DROP TABLE IF EXISTS weave_projects_archive;
DROP TABLE IF EXISTS weave_releases;
DROP TABLE IF EXISTS weave_namespace_bindings;
DROP TABLE IF EXISTS weave_project_ontology_versions;
DROP TABLE IF EXISTS weave_field_ontology_refs;
DROP TABLE IF EXISTS weave_ontology_relations;
DROP TABLE IF EXISTS weave_ontology_properties;
DROP TABLE IF EXISTS weave_ontology_classes;
DROP TABLE IF EXISTS weave_ontology_versions;
DROP TABLE IF EXISTS weave_ontologies;
DROP TABLE IF EXISTS weave_ontology_families;
DROP TABLE IF EXISTS weave_entity_counters;
DROP TABLE IF EXISTS weave_override_refs;
DROP TABLE IF EXISTS weave_field_overrides;
DROP TABLE IF EXISTS weave_collections;
DROP TABLE IF EXISTS weave_models;
DROP TABLE IF EXISTS weave_path_elements;
DROP TABLE IF EXISTS weave_fields;
DROP TABLE IF EXISTS weave_concept_list_entries;
DROP TABLE IF EXISTS weave_concept_lists;
DROP TABLE IF EXISTS weave_project_vocabularies;
DROP TABLE IF EXISTS weave_vocabulary_entries;
DROP TABLE IF EXISTS weave_vocabularies;
DROP TABLE IF EXISTS weave_categories;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS weave_memberships;
DROP TABLE IF EXISTS weave_auth;
DROP TABLE IF EXISTS weave_project_actors;
DROP TABLE IF EXISTS weave_projects;
DROP TABLE IF EXISTS weave_actors;
DROP TABLE IF EXISTS weave_import_staging;
