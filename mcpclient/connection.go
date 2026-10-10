package mcpclient

import "github.com/modelcontextprotocol/go-sdk/mcp"

type ServerConnection struct {
	Config  ServerConfig
	Client  *mcp.Client
	Session *mcp.ClientSession
}
