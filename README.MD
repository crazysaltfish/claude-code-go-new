# claude-code-go

> 基于 [Claude Code](https://www.npmjs.com/package/@anthropic-ai/claude-code) v2.1.88 Source Map 还原并翻译为 Go 语言的非官方实现

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-Research%20Use%20Only-yellow.svg)](LICENSE)

> ⚠️ **警告**：本仓库为非官方整理版，基于公开 npm 发布包与 Source Map 分析还原，仅供研究使用。不代表官方原始内部开发仓库结构。

---

## 📖 项目说明

### 什么是 Claude Code？

Claude Code 是 Anthropic 官方推出的 AI 编程助手命令行工具，可以帮助开发者：
- 理解和导航代码库
- 编写和编辑代码
- 执行终端命令
- 调试和修复问题
- 与 MCP (Model Context Protocol) 服务器集成

### 本项目目的

本项目通过 `@anthropic-ai/claude-code` npm 发布包内附带的 Source Map（`cli.js.map`）还原了 TypeScript 源码（版本 2.1.88），并将其翻译为 Go 语言实现。

**还原规模：**
- 还原文件数：**4756** 个（含 1884 个 `.ts` / `.tsx` 源文件）
- 还原方式：提取 `cli.js.map` 中的 `sourcesContent` 字段

### 为什么选择 Go 版本？

- **单二进制分发**：无需 Node.js 运行时，编译后单个可执行文件即可运行
- **跨平台支持**：支持 Linux、macOS、Windows 等多平台
- **性能优势**：Go 语言的并发模型更适合处理 I/O 密集型任务
- **易于部署**：静态编译，无外部依赖

项目学习文档：https://nangongwentian-fe.github.io/learn-claude-source/

---

## 🔄 翻译进度

### 已完成模块

| 模块 | TypeScript 文件数 | Go 文件数 | 状态 |
|------|------------------|----------|------|
| **认证系统 (OAuth)** | ~15 | 3 | ✅ 完成 |
| **API 客户端** | ~20 | 13 | ✅ 完成 |
| **重试机制** | ~5 | 1 | ✅ 完成 |
| **MCP 协议** | ~10 | 7 | ✅ 完成 |
| **Bash 解析器** | ~16 | 6 | ✅ 完成 |
| **命令前缀提取** | ~5 | 1 | ✅ 完成 |
| **Shell 安全检查** | ~10 | 3 | ✅ 完成 |
| **类型定义** | ~8 | 10 | ✅ 完成 |
| **常量定义** | ~21 | 22 | ✅ 完成 |
| **CLI 框架** | ~10 | 2 | ✅ 完成 |
| **任务系统 (Tasks)** | ~9 | 4 | ✅ 完成 |
| **代理系统 (Agent)** | ~16 | 2 | ✅ 完成 |
| **权限系统** | ~24 | 1 | ✅ 完成 |
| **Hooks 系统** | ~85 | 1 | ✅ 完成 |
| **Vim 模式** | ~5 | 2 | ✅ 完成 |
| **语音交互** | ~1 | 1 | ✅ 完成 |
| **LSP 服务** | ~20 | 4 | ✅ 完成 |
| **对话压缩 (Compact)** | ~15 | 2 | ✅ 完成 |
| **工具执行服务** | ~30 | 2 | ✅ 完成 |
| **Token 估算服务** | ~5 | 1 | ✅ 完成 |
| **插件服务** | ~10 | 2 | ✅ 完成 |
| **通知服务** | ~5 | 1 | ✅ 完成 |
| **分析服务** | ~5 | 1 | ✅ 完成 |
| **配置服务** | ~5 | 1 | ✅ 完成 |
| **速率限制消息** | ~3 | 1 | ✅ 完成 |
| **工具系统 (Tools)** | ~140 | 6 | ✅ 完成 |
| **命令系统 (Commands)** | ~101 | 2 | ✅ 完成 |
| **UI 组件库** | ~144 | 9 | ✅ 完成 |

### 进行中模块

无

### 待优化模块

| 模块 | 当前状态 | 优化方向 |
|------|---------|---------|
| **UI 交互优化** | 基础功能完成 | 增强 Bubble Tea 组件交互体验 |
| **LSP 高级功能** | 基础框架完成 | 调用层次、类型层次等高级特性 |

---

## ✨ Go 版本实现功能

### 🔐 认证系统

#### OAuth 认证
- **Token 管理**：支持 Access Token、Refresh Token 的存储与自动刷新
- **并发安全**：使用 mutex 和 channel 实现线程安全的 token 操作
- **跨进程协调**：文件锁确保多进程环境下 token 刷新的正确性
- **自动刷新**：检测 token 过期或 401 错误时自动刷新
- **环境变量支持**：`CLAUDE_CODE_OAUTH_TOKEN` 环境变量覆盖

#### 多云供应商认证
- **AWS Bedrock**：AWS SigV4 签名、STS 临时凭证、SSO 登录
- **Google Vertex AI**：ADC (Application Default Credentials) 认证
- **Azure Foundry**：API Key 和 Azure AD 认证

### 🔄 重试机制

- **指数退避**：基础延迟 500ms，最大延迟 32 秒
- **抖动**：随机化退避时间避免惊群效应
- **持久重试**：支持长时间运行的会话（最大 6 小时）
- **错误分类处理**：
  - 429 速率限制：尊重 `Retry-After` 和 `Anthropic-Ratelimit-Unified-Reset` 头
  - 529 过载错误：区分前台/后台任务重试策略
  - 401 认证错误：自动刷新 OAuth token
  - 5xx 服务器错误：指数退避重试
- **心跳机制**：长时间等待时定期输出心跳日志

### 🔌 MCP (Model Context Protocol)

- **传输层**：支持 stdio 和 SSE 传输
- **工具过滤**：基于 URL 模式的工具名称过滤
- **配置管理**：支持 `${VAR:-default}` 环境变量扩展语法
- **权限检查**：细粒度的工具权限验证

### 🔧 Bash 安全解析

- **命令解析**：完整的 Bash 命令词法分析
- **安全检查**：检测命令注入、进程替换、危险内置命令
- **前缀提取**：智能提取命令前缀用于权限控制
- **只读模式验证**：检测破坏性命令(rm、chmod 等)

### 📊 速率限制管理

- **智能消息生成**：根据限制状态生成用户友好的提示消息
- **多种限制类型**：支持周限制、会话限制、Opus限制、Sonnet限制等
- **超量使用跟踪**：跟踪额外使用量的消耗和重置时间
- **提前警告**：在接近限制时提前发出警告

### 🔔 通知系统

- **多通道支持**：支持 iTerm2、Kitty、Ghostty 等终端通知
- **自动检测**：根据终端类型自动选择最佳通知方式
- **可配置通道**：用户可配置首选通知通道
- **Apple Terminal 特殊处理**：检测并尊重终端铃声设置

### 🔧 LSP (Language Server Protocol)

- **语言服务器管理**：支持多种语言的 LSP 服务器启动和管理
- **诊断功能**：实时获取代码诊断信息（错误、警告）
- **代码补全**：智能代码补全建议
- **转到定义**：跳转到符号定义位置
- **查找引用**：查找符号的所有引用
- **文档悬停**：显示符号的文档信息
- **多工作区支持**：支持多工作区目录配置

### 🗜️ 对话压缩 (Compact)

- **智能压缩**：自动压缩长对话历史以节省 token
- **保留关键信息**：保留重要的上下文和决策信息
- **分段策略**：按段落智能压缩，保持语义完整性
- **Token 估算**：精确估算压缩后的 token 使用量

### ⚙️ 工具执行服务

- **工具注册**：支持动态注册和管理工具
- **工具钩子**：工具执行前后的事件钩子
- **权限检查**：工具调用前的权限验证
- **错误处理**：统一的工具错误处理和恢复机制
- **超时控制**：工具执行的超时管理

### 🔌 插件系统

- **动态加载**：支持动态加载插件
- **插件隔离**：插件运行在独立环境中
- **API 扩展**：提供插件 API 扩展点
- **生命周期管理**：插件的初始化、启动、停止管理

---

## 🚀 安装部署

### 前置要求

- **Go 版本**：Go 1.26 或更高版本
- **操作系统**：Linux、macOS、Windows
- **API 访问**：Anthropic API 密钥或 OAuth 认证

### 方式一：从源码构建

```bash
# 1. 克隆仓库
git clone <repository-url>
cd claude-code-go

# 2. 下载依赖
go mod download

# 3. 构建
go build -o claude-code ./cmd/cli

# 4. (可选) 安装到系统路径
sudo mv claude-code /usr/local/bin/
```

### 方式二：直接运行

```bash
# 克隆仓库
git clone <repository-url>
cd claude-code-go

# 直接运行
go run ./cmd/cli [命令] [参数]
```

### 方式三：交叉编译

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o claude-code-linux-amd64 ./cmd/cli

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o claude-code-darwin-arm64 ./cmd/cli

# macOS AMD64 (Intel)
GOOS=darwin GOARCH=amd64 go build -o claude-code-darwin-amd64 ./cmd/cli

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -o claude-code-windows-amd64.exe ./cmd/cli
```

### 验证安装

```bash
# 检查版本
claude-code version

# 运行诊断
claude-code doctor
```

---

## 📚 如何使用

### 基本用法

```bash
# 交互式对话
claude-code

# 带初始提示启动
claude-code "帮我分析这个项目的结构"

# 指定模型
claude-code --model claude-sonnet-4-20250514 "优化这段代码"

# 调试模式
claude-code --debug "为什么这个测试失败了？"
```

### 命令行参数

| 参数 | 简写 | 说明 |
|------|------|------|
| `--debug` | `-d` | 启用调试模式，输出详细日志 |
| `--verbose` | `-v` | 启用详细输出 |
| `--print` | `-p` | 打印模式（非交互式） |
| `--model` | `-m` | 指定使用的模型（如 claude-sonnet-4-20250514） |
| `--permission-mode` | | 权限模式：default, acceptEdits, bypassPermissions |
| `--max-turns` | | 最大对话轮数（默认 100） |
| `--max-tokens` | | 单次 API 响应的最大输出 token 数（默认 4096） |

### 子命令

```bash
# 显示版本信息
claude-code version

# 配置管理
claude-code config

# MCP 服务器管理
claude-code mcp

# 运行系统诊断
claude-code doctor
```

### 认证配置

#### 使用 API Key

```bash
# 设置环境变量
export ANTHROPIC_API_KEY="your-api-key"

# 或者使用 OAuth token
export ANTHROPIC_AUTH_TOKEN="your-oauth-token"
```

#### 使用 AWS Bedrock

```bash
export CLAUDE_CODE_USE_BEDROCK=true
export AWS_REGION="us-east-1"
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
```

#### 使用 Google Vertex AI

```bash
export CLAUDE_CODE_USE_VERTEX=true
# 使用 ADC 认证，需先运行：gcloud auth application-default login
```

#### 使用 Azure Foundry

```bash
export CLAUDE_CODE_USE_FOUNDRY=true
export ANTHROPIC_API_KEY="your-azure-api-key"
```

---

## 🎯 场景用例

### 1. 代码审查

```bash
# 审查最近的代码变更
claude-code "审查 git diff 的变更，指出潜在问题"

# 审查特定文件
claude-code "审查 src/api/client.go，检查错误处理是否完善"
```

### 2. 代码重构

```bash
# 重构建议
claude-code "重构 internal/services/api/retry.go，使其更易测试"

# 代码优化
claude-code "优化这个函数的性能，减少内存分配"
```

### 3. 调试问题

```bash
# 启用调试模式分析问题
claude-code --debug "为什么我的程序在处理并发请求时出现死锁？"

# 分析错误日志
claude-code "分析这个错误日志，找出根本原因：$(cat error.log)"
```

### 4. 文档生成

```bash
# 生成函数文档
claude-code "为 internal/utils/auth.go 中的所有公开函数生成 Go 文档注释"

# 生成 README
claude-code "为我的新项目生成 README.md，包括安装和使用说明"
```

### 5. 测试辅助

```bash
# 生成测试用例
claude-code "为 internal/services/api/client.go 生成单元测试"

# 分析测试覆盖率
claude-code "分析测试覆盖率报告，找出未覆盖的关键路径"
```

### 6. 架构理解

```bash
# 理解项目结构
claude-code "解释这个项目的架构设计，包括各模块的职责"

# 代码导航
claude-code "找到所有实现了 io.Reader 接口的类型"
```

### 7. 安全审计

```bash
# 安全检查
claude-code "检查代码中是否存在 SQL 注入、XSS 等安全漏洞"

# 敏感信息检查
claude-code "检查代码中是否意外暴露了敏感信息（API key、密码等）"
```

### 8. 依赖管理

```bash
# 分析依赖
claude-code "分析 go.mod 中的依赖，找出可升级的版本"

# 检查漏洞
claude-code "检查项目依赖是否存在已知安全漏洞"
```

---

## 🔧 核心功能模块

### 任务系统 (Tasks)

任务系统管理各种异步和后台任务的执行：

```go
// 创建任务注册表
registry := tasks.NewRegistry()

// 创建本地代理任务
task := tasks.CreateLocalAgentTask(
    "task-123",
    "分析代码库结构",
    "assistant",
    "代码分析任务",
)

// 注册任务
registry.Register(task)

// 启动任务
tasks.StartTask(task)

// 完成任务
tasks.CompleteTask(task)
```

**支持的类型**：
- `LocalAgentTask`: 本地代理执行
- `LocalShellTask`: Shell 命令执行
- `RemoteAgentTask`: 远程代理任务

### 权限系统 (Permissions)

权限系统提供细粒度的操作权限控制：

```go
// 创建权限管理器
mgr := permissions.NewManager()

// 添加权限规则
mgr.AddRule(permissions.SourceUserSettings, permissions.PermissionRule{
    Source:       permissions.SourceUserSettings,
    RuleBehavior: permissions.PermissionBehaviorAllow,
    RuleValue: permissions.PermissionRuleValue{
        ToolName:    "Bash",
        RuleContent: "git:*",
    },
})

// 检查权限
result := mgr.CheckPermission(ctx, "Bash", "git status")
if result.IsAllowed() {
    // 执行操作
}
```

**支持的权限模式**：
- `default`: 默认模式，需要确认
- `accept`: 自动接受所有操作
- `plan`: 规划模式，只读操作

### Hooks 系统

Hooks 系统提供事件钩子机制：

```go
// 创建钩子注册表
registry := hooks.NewRegistry()

// 注册日志钩子
registry.Register(hooks.HookEventSessionStart, hooks.LoggingHook)

// 注册权限检查钩子
registry.Register(hooks.HookEventPreToolUse, hooks.PermissionCheckHook)

// 执行钩子
outputs, err := registry.Execute(ctx, hooks.HookInput{
    EventName: hooks.HookEventPreToolUse,
    ToolName:  "Bash",
    ToolInput: map[string]interface{}{"command": "rm -rf /"},
})
```

**支持的事件**：
- `SessionStart`/`SessionEnd`: 会话生命周期
- `PreToolUse`/`PostToolUse`: 工具执行前后
- `PermissionRequest`: 权限请求
- `UserPromptSubmit`: 用户提交提示

### Vim 模式

Vim 模式提供完整的 Vim 键绑定支持：

```go
// 创建 Vim 状态
vimState := vim.NewVimState()
persistent := vim.NewPersistentState()

// 创建状态转换器
transition := vim.NewTransition(vimState, persistent)

// 处理按键
action, err := transition.HandleKey("d")
action, err = transition.HandleKey("d")
// 返回 "delete:line"
```

**支持的操作**：
- 基本移动: `h`, `j`, `k`, `l`, `w`, `b`, `e`
- 操作符: `d` (删除), `c` (修改), `y` (复制)
- 文本对象: `iw`, `aw`, `i"`, `a"`
- 查找: `f`, `F`, `t`, `T`
- 其他: `u` (撤销), `.` (重复), `p` (粘贴)

### 语音交互

语音交互支持语音输入和命令解析：

```go
// 创建语音服务
config := voice.DefaultRecordingConfig()
service := voice.NewService(config, &voice.MockTranscriber{
    Response: "go to main function",
})

// 启用语音输入
service.Enable()

// 开始录音
err := service.StartRecording(ctx)

// 停止录音并获取转录
text, err := service.StopRecording(ctx)

// 解析语音命令
cmd := voice.ParseCommand(text)
// cmd.Type = "navigation"
// cmd.Action = "goto"
```

**支持的命令类型**：
- `navigation`: 导航命令
- `edit`: 编辑命令
- `search`: 搜索命令
- `input`: 文本输入

### 9. CI/CD 集成

```bash
# 在 CI 中非交互式运行
claude-code --print --permission-mode bypassPermissions "检查代码是否符合项目规范"

# 自动化代码审查
claude-code --max-turns 1 "审查当前 PR 的变更并给出建议"
```

### 10. 多云部署场景

```bash
# 使用 AWS Bedrock
export CLAUDE_CODE_USE_BEDROCK=true
claude-code "生成部署到 AWS Lambda 的配置文件"

# 使用 Google Vertex AI
export CLAUDE_CODE_USE_VERTEX=true
claude-code "生成 Cloud Run 的部署配置"
```

---

## 📂 目录结构

### TypeScript 原始代码

```
restored-src/src/
├── main.tsx              # CLI 入口
├── tools/                # 工具实现（Bash、FileEdit、Grep、MCP 等 170+ 个文件）
│   ├── BashTool/         # Bash 命令执行工具
│   ├── FileReadTool/     # 文件读取工具
│   ├── FileEditTool/     # 文件编辑工具
│   ├── GrepTool/         # 文件内容搜索工具
│   ├── GlobTool/         # 文件名搜索工具
│   ├── MCPTool/          # MCP 协议工具
│   ├── AgentTool/        # 多代理系统
│   └── ...               # 其他 30+ 个工具
├── commands/             # 命令实现（commit、review、config 等 100+ 个文件）
├── services/             # API、MCP、分析等服务（36 个文件）
├── utils/                # 工具函数（git、model、auth、env 等 430+ 个文件）
│   ├── bash/             # Bash 解析器（16 个文件）
│   ├── model/            # 模型管理（17 个文件）
│   ├── permissions/      # 权限系统（24 个文件）
│   └── shell/            # Shell 提供者（10 个文件）
├── components/           # React UI 组件（144 个文件）
├── context/              # React Context
├── coordinator/          # 多 Agent 协调模式
├── assistant/            # 助手模式（KAIROS）
├── buddy/                # AI 伴侣 UI
├── remote/               # 远程会话
├── plugins/              # 插件系统
├── skills/               # 技能系统
├── voice/                # 语音交互
└── vim/                  # Vim 模式
```

### Go 实现代码

```
internal/
├── constants/            # 常量定义
│   ├── oauth.go          # OAuth 配置（scopes、endpoints）
│   ├── betas.go          # API beta headers
│   ├── tools.go          # 工具名称常量和权限配置
│   ├── common.go         # 通用常量和日期函数
│   ├── api_limits.go     # API 限制常量
│   ├── keys.go           # GrowthBook 密钥配置
│   ├── files.go          # 文件类型检测
│   ├── prompts.go        # 系统提示构建
│   ├── messages.go       # 消息常量
│   ├── error_ids.go      # 错误ID常量
│   ├── github_app.go     # GitHub App 配置
│   ├── product.go        # 产品常量
│   ├── turn_completion_verbs.go # 轮次完成动词
│   ├── cyber_risk.go     # 安全风险指令
│   ├── tool_limits.go    # 工具限制
│   ├── spinner_verbs.go  # 加载动画动词
│   ├── figures.go        # Unicode 图形字符
│   ├── xml.go            # XML 常量
│   ├── output_styles.go  # 输出样式配置
│   ├── system.go         # 系统提示前缀和归属头
│   └── system_prompt_sections.go # 系统提示段落管理
├── services/
│   ├── api/              # API 客户端
│   │   ├── client.go     # Anthropic API 客户端
│   │   ├── retry.go      # 重试逻辑
│   │   ├── errors.go     # 错误类型定义
│   │   ├── auth_aws.go   # AWS Bedrock 认证
│   │   ├── auth_vertex.go # Google Vertex 认证
│   │   ├── auth_azure.go # Azure Foundry 认证
│   │   ├── signer_aws.go # AWS SigV4 签名
│   │   ├── empty_usage.go # 空使用量定义
│   │   ├── error_utils.go # 错误工具函数
│   │   ├── usage.go      # API使用量查询
│   │   ├── bootstrap.go  # 引导数据获取
│   │   └── logging.go    # API日志记录
│   ├── oauth/            # OAuth 认证
│   │   └── client.go     # OAuth token 管理
│   ├── mcp/              # MCP 协议实现
│   │   ├── client.go     # MCP 客户端
│   │   ├── config.go     # 配置管理
│   │   ├── manager.go    # MCP 服务器管理
│   │   ├── types.go      # 类型定义
│   │   ├── mcpUtils.go   # MCP 工具函数
│   │   ├── normalization.go # 规范化工具
│   │   └── mcpStringUtils.go # 字符串工具
│   ├── config.go         # 配置服务
│   ├── compact.go        # 对话压缩服务
│   ├── compact_prompt.go # 压缩提示词生成
│   ├── plugins.go        # 插件服务
│   ├── plugin_loader.go  # 插件加载器
│   ├── analytics.go      # 分析服务
│   ├── lsp.go            # LSP 服务入口
│   ├── lsp_client.go     # LSP 客户端
│   ├── lsp_server_manager.go # LSP 服务器管理
│   ├── rate_limit_messages.go # 速率限制消息
│   ├── notifier.go       # 通知服务
│   ├── tool_execution.go # 工具执行服务
│   ├── tool_hooks.go     # 工具钩子
│   └── token_estimation.go # Token 估算服务
├── utils/                # 工具函数
│   ├── auth.go           # 认证管理器
│   ├── providers.go      # API 提供商检测
│   ├── uuid.go           # UUID 生成
│   ├── file.go           # 文件操作
│   ├── http.go           # HTTP 工具
│   ├── git_utils.go      # Git 操作
│   ├── lru_cache.go      # LRU 缓存
│   ├── bash_parser.go    # Bash 命令解析器
│   ├── bash_security.go  # Bash 安全检查
│   ├── shell_ast.go      # Shell AST 解析
│   ├── shell_quote.go    # Shell 引用处理
│   ├── command_spec.go   # 命令规范
│   ├── command_prefix.go # 命令前缀提取
│   └── ...               # 其他工具函数
├── types/                # 类型定义
│   ├── ids.go            # ID 类型
│   ├── logs.go           # 日志类型
│   ├── task.go           # 任务类型
│   ├── tool.go           # 工具类型
│   ├── hooks.go          # 钩子类型
│   ├── plugin.go         # 插件类型
│   ├── command.go        # 命令类型
│   ├── appstate.go       # 应用状态
│   ├── permissions.go    # 权限类型
│   └── tokens.go         # Token计算工具
├── tools/                # 工具实现
│   ├── tools.go          # 工具注册
│   ├── advanced_tools.go # 高级工具
│   ├── additional_tools.go # 其他工具
│   ├── missing_tools.go  # 缺失工具补充
│   ├── bash_utils.go     # Bash 工具辅助
│   └── grep_utils.go     # Grep 工具辅助
├── commands/             # 命令实现
│   ├── commands.go       # 命令注册
│   └── prompt_commands.go # 提示命令
├── tasks/                # 任务系统
│   ├── types.go          # 任务类型定义
│   ├── framework.go      # 任务管理框架
│   ├── executor.go       # 任务执行器
│   └── ui_integration.go # UI 集成
├── permissions/          # 权限系统
│   └── manager.go        # 权限管理器
├── hooks/                # 钩子系统
│   └── hooks.go          # 钩子实现
├── vim/                  # Vim 模式
│   ├── types.go          # Vim 类型
│   └── transitions.go    # 状态转换
├── voice/                # 语音交互
│   └── voice.go          # 语音服务
├── cli/                  # CLI 应用
│   └── app.go            # 应用逻辑
├── query/                # 查询引擎
│   └── engine.go         # 查询引擎实现
├── state/                # 状态管理
└── ui/                   # UI 组件
    ├── ui.go             # UI 主入口
    ├── app.go            # 应用管理
    ├── hooks/            # UI Hooks
    └── components/       # UI 组件
        ├── chat.go       # 聊天组件
        ├── input.go      # 输入组件
        ├── dialog.go     # 对话框组件
        ├── status.go     # 状态栏组件
        ├── message.go    # 消息显示组件
        └── tool_result.go # 工具结果渲染

cmd/
└── cli/
    └── main.go           # 程序入口

pkg/
└── api/
    └── client.go         # 公开 API 客户端
```

---

## 🔗 来源信息

| 项目 | 说明 |
|------|------|
| npm 包 | `@anthropic-ai/claude-code` |
| 还原版本 | 2.1.88 |

---

## 🛠️ 环境变量

### OAuth 配置
| 变量 | 说明 |
|------|------|
| `CLAUDE_CODE_OAUTH_TOKEN` | 直接设置 OAuth access token |
| `CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR` | 从文件描述符读取 token |
| `CLAUDE_CODE_OAUTH_CLIENT_ID` | 覆盖 OAuth client ID |
| `CLAUDE_CODE_CUSTOM_OAUTH_URL` | 自定义 OAuth 端点（仅限 FedStart） |

### 多云认证
| 变量 | 说明 |
|------|------|
| `CLAUDE_CODE_USE_BEDROCK` | 使用 AWS Bedrock |
| `CLAUDE_CODE_USE_VERTEX` | 使用 Google Vertex AI |
| `CLAUDE_CODE_USE_FOUNDRY` | 使用 Azure Foundry |
| `ANTHROPIC_API_KEY` | API 密钥 |
| `ANTHROPIC_AUTH_TOKEN` | OAuth bearer token |

### 重试配置
| 变量 | 说明 |
|------|------|
| `API_TIMEOUT_MS` | API 请求超时（毫秒） |
| `CLAUDE_CODE_DISABLE_PERSISTENT_RETRY` | 禁用持久重试 |

### 工具曝光策略
| 变量 | 说明 |
|------|------|
| `CLAUDE_CODE_SIMPLE` | 设为 `1`/`true` 时仅向模型暴露 Bash、Read、Edit |
| `CLAUDE_CODE_DISABLE_TOOLS` | 按规范名称禁用工具，多个名称使用逗号分隔，如 `Bash,Grep` |

---

## 🔨 开发指南

### 构建

```bash
# 编译所有包
go build ./...

# 运行测试
go test ./...

# 静态分析
go vet ./...

# 构建二进制
go build -o claude-code ./cmd/cli
```

### 代码规范

- 遵循 [Go 代码规范](https://golang.org/doc/effective_go)
- 使用 `gofmt` 格式化代码
- 添加必要的注释和文档

---

## 🤝 贡献指南

欢迎贡献代码！请遵循以下步骤：

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add some amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

---

## 📋 开发路线

### 已完成 ✅
- [x] OAuth 认证流程实现
- [x] 多云供应商认证（AWS Bedrock、Google Vertex、Azure Foundry）
- [x] API 客户端与重试机制
- [x] MCP 协议完整支持（Stdio/SSE/WebSocket 传输、JSON-RPC 2.0）
- [x] LSP 集成基础框架（语言服务器管理、诊断、补全）
- [x] Bash 命令解析器
- [x] Shell 安全检查
- [x] 命令前缀提取
- [x] CLI 框架（基于 Cobra）
- [x] 完整的常量模块（22个文件，涵盖所有核心常量）
- [x] 系统提示构建系统
- [x] 工具权限配置系统
- [x] 工具系统完整实现
- [x] 命令系统完整实现
- [x] 任务系统（本地代理、Shell、远程代理）
- [x] 权限系统（规则管理、权限检查）
- [x] Hooks 系统（事件钩子和回调）
- [x] Vim 模式支持（状态机、键绑定）
- [x] 语音交互支持（录音、命令解析）
- [x] UI 组件框架（Bubble Tea：消息显示、输入框、对话框、状态栏、工具结果渲染）
- [x] LSP 服务完整实现（语言服务器管理、诊断、补全、定义跳转、引用查找）
- [x] 对话压缩服务（智能压缩、Token 估算）
- [x] 工具执行服务（工具注册、钩子、权限检查）
- [x] Token 估算服务
- [x] 插件系统基础框架

### 进行中 🔄
- [ ] 插件系统高级功能

### 计划中 📝
- [ ] LSP 高级功能（调用层次、类型层次）
- [ ] UI 完善与交互优化

---

## ❓ 常见问题

### Q: 如何更新到最新版本？

```bash
git pull origin main
go build -o claude-code ./cmd/cli
```

### Q: 遇到认证错误怎么办？

1. 检查 `ANTHROPIC_API_KEY` 或 `ANTHROPIC_AUTH_TOKEN` 是否正确设置
2. 运行 `claude-code doctor` 检查系统状态
3. 尝试使用 `--debug` 参数查看详细日志

### Q: 如何查看支持的模型列表？

```bash
claude-code --help
# 或查看 Anthropic 官方文档
```

### Q: 在企业环境中如何使用？

配置 `CLAUDE_CODE_CUSTOM_OAUTH_URL` 使用企业 OAuth 端点，或使用 AWS Bedrock / Azure Foundry 等云服务。

### Q: 编译时报错怎么办？

确保使用 Go 1.26 或更高版本：
```bash
go version
go build ./...
```

---

## ⚖️ 声明

- 源码版权归 **Anthropic** 所有
- 本仓库仅用于技术研究与学习，请勿用于商业用途
- 如有侵权，请联系删除

---

## 📄 License

本项目仅供学习和研究使用。原始 TypeScript 源码版权归 Anthropic 所有。
