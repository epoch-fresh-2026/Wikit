// Package contract embeds the generated MCP catalog used by the server.
package contract

import _ "embed"

//go:embed mcp-tools.json
var catalog []byte

func MCPCatalog() []byte { return append([]byte(nil), catalog...) }
