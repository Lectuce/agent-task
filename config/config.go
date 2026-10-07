package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

var WORKDIR = initWorkDir()
var SKILLSDIR = filepath.Join(WORKDIR, ".skills")
var TOOL_RESULTS_DIR = filepath.Join(WORKDIR, ".task_outputs", "tool-results")
var TRANSCRIPT_DIR = filepath.Join(WORKDIR, ".transcripts")
var MEMORY_DIR = filepath.Join(WORKDIR, ".memory")
var MEMORY_INDEX = filepath.Join(MEMORY_DIR, "MEMORY.md")
var TASKS_DIR = filepath.Join(WORKDIR, ".tasks")
var DURABLE_PATH = filepath.Join(WORKDIR, ".cron")
var MAILBOX_DIR = filepath.Join(WORKDIR, ".mailboxes")

const (
	KEEP_RECENT_TOOL_RESULTS = 3
	MAX_CONSECUTIVE_529      = 3
	MAX_RECOVERY_RETRIES     = 3
	CONTEXT_LIMIT            = 50000
	KEEP_RECENT              = 3
	MAX_REACTIVE_RETRIES     = 1
	PERSIST_THRESHOLD        = 30000
	TOOL_RESULT_MAX_BYTES    = 200000
	MAX_MESSAGES             = 50
	MAX_AGENT_ROUNDS         = 20
	IDLE_POLL_INTERVAL       = 5 * time.Second
	IDLE_TIMEOUT             = 60 * time.Second
)

var Client = anthropic.NewClient(
	option.WithAPIKey(os.Getenv("API_KEY")),
	option.WithBaseURL(os.Getenv("BASE_URL")),
)

var FALLBACK_MODEL = os.Getenv("FALLBACK_MODEL")
var MODEL = os.Getenv("MODEL")
var PRIMARY_MODEL = MODEL

func envInt64(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

var DEFAULT_MAX_TOOKENS = envInt64("DEFAULT_MAX_TOOKENS", 8000)
var ESCALATED_MAX_TOKENS = envInt64("ESCALATED_MAX_TOKENS", 64000)

var DEFAULT_COMPACT_TOOKENS = envInt64("DEFAULT_COMPACT_TOOKENS", 8000)
var DEFAULT_MEMORY_TOOKENS = envInt64("DEFAULT_COMPACT_TOOKENS", 200)

var SUBSYSTEM = []anthropic.TextBlockParam{
	{
		Text: fmt.Sprintf("You are a coding agent at %v.", WORKDIR),
	},
	{
		Text: "Complete the task you were given, then return a concise summary.",
	},
	{
		Text: "Do not delegate further.",
	},
}
