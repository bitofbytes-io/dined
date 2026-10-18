package migrations

import "embed"

// Files exposes all SQL migrations embedded into the binary.
//
//go:embed *.sql
var Files embed.FS
