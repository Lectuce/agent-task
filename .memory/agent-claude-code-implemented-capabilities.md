---
name: agent-claude-code-implemented-capabilities
description: List of complete agent capabilities implemented in the agent-claude-code project
type: project
---

The project implements a complete set of AI agent capabilities including:
- Core LLM → Tool → Result agent loop with a unified tool registry
- Permission gate for dangerous operations + hook-based behavior extension
- Explicit todo planning for long-running tasks, with isolated subtask handling via independent subagents
- On-demand skill loading to reduce irrelevant context bloat
- Dynamic system prompt construction + error recovery for common LLM API errors
- Multi-layer context compression for long conversation histories
- Cross-session long-term memory management, with multi-session isolation and switching
- Built using Anthropic's official `anthropic-sdk-go` for LLM API calls.
