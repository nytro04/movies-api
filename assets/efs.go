package assets

import "embed"

//go:embed "migration"
var EmbeddedFiles embed.FS
