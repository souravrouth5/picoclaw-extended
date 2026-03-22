package mcp

import "embed"

// Assets holds all embedded MCP RE server files.
// assets/re is a mirror of mcp/re kept in-tree so go:embed works without .. traversal.
//
//go:embed assets/re
var Assets embed.FS
