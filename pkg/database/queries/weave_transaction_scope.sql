-- name: WeaveTransactionIsolation :one
SELECT current_setting('transaction_isolation')::text AS isolation;
