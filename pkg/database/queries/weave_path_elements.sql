-- name: FindFieldsByContiguousSequence2 :many
-- Mode 1: Contiguous — two elements must be adjacent in path
SELECT DISTINCT f.*
FROM weave_path_elements pe1
JOIN weave_path_elements pe2 ON pe2.field_id = pe1.field_id AND pe2.position = pe1.position + 1
JOIN weave_fields f ON f.id = pe1.field_id
WHERE pe1.local_name = @name1::text
  AND pe2.local_name = @name2::text;

-- name: FindFieldsByContiguousSequence3 :many
-- Mode 1: Contiguous — three elements must be adjacent
SELECT DISTINCT f.*
FROM weave_path_elements pe1
JOIN weave_path_elements pe2 ON pe2.field_id = pe1.field_id AND pe2.position = pe1.position + 1
JOIN weave_path_elements pe3 ON pe3.field_id = pe1.field_id AND pe3.position = pe2.position + 1
JOIN weave_fields f ON f.id = pe1.field_id
WHERE pe1.local_name = @name1::text
  AND pe2.local_name = @name2::text
  AND pe3.local_name = @name3::text;

-- name: FindFieldsBySubsequence2 :many
-- Mode 2: Subsequence — two elements in order, gaps allowed
SELECT DISTINCT f.*
FROM weave_path_elements pe1
JOIN weave_path_elements pe2 ON pe2.field_id = pe1.field_id AND pe2.position > pe1.position
JOIN weave_fields f ON f.id = pe1.field_id
WHERE pe1.local_name = @name1::text
  AND pe2.local_name = @name2::text;

-- name: FindFieldsBySubsequence3 :many
-- Mode 2: Subsequence — three elements in order, gaps allowed
SELECT DISTINCT f.*
FROM weave_path_elements pe1
JOIN weave_path_elements pe2 ON pe2.field_id = pe1.field_id AND pe2.position > pe1.position
JOIN weave_path_elements pe3 ON pe3.field_id = pe1.field_id AND pe3.position > pe2.position
JOIN weave_fields f ON f.id = pe1.field_id
WHERE pe1.local_name = @name1::text
  AND pe2.local_name = @name2::text
  AND pe3.local_name = @name3::text;

-- name: FindFieldsByAnchoredSequence2 :many
-- Mode 3: Anchored — scope class filter + two-element subsequence
SELECT DISTINCT f.*
FROM weave_fields f
JOIN weave_path_elements pe1 ON pe1.field_id = f.id
JOIN weave_path_elements pe2 ON pe2.field_id = f.id AND pe2.position > pe1.position
WHERE f.ontology_scope->>'local_name' = @anchor::text
  AND pe1.local_name = @name1::text
  AND pe2.local_name = @name2::text;
