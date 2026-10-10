package mcpclient

import "time"

type ServerConfig struct {
	Name      string
	Enabled   bool
	Required  bool
	Transport string // stdio 或 http

	// stdio
	Command string
	Args    []string
	Env     map[string]string

	// HTTP
	URL     string
	Headers map[string]string

	Timeout time.Duration

	AllowTools []string
	DenyTools  []string
}
