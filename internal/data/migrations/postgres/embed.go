package postgres

import "embed"

//go:embed schema/*.sql
var SchemaFS embed.FS
