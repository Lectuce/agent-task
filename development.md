# 开发记录

这份文档主要记两件事：代码为什么这样拆，以及开发过程中踩过哪些坑。

运行方法和配置项放在 [`README.md`](./README.md)，这里不再重复。想从代码入手，可以先看 `app.go` 和 `loop/loop.go`，再顺着工具、上下文和任务模块往下读。

## 1. Agent Loop

最早的版本只有一次模型请求：

```text
User -> LLM -> Answer
```

这还不够。模型即使返回了 `tool_use`，程序也没有机会执行工具，更不会把结果交回去继续判断。

现在的主循环是：

```text
messages
   |
   v
  LLM ---- end_turn ----> 输出答案
   |
 tool_use
   |
   v
执行工具
   |
   v
tool_result
   |
   +---------------------> 回到 messages
```

`loop.AgentLoop` 每轮检查 `stop_reason`。只要模型还在调用工具，就把 Assistant 消息和 Tool Result 都追加到历史中，再发起下一次请求。单个任务最多执行 `MAX_AGENT_ROUNDS` 轮，避免模型陷入没有出口的循环。

轮数必须是 `AgentLoop` 的局部变量，不能放在 package 级别。否则多个请求会共用一个计数器，新任务可能一开始就撞上上一轮留下的上限。

## 2. 两份消息

Loop 里同时保留 `history` 和 `requestMessages`，两者用途不同：

- `history` 是 Session 的真实消息历史。
- `requestMessages` 是这一轮实际发给模型的副本，可以注入 Memory，也可以被压缩。

如果直接在 Session 历史上做压缩或 Memory 注入，临时内容会永久留下。下一轮再处理一次，就会出现重复 Memory、重复摘要，最后很难分清用户到底说过什么。

`memory.CloneMessages` 通过 JSON 编解码做深拷贝。新消息写入时，`memory.AppendMessage` 会分别更新两份数据。Loop 结束后只把 `history` 写回当前 Session。

Session 要以指针传给 Loop：

```go
func AgentLoop(
    query string,
    ctx context.Context,
    promptCtx *prompt.PromptContext,
    currentSession *session.Session,
    cronManager *cron.Manager,
    mcpManager *mcpclient.Manager,
) error
```

传值会复制 `Session` 结构，后续对消息、Todo 轮数和后台任务状态的修改容易丢失。

## 3. 工具注册

工具多起来以后，不能继续在 Loop 里写一长串 `if block.Name == ...`。本地工具统一放进 Handler 表：

```go
type ToolHandler func(map[string]any) (string, error)

var ToolHandlers = map[string]ToolHandler{
    "bash":       runBash,
    "read_file":  runRead,
    "write_file": runWrite,
    "edit_file":  runEdit,
    "glob":       runGlob,
}
```

模型看到的是 Tool Schema；执行时再用工具名查 Handler。这样新增工具通常只需要补 Schema 和 Handler，不用改主循环的分支。

目前有三类工具：

```text
本地工具    由 Go Handler 执行，例如 bash、read_file、calculator
服务端工具  由模型服务执行，目前是 web_search
MCP 工具   通过 MCP Client 转发给外部 Server
```

`web_search` 不在 `ToolHandlers` 里，因为它不是本地调用。MCP 工具会在启动时动态加入 Handler 路由。

### 工具名必须一致

Schema 里的名称和 Handler 表的 Key 必须完全相同。比如 Schema 注册 `read_file`，Handler 却写成 `readFile`，运行时只会得到：

```text
unknown tool: read_file
```

这个问题看起来像模型调用错了，实际是本地注册表没有对齐。

### 不要直接断言工具参数

工具参数来自 JSON，数字一般会解成 `float64`，可选字段也可能根本不存在。下面这种写法很容易 panic：

```go
offset := input["offset"].(int)
```

需要先检查字段，再处理可能的类型：

```go
if value, ok := input["offset"]; ok {
    switch number := value.(type) {
    case float64:
        offset = int(number)
    case int:
        offset = number
    }
}
```

必填字符串和布尔值则通过 `requireString`、`requireBool` 这类小函数统一检查。

## 4. 工具失败不是 Loop 失败

`read_file` 找不到文件、Shell 返回非零状态、Permission 拒绝操作，这些都只是一次工具调用失败。直接 `return err` 会结束整个任务，模型也看不到失败原因。

现在会把错误放进 `tool_result`：

```go
anthropic.NewToolResultBlock(
    block.ID,
    err.Error(),
    true,
)
```

模型拿到错误后可以换路径、修改命令，或者向用户解释为什么做不了。只有 Runtime 自己无法继续工作时，例如请求结构损坏或重试耗尽，Loop 才返回错误。

同样的原则也用于 Permission。拒绝危险操作时，返回的是错误 Tool Result，而不是直接杀掉当前会话。

## 5. Hooks 和权限检查

下面这些逻辑都不属于主循环本身：

- 给用户输入加上工作目录
- 工具执行前做权限检查
- 记录工具调用
- 检查过大的输出
- 本轮结束时打印统计

它们通过四个 Hook 接入：

```text
UserPromptSubmit
PreToolUse
PostToolUse
Stop
```

Hook 要真正修改 `HookContext` 才有用。只打印“当前工作目录是什么”不会让模型知道工作目录。`contextInjectHook` 会直接更新 `ctx.Query`，调用方随后使用更新后的 Query。

Shell 工具目前有三层限制：

1. 固定在工作区执行。
2. 运行超过 120 秒会取消。
3. 明确危险的命令走 deny list，可能破坏文件的命令走人工确认。

文件工具还会通过 `filepath.Rel` 检查最终路径，拒绝逃出工作区。

这些检查只是 Runtime 内的防护，不是操作系统沙箱。子进程仍然继承启动 Agent 的用户权限。

## 6. 上下文压缩

不能等 API 报 context too long 才处理历史。当前每次模型请求前都会依次经过：

```text
ToolResultBudget
       |
       v
SnipCompact
       |
       v
MicroCompact
       |
       v
EstimateSize > CONTEXT_LIMIT ?
       |
       v
CompactHistory
```

各层做的事情不一样：

- `ToolResultBudget` 检查最近一条消息里的工具结果。总量过大时，把大结果写入 `.task_outputs/tool-results`，上下文里只留路径和预览。
- `SnipCompact` 在消息超过 50 条时保留开头和最近部分，中间换成一条裁剪说明。裁剪时会尽量保证 `tool_use` 和 `tool_result` 成对。
- `MicroCompact` 保留最近三个工具结果，较旧且超过 120 字节的结果替换为占位文本。
- `CompactHistory` 在估算大小超过阈值后保存完整 transcript，再让模型把历史压成一条摘要。

如果 API 仍然返回 prompt too long，会再走一次 `ReactiveCompact`。它同样先保存 transcript，但摘要后还会带上最近五条消息，尽量保住正在进行的工具调用。

大结果先落盘很重要。单纯截断虽然省 Token，但模型不知道完整内容还能去哪里找。

## 7. Memory

Memory 不直接追加到 `Session.Messages`。每轮都永久追加会造成三件事：相同内容反复出现、Context 越来越大、长期信息混进真实对话。

当前流程是：

```text
最近的用户消息
      |
      v
选择最多 5 个相关 Memory
      |
      v
读取 .memory 中的文件
      |
      v
注入 requestMessages 的当前用户消息
      |
      v
发送给模型
```

相关性优先交给模型判断，只要求返回文件索引。如果选择请求失败或格式不对，再退回到关键词匹配。

一轮任务结束后，`ExtractMemories` 查看最近十条真实历史，提取用户偏好、反馈、项目事实和外部引用。没有新内容就返回空数组，不写文件。

这里有一个容易忽略的边界：压缩只作用在 `requestMessages`，Memory 提取使用的是 `history`。否则摘要过程可能把值得长期保留的细节提前丢掉。

## 8. Todo、Task 和 Subagent

这三个模块解决的不是同一件事。

### Todo

Todo 是当前 Session 里的短期清单，状态只有：

```text
pending
in_progress
completed
```

如果模型连续三轮没有更新 Todo，Loop 会插入一条提醒。Todo 不负责跨进程保存，也不处理任务所有者和依赖关系。

### Task

Task 写在 `.tasks` 中，除了状态还有 `Owner`、`BlockedBy` 和 `Worktree`。领取任务前会检查依赖是否完成，认领过程用互斥锁保护，避免两个 Teammate 同时拿到同一个任务。

完成 Task 后会重新扫描任务列表，并报告哪些任务刚刚解除阻塞。

### Subagent

`task` 工具启动的 Subagent 使用独立的消息列表和较小的工具集。主 Agent 只拿最终结果，不接收子任务的每一步历史。

```text
Main Agent -> task -> Subagent Loop -> final result -> tool_result
```

这样能隔开上下文，但 Subagent 仍在同一个进程和工作区中运行。它适合一次性的独立工作，不负责长期协作。

## 9. Teams 和 Worktree

Teammate 是后台 goroutine，拥有自己的消息历史、工具和空闲轮询。它可以读取收件箱、领取 Task、提交计划，并把结果发回 `lead`。

消息总线使用 `.mailboxes/<agent>.jsonl`。写入时追加 JSON 行，读取时一次取走整个收件箱并删除文件。协议消息带 `request_id`，用于匹配计划审核和关闭请求。

空闲 Teammate 会定期查看两类工作：

1. 自己的收件箱有没有新消息。
2. 有没有依赖已经满足、但还没人领取的 Task。

多个成员直接写同一个目录容易互相覆盖，因此 Task 可以绑定 Worktree。创建 Worktree 时会建立 `wt/<name>` 分支，并把目录放在 `.worktrees/<name>`。

删除 Worktree 前会检查未提交文件和未推送提交。无法确认状态或检测到改动时，默认拒绝删除；调用方必须明确选择强制丢弃，或者用 `keep_worktree` 留给人工检查。

## 10. 后台命令

构建、测试和安装经常跑得比一次模型请求久。如果一直阻塞 Loop，Agent 只能原地等命令结束。

`background.Manager` 会把识别为耗时的 Shell 命令放进 goroutine，并立即返回一个 `bg_xxxx` ID。命令结果按 Session 保存，在用户下一次交互时收集并注入历史。

目前是否属于耗时命令主要靠关键词判断，例如 `build`、`test`、`install`、`compile` 和 `docker build`。这种判断简单直接，但不是严格的命令分类器；新增工具或命令形态时要一起检查这里。

## 11. Cron 和执行锁

Cron Scheduler 每秒检查一次五字段表达式。匹配后不直接调用模型，而是先把 Job 放进队列：

```text
Scheduler -> Cron Queue -> QueueProcessor -> AgentLoop
```

REPL 请求和 Cron 都可能进入 Agent Loop，因此共用一把 `AgentLock`。REPL 会正常加锁；Queue Processor 用 `TryLock`，主循环忙时先把任务留在队列，下一次轮询再处理。

一次性 Job 触发后会从注册表删除。带 `durable` 的 Job 保存到 `.cron`，程序启动时重新加载。

调度和执行分开后，不需要用后台 Shell、`sleep` 循环或额外进程实现定时任务。

## 12. Skills 和 System Prompt

如果把所有 `SKILL.md` 都塞进 System Prompt，Skill 越多，固定上下文就越大，而且大部分内容与当前任务无关。

启动时只扫描每个 Skill 的 `name` 和 `description`，生成一个目录。模型真正需要某个 Skill 时，再调用 `load_skill` 读取正文。

System Prompt 由 `prompt.GetSystemPrompt` 组装，主要包含身份、工作区、Memory 提示、调度规则、工具名称和 Skill 目录。真实对话继续放在 `messages` 中，两者不混在一起。

## 13. MCP

MCP 连接发生在应用启动阶段：

```text
读取环境变量
    |
    v
选择 HTTP 或 stdio Transport
    |
    v
连接 Server
    |
    v
tools/list
    |
    v
过滤并注册工具
```

远程工具名会被整理成：

```text
mcp_<server>_<tool>
```

连字符、空格、点和斜杠都会换成下划线。注册前会先检查转换后的 Schema，也会检查工具名冲突。

`MCP_ALLOW_TOOLS` 和 `MCP_DENY_TOOLS` 针对 Server 返回的原始工具名。Deny 优先；Allow 为空时默认接受其余工具。

Manager 内部按 Server 保存 Session 和路由，但当前 `setupMCP` 只从环境变量组装一个 Server。要接多个 Server，扩展点在应用组装层，不需要改工具执行逻辑。

## 14. API 恢复

恢复逻辑集中在 `recovery` 包，避免每个模型请求各写一套重试。

### 429 和服务过载

Rate Limit 和 Overloaded Error 使用指数退避并加一点随机抖动。连续遇到三次 Overloaded 后，如果配置了 `FALLBACK_MODEL`，后续请求切到备用模型。

### 输出被截断

`max_tokens` 是 `stop_reason`，不是普通 API Error。第一次出现时提高 `maxTokens`；仍然被截断时，保留已经生成的 Assistant 内容，再追加一条继续输出的消息。最多续写三次。

### 输入过长

检测到 prompt too long 后只尝试一次 `ReactiveCompact`。如果压缩后仍然过长，就把错误写进历史并结束本轮，避免无限压缩和重试。

## 15. Session 和日志

一个进程可以维护多个逻辑 Session，不需要为每个话题启动一个终端：

```go
type SessionManager struct {
    Sessions map[string]*Session
    Current  string
}
```

每个 Session 保存自己的消息、Todo 轮数、Logger 和后台任务 Manager。日志写到 `logs/<session>.log`，记录工具调用、结果和错误，排查问题时不会和其他 Session 混在一起。

Session 当前只存在内存中。进程退出后，消息历史不会恢复；需要跨进程保存的数据分别由 Memory、Task 和 Cron 处理。

## 16. 排查问题时先分层

开发过程中有几类错误看起来很像，但处理方向完全不同。

### 命令没有被 REPL 识别

例如：

```text
/ session switch default
```

因为 `/` 后面多了空格，它不会匹配 Session 命令，而是被当作普通问题发给模型。正确写法是：

```text
/session switch default
```

### Provider 配置错误

`UnsupportedModel`、404、凭证错误通常先检查：

- `BASE_URL` 是否正确
- `MODEL` 是否存在
- Provider 是否支持当前请求中的工具类型
- API Key 是否传给了正确的服务

这类错误不一定来自 Agent Loop。

### Runtime 错误

如果报 `unknown tool`、Tool Result 配对失败或 JSON 类型断言错误，再去查 Schema、Handler、消息顺序和参数解析。

先判断错误属于 CLI、Provider 还是 Runtime，通常比直接改 Loop 更快。

## 17. 目前的调用关系

```text
                       App
                        |
          +-------------+-------------+
          |             |             |
        REPL        Cron Queue      MCP Manager
          |             |             |
          +-------------+-------------+
                        |
                    AgentLoop
                        |
          +-------------+-------------+
          |             |             |
       Memory        Compact        Recovery
          |             |             |
          +-------------+-------------+
                        |
                       LLM
                        |
                  local tool_use
                        |
              Hooks -> Handler
                        |
       +----------------+----------------+
       |                |                |
   File/Shell       Task/Team       Cron/Worktree
```

主循环只处理消息协议和调度。文件、任务、记忆、MCP、定时任务等状态各自留在对应包里。新增模块时，优先接现有的 Handler、Hook 或 Manager，不要继续往 `AgentLoop` 里塞特例。

## 18. 提交前检查

```bash
go fmt ./...
go test ./...
go build ./...
```

如果改了工具，还要额外确认：

- Schema 名称和 Handler Key 一致
- 必填字段与实际解析一致
- 普通执行错误会返回 Tool Result
- 写文件时没有越出工作区
- Subagent 或 Teammate 是否也需要这个工具
- 大输出是否会进入压缩或落盘流程
