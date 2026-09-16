// Package migrations embeds shop_ingestor's schema migration files so the
// migrate binary (and, in future, any other binary that needs them) can
// read them without relying on a filesystem path being correct at runtime.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
