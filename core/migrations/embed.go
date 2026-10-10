// Package migrations embeds the SQL migrations so binaries are self-contained.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
