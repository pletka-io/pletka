-- +goose Up
-- A linked example may belong to an immutable source release. Its hot row is
-- neither the definition being referenced nor an appropriate cascade owner.
-- Scoped validation checks the selected target; owned draft deletion explicitly
-- clears local link columns while retaining recorded payload evidence.
ALTER TABLE weave_example_values DROP CONSTRAINT weave_example_values_linked_example_id_fkey;

-- +goose Down
-- Refuse downgrade if valid archived-only links cannot satisfy the old hot FK.
-- Never discard or rewrite those references to make downgrade succeed.
ALTER TABLE weave_example_values
 ADD CONSTRAINT weave_example_values_linked_example_id_fkey
 FOREIGN KEY (linked_example_id) REFERENCES weave_examples(id) ON DELETE SET NULL;
