# Homework Canvas 架构设计

> 状态：初稿  
> 目标阶段：MVP 到早期多人使用  
> 核心原则：业务系统为主，AI 仅承担信息解析与结构化抽取

## 1. 产品目标与边界

Homework Canvas 将老师通过 PPT、PDF 或文字消息发布的作业信息，整理为统一的课程、课表、作业和截止时间视图。

系统需要解决三个核心问题：

1. 将分散、格式不统一的作业信息转为结构化数据；
2. 让同一教学班的学生共享同一份课程与作业数据；
3. 保留来源和人工确认入口，避免 AI 误识别直接变成正式作业。

当前阶段不做：

- 复杂 Section、行政班与教学班的多层映射；
- 完整教务系统、成绩系统和在线提交系统；
- 面向任意文档的通用知识库或对话式 AI；
- 微服务拆分；
- 为每个学生单独保存一份课表或共享作业副本。

## 2. 架构原则

- **模块化单体优先**：业务 API 保持单体部署，通过清晰模块边界降低耦合；文档解析和 AI 抽取放入独立 Worker，避免阻塞请求。
- **PostgreSQL 是业务事实来源**：课程、选课关系、作业和处理状态以数据库为准；Redis 不保存不可恢复的业务事实。
- **共享数据只存一份**：官方作业属于 `CourseInstance`，学生通过 `Enrollment` 获得访问权，不复制官方作业；个人计划状态单独保存。
- **课表动态生成**：用户课表由 Enrollment 和课程上课安排实时查询得到，不维护用户课表副本。
- **候选与正式数据分离**：AI 输出先成为待确认候选，经过用户确认后才写入正式作业。
- **来源可追溯**：正式作业保留来源文件、页码或文字片段，便于纠错。
- **为扩展保留接口，不提前拆服务**：解析器、文件存储、任务队列和 AI 提供方均通过稳定接口接入。

当前技术选型及其复核条件见 [技术选型](technology-selection.md)。总架构不依赖某一种编程语言；首版以 Go 实现是当前工程决策，而非 UniSSO 或部署平台的强制要求。

## 3. 系统总览

```mermaid
flowchart LR
    Client[Web / 移动端] --> API[业务 API<br/>模块化单体]
    API --> PG[(PostgreSQL)]
    API --> Queue[(Redis / 任务队列)]
    Queue --> Worker[解析与抽取 Worker]
    API --> Temp[临时工作目录]
    Worker --> Temp
    Worker --> AI[算力中转 / MaaS]
    Worker --> PG

    subgraph Backend[同一后端代码库]
      API
      Worker
    end
```

对用户而言，系统仍是一个完整产品；API 和 Worker 可以使用同一代码库、同一领域模型，分别以不同进程启动。这样既保持部署简单，也隔离了耗时和不稳定的文档处理任务。

## 4. 核心模块

### 4.1 身份与访问模块

职责：

- 用户登录和身份识别；
- 判断用户是否通过 Enrollment 加入某个 CourseInstance；
- 控制课程、作业、源文件和候选结果的访问权限。

初期只需区分普通学生与课程维护者。更细的角色权限可以后续增加，不在 MVP 中引入复杂 RBAC。

### 4.2 课程模块

职责：

- 维护课程基本信息与具体开课实例；
- 维护用户选课关系；
- 维护每周上课时间、地点及有效日期；
- 根据用户 Enrollment 动态生成课表。

关键概念：

- `Course`：相对稳定的课程定义，例如“软件测试”。
- `CourseInstance`：某学期、某老师实际开设的一门课。
- `Enrollment`：用户加入 CourseInstance 的关系。
- `CourseMeeting`：CourseInstance 的周期性上课安排。

暂不设置 Section。未来只有出现同一 CourseInstance 内作业或课表明显分组的需求时，再增加 Section，并让 Enrollment 可选关联 Section。

### 4.3 作业模块

职责：

- 维护课程实例下的共享作业；
- 提供跨课程和单课程的截止时间排序查询；
- 支持用户确认、修改或拒绝 AI 生成的作业候选；
- 保存作业与原始来源之间的关联。

作业的官方内容和官方截止时间属于 CourseInstance，不属于单个学生。学生的个人计划截止时间、完成状态等个性化信息保存到 `AssignmentUserState`，而不是复制或修改共享 Assignment。

#### 前端展示约定

“作业”是用户进入系统后的核心视图。首页和作业页默认展示当前用户已加入课程的全部作业，而不是要求用户逐门课程查看。

- 待处理作业按个人有效截止时间升序排列：设置个人计划截止时间时使用该时间，否则使用官方 `due_at`；
- 官方截止时间与个人计划截止时间同时存在时，卡片同时展示，个人计划不修改全班共享作业；
- 已逾期作业依据官方 `due_at` 单独分组并明显标记，避免个人计划时间掩盖真实逾期；
- 无法确定截止时间的作业放在“截止时间待确认”分组，不参与正常时间排序；
- 每项至少显示课程名称、作业标题、截止时间和当前状态；
- 用户进入课程详情后，仍可查看只属于该 CourseInstance 的同样排序列表。

排序由后端查询保证，前端负责分组和展示。这样分页、多个客户端以及后续通知功能使用的是同一套顺序定义。相同截止时间使用 Assignment ID 作为稳定的次级排序键。

### 4.4 来源与导入模块

职责：

- 接收 PPT、PDF 和纯文字消息；
- 将上传文件仅写入 API 与 Worker 可访问的临时工作目录，将元数据与文字消息保存到 PostgreSQL；
- 创建异步处理任务并向用户返回可查询的任务 ID；
- 展示处理进度、失败原因和来源文字证据。

来源在进入系统时必须关联目标 CourseInstance，避免 AI 猜测课程归属。

PPT、PDF、图片和候选页预览不作为产品数据持久化。Worker 在任务完成、失败或超过临时文件保留期后删除它们；正式作业仅保留来源类型、原始文件名、校验值、页码和必要的文字证据。处理完成后如需重新解析原始文档，用户必须重新上传。

### 4.5 文档解析与 AI 抽取模块

职责：

- 从 PPT/PDF 提取页面文本和必要的页面预览；
- 使用规则为每页计算“疑似作业页”分数；
- 将候选页与少量相邻页组成最小上下文；
- 通过算力中转调用 MaaS，要求返回固定结构的 JSON；
- 执行结构校验、时间标准化和结果落库。

该模块只产出 `AssignmentCandidate`，不直接创建正式 Assignment。

## 5. 核心数据模型

```mermaid
erDiagram
    USER ||--o{ ENROLLMENT : joins
    COURSE ||--o{ COURSE_INSTANCE : offers
    COURSE_INSTANCE ||--o{ ENROLLMENT : has
    COURSE_INSTANCE ||--o{ COURSE_MEETING : schedules
    COURSE_INSTANCE ||--o{ ASSIGNMENT : shares
    USER ||--o{ ASSIGNMENT_USER_STATE : maintains
    ASSIGNMENT ||--o{ ASSIGNMENT_USER_STATE : personalizes
    COURSE_INSTANCE ||--o{ SOURCE_ITEM : receives
    USER ||--o{ SOURCE_ITEM : submits
    SOURCE_ITEM ||--o{ INGESTION_JOB : processes
    SOURCE_ITEM ||--o{ ASSIGNMENT_CANDIDATE : produces
    ASSIGNMENT_CANDIDATE o|--o| ASSIGNMENT : becomes
```

建议的最小实体如下：

| 实体 | 关键字段 | 说明 |
| --- | --- | --- |
| User | id, name, email, status | 系统用户 |
| Course | id, code, name | 稳定课程定义 |
| CourseInstance | id, course_id, term, teacher_name, status | 某学期的具体课程 |
| Enrollment | user_id, course_instance_id, role, status | 用户与课程实例的唯一关系 |
| CourseMeeting | course_instance_id, weekday, start_time, end_time, location, valid_from, valid_to | 生成课表的依据 |
| Assignment | course_instance_id, title, description, due_at, status, created_by | 全班共享的正式作业 |
| AssignmentUserState | assignment_id, user_id, personal_due_at, completion_status | 学生个人计划与完成状态，不改变共享作业 |
| SourceItem | course_instance_id, submitted_by, type, original_filename/raw_text, checksum, status | 原始文件或文字来源的元数据；二进制原件不持久化 |
| IngestionJob | source_item_id, stage, status, attempts, error_code, started_at, finished_at | 异步处理记录 |
| AssignmentCandidate | source_item_id, title, description, due_at, evidence, confidence, review_status, reviewed_by | AI 抽取候选及证据 |

关键约束：

- `Enrollment(user_id, course_instance_id)` 唯一；
- `AssignmentUserState(assignment_id, user_id)` 唯一，`personal_due_at` 为空表示沿用官方截止时间；
- CourseMeeting 只描述课程安排，不生成和保存用户课表行；
- AssignmentCandidate 与 Assignment 分离，`review_status` 至少包括 `pending / accepted / rejected`；
- 所有时间在数据库中使用带时区时间，课程实例保存默认时区；
- 二进制文件不直接存入数据库或对象存储；仅在任务的临时工作目录中存在，数据库保存原始文件名、校验值、状态和抽取后的必要文字证据。

## 6. 作业导入链路

### 6.1 PPT / PDF

```mermaid
sequenceDiagram
    participant U as 用户
    participant A as API
    participant Q as 任务队列
    participant W as Worker
    participant M as AI
    participant D as PostgreSQL

    U->>A: 上传文件并选择课程
    A->>D: 保存 SourceItem 与 IngestionJob
    A->>Q: 投递任务
    A-->>U: 返回任务 ID
    Q->>W: 消费任务
    W->>W: 提取页面文本/预览
    W->>W: 规则筛选疑似作业页
    W->>M: 候选页 + 少量相邻页 + 固定 Schema
    M-->>W: 结构化候选
    W->>W: Schema 校验与时间标准化
    W->>D: 保存 AssignmentCandidate
    U->>A: 确认或修改候选
    A->>D: 创建或更新共享 Assignment
```

规则筛选只负责提高召回并控制 AI 成本，不负责最终判断。首版规则可关注：

- “作业、提交、截止、deadline、due”等关键词；
- 日期、时间、周次与提交平台表达；
- 页面标题和文本密度；
- 同一页同时出现任务动作与时间信息。

候选上下文默认使用命中页及前后各 1 页；具体窗口应可配置，但不做全文默认发送。

### 6.2 纯文字消息

文字消息无需文档预筛选，直接进入固定 Schema 的 AI 抽取，再走相同的校验、候选确认和正式入库流程。

### 6.3 AI 输出边界

AI 输出字段应限制为：

- 作业标题；
- 作业描述；
- 截止时间及其原始表达；
- 提交方式；
- 来源证据片段与页码；
- 字段级置信度；
- 无法确定的字段列表。

不允许 AI 决定课程归属、用户权限或直接发布作业。日期缺少年份、时区或表述冲突时，应标记为不确定并等待用户确认，不进行静默猜测。

## 7. 状态与失败处理

导入任务采用简单、可恢复的状态机：

`queued -> parsing -> locating -> extracting -> validating -> review_ready -> completed`

任一步骤失败进入 `failed`，记录稳定错误码与可读说明。Worker 只对临时故障自动重试，例如网络超时；格式不支持、文件损坏和结构校验失败不应无限重试。

为避免重复上传或任务重试产生重复结果：

- SourceItem 保存文件或文字内容的校验值；
- 每个处理步骤按 `job_id + stage` 保持幂等；
- 接受候选时使用数据库事务，并保证同一候选最多生成一个 Assignment。

## 8. API 边界

首版 API 只需覆盖以下资源：

- `/courses`、`/course-instances`：课程和开课实例；
- `/course-instances/{id}/enrollments`：加入和管理课程成员；
- `/course-instances/{id}/meetings`：课程上课安排；
- `/me/schedule`：根据 Enrollment 动态查询个人课表；
- `/me/assignments`：聚合当前用户所有已选课程的作业，按个人有效截止时间升序返回，并同时返回官方与个人计划截止时间；
- `/course-instances/{id}/assignments`：单门课程的共享作业列表与维护；
- `/assignments/{id}/my-state`：维护当前用户的个人计划截止时间和完成状态；
- `/sources`：上传文件或提交文字消息；
- `/ingestion-jobs/{id}`：查询异步任务状态；
- `/assignment-candidates`：查看、修改、接受或拒绝候选。

模块之间通过应用服务调用，不允许控制器直接跨模块修改数据表。Worker 通过任务载荷中的 ID 读取数据库，不在队列中传输完整文件或大段正文。

作业列表以 `COALESCE(assignment_user_states.personal_due_at, assignments.due_at) ASC NULLS LAST, assignments.id ASC` 作为稳定排序。接口需要返回 CourseInstance 的名称或简要信息，避免前端为列表中的每项作业再次请求课程数据；同时返回官方截止时间，供逾期判断。Assignment 应建立以 `course_instance_id`、`due_at` 为核心的查询索引。

## 9. 部署方案

首版使用 Docker Compose 即可：

- `web`：前端静态资源；
- `api`：业务 API；
- `worker`：文档解析与 AI 抽取；
- `postgres`：业务数据库；
- `redis`：任务队列及短期缓存；
- `ingestion-tmp`：API 与 Worker 可访问的临时工作目录，仅保存正在处理或允许自动重试的上传文件；任务终态后清理。

`api` 与 `worker` 使用同一后端镜像和不同启动命令。生产扩容时，优先独立增加 Worker 数量；只有出现明确的容量、团队或故障隔离需求后，再考虑拆分微服务。

临时目录不是文件存储能力，不承诺重启后保留。首版 API 与 Worker 在同一部署主机上运行；未来若需要多主机、跨节点 Worker 或支持重新查看原件，再单独决策是否引入对象存储。

## 10. 安全与可观测性

首版必须具备：

- 每次课程、作业和文件访问都校验 Enrollment 或维护者权限；
- 临时工作目录不作为静态目录暴露；任务结束后的原始文件和预览必须被删除；
- 限制上传文件类型、大小和单任务处理时间；
- 日志携带 `request_id`、`job_id`、`source_item_id`，但不记录完整课件和用户文字；
- 记录任务耗时、失败率、候选接受/修改/拒绝情况；
- AI 密钥只通过环境变量或密钥服务注入。

候选接受率和修改率可衡量抽取效果，但在积累足够样本前，不应据此自动发布作业。

## 11. 建议的代码组织

```text
backend/
  app/
    modules/
      identity/
      courses/
      assignments/
      ingestion/
    workers/
      document_parser/
      candidate_locator/
      ai_extractor/
    shared/
      database/
      temporary_files/
      task_queue/
      ai_provider/
  migrations/
frontend/
deploy/
  docker-compose.yml
docs/
  architecture.md
```

每个业务模块内部包含自己的领域模型、应用服务、持久化实现和 API 入口。`shared` 只放跨模块的技术适配器，不放课程或作业业务规则。

## 12. 分阶段落地

### 阶段一：可用闭环

- 课程、开课实例、Enrollment 和 CourseMeeting；
- 动态课表，以及跨课程按个人计划截止时间排序、保留官方截止时间的作业视图；
- 文字消息抽取；
- 候选确认后生成正式作业；
- Docker Compose 本地部署。

### 阶段二：文档导入

- PDF/PPT 页面解析；
- 规则定位候选页；
- 候选页与相邻页的 AI 抽取；
- 来源页预览、失败重试和处理进度。

### 阶段三：多人使用加固

- 更完整的课程维护权限；
- 通知与个人作业状态；
- 基于真实数据优化候选规则和抽取质量；
- 按实际需求评估 Section，而非预先引入。

## 13. 关键架构决策

1. **不把 AI 放在核心业务路径中央**：即使 AI 暂时不可用，用户仍可手工创建、查看和管理课程与作业。
2. **正式作业与 AI 候选分离**：保证共享数据的可信度，也保留后续自动化策略的演进空间。
3. **CourseInstance 是共享边界**：课程成员、课表安排和作业围绕同一教学实例组织。
4. **API 与 Worker 逻辑分离、代码复用**：避免长任务阻塞，同时保持单仓库和简单部署。
5. **先保留来源与反馈数据，再优化模型**：后续改进应基于真实误差，而不是提前堆叠复杂 AI 能力。
