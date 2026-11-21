// Package db embeds the SQL migrations so the API binary can apply them
// (`api migrate up`) without shipping loose files in the runtime image.
package db

import "embed"

// Migrations holds the golang-migrate files under migrations/.
//
//go:embed migrations/*.sql
var Migrations embed.FS
