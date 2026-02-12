package ingestion

import "embed"

//go:embed migrations/*.sql
var MigrationFS embed.FS
