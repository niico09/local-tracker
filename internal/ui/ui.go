// Package ui holds the templates and static assets embedded in the binary.
// All embeddable assets MUST live under this directory: //go:embed cannot
// reference parent directories.
package ui

import "embed"

//go:embed all:templates all:static
var files embed.FS

// FS returns the embedded asset tree rooted at the ui package directory.
func FS() embed.FS { return files }
