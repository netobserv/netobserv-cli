// Package res embeds capture manifests so the plugin is a standalone binary.
package res

import "embed"

// Files contains the same manifests used by the collector deployment.
//
//go:embed *.yml *.json
var Files embed.FS
