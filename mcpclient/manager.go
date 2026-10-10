package mcpclient

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"sort"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Manager struct {
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

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*mcp.ClientSession, 0),
		tools:    make(map[string]RemoteTool, 0),
	}
}

// 检查配置。
// 创建 MCP Client。
// 创建 Transport。
// 建立 Session。
// 调用 tools/list。
// 注册工具路由。

func (m *Manager) Connect(ctx context.Context, config ServerConfig) error {

	if config.Name == "" {
		return fmt.Errorf("MCP server name is required.")
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    config.Name,
		Version: "v1.0.0",
	}, nil)

	var transport mcp.Transport

	switch config.Transport {
	case "stdio":
		if config.Command == "" {
			return fmt.Errorf(
				"MCP server %q: command is required",
				config.Name,
			)
		}

		transport = &mcp.CommandTransport{
			Command: exec.Command(
				config.Command,
				config.Args...,
			),
		}

	case "http":
		if config.URL == "" {
			return fmt.Errorf(
				"MCP server %q: URL is required",
				config.Name,
			)
		}

		transport = &mcp.StreamableClientTransport{
			Endpoint: config.URL,
		}

	default:
		return fmt.Errorf(
			"MCP server %q: unsupported transport %q",
			config.Name,
			config.Transport,
		)
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect MCP server %q: %w", config.Name, err)
	}

	discovered, err := listAllTools(ctx, session)
	if err != nil {
		closeErr := session.Close()
		if closeErr != nil {
			return closeErr
		}
		return fmt.Errorf("list tools from MCP server %q: %w", config.Name, err)
	}

	newRoutes := make(map[string]*RemoteTool)

	for _, defination := range discovered {

		if defination == nil || defination.Name == "" {
			continue
		}

		exposedName := buildExposedName(config.Name, defination.Name)

		route := &RemoteTool{
			ExposeName:   exposedName,
			ServerName:   config.Name,
			OriginalName: defination.Name,
			Definition:   defination,
		}

		_, err = toAnthropicTool(route)
		if err != nil {
			_ = session.Close()
			return fmt.Errorf("convert MCP tool %v: %v", exposedName, err)
		}

		_, ok := newRoutes[exposedName]
		if ok {
			_ = session.Close()

			return fmt.Errorf("duplicate MCP tool name: %s", exposedName)
		}

		newRoutes[exposedName] = route

	}

	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.sessions[config.Name]
	if ok {
		_ = session.Close()

		return fmt.Errorf("MCP server already connected: %s", config.Name)
	}

	for name := range newRoutes {
		_, ok := m.tools[name]
		if ok {
			_ = session.Close()

			return fmt.Errorf("MCP tool name collision: %s", name)
		}
	}

	m.sessions[config.Name] = session

	for name, route := range newRoutes {
		m.tools[name] = *route
	}

	return nil

}

func (m *Manager) BuildTools() ([]anthropic.ToolUnionParam, error) {

	m.mu.Lock()

	routes := make([]*RemoteTool, 0, len(m.tools))
	for _, route := range m.tools {
		routes = append(routes, &route)
	}

	m.mu.Unlock()

	sort.Slice(routes, func(i, j int) bool {
		return routes[i].ExposeName < routes[j].ExposeName
	})

	result := make(
		[]anthropic.ToolUnionParam,
		0,
		len(routes),
	)

	for _, route := range routes {
		toolParam, err := toAnthropicTool(route)
		if err != nil {
			return nil, fmt.Errorf(
				"convert MCP tool %q: %w",
				route.ExposeName,
				err,
			)
		}

		result = append(
			result,
			anthropic.ToolUnionParam{
				OfTool: &toolParam,
			},
		)
	}

	return result, nil

}

func (m *Manager) ToolNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := make([]string, 0, len(m.tools))

	for _, name := range m.tools {
		names = append(names, name.)
	}

	slices.Sort(names)

	return names


}

func (m *Manager) Execute(ctx context.Context, exposedName string, input map[string]any) (string, error)

func (m *Manager) Close() error
