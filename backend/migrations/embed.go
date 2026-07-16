// Package migrations embeds the SQL migration files so api can apply them at
// startup without shipping a separate migrations directory in the image.
package migrations

import "embed"

// FS holds all *.sql goose migrations.
//
//go:embed *.sql
var FS embed.FS
