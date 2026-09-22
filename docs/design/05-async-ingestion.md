# 异步任务设计

> 参考：[总架构](../architecture.md) · [领域模型](01-domain-model.md) · [技术选型](../technology-selection.md)

## 1. 处理链路

```mermaid
flowchart LR
  Upload[上传文件或文字] --> Create[创建 SourceItem + IngestionJob]
  Create --> Enqueue[Asynq 投递 job_id]
  Enqueue --> Parse[解析 / 转 PDF]
  Parse --> Locate[规则筛选候选页]
  Locate --> Extract[MaaS 结构化抽取]
  Extract --> Validate[Schema 与时间校验]
  Validate --> Candidate[写入 Candidate]
  Candidate --> Review[维护者确认或拒绝]
  Review --> Assignment[创建共享 Assignment]
```

文件上传只在临时工作目录保留到任务终态或自动重试耗尽。文字消息不需要临时文件。

## 2. Job 状态机

状态按总架构定义：

```text
queued -> parsing -> locating -> extracting -> validating -> review_ready -> completed
                                      \-> failed
```

含义：

| 状态 | 含义 | 可重试 |
| --- | --- | --- |
| queued | 已建来源和任务，等待 Worker | 是 |
| parsing | 正在读取文字、转换或渲染页面 | 视错误而定 |
| locating | 正在规则筛选候选页 | 否，除瞬时进程故障 |
| extracting | 正在调用算力中转/MaaS | 是，限次 |
| validating | 正在校验 Schema、日期和证据 | 否，输出不合法时失败 |
| review_ready | Candidate 已落库，等待人工审核 | 不由 Worker 重跑 |
| completed | 该来源的候选均已审核结束 | 否 |
| failed | 不可恢复或重试耗尽 | 用户重新上传后新建任务 |

`completed` 由 Candidate 审核服务在该来源不存在 `pending` 候选时设置；它不是 Worker 自动发布作业的信号。

## 3. Worker 步骤

### 3.1 创建与投递

1. API 验证用户为 CourseInstance 的有效成员；
2. 文件检查类型、大小和文件名；文件写入与 job_id 绑定的临时目录；
3. 单一数据库事务创建 SourceItem 与 `queued` Job；
4. 事务提交后投递仅含 `job_id` 的 Asynq 任务；
5. 投递失败时 Job 保持 `queued`，补偿扫描重新投递。

### 3.2 parsing

- 文字：直接形成一页文本上下文；
- PDF：Poppler 提取逐页文字，按需要临时渲染页面；
- PPT/PPTX：LibreOffice Headless 临时转换为 PDF，再走 PDF 流程；
- 不支持格式、损坏文件、转换超时直接进入 `failed`。

每个转换进程必须有独立临时目录、超时和资源上限。Worker 不把完整文件或页面图片写回 PostgreSQL、Redis 或持久卷。

### 3.3 locating

对每页计算可解释的规则分数，首版包括：作业关键词、时间表达、提交动作、标题权重和文本密度。取命中页及前后各一页去重；无命中页时写入 `failed`，错误码为 `ingestion.no_assignment_page`，不把全文发送给 MaaS。

### 3.4 extracting 与 validating

传给 MaaS 的输入仅为候选页及相邻页的文字、页码和临时预览，不含用户身份或全份文档。`AIProvider` 要求返回固定 JSON：作业字段、证据、置信度和不确定字段。

校验器负责：

- JSON Schema 与字段长度；
- 页码必须属于实际输入页；
- 证据片段必须能在相应页面文字中定位；
- 时间能否标准化为课程时区的 `official_due_at`；不能确定时保留原始表达并标记不确定；
- 删除空候选和完全重复候选。

通过校验后，以 `(source_item_id, ordinal)` 事务性替换该来源当前的 pending Candidate，再将 Job 设为 `review_ready`。

## 4. 重试与幂等

| 情形 | 策略 |
| --- | --- |
| 算力中转超时、MaaS 临时不可用 | 指数退避，最多 `max_attempts` 次 |
| Worker 崩溃、临时进程异常 | 由 Asynq 重投；从可安全重入的 stage 开始 |
| 格式不支持、文件损坏、页码/Schema 不合法 | 不重试，标为 failed |
| 重复 HTTP 上传 | 用 `Idempotency-Key` 返回同一 Source/Job |
| 同一 Job 重投 | 条件更新 Job 状态；已到 review_ready/completed 的任务直接跳过 |
| 候选重复写入 | `(source_item_id, ordinal)` 唯一约束和事务替换 |
| 重复接受候选 | 候选行锁 + `assignments.source_candidate_id` 唯一约束 |

## 5. 清理与观测

- Job 到达 `review_ready`、`completed` 或 `failed` 后清理临时文件；自动重试期间保留。
- 清理失败记录警告并由定时清理器按 job_id 与保留期补偿；不因此保留成长期文件库。
- 每步记录 `job_id`、stage、attempt、耗时、错误码和候选数；日志不记录完整文档和文字消息。
- 任务查询 API 只显示用户可理解的阶段、状态和错误说明，不暴露 MaaS 密钥、命令行参数或本地路径。
