package config

import (
	"os"
	"strconv"
	"time"
)

func envString(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func envBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}

	return parsed
}

func envDuration(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}

	return parsed
}

func MCPHeaders() map[string]string {
	if MCP_TOKEN == "" {
		return nil
	}

	value := MCP_TOKEN
	if MCP_AUTH_SCHEME != "" {
		value = MCP_AUTH_SCHEME + " " + MCP_TOKEN
	}

	return map[string]string{
		MCP_AUTH_HEADER: value,
	}
}
