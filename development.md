# AI Development Record

本文档记录 Go Agent 开发过程中使用 AI 辅助分析、设计与调试的主要问题。

## 1. Agent Loop：从单次 LLM 调用到循环

### Prompt

如何用 Go 从零实现一个最小 Agent Loop？模型需要能够自己决定直接回答还是调用工具，工具执行后再把结果返回给模型继续判断。

### Problem

如果只做一次模型调用：

```text
User
 ↓
LLM
 ↓
Answer
```

即使模型返回了工具调用，也没有形成真正的 Agent Runtime。

### Solution

维护持续增长的 messages[]，并用循环反复调用模型：

```text
              ┌──────────────────────────────┐
              │                              │
              ▼                              │
       ┌─────────────┐                       │
       │ messages[]  │                       │
       └──────┬──────┘                       │
              ▼                              │
       ┌─────────────┐                       │
       │     LLM     │                       │
       └──────┬──────┘                       │
              ▼                              │
       ┌─────────────┐                       │
       │ tool_use ?  │                       │
       └──────┬──────┘                       │
          是  │  否                          │
              │   └──────────► 最终回答      │
              ▼                              │
       ┌─────────────┐                       │
       │ 执行工具    │                       │
       └──────┬──────┘                       │
              ▼                              │
       ┌─────────────┐                       │
       │ tool_result │                       │
       └──────┬──────┘                       │
              └──────────────────────────────┘
```

核心思想：

```text
Model → Tool → Result → Model
```

## 2. Tool Registry：把工具从 Agent Loop 中拆出去

### Prompt

bash、read_file、write_file 等工具越来越多，如果全部写在 AgentLoop 里会很乱，应该怎么组织？

### Problem

如果不断在主循环中写：

```go
if block.Name == "bash" {
    ...
} else if block.Name == "read_file" {
    ...
}
```

新增工具会不断修改核心 Loop。

### Solution

统一定义 Tool Handler：

```go
type ToolHandler func(map[string]any) (string, error)
```

并建立 Registry：

```go
var ToolHandlers = map[string]ToolHandler{
    "bash":       runBash,
    "read_file":  runRead,
    "write_file": runWrite,
    "edit_file":  runEdit,
    "glob":       runGlob,
    "todo_write": runTodoWrite,
}
```

模型只需要看到 Tool Schema，Runtime 根据：

```go
handler, ok := ToolHandlers[block.Name]
```

找到对应实现。

### Result

Agent Loop 只负责协议与调度，具体工具逻辑独立维护。

## 3. Tool Name 不一致导致 unknown tool

### Problem

曾出现：

```text
unknown tool: read_file
```

### Cause

Tool Schema 中注册的是：

```text
read_file
```

而 Handler 使用了不同名称，例如：

```text
readFile
```

### Solution

统一 Tool Schema Name 和 Handler Registry Key：

```go
"read_file": runRead
```

### Result

模型返回的 tool_use.name 可以稳定匹配本地 Handler。

## 4. Tool 输入解析导致类型断言问题

### Problem

读取 Tool 参数时曾出现类似：

```text
panic: interface conversion: interface {} is nil, not int
```

### Cause

直接进行：

```go
offset := input["offset"].(int)
```

当字段不存在或 JSON 数字类型不是 int 时会 panic。

### Solution

先判断字段是否存在，并安全转换：

```go
offset := 0

if v, ok := input["offset"]; ok {
    switch n := v.(type) {
    case float64:
        offset = int(n)
    case int:
        offset = n
    }
}
```

### Result

Tool 参数缺失时使用默认值，而不是让整个 Runtime 崩溃。

## 5. Bash Tool：超时和危险命令限制

### Prompt

Agent 可以执行 bash，但如何避免命令无限运行或者执行明显危险操作？

### Solution

Bash Tool 增加：

- 执行超时
- 危险命令 deny list
- 输出长度限制

示意：

```go
ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
defer cancel()

cmd := exec.CommandContext(ctx, "bash", "-lc", command)
output, err := cmd.CombinedOutput()
```

危险操作示例：

```text
rm -rf /
sudo
shutdown
reboot
> /dev/
```

### Result

Bash Tool 从“直接执行任意 shell”变成受 Runtime 管理的工具。

## 6. Permission：拒绝工具后为什么不能直接退出

### Prompt

PreToolUse 判断一个工具危险后，是直接 return error，还是返回给模型？

### Problem

如果 Permission Denied 后直接：

```go
return errors.New("permission denied")
```

整个 Agent Loop 会结束。

### Solution

把拒绝转换成 Error ToolResult：

```go
anthropic.NewToolResultBlock(
    block.ID,
    "permission denied",
    true,
)
```

流程：

```text
tool_use
   ↓
PreToolUse
   ↓
Permission denied
   ↓
error tool_result
   ↓
LLM
```

### Result

模型可以根据失败结果修改策略，而不是 Runtime 直接退出。

## 7. Hooks：把扩展逻辑从主循环中抽离

### Prompt

Permission、Context Injection、Log 都放在 AgentLoop 里会越来越复杂，如何解耦？

### Solution

增加生命周期 Hook：

```text
UserPromptSubmit
PreToolUse
PostToolUse
Stop
```

用途：

- UserPromptSubmit：处理用户输入和上下文注入
- PreToolUse：执行前检查，例如 Permission
- PostToolUse：工具执行后处理
- Stop：本轮结束阶段处理

### Result

新增扩展时不需要不断修改核心 Agent Loop。

## 8. UserPromptSubmit：Hook 不能只打印日志

### Problem

如果 Hook 只是输出：

```text
[HOOK] UserPromptSubmit: working in /home/yujun/projects/agent
```

模型实际上没有收到工作目录信息。

### Solution

让 Hook 修改 HookContext.Query：

```go
ctx.Query = fmt.Sprintf(
    "<context>\nWorking directory: %s\n</context>\n\n%s",
    config.WORKDIR,
    ctx.Query,
)
```

Main 中再使用更新后的 Query：

```go
hookCtx := &hook.HookContext{Query: query}
hook.TriggerHooks(hook.UserPromptSubmit, hookCtx)
query = hookCtx.Query
```

## 9. Todo Write：让 Agent 显式维护长任务

### Prompt

长任务里模型容易忘记自己做到哪里，能不能增加 Todo？

### Solution

增加 todo_write Tool，并维护三种状态：

```text
pending
in_progress
completed
```

输出示例：

```text
## Current Tasks

[in_progress] 检查项目结构
[pending] 阅读 main.go
[pending] 总结结果
```

同时使用 RoundsSinceTodo 记录多久没有更新 Todo，必要时注入提醒：

```text
<reminder>Update your todos.</reminder>
```

### Result

Todo 成为复杂任务中的显式短期规划状态。

## 10. Subagent：使用独立 Context

### Prompt

task 工具启动的 Subagent 是否应该直接共用 Main Agent 的 messages？

### Problem

如果共享 messages[]：

- 子任务中间过程会污染主 Context
- Token 增长更快
- 主任务和子任务边界不清晰

### Solution

Subagent 使用独立：

```text
sub messages[]
```

形成自己的 Agent Loop：

```text
Main Agent
    ↓
 task
    ↓
Subagent
    ↓
Sub LLM
    ↓
Sub Tool
    ↓
Sub Final Result
    ↓
main tool_result
```

### Result

Main Agent 只接收 Subagent 的最终结果。

## 11. Skills：按需加载，而不是全部放进 Context

### Prompt

Skill 很多时，是不是应该把所有 SKILL.md 都直接放到 System Prompt？

### Problem

全部加载会占用大量 Context，而且很多内容与当前任务无关。

### Solution

先提供 Skill Catalog：

```text
name
description
```

模型需要某个 Skill 时，再加载对应：

```text
SKILL.md
```

### Result

Skills 变成按需加载的能力。

## 12. System Prompt：稳定信息和 messages 分开

### Prompt

Agent Identity、Workspace、Tools、Skills、Memory 这些稳定信息应该放在哪里？

### Solution

使用：

```text
prompt.GetSystemPrompt(...)
```

动态组装 System Prompt。

模型请求可以概括为：

```text
System Prompt
+
messages[]
+
tools
```

### Result

Runtime 配置与真实会话历史分离。

## 13. Tool Error：为什么不能 return err

### Prompt

read_file 或 bash 执行失败时，AgentLoop 应该直接退出吗？

### Problem

如果 Handler 一报错就：

```go
return err
```

一个普通工具失败就会结束整个任务。

### Solution

转换为 Error ToolResult：

```go
toolResults = append(
    toolResults,
    anthropic.NewToolResultBlock(
        block.ID,
        err.Error(),
        true,
    ),
)
continue
```

### Result

工具失败成为模型可以继续处理的环境反馈。

## 14. Error Recovery：429 / 529、max_tokens、prompt_too_long

### Prompt

API 临时失败、输出被 max_tokens 截断、Context 太长时，Agent 应该如何恢复？

### Solution

使用 RecoveryState 和 WithRetry() 管理恢复逻辑。

#### 429 / 529

```text
429 / 529
    ↓
backoff
    ↓
retry
```

#### max_tokens

max_tokens 是 Stop Reason，不是普通 API Error：

```text
stop_reason = max_tokens
        ↓
increase maxTokens / continue
        ↓
call LLM again
```

#### prompt_too_long

```text
prompt_too_long
       ↓
ReactiveCompact
       ↓
retry
```

### Result

可恢复错误尽量不会直接终止整个 Agent Turn。

## 15. Context Compact：分层压缩

### Prompt

messages[] 越来越长，应该如何控制 Context？

### Solution

当前压缩管线：

```text
messages[]
    ↓
ToolResultBudget
    ↓
SnipCompact
    ↓
MicroCompact
    ↓
size > limit ?
    ↓
AutoCompact
    ↓
LLM
```

含义：

- ToolResultBudget：限制大型 Tool Result
- SnipCompact：裁剪中间历史
- MicroCompact：压缩较旧结果
- AutoCompact：超过阈值后摘要
- ReactiveCompact：API 实际返回 Prompt Too Long 后再次压缩

### Result

Context 管理由简单截断升级为分层处理。

## 16. Memory：为什么不能直接 append 到 Session.Messages

### Prompt

Memory 是不是每轮直接永久追加到 Session.Messages？

### Problem

如果长期 Memory 每轮都追加：

- 相同 Memory 会重复
- Context 越来越大
- 长期信息和真实 Session History 混在一起

### Solution

Memory 流程：

```text
.memory/
   ↓
LoadMemories
   ↓
select relevant
   ↓
InjectMemories
   ↓
requestMessages
   ↓
LLM
```

其中：

```text
Session.Messages
```

保存真实会话历史；

```text
requestMessages
```

只是当前请求给 LLM 的临时版本。

Agent Turn 结束后，再通过：

```text
memory.ExtractMemories(...)
```

把适合长期保存的信息写回 .memory/。

### Result

长期 Memory 与 Session History 解耦。

## 17. Memory 与 Context Compact 的顺序

### Problem

Compact 会改变 messages[] 的长度和索引，如果提前记录当前用户消息位置，压缩后可能失效。

### Solution

Compact 后重新查找当前 User Turn：

```go
memoryTurn := memory.FindUserTurn(messages, query)
```

再执行：

```go
requestMessages = memory.InjectMemories(
    messages,
    memoryTurn,
    memoriesContent,
)
```

同时在压缩前保存：

```go
preCompress, err := memory.CloneMessages(messages)
```

结束时基于 preCompress 做 Memory Extract。

### Result

既保证 Memory 注入位置正确，也避免 Compact 导致长期信息提取丢失。

## 18. Multi-Session：Session 不等于进程

### Prompt

要求 window1 / window2，是不是必须开两个终端或两个进程？

### Conclusion

不需要。一个 Go 进程可以维护多个逻辑 Session：

```go
type SessionManager struct {
    Sessions map[string]*Session
    Current  string
}
```

Session：

```go
type Session struct {
    ID              string
    Messages        []anthropic.MessageParam
    RoundsSinceTodo int
}
```

命令：

```text
/session new <name>
/session switch <name>
/session list
```

### Result

用户可以在同一个 Agent 进程中创建、切换和恢复多个独立 Session。

## 19. Session 必须传指针，round 必须是局部变量

### Problem 1：Session 传值

如果：

```go
func AgentLoop(..., currentSession session.Session)
```

进入 AgentLoop 后修改的状态可能无法正确写回原 Session。

### Solution

使用：

```go
func AgentLoop(..., currentSession *session.Session) error
```

并在结束时写回：

```go
defer func() {
    currentSession.Messages = messages
}()
```

### Problem 2：round 是 package-global

如果使用：

```go
var round = 0
```

多个请求和 Session 会共享同一计数。

### Solution

在每次 AgentLoop 内：

```go
round := 0

for {
    round++

    if round > config.MAX_AGENT_ROUNDS {
        return fmt.Errorf(
            "maximum agent rounds exceeded: %d",
            config.MAX_AGENT_ROUNDS,
        )
    }
}
```

### Result

Session 状态可以正确保存，同时每个 Agent Turn 独立计算最大轮数。

## 20. Calculator、Web Search 与 Tool 类型

### Prompt

Calculator 应该怎么实现？Web Search 是不是本地 Handler？远程 Tool 是不是 MCP？

### Calculator

Calculator 只接收数学表达式：

```json
{
  "expression": "(123+456)*2"
}
```

内部使用受限表达式解析，而不是任意代码执行。

### Tool 类型

```text
Client Tool
→ 本地 Go Runtime 执行
→ bash / read_file / calculator

Server Tool
→ API Provider 执行
→ web_search

MCP Tool
→ 通过 MCP 协议连接独立 Tool Server
```

当前 web_search 属于 Server Tool，因此不需要：

```go
ToolHandlers["web_search"]
```

### Result

Runtime 可以同时管理本地工具和 Provider 提供的 Server Tool。

## 21. Provider / Credential 问题与 Runtime 问题要分开

### Problem 1：UnsupportedModel 404

曾遇到类似：

```text
404 Not Found
UnsupportedModel
The requested model does not support the agent plan feature.
```

### Analysis

这类问题通常来自：

- API Endpoint
- Base URL
- Model
- Provider 对接口能力的支持

不一定是 Agent Loop 本身的问题。

### Problem 2：Anthropic Credential Error

曾输入：

```text
/ session switch default
```

因为 / 后多了空格，没有被 Session Command Handler 匹配，于是被当成普通 Query 送入 AgentLoop，随后 SDK 才报：

```text
no Anthropic credentials found
```

正确命令：

```text
/session switch default
```

### Result

调试时需要区分：

```text
CLI Command Error
Provider Configuration Error
Runtime Logic Error
```

## 22. Per-Session Log

### Prompt

Tool Call Trace 是写一个全局日志，还是每个 Session 独立记录？

### Solution

每个 Session 使用独立 Logger：

```text
logs/
├── default.log
├── weather.log
└── weekly.log
```

记录：

```text
[tool_call]
[tool_result]
[tool_error]
```

### Result

不同 Session 的执行过程不会混在一起，便于调试和追踪。

## Final Architecture

最终 Runtime 可以概括为：

```text
                    SessionManager
                          │
                          ▼
                    Current Session
                          │
                          ▼
                    User Prompt
                          │
                   UserPromptSubmit
                          │
                          ▼
                      messages[]
                          │
            ┌─────────────┴─────────────┐
            │                           │
            ▼                           ▼
     Context Compact               Load Memory
            │                           │
            └─────────────┬─────────────┘
                          ▼
                    Inject Memory
                          │
                          ▼
                    System Prompt
                          │
                          ▼
                         LLM
                          │
                    tool_use ?
                   ┌──────┴──────┐
                   │             │
                  Yes            No
                   │             │
                   ▼             ▼
              PreToolUse      Final Answer
                   │             │
                   ▼             ▼
              Execute Tool   Extract Memory
                   │
                   ▼
              PostToolUse
                   │
                   ▼
              tool_result
                   │
                   └──────────────► messages[]
```

外围能力包括：

```text
Error Recovery
├── 429 / 529 Retry
├── max_tokens Recovery
└── prompt_too_long → ReactiveCompact

Planning
├── todo_write
└── Subagent

Knowledge
└── Skills

Safety
└── Permission

Observability
└── Per-Session Log
```

## Summary

本项目包含：

1. Agent Loop
2. Tool Registry
3. Tool Error Handling
4. Permission
5. Hooks
6. Todo
7. Subagent
8. Skills
9. System Prompt
10. Error Recovery
11. Context Compact
12. Memory
13. Multi-Session
14. MAX_AGENT_ROUNDS
15. Client Tool / Server Tool
16. Per-Session Log
