// Package web embeds the templates and static assets served by the HTTP layer.
package web

import "embed"

//go:embed templates static
var FS embed.FS
