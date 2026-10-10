package main

import (
	"agent/config"
	"agent/mcpclient"
	"context"
	"fmt"
)

func buildMCPConfig() mcpclient.ServerConfig {
	return mcpclient.ServerConfig{
		Name:      config.MCP_SERVER_NAME,
		Enabled:   config.MCP_ENABLED,
		Required:  config.MCP_REQUIRED,
		Transport: config.MCP_TRANSPORT,

		URL:     config.MCP_URL,
		Headers: config.MCPHeaders(),

		Command: config.MCP_COMMAND,
		Args:    config.MCP_ARGS,

		Timeout:    config.MCP_TIMEOUT,
		AllowTools: config.MCP_ALLOW_TOOLS,
		DenyTools:  config.MCP_DENY_TOOLS,
	}
}

func validateMCPConfig(serverConfig mcpclient.ServerConfig) error {

	if serverConfig.Name == "" {
		return fmt.Errorf("MCP_SERVER_NAME is required")
	}

	switch serverConfig.Transport {
	case "http":
		if serverConfig.URL == "" {
			return fmt.Errorf("MCP_URL is required for HTTP transport")
		}

	case "stdio":
		if serverConfig.Command == "" {
			return fmt.Errorf("MCP_COMMAND is required for stdio transport")
		}

	default:
		return fmt.Errorf("unsupported MCP transport: %s", serverConfig.Transport)
	}

	return nil
}

func setupMCP(ctx context.Context) (*mcpclient.Manager, error) {
	mcpManager := mcpclient.NewManager()
	serverConfig := buildMCPConfig()

	if serverConfig.Enabled == false {
		return mcpManager, nil
	}

	err := validateMCPConfig(serverConfig)
	if err != nil {
		return nil, err
	}

	err = mcpManager.Connect(ctx, serverConfig)

	if err != nil {
		return nil, err
	}

	return mcpManager, nil
}
