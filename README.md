# Homework Canvas

Homework Canvas 是课程作业管理工作台：把老师发布的文字、PDF、PPT/PPTX 整理为待审核候选，再由课程维护者确认并发布为全班共享作业。学生可以维护个人计划截止时间与完成状态，但不会改动官方截止时间。

> 状态：已完成阶段一至阶段三的工程实现。当前使用本地身份适配器，尚未接入真实 UniSSO；未配置模型时使用本地规则抽取器。

## 设计边界

- 共享作业属于课程实例；个人 DDL 和完成状态独立保存。
- 模型只能生成候选，不能自动发布作业。
- 原始文件、逐页文本和预览只在处理期间存在，任务终态即清理。
- API 与 Worker 分进程运行，但共用同一 Go 代码库和领域模型。
- PostgreSQL 保存业务事实；Redis / Asynq 只承担异步任务。

当前不做 Section 或行政班映射、作业提交与评分、完整教务系统、原件历史浏览、通用知识库和微服务拆分。

## 功能

| 阶段 | 已实现能力 |
| --- | --- |
| 阶段一 | 课程实例、成员关系、课程时间、动态课表、共享作业、个人 DDL、文字导入、候选审核发布 |
| 阶段二 | PDF/PPT/PPTX 临时上传、逐页解析、规则定位、候选页预览、异步进度与失败反馈 |
| 阶段三 | 成员角色/状态维护、最后维护者保护、站内通知、候选审核指标、通用模型接口 |

同一用户向同一课程重复上传相同文件与相同随附文字时，会返回已有的处理任务，不再生成第二批候选；原任务失败后可以重新上传。随附文字变化会生成新任务，纯文字来源暂未按内容去重。

## 架构

~~~
Vue 前端
  │ REST + 当前身份
  ▼
Gin API ─────────────────► PostgreSQL
  │                           课程、作业、候选、通知
  └── job_id ────────────► Redis / Asynq ───► Go Worker
                                                   │
                        临时工作目录 ◄─────────────┤
                                                   │
                        OpenAI 兼容模型服务 ◄──────┘
~~~

- API 负责身份、权限、课程/作业查询和来源任务创建。
- Worker 负责文档转换、逐页解析、规则定位、模型抽取、结果校验与临时文件清理。
- 候选确认、正式作业创建和通知生成在同一数据库事务内完成。

详细设计见 docs/architecture.md、docs/design/02-database-schema.md 和 docs/design/05-async-ingestion.md。

## 导入流程

~~~
文字 / PDF / PPT / PPTX
  → SourceItem + IngestionJob
  → 逐页解析
  → 规则定位
  → 模型抽取最小上下文
  → 校验候选、页码和文字证据
  → review_ready
  → 维护者接受或拒绝
  → 发布共享作业并通知成员
~~~

规则优先寻找作业、提交、截止、DDL 等明确信号。若整份文件是完整实验任务包，会保留整份页面上下文。候选证据的页码和文字摘录必须能在输入页面中找到。

## 本地运行

### 前置条件

- Go 1.26+
- Node.js 20+ 与 npm
- PostgreSQL
- Redis
- Poppler：pdfinfo、pdftotext、pdftoppm
- LibreOffice：仅 PPT/PPTX 需要，soffice 必须在 PATH 中

本地开发不需要 Docker。创建 PostgreSQL 数据库 homework_canvas 后，设置：

~~~powershell
$env:DATABASE_URL='postgres://canvas:canvas@localhost:5432/homework_canvas?sslmode=disable'
$env:REDIS_ADDR='127.0.0.1:6379'
~~~

可选设置临时目录：

~~~powershell
$env:INGESTION_TMP_DIR='D:	emphomework-canvas-ingestion'
~~~

未设置时使用系统临时目录下的 homework-canvas-ingestion。它不是文件存储：任务处理成功、失败或重试耗尽后会被清理，重新处理需重新上传。

执行迁移并启动后端：

~~~powershell
Set-Location backend
goose -dir migrations postgres "$env:DATABASE_URL" up

# 终端一
go run ./cmd/worker

# 终端二
go run ./cmd/api
~~~

API 默认监听 http://localhost:8080，健康检查为 GET /healthz。

启动前端：

~~~powershell
Set-Location frontend
npm install
npm run dev
~~~

前端默认请求 http://localhost:8080/api/v1。

## 身份与本地调试

本地身份适配器使用以下请求头创建或读取用户：

~~~http
X-Canvas-Subject: dev-user
X-Canvas-Name: Local Developer
~~~

生产环境应替换为 UniSSO 验证器。浏览器不应自行传递可伪造身份；课程、作业、来源、任务与候选都按有效 Enrollment 或维护者角色校验。

## 模型配置

Worker 使用通用 OpenAI 兼容 Chat Completions 接口。API Key 仅从 Worker/API 环境变量读取，绝不经前端传递或写入数据库。

### 内置提供商

设置提供商和密钥即可；模型名可选。

~~~powershell
# OpenAI
$env:MODEL_PROVIDER='openai'
$env:MODEL_API_KEY='your-api-key'
$env:MODEL_NAME='gpt-4o-mini'

# DeepSeek
$env:MODEL_PROVIDER='deepseek'
$env:MODEL_API_KEY='your-api-key'
$env:MODEL_NAME='deepseek-chat'

# Qwen / DashScope
$env:MODEL_PROVIDER='qwen'
$env:MODEL_API_KEY='your-api-key'
$env:MODEL_NAME='qwen-plus'
~~~

| 提供商 | 默认 Base URL | 默认模型 |
| --- | --- | --- |
| openai | https://api.openai.com/v1 | gpt-4o-mini |
| deepseek | https://api.deepseek.com | deepseek-chat |
| qwen / dashscope | https://dashscope.aliyuncs.com/compatible-mode/v1 | qwen-plus |

### 自定义兼容提供商或覆盖地址

MODEL_BASE_URL 优先级最高。地址可写到 /v1，也可完整写到 /chat/completions；程序会自动补全后者。

~~~powershell
$env:MODEL_PROVIDER='custom'
$env:MODEL_BASE_URL='https://models.example.com/v1'
$env:MODEL_API_KEY='your-api-key'
$env:MODEL_NAME='your-model'
~~~

没有配置 MODEL_PROVIDER 或 MODEL_API_KEY 时，系统会明确退回本地规则抽取器；它只生成待审核候选，不能视为模型抽取结果。

## 权限、个人状态与通知

| 角色 | 能力 |
| --- | --- |
| student | 查看已加入课程、导入来源、维护自己的 DDL 与完成状态 |
| maintainer | 包含学生能力；可维护成员、课表、共享作业与候选审核 |

- personal_due_at 为空时，个人有效截止时间使用 official_due_at。
- 官方逾期只按 official_due_at 判断，不会被个人计划时间掩盖。
- 手工发布作业或确认候选作业后，会为所有有效课程成员创建站内通知。
- 系统不允许把最后一名有效维护者降级或设为离开。

## 主要接口

全部业务接口的前缀为 /api/v1。

| 接口 | 用途 |
| --- | --- |
| POST /course-instances | 创建课程实例；创建者成为维护者 |
| GET /course-instances | 查询当前用户已加入课程 |
| GET / POST / PATCH /course-instances/{id}/enrollments | 查询、添加和修改课程成员 |
| PUT /course-instances/{id}/meetings | 整体替换课程时间安排 |
| GET /me/schedule | 动态生成个人课表 |
| GET /me/assignments?bucket=... | 作业视图；支持 upcoming、overdue、undated、all |
| POST /course-instances/{id}/assignments | 手工发布共享作业 |
| PUT /assignments/{id}/my-state | 更新当前用户个人 DDL 与完成状态 |
| POST /course-instances/{id}/sources/text | 提交文字来源 |
| POST /course-instances/{id}/sources/file | 上传 PDF/PPT/PPTX，表单字段为 file |
| GET /ingestion-jobs/{id} | 查询处理状态、阶段和失败原因 |
| GET /course-instances/{id}/assignment-candidates | 维护者查看候选 |
| POST /assignment-candidates/{id}/accept | 原子确认候选并发布作业 |
| POST /assignment-candidates/{id}/reject | 拒绝候选 |
| GET /course-instances/{id}/ingestion-metrics | 查询 pending、accepted、rejected 指标 |
| GET /me/notifications | 查询站内通知 |
| POST /notifications/{id}/read | 标记通知已读 |

错误格式：

~~~json
{
  "error": {
    "code": "courses.not_maintainer",
    "message": "你没有维护该课程的权限"
  }
}
~~~

## 数据与隐私

- PostgreSQL 保存课程、成员、作业、个人状态、来源元数据、任务、候选、审核结果和通知。
- 文件来源只保存文件名、MIME、大小与校验值，不持久化二进制原件。
- 候选只保存必要页码和文字证据；任务结束后不能回看原页。
- 日志不应记录完整课件、用户原文、模型密钥或临时路径。

## 验证

已执行：

~~~powershell
Set-Location backend
go test ./...
go build ./...
go vet ./...

Set-Location ..rontend
npm run build
~~~

单元测试和隔离 PostgreSQL 集成测试的范围、运行方法以及尚未覆盖的风险见 [当前质量基线](docs/quality-status.md)。构建和静态检查不能证明业务行为正确；真实模型调用仍取决于提供商、Key、模型权限和网络环境。

## 目录

~~~
backend/
  cmd/api/                 HTTP API
  cmd/worker/              Asynq Worker
  internal/
    assignments/           共享作业、个人状态、候选审核
    courses/               课程实例、成员关系、课表
    ingestion/             文件处理、定位、模型抽取
    notifications/         站内通知
    identity/              本地身份适配器
    shared/                数据库、队列、错误、临时文件
  migrations/              goose 迁移
frontend/src/              Vue 3 前端
docs/                      架构与详细设计
~~~

## 进一步阅读

- docs/architecture.md
- docs/technology-selection.md
- docs/design/01-domain-model.md
- docs/design/03-backend-modules.md
- docs/design/04-api-design.md
- docs/design/05-async-ingestion.md
- docs/design/06-frontend-information-architecture.md
