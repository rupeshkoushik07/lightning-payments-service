// Package migrations carries the SQL files inside the program,
// so the binary can set up its own tables.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
