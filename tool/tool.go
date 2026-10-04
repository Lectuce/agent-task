package tool

import (
	"sort"

	"github.com/anthropics/anthropic-sdk-go"
)

var ClientTools = []anthropic.ToolParam{
	{
		Name:        "bash",
		Description: anthropic.String("Run a shell command."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"command": map[string]interface{}{
					"type": "string",
				},
				"bash": map[string]interface{}{
					"type": "boolean",
				},
			},
			Required: []string{"command"},
		},
	},
	{
		Name:        "read_file",
		Description: anthropic.String("Read file contents."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type": "string",
				},
				"limit": map[string]interface{}{
					"type": "integer",
				},
			},
			Required: []string{"path"},
		},
	},
	{
		Name:        "write_file",
		Description: anthropic.String("Write content to a file."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type": "string",
				},
				"content": map[string]interface{}{
					"type": "string",
				},
			},
			Required: []string{"path", "content"},
		},
	},
	{
		Name:        "edit_file",
		Description: anthropic.String("Replace exact text in a file once."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type": "string",
				},
				"old_text": map[string]interface{}{
					"type": "string",
				},
				"new_text": map[string]interface{}{
					"type": "string",
				},
			},
			Required: []string{"path", "old_text", "new_text"},
		},
	},
	{
		Name:        "glob",
		Description: anthropic.String("Find files matching a glob pattern."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"pattern": map[string]interface{}{
					"type": "string",
				},
			},
			Required: []string{"pattern"},
		},
	},
	{
		Name:        "todo_write",
		Description: anthropic.String("Create and manage a task list ..."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"todos": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"content": map[string]interface{}{
								"type": "string",
							},
							"status": map[string]interface{}{
								"type": "string",
								"enum": []string{"pending", "in_progress", "completed"},
							},
						},
					},
				},
			},
			Required: []string{"todos"},
		},
	},

	{
		Name:        "task",
		Description: anthropic.String("Launch a subagent to handle a complex subtask. Returns only the final conclusion."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"description": map[string]interface{}{
					"type": "string",
				},
			},
			Required: []string{"description"},
		},
	},

	{
		Name:        "load_skill",
		Description: anthropic.String("Load the full content of a skill by name."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"name": map[string]interface{}{
					"type": "string",
				},
			},
			Required: []string{"name"},
		},
	},

	{
		Name:        "calculator",
		Description: anthropic.String("Evaluate a mathematical expression."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"expression": map[string]interface{}{
					"type":        "string",
					"description": "Evaluate an arithmetic expression containing numbers, parentheses, and arithmetic operators such as +, -, *, /, and %.",
				},
			},
			Required: []string{"expression"},
		},
	},

	{
		Name:        "create_task",
		Description: anthropic.String("Create a new task with optional blockedBy dependencies."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"subject": map[string]interface{}{
					"type": "string",
				},
				"description": map[string]interface{}{
					"type": "string",
				},
				"blockedBy": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
			},
			Required: []string{"subject"},
		},
	},

	{
		Name:        "list_tasks",
		Description: anthropic.String("List all tasks with status, owner, and dependencies."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"properties": map[string]interface{}{},
			},
			Required: []string{},
		},
	},

	{
		Name:        "get_task",
		Description: anthropic.String("Get full details of a specific task by ID."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"task_id": map[string]interface{}{
					"type": "string",
				},
			},
			Required: []string{"task_id"},
		},
	},

	{
		Name:        "claim_task",
		Description: anthropic.String("Claim a pending task. Sets owner, changes status to in_progress."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"task_id": map[string]interface{}{
					"type": "string",
				},
			},
			Required: []string{"task_id"},
		},
	},

	{
		Name:        "complete_task",
		Description: anthropic.String("Complete an in-progress task. Reports unblocked downstream tasks."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"task_id": map[string]interface{}{
					"type": "string",
				},
			},
			Required: []string{"task_id"},
		},
	},
	{
		Name:        "schedule_cron",
		Description: anthropic.String("Schedule a cron job. cron is 5-field: min hour dom month dow."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"cron_expr": map[string]interface{}{
					"type":        "string",
					"description": "5-filed cron expression",
				},
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "Message to inject when fired",
				},
				"recurring": map[string]interface{}{
					"type":        "boolean",
					"description": "True=recurring, False=one-shot",
				},
				"durable": map[string]interface{}{
					"type":        "boolean",
					"description": "True=persist to disk",
				},
			},
			Required: []string{"cron_expr", "prompt"},
		},
	},

	{
		Name:        "list_crons",
		Description: anthropic.String("List all registered cron jobs."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"cron": map[string]interface{}{},
			},
			Required: []string{},
		},
	},

	{
		Name:        "cancel_cron",
		Description: anthropic.String("Cancel a cron job by ID."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"job_id": map[string]interface{}{
					"type": "string",
				},
			},
			Required: []string{"job_id"},
		},
	},
	{
		Name:        "spawn_teammate",
		Description: anthropic.String("Spawn a teammate agent in a background thread."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"name": map[string]interface{}{
					"type": "string",
				},
				"role": map[string]interface{}{
					"type": "string",
				},
				"prompt": map[string]interface{}{
					"type": "string",
				},
			},
			Required: []string{"name", "role", "prompt"},
		},
	},
	{
		Name:        "send_message",
		Description: anthropic.String("Send a message to a teammate via MessageBus."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: "object",
			Properties: map[string]interface{}{
				"to": map[string]interface{}{
					"type": "string",
					"content": map[string]interface{}{
						"type": "string",
					},
				},
			},
			Required: []string{"to", "content"},
		},
	},
	{
		Name:        "check_inbox",
		Description: anthropic.String("Check Lead's inbox for teammate messages."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type:       "object",
			Properties: map[string]interface{}{},
			Required:   []string{},
		},
	},
}

var ServerTools = []anthropic.ToolUnionParam{
	{
		OfWebSearchTool20250305: &anthropic.WebSearchTool20250305Param{
			MaxUses: anthropic.Int(5),
		},
	},
}

func BuildTools() []anthropic.ToolUnionParam {
	tools := make([]anthropic.ToolUnionParam, 0, len(ClientTools)+len(ServerTools))

	for i := range ClientTools {
		toolParam := ClientTools[i]

		tools = append(tools,
			anthropic.ToolUnionParam{
				OfTool: &toolParam,
			},
		)
	}

	tools = append(tools, ServerTools...)

	return tools
}

func ToolNames() []string {
	names := make([]string, 0)

	for _, clientTool := range ClientTools {
		names = append(names, string(clientTool.Name))
	}

	names = append(names, "web_search")
	sort.Strings(names)

	return names
}
