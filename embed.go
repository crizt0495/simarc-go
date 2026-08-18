package arsippro

import "embed"

// Embedded contains the web templates and static files compiled into the binary.
// This is needed for Vercel deployment where source files are not available at runtime.
//
//go:embed web/templates web/static
var Embedded embed.FS
