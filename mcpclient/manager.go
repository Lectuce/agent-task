package mcpclient

import (
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Mannager struct {
	mu       sync.Mutex
	sessions map[string]*mcp.ClientSession
	tools    map[string]RemoteTool
}

type RemoteTool struct {
	ExposeName   string
	ServerName   string
	OriginalName string
	Definition   *mcp.Tool
}
