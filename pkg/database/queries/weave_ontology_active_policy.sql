-- name: WeaveSelectActiveOntologyVersion :one
-- Single atomic operation shared with artifact restore. A false result means
-- the target version is missing or belongs to another ontology.
SELECT weave_set_ontology_active_version(@ontology_id::text, @version_id::text, @only_if_unset::boolean)::boolean AS found;
