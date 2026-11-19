// Package webassets contains the buildless browser client embedded in the binary.
package webassets

import "embed"

// Files contains the complete browser application.
//
//go:embed index.html app.js style.css agent-apps.js
var Files embed.FS
