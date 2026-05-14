// Package migrations embeds Pletka's goose SQL migrations.
package migrations

import "embed"

// FS contains the SQL migrations applied by store/postgres.
//
//go:embed *.sql
var FS embed.FS
