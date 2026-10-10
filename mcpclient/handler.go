package mcpclient

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listAllTools(ctx context.Context, session *mcp.ClientSession) ([]*mcp.Tool, error) {

	var tools []*mcp.Tool
	cursor := ""

	for {

		result, err := session.ListTools(ctx, &mcp.ListToolsParams{
			Cursor: cursor,
		})
		if err != nil {
			return nil, err
		}

		tools = append(tools, result.Tools...)

		if result.NextCursor == "" {
			return tools, nil
		}

		cursor = result.NextCursor
	}

}

func buildExposedName(serverName string, toolName string) string {
	serverName = normalizeName(serverName)
	toolName = normalizeName(toolName)

	return "mcp_" + serverName + "_" + toolName

}

func normalizeName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)

	replacer := strings.NewReplacer(
		"-", "_",
		" ", "_",
		".", "_",
		"/", "_",
	)

	return replacer.Replace(value)

}

func toAnthropicTool(remote *RemoteTool) (anthropic.ToolParam, error) {

	var inputSchema anthropic.ToolInputSchemaParam

	if remote.Definition.InputSchema != nil {

		data, err := json.Marshal(remote.Definition.InputSchema)

		if err != nil {
			return anthropic.ToolParam{}, err
		}

		err = json.Unmarshal(data, &inputSchema)
		if err != nil {
			return anthropic.ToolParam{}, err
		}
	}

	return anthropic.ToolParam{
		Name:        remote.ExposeName,
		Description: anthropic.String(remote.Definition.Description),
		InputSchema: inputSchema,
	}, nil

}

func formatToolResult(result *mcp.CallToolResult) (string, error) {
	parts := make([]string, 0)

	for _, content := range result.Content {
		switch value := content.(type) {
		case *mcp.TextContent:
			parts = append(parts, value.Text)

		default:
			data, err := json.Marshal(value)
			if err != nil {
				return "", err
			}
			parts = append(parts, string(data))
		}
	}

	if result.StructuredContent != nil {
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return "", err
		}
		parts = append(parts, string(data))
	}

	if len(parts) == 0 {
		return "MCP tool completed successfully", nil
	}

	return strings.Join(parts, "\n"), nil
}
