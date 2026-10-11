# Agent

这是一个使用 Go 实现的 agent。



文件读写、Shell、任务拆分、记忆、定时任务和 MCP 都接在这个循环上。程序启动时的目录就是 Agent 的工作区。

## 功能

- 读写和查找文件，执行 Shell 命令
- Todo 和带依赖关系的持久任务
- Subagent，以及可以互相发消息的 Teams
- 从 `.skills` 目录加载 Skill
- 自动压缩过长的对话和工具输出
- 从历史对话里提取并召回 Memory
- 在一个进程里创建多个 Session
- 后台执行耗时的构建、测试和安装命令
- 一次性或周期性的 Cron 任务
- 通过 HTTP 或 stdio 连接 MCP Server
- 为并行任务创建 Git Worktree
- API 重试、备用模型和输出截断续写

模型可以使用的本地工具包括：

```text
bash              read_file          write_file
edit_file         glob               calculator
todo_write        task               load_skill
create_task       list_tasks         get_task
claim_task        complete_task      schedule_cron
list_crons        cancel_cron        spawn_teammate
send_message      check_inbox        request_shutdown
request_plan      review_plan        create_worktree
remove_worktree   keep_worktree
```

另外还会注册服务端的 `web_search`，以及 MCP Server 提供的工具。

## 运行


下载依赖：

```bash
go mod download
```

配置模型：

```bash
export API_KEY="your-api-key"
export BASE_URL="https://your-api-endpoint"
export MODEL="your-model"
```

PowerShell：

```powershell
$env:API_KEY = "your-api-key"
$env:BASE_URL = "https://your-api-endpoint"
$env:MODEL = "your-model"
```

在项目目录启动：

```bash
go run .
```

启动后直接输入任务：

```text
输入问题，回车发送。输入 q 退出。
agent[default] >> 修复测试，并说明问题出在哪里
```

输入 `q`、`exit` 或空行退出。

## Session

每个 Session 有各自的消息历史和后台任务。下面三个命令由 REPL 直接处理，不会发给模型：

```text
/session new <name>       新建并切换 Session
/session switch <name>    切换 Session
/session list             查看所有 Session
```

Session 只保留在当前进程中，退出程序后不会恢复。Memory、Task 和 durable Cron 有各自的持久化文件，不受这个限制。

## 配置

模型相关配置：

| 环境变量 | 默认值 | 用途 |
| --- | --- | --- |
| `API_KEY` | 无 | API Key |
| `BASE_URL` | 无 | API 地址 |
| `MODEL` | 无 | 主模型 |
| `FALLBACK_MODEL` | 无 | 服务连续过载时切换的模型 |
| `DEFAULT_MAX_TOOKENS` | `8000` | 一般请求的最大输出 Token 数 |
| `ESCALATED_MAX_TOKENS` | `64000` | 输出被截断后使用的上限 |
| `DEFAULT_COMPACT_TOOKENS` | `8000` | 压缩历史时的输出上限 |

前三项需要在启动前设置。

代码里还有一些固定参数，例如单次任务最多执行 20 轮、Shell 超时 120 秒、上下文阈值 50000。它们目前定义在 [`config/config.go`](./config/config.go) 中。

## Skills

Skill 放在工作区的 `.skills` 目录下：

```text
.skills/
└── my-skill/
    └── SKILL.md
```

`SKILL.md` 可以带一段 YAML frontmatter：

```markdown
---
name: my-skill
description: 这个 Skill 适合处理什么任务
---

这里写具体指令。
```

启动时只读取名称和描述。模型决定使用某个 Skill 后，才会通过 `load_skill` 读取正文。

## Cron

Cron 使用五字段格式：

```text
分钟 小时 日期 月份 星期
```

支持 `*`、`*/n`、逗号枚举、范围和具体数字。例如：

```text
*/10 * * * *
```

表示每十分钟执行一次。任务可以是一次性的，也可以重复执行。设置 `durable=true` 后，任务会写入 `.cron`，下次启动时重新加载。

到点后，Cron 只是把 Prompt 放进主队列，实际工作仍由同一个 Agent Loop 完成。

## MCP

MCP 默认关闭。打开后，程序会在启动阶段连接 Server 并读取工具列表。

HTTP 示例：

```powershell
$env:MCP_ENABLED = "true"
$env:MCP_SERVER_NAME = "example"
$env:MCP_TRANSPORT = "http"
$env:MCP_URL = "http://localhost:8080/mcp"
$env:MCP_TOKEN = "your-token"
go run .
```

stdio 示例：

```powershell
$env:MCP_ENABLED = "true"
$env:MCP_SERVER_NAME = "example"
$env:MCP_TRANSPORT = "stdio"
$env:MCP_COMMAND = "npx"
$env:MCP_ARGS = "-y,@example/mcp-server"
go run .
```

可用配置如下：

| 环境变量 | 默认值 | 用途 |
| --- | --- | --- |
| `MCP_ENABLED` | `false` | 是否连接 MCP Server |
| `MCP_SERVER_NAME` | 无 | Server 名称 |
| `MCP_TRANSPORT` | `http` | `http` 或 `stdio` |
| `MCP_URL` | 无 | HTTP 地址 |
| `MCP_COMMAND` | 无 | stdio 启动命令 |
| `MCP_ARGS` | 无 | 命令参数，逗号分隔 |
| `MCP_TIMEOUT` | `30s` | 工具调用超时 |
| `MCP_TOKEN` | 无 | HTTP Token |
| `MCP_AUTH_HEADER` | `Authorization` | 鉴权 Header |
| `MCP_AUTH_SCHEME` | `Bearer` | Token Scheme |
| `MCP_ALLOW_TOOLS` | 无 | 只加载这些工具，逗号分隔 |
| `MCP_DENY_TOOLS` | 无 | 排除这些工具，逗号分隔 |

MCP 工具在模型侧的名称是 `mcp_<server>_<tool>`。名称里的连字符、空格、点和斜杠会被换成下划线。

目前启动配置只接一个 MCP Server；`mcpclient.Manager` 本身可以管理多条连接。

## 工作区里会生成什么

| 路径 | 内容 |
| --- | --- |
| `.memory/` | 长期记忆 |
| `.tasks/` | 任务和依赖状态 |
| `.cron` | durable Cron |
| `.mailboxes/` | Team 消息 |
| `.worktrees/` | Agent 创建的 Worktree |
| `.transcripts/` | 压缩前保存的对话 |
| `.task_outputs/tool-results/` | 放不进上下文的工具输出 |
| `logs/` | Session 日志 |

`.skills/` 也在工作区内，不过它通常由用户提前准备，不是运行时生成的。

## 安全限制

文件工具只能访问工作区。Shell 也固定在工作区运行，并带有 120 秒超时和输出长度限制。

一些明确危险的命令会直接拒绝，例如 `rm -rf /`、`sudo`、`shutdown`、`reboot`、`mkfs` 和 `dd if=`。删除文件、写工作区外部或修改高风险权限时，Permission Hook 会要求确认。

这只是程序内的检查。Agent 仍然继承当前进程的系统权限。

## 代码结构

```text
main.go / app.go     进程入口和各模块的组装
repl.go              命令行交互
loop/                Agent Loop
tool/                工具定义、执行和权限检查
hook/                工具调用前后的 Hooks
compact/             上下文压缩
memory/              Memory 的提取、保存和召回
session/             Session
subagent/            Subagent Loop
teams/               多 Agent 协作和消息队列
task/                持久任务
cron/                定时任务
mcpclient/           MCP 连接和工具转发
skills/              Skill 扫描和加载
worktree/            Git Worktree 管理
recovery/            重试、回退和续写
background/          后台命令
```

实现过程和记录放在 [`development.md`](./development.md)。

## 开发

```bash
go fmt ./...
go test ./...
go build ./...
```
