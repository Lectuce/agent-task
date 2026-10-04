package tool

type ToolHandler func(map[string]any) (string, error)

var ToolHandlers = map[string]ToolHandler{
	"bash":          runBash,
	"read_file":     runRead,
	"write_file":    runWrite,
	"edit_file":     runEdit,
	"glob":          runGlob,
	"todo_write":    runTodoWrite,
	"load_skill":    loadSkill,
	"calculator":    runCalculator,
	"create_task":   runCreateTask,
	"list_tasks":    runListTasks,
	"get_task":      runGetTask,
	"claim_task":    runClaimTask,
	"complete_task": runCompleteTask,
	"schedule_cron": runScheduleCron,
	"list_crons":    runListCrons,
	"cancel_cron":   runCancleCron,
}

var SubHandlers = map[string]ToolHandler{
	"bash":       runBash,
	"read_file":  runRead,
	"write_file": runWrite,
	"edit_file":  runEdit,
	"glob":       runGlob,
	"calculator": runCalculator,
}
