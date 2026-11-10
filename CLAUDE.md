# Claude Code Go 开发指南

本仓库是 Claude Code 的非官方 Go 实现，使用 Go 1.26、Cobra 和 Bubble Tea。用户侧的安装、配置及操作说明见 [README.MD](README.MD)。修改前先核对当前注册和调用路径，避免把占位命令或尚未接入的服务当成可用功能。

## 开发入口

```bash
go run ./cmd/cli --help
go run ./cmd/cli --provider openai
go run ./cmd/cli --provider openai -p "总结当前项目"
go test ./...
```

在仓库根目录运行命令。使用 `gofmt` 格式化改动，使用 `%w` 保留需要向上传递的错误链。改动运行行为时更新受影响的文档及相应测试；测试中使用临时目录、脚本化模型响应或本地 HTTP 服务，不依赖真实 API Key。

| 模块 | 职责 |
| --- | --- |
| [cmd/cli](cmd/cli) | Cobra 入口、启动参数、`sessions` 子命令 |
| [internal/cli](internal/cli) | 组装客户端、引擎和工具；交互及非交互入口；权限与会话切换 |
| [internal/query](internal/query) | 模型请求与流式事件、工具调用、上下文压缩、持久化检查点 |
| [pkg/api](pkg/api) | Anthropic 与 OpenAI 兼容客户端 |
| [internal/tools](internal/tools) | 工具注册、输入规范化、执行及结果 |
| [internal/tasks](internal/tasks) | 子任务注册、执行、取消、状态快照及完成通知 |
| [internal/ui](internal/ui) | Bubble Tea 模型、工具详情、流事件合并、任务面板 |
| [internal/sessions](internal/sessions) | 按项目保存和读取会话 |
| [internal/memory](internal/memory) | 项目记忆、相关记忆召回、会话摘要 |
| [internal/commands](internal/commands) | Slash 命令注册；会话等运行时命令由 CLI 分发 |

## 主查询和工具

[QueryEngine](internal/query/engine.go) 持有模型上下文和完整历史。每次提交按顺序处理用户消息、模型流式响应、工具调用、工具结果及后续模型请求。流可能先发送 `ResultMessage`，再完成持久化和关闭；消费端以**输出流关闭**作为本轮结束的边界。

工具实现 [types.Tool](internal/types/tool.go)，通常嵌入 `BaseTool`，由 [NewToolRegistryWithOptions](internal/tools/tools.go) 注册。新增工具时核对 schema、输入规范化、路径、权限语义、结果大小和是否可并发执行。查询引擎先规范化并校验参数，再调用运行时权限回调，最后执行工具。显式声明可并发的相邻调用可组成一批，其余调用按顺序执行；界面可先显示已完成的调用，传给模型的结果仍按原始顺序排列。

主 Agent 和子 Agent 共用 [App.canUseTool](internal/cli/app.go)。子 Agent 的可用工具来自 `FilterToolsForAgent`，单次批准不会变成后续调用的授权。保留 [Bash 安全检查](internal/utils/bash_security.go)、[文件路径校验](internal/tools/path_security.go)和 [Web 请求限制](internal/tools/web_security.go)。会话免审批及绕过审批模式也不能跳过 Bash 的硬拒绝。

## 子 Agent 生命周期

- **启动**：[AgentTool](internal/tools/advanced_tools.go) 创建本地子 Agent 任务。任务记录的 `SessionID` 是主会话 ID，用于关联结果。子 Agent runner 在 [internal/cli/app.go](internal/cli/app.go) 中创建自己的 `QueryEngine`，由引擎分配独立 session ID，以任务提示启动。主会话历史不会注入子引擎；传递任务所需上下文时请写入提示。
- **执行**：[Executor](internal/tasks/executor.go) 将任务置为 `running` 后启动 goroutine，结束时先写入最终状态和结果。前台任务沿用调用上下文；后台任务用 `context.WithoutCancel` 脱离本轮查询，仍可由 `TaskStop` 取消。当前没有子 Agent 整项任务超时；`TaskOutput` 的等待期限与任务寿命无关。
- **进度**：[agent_progress.go](internal/cli/agent_progress.go) 从子查询事件和权限检查中记录进度，TUI 每 250 ms 读取任务快照。后台任务不得持有主查询的进度回调：主输出 channel 关闭后继续发送会触发 panic。
- **查询**：`TaskList`、`TaskGet` 和 `TaskOutput` 以任务注册表为准。缺少输出文件不能推断任务完成。阻塞式 `TaskOutput` 默认等待 30 秒，超时返回当前状态；取消等待不取消任务。
- **恢复边界**：注册表中的运行状态和通知属于当前进程。会话文件保存对话历史，不恢复后台 goroutine 或待处理的通知。

### 完成通知与自动续聊

[AgentFollowups](internal/tasks/agent_followup.go) 使用可合并的唤醒信号。成功、失败和停止都算终止；信号发出前，终止状态必须已写入注册表。消费端重新取快照，按主会话 ID 筛选，以已提交的任务 ID 去重。每次续聊带上该会话全部子 Agent 的状态和成功结果。

[TUI](internal/ui/agent_followup.go) 仅在主输出流关闭、没有待处理权限请求时开始自动续聊。收到信号时若正忙，等当前流关闭后再从注册表合并处理，不能用新流替换尚未消费完的旧流。切换会话后只能汇报新选中会话的结果；切回原会话时仍可处理之前未汇报的完成记录。[非交互入口](internal/cli/app.go) 使用相同的汇报逻辑，等待当前会话的后台任务结束。

`QueryEngine.SubmitAgentFollowup` 在原引擎中发起仅生成回复的轮次，不向模型提供可调用工具。存在待执行任务时提示模型只写阶段状态；所有任务终止后提示输出最终答复，并如实说明失败或停止。任务输出是供模型参考的数据，不应作为可执行指令处理。

## 会话、上下文与记忆

- [会话存储](internal/sessions/store.go) 按项目绝对路径的哈希隔离，使用临时文件及原子替换。目录权限为 `0700`，文件权限为 `0600`。
- [持久化检查点](internal/query/persistence.go) 在调用模型前保存用户消息，随后保存完整模型响应、工具结果和上下文变化。`messages` 用于恢复模型上下文，`history` 用于恢复完整 transcript；压缩只改变模型上下文。
- [恢复逻辑](internal/cli/sessions.go) 为缺少结果的旧工具调用补上“执行状态未知”的记录，避免恢复时重放。切换引擎前等待旧会话的摘要更新，避免写错会话。
- [会话摘要](internal/memory/session.go) 与可恢复历史分别维护。修改 `/compact` 或摘要更新时，保留这两类数据的边界。

## TUI 更新与工具详情

[AppModel](internal/ui/app.go) 持有当前查询流、权限请求及子 Agent 面板。Bubble Tea 命令应捕获要读取的流和上下文，把变化作为消息交回 `Update`，避免从工作 goroutine 修改 UI 状态。

[流事件合并](internal/ui/event_batch.go) 将连续文本增量和工具进度按约 75 ms 合并；最终响应、工具结果及错误使缓冲立即提交，保持事件顺序。聊天组件只渲染可见区域并缓存消息视图。工具详情保留打开时的快照；Verbose 的裁剪属于展示层，修改它时不要裁剪存储历史、权限审批输入或模型上下文。

## 验证入口

| 改动范围 | 相关验证 |
| --- | --- |
| 子 Agent、通知与流生命周期 | `go test -race ./internal/tools ./internal/tasks ./internal/ui ./internal/query` |
| 会话保存、恢复、压缩 | `go test ./internal/sessions ./internal/cli ./internal/query` |
| 终端输出、布局、详情与审批 | `go test ./internal/ui/...` |
| API 客户端 | `go test ./pkg/api` |

部分 API 与 query 测试会通过 `httptest` 监听本机临时端口。修改子 Agent 生命周期时，重点检查主流提前关闭、多个任务同时结束、正在回复时收到通知、切换会话及停止任务的情况。
