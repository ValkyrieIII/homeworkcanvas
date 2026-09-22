# homeworkcanvas 后端审查报告

- 日期：2026-09-23
- 范围：`backend/` 全部 19 个 Go 文件、3 个 goose 迁移、与 `frontend/src` 的接口契约对照
- 方法：先跟错误路径（每个 error 追到它最后可见的位置）和最坏输入规模，再逐文件核对类型/边界/事务/并发
- 判据标注：**读码确定** = 沿代码路径可确定；**源码确认** = 查过依赖库源码；**推断** = 结构性缺陷，触发概率未知

---

## 0. 先修这三件

1. ~~**模型错误信息在入库前就丢了（P0-1）** —— 不修这条，后面所有线上问题都无从排查。~~ 已修复（2026-09-23）。
2. ~~**送进模型的文本没有任何长度上限（P0-2）** —— 撞上就是必现失败，且因为 P0-1 连原因是"太长"都看不到。~~ 已修复（2026-09-23）。
3. ~~**数据库约束错误一律 500 且无日志（P0-3）** —— 重复加成员、重复提交个人状态、非法时间格式，全部表现为"服务暂时不可用"。~~ 已修复（2026-09-23）。
4. ~~**入队失败静默（P0-4）**、**终态不清理临时文件（P0-5）**~~ 已修复（2026-09-23）。

---

## 1. P0 — 会静默出错，或出事时无法排查

### P0-1 模型错误信息全丢
- 位置：`ingestion/relay_extractor.go:129-131` → `ingestion/service.go:232-234`
- 现象：非 2xx 时只保留 `provider + resp.Status`，`resp.Body` 从未读取；service 层再把它换成固定文案"暂时无法抽取作业候选"；worker 进程内没有任何 logger。
- 后果：`invalid_api_key`、`insufficient_quota`、`model_not_found`、`context_length_exceeded`、中转网关 502 —— 全部塌成同一句话。DB 里只有 `ingestion.ai_unavailable`，日志里什么都没有。这同时也是 P0-2、2.1 无法收敛的原因：你连分类所需的输入都丢了。
- 判据：读码确定。已把 `Extract` 的 8 条错误返回逐条跟到最终落库字段与接口返回。
- 方向：读响应体（限长 4KB 即可，中转网关会回 HTML）提取 `error.code`/`error.message` 写入 `error_code`/`error_message`，并至少加一行 stderr 日志。
- **状态：已修复（2026-09-23）**。`modelErrorDetail` 从错误响应体取 `error.code`/`error.type` + `error.message`（限读 4KB、压成一行、300 字截断，取不到就回退原文）；`Process` 把它拼进 `error_message`；`fail`/`retryOrFail` 各加一行 `log.Printf` 到 stderr。`error_code` 仍保持 `ingestion.ai_unavailable` 不变（对外契约）。

### P0-2 送进模型的文本没有上限
- 位置：`ingestion/locator.go:17-20, 40-46, 72-87`；`ingestion/service.go:211, 292-293`
- 现象：三条路径都不封顶。
  1. `locate` 在 `isDocumentWideTask` 命中时直接 `return pages`（整份文档、每页全文）。而 `isDocumentWideTask` 的第一条要求是首页出现"作业"三个字里的任意一个，命中率不低。
  2. 非文档级路径也不是界：`locate` 取的是"命中页 ± 1"，而页面只要同时含作业类词（+3）和任一日期表达（+2）就够 4 分被选中 —— 一份每页都有"作业/提交/截止"的课件，100 页全都会被选中。所以那个 ±1 窗口不是上限。
  3. 文字来源根本不走 `locate`（只对 `SourceType == "file"` 调用），粘多长就是多长。
- 后果：必现 `context_length_exceeded`；因为 P0-1，报出来的是"暂时无法抽取作业候选"。`maxUploadBytes`（25MB）管的是上传文件，不是 prompt 字符数。
- 判据：读码确定（路径已逐条走）。
- 方向：在 `ExtractionInput` 组装处按字符数（或 token 估算）截断并记录被截断的页数；文字来源同样要过这个闸。这是根因修复，调超时/加提示都不解决。
- **状态：已修复（2026-09-23）**。新增 `boundPages`，在 `Process` 组装 `ExtractionInput` 处统一封顶：单页 12000 字、总量 40000 字（按 rune 截断，不会切坏字符）；文件与文字来源共用这一道闸，被截断/丢弃的页数写进 worker 日志。校验仍用未截断的原文，被截断部分只是前缀，证据片段照样能对上。

### P0-3 数据库约束错误全部塌成 500 且无日志
- 位置：`shared/http.go:9-16`（非 `AppError` 一律 500）；触发点如 `cmd/api/main.go:79`、`assignments/service.go:127`、`courses/service.go:133`
- 现象：`RespondError` 只对 `*AppError` 特判，其余（唯一约束、外键、类型转换失败）统一回 `shared.internal`"服务暂时不可用"，且没有任何日志。
- 具体会踩到的三种：
  - 对已有成员重复 `POST /enrollments` → 撞 `ux_enrollment` → 500（文档约定是 409）；`user_id` 不存在 → 外键 → 500。
  - `PUT /assignments/{id}/my-state` 快速双击 → 两个请求都 `First` 不到再各自 `Save` → 撞 `(assignment_id,user_id)` 唯一约束 → 一个 500。
  - `PUT /meetings` 传非法时间格式 → PG 报 `invalid input syntax for type time` → 500。
- 判据：读码确定。
- 方向：给 GORM 错误加一层映射（23505→409、23503→400/404、22P02→400），并统一打日志。这是"入参错误返回 500"的系统性来源，改一处覆盖多个接口。
- **状态：已修复（2026-09-23）**。`RespondError` 增加 `classify`：23505→409 `shared.conflict`、23503→400、23502→400、22P02/22007/22008→400、`gorm.ErrRecordNotFound`→404；其余仍是 500。凡不是 `AppError` 的错误都记一行 stderr（method/path/status/code/err），所以重复点击这类"用户手滑"在日志里也看得见。
- **未修（留给 P1）**：`PUT /assignments/{id}/my-state` 重复提交现在返回 409，不再 500，但根因是"先 `First` 后 `Save`"的竞态，要改成 `ON CONFLICT` upsert 才会静默成功。

### P0-4 入队失败静默，任务永远停在 queued
- 位置：`ingestion/service.go:172-185`
- 现象：`createSourceAndJob` 已提交（source+job 落库），随后 `enqueue` 里 `Enqueue` 出错只是 `if e == nil` 跳过，不返回错误、不记录、不标失败。另外 `NewJobTask` 的 `if e != nil { return }` 是不可达分支（`json.Marshal` 一个 `map[string]string` 不会失败）。
- 后果：DB 里有一条 `queued` 的 job 和 `queued` 的 source，队列里没有任务，没有任何补偿机制，前端每 1.8 秒轮询到永久。文档 05 §3.1.5 承诺的"补偿扫描重新投递"不存在。
- 判据：读码确定。
- 方向：`enqueue` 返回 error，创建接口把它落成 `failed` + 明确错误码；或按文档补一个启动时的补偿扫描。
- **状态：已修复（2026-09-23）**。`enqueue` 改为返回 error（含 `Queue == nil`），两个创建接口走 `enqueueFailed`：落 `ingestion.enqueue_failed` + 真实原因，`source_items.status='failed'`，临时目录就地删掉。接口仍返回 202，但响应体里的 job 已是 `failed` —— 前端的 `watchJob` 见 `failed` 立即停轮询并显示 `error_message`。`fail()` 现在同时同步内存里的 job 字段（此前只写库，返回给调用方的还是旧值）。未做文档 05 §3.1.5 的补偿扫描：投递失败已经在同步路径里变成可见终态，再补一个扫描器是多余机制。

### P0-5 「任务终态即清理」这条隐私承诺不成立
- 位置：`ingestion/service.go:259-261`（唯一清理点）、`:304-311`（`fail` 里清理）
- 现象：临时目录只在"走到 `review_ready`"和"显式 `fail`"两个时刻被删。以下四类情况永久残留上传原件、转换出的 PDF 和预览 PNG：
  1. 入队失败（P0-4）；
  2. 出现不走 `retryOrFail`/`fail` 的错误（DB 错误等）→ asynq 重试耗尽进死信队列，job 停在 `processing`；
  3. worker 进程被杀；
  4. asynq 判定超时（默认 30 分钟）后重投 —— `RenderPreviews` 用的是外层无超时的 ctx，大文件有可能跑到这个量级。
  README 写的"原始文件、逐页文本和预览只在处理期间存在，任务终态即清理"和设计文档 05 §5 的"定时清理器补偿"都没有兜住。
- 判据：读码确定（清理点只有两处，路径穷举完毕）。
- 方向：把清理挂到"任务进入任何终态"的单一出口 + 按 `job_id` 的定时清扫兜底。
- **状态：已修复（2026-09-23）**。分两层：
  1. 终态收口。四类残留里 (1) 走 `fail()`（P0-4 已修），(2) 由 worker handler 里的 `settleIfExhausted` 兜住 —— `Process` 里未归类的错误（数据库故障等）此前会交给 asynq 无限重试，任务永远停在 `processing`；现在尝试次数用尽就落 `ingestion.internal_error` + 原因并清理，收尾用 `context.WithoutCancel`，因为 asynq 在超时/停机时已经取消了 ctx。
  2. 定时兜底。`SweepStale` 扫描临时根目录：任务已终态、任务记录不存在、或目录 24 小时没动过，就删掉。worker 启动先扫一遍，之后每小时一次 —— 覆盖 (3) worker 被杀、(4) 任务被 asynq 丢弃这两种进程不在了的情况。保留期取 24 小时，远大于 asynq 默认 30 分钟的处理上限，所以不会误删一个刚被重投、正要复用原件的任务。

---

## 2. P1 — 应修

### 2.1 重试不分类，且重试不从这里断点续跑
- `ingestion/service.go:232-234`：`Extract` 的任何错误都走 `retryOrFail`，包括重试不可能好的两类（密钥/额度类 4xx、temperature 0 下大概率复现的 JSON 解析失败），纯烧 token。
- `ingestion/service.go:187-264`：重跑从 `parsing` 开始，PPT 要重新 `soffice` 转换、逐页重新 `pdftotext`、重新渲染预览 —— 3 次尝试 = 3 倍文档处理成本。文档 05 §4 写的"从可安全重入的 stage 开始"未实现。
- 关于次数：`MaxAttempts: 3` + `asynq.MaxRetry(3)` 看着是"最多 4 次执行"，实际是 **3 次**（1 次首跑 + 2 次重试）。因为 `j.Attempts` 是进程内旧值，`service.go:313` 判断的是 `(N-1)+1 >= 3`，第 3 次执行时自判失败，`fail()` 返回 nil 让 asynq 当成功。数字自洽，但两个"3"表达的不是同一件事，容易误读。
- 判据：读码确定（执行次数那条是沿代码推算，未实跑，标注为推断）。

### 2.2 45 秒共享预算 + 把超时报成"文件格式问题"
- `ingestion/documents.go:16` 的一个 ctx 覆盖：`soffice` 转换（:25）+ `pdfinfo`（:34）+ 每页一次 `pdftotext`（:39-51）。60 页 PPT = 61 个子进程共享 45 秒。
- 失败后 `service.go:206-208` 统一报 `ingestion.parse_failed`"文件无法解析，请检查格式后重新上传" —— 这是**错误的引导**：用户按提示重传多少次都一样。而且它走 `fail` 不可重试。
- `RenderPreviews`（`documents.go:55-70`）用的是外层无超时的 ctx，每页再起一个 `pdftoppm`，是唯一没被任何预算覆盖的循环。
- 判据：读码确定（结构性问题）；第几页会触发是**推断**，未实测。

### 2.3 预览生成后没有任何出口（纯浪费）
- `service.go:212-217` 每个文件来源都渲染定位页 PNG；`Page.PreviewPath` 只存在于内存（`documents.go:67` 写、全仓无读取），不入库、不出接口、任务结束即删。`relay_extractor.go:28` 的 `PreviewPNGBase64` 声明了但从未赋值 —— 发给模型的只有纯文本。
- 文档 05 §3.4 写"传给 MaaS 的输入仅为候选页及相邻页的文字、页码和临时预览"，预览这半句没实现。
- 两件事二选一：要么真的用起来（作为多模态输入或给审核者看），要么删掉 `RenderPreviews` 及其调用 —— 后者能顺带把 2.2 的 45 秒还回来大半。
- 判据：读码确定。

### 2.4 证据校验：一条对不上就废掉整批
- `service.go:236` + `:321-337`：任一 `text_excerpt` 不是页面文本的字面子串就 `fail` 整个任务。模型少一个空格、统一了全角/半角标点就会触发，用户看到"抽取结果无法校验"，且不可重试。
- 反向的洞：空 `text_excerpt` 会 `strings.Contains(t, "")` 恒真而通过。
- 文档 05 §3.4 要求"删除空候选和完全重复候选"，代码也没做去重。
- 方向：单条证据失败降级为"该候选标记 `uncertain` 或丢弃该证据"，不要让整批陪葬；空摘录直接判无效。

### 2.5 日期：只认 RFC3339 + 从不做时区归一
- `relay_extractor.go:37` 的 `OfficialDueAt *time.Time` 走 `json.Unmarshal`（`:154`），只接受 RFC3339。模型回 `"2026-10-01"` 这种常见写法 → 解码失败 → 这次抽到的**所有**候选一起作废。改成收 `string` 再自己宽松解析可根治。
- 更安静的一条：**时区从不归一化**。`service.go:287` 只在"文本里查不到四位年份"时才把时间重建并挂上课程时区；一旦页面里出现"2026年10月1日"这种带年份的常见写法就直接 `continue`。模型若回 `...T00:00:00Z`，北京课程的 `official_due_at` 就早 8 小时；`ListMine` 用 `NOW()` 比较 timestamptz 是绝对时刻，于是"逾期/即将截止"静默偏移 8 小时。
- `service.go:283` 的年份检测还有个假阴性：它扫的是"原始表达式 + 全部证据页原文"，页面里出现 `2025级`、`CS2023` 这类数字就会命中 `(19|20)\d{2}` 从而**跳过**归一化，模型猜错的年份被原样保留。
- `service.go:287` 无条件套 `now.Year()`：跨年时会算错 —— 1 月 5 日提交的"12月30日"会被定成当年 12 月 30 日，晚了一整年。
- 文档 05 §3.4 要求校验器负责"时间能否标准化为课程时区的 `official_due_at`"，`RawDueExpression` 存了但从不参与解析，这段完全没实现。
- 判据：读码确定。
- **状态：已修复（2026-09-23），时区统一为 UTC+8**（不再读 `course_instances.timezone`）：
  1. **宽松解析**。新增 `duetime.go`：一个整串匹配的模式同时吃下 RFC3339（带 `Z` 或偏移）、`2026-10-01T23:59:00`、`2026-10-01 23:59`、`2026-10-01`、`2026/10/1`、`2026年10月1日 23:59`、以及不带年份的 `10月1日` / `10-01`。解析不出来只把该候选的 `official_due_at` 置空并标记 `uncertain`，不再让一条脏日期废掉整批。
  2. **不做偏移换算**。模型写 `...T00:00:00Z` 时想表达的是当地零点，所以取字面墙上时间落在 UTC+8，而不是换算成 08:00。
  3. **年份规则换成"最近一次"**。`anchorYear` 在 `{去年, 今年, 明年}` 里取离当前最近的那一年：9 月的"12月30日"→今年 12 月，1 月的"12月30日"→刚过去的 12 月，12 月的"1月5日"→明年 1 月，跨年不再差一整年。
  4. **年份是否明确只看表达式**。不再扫证据页原文，页面里的 `2025级`、`CS2023` 不会再让"模型猜的年份"被当成已确认；表达式没写年份时一律重算年份。
  5. **逐条解码**。候选数组先按 `json.RawMessage` 拆开再逐条解码，某条候选字段类型不对（例如日期给成时间戳）只丢这一条并记日志。
  6. 日期字段改成 `json.RawMessage` + `rawText`：模型的 `null`、数字、字符串都能接住，不再触发整体解码失败。
- **未修**：`RawDueExpression` 仍不参与"时间是否为课程时区"的校验（现在没有校验对象了）；改写日期值本身仍不做（只做时区/年份归一）。

### 2.6 `PATCH /assignments/{id}` 会清空没提到的字段
- `cmd/api/main.go:159-172` 复用 `CreateInput`，`assignments/service.go:57-60` 无条件覆盖四个字段。而 `Description` 是非指针 `string`、`OfficialDueAt` 是指针 —— 只发 `{"title":"..."}` 就会把描述清空、官方截止时间置 NULL。PATCH 应该是局部更新（字段用指针 + 只覆盖非 nil）。
- 文档 04 §3 还说这个接口能"取消作业"，而 `status='cancelled'` 这条路径根本没有实现。

### 2.7 新增成员写入的 `joined_at` 是零值时间
- `cmd/api/main.go:78` 构造 `Enrollment` 时没设 `JoinedAt`；`CreateInstance`（`courses/service.go:67`）设了 `time.Now()`，这条路径漏了。
- 列是 `timestamptz NOT NULL` 且无默认值。已查 GORM `callbacks/create.go` 的 `ConvertToCreateValues`：无 `default` tag 的字段一律进 INSERT（只有 `CreatedAt`/`UpdatedAt` 会按名字自动填时间），所以会写入 `0001-01-01`。PG 的 timestamptz 范围接受这个值，**不报错**，属于静默坏数据，还会经 `ListEnrollments` 返回给前端。
- 判据：源码确认。

### 2.8 `markRead` 不是幂等操作
- `notifications/service.go:47-57`：带 `read_at IS NULL` 条件，再根据 `RowsAffected == 0` 返回 404。重复标记已读的通知返回"通知不存在"。前端只要重放一次就报错。应改成"找不到（或不属于该用户）才 404"。

### 2.9 `pdfinfo` 输出解析绑定英文
- `documents.go:76` 的正则要求行首 `Pages:`。poppler 是带 gettext 本地化的，系统语言非英文时输出会变（如"页数:"），`pageCount` 直接返回 `page count missing`，整个解析失败，报出来的还是"文件无法解析"。执行外部命令时应固定 `LC_ALL=C`。

### 2.10 写入路径的错误全部被 `_ =` 吞掉
- `service.go:196, 205, 210, 222, 235, 254, 262` 等，每个 stage/status/source 状态更新都丢弃了错误。
- 具体后果之一：`attempts` 的更新（`:196`）若静默失败，重试预算就失去上界（`retryOrFail` 永远读到旧值），最终变成 P0-4 那种"停住不动"的任务。
- 文档 05 §5 要求"每步记录 job_id、stage、attempt、耗时、错误码和候选数"，一行都没有。目前整个 worker 没有任何日志。

### 2.11 接口契约把 Go 结构体直接当 DTO
- `shared/models.go:107-123` 的 `AssignmentCandidate` 没有 json tag，于是响应里的键是 Go 字段名 `Title`/`Description`/`OfficialDueAt`/`UncertainFields`，和同结构里 `Base` 带 tag 的 `id`/`created_at` 混在一起。前端 `CourseCandidatesView.vue` 里手写的 `Title` 就是这么来的 —— 现在能跑，但改 Go 字段名就会静默破坏前端。
- `Evidence`/`UncertainFields`/`FieldConfidence` 是 `string`，JSON 里是**字符串化的 JSON**，不是数组/对象。前端把 `UncertainFields` 原样渲染，审核者看到的是 `["official_due_at"]` 字面量。
- 更实质的问题：前端从不显示 `Evidence`，也就是说**"人工审核"这一步是盲审** —— 审核者看到标题、描述、日期，看不到原文证据。设计文档 04 §4 说"候选审核页面只返回页码和文字证据"，能力有了但没接到界面上。

### 2.12 没有任何请求体大小限制
- 文本来源走 `ShouldBindJSON`（`cmd/api/main.go:196-199`），上传走 `c.FormFile`（`:209`）—— 全链路没有 `MaxBytesReader`。25MB 的检查发生在 multipart 已经落盘之后（`service.go:116`），且对 JSON 文本来源完全不适用。配合 2.1 的无上限 prompt，一个超大文本粘贴就能把内存、DB 和模型额度一起吃掉。
- 输出侧同样无界：`ListMine`、`ListEnrollments` 都没有 `LIMIT`，`next_cursor` 恒为 `null`。

### 2.13 死代码与只写不读的字段
- `shared/models.go:100` `QueueTaskID` 只写不读（`service.go:182-183`），也没有取消任务的接口，等于白存一列。
- `shared/errors.go:13` `ErrUnauthenticated` 声明后从未使用。
- `documents.go:67` `PreviewPath`、`relay_extractor.go:25-29` `relayPage`/`relayRequest` 类型 —— 死代码。
- `backend/cmd/migrate/` 是空的已跟踪目录。
- `success` 路径把 `source_items.raw_text` 永久留在库里（`service.go:103` 写入，`:292-293` 只用于重跑），没有任何接口能读回它，也没有清理。设计文档的口径是"只保留必要的页码和文字证据"，这里保留的是用户原文全文。

---

## 3. P2 — 文档承诺与实现对照

| 文档承诺 | 位置 | 实现情况 |
| --- | --- | --- |
| `Idempotency-Key` 重复上传返回同一 Source/Job | 文档 04 §5（CORS 白名单里都加了该头） | 未实现，重复上传建两份 source+job |
| `limit` 默认 30 / 最大 100 + 游标分页 | 文档 04 §1 | 未实现，`next_cursor` 恒为 null |
| 投递失败的补偿扫描 | 文档 05 §3.1 | 未实现，但已由 P0-4 的同步落 `failed` 替代：不引入扫描器，投递失败当场变成可见终态 |
| 定时清理器按保留期补偿清理 | 文档 05 §5 | 已实现（见 P0-5）：`SweepStale` 在 worker 启动与每小时按任务状态 + 24 小时保留期清扫 |
| 每步记录 job_id/stage/attempt/耗时/候选数 | 文档 05 §5 | 未实现（见 2.10） |
| 时间标准化到课程时区 | 文档 05 §3.4 | 已实现，但时区统一为 UTC+8，不再按课程配置（见 2.5） |
| 删除空候选与完全重复候选 | 文档 05 §3.4 | 未实现（见 2.4） |
| `GET`/`PATCH /course-instances/{id}` | 文档 04 §2 | 未实现 |
| `GET /course-instances/{id}/assignments` | 文档 04 §3 | 未实现（前端靠 `/me/assignments` 过滤绕过） |
| `/me/assignments` 返回 `is_official_overdue` 与嵌套 `course_instance` | 文档 04 §3 | 未实现，实际是扁平字段 |
| `PATCH /assignments/{id}` 支持取消作业 | 文档 04 §3 | 未实现（见 2.6） |

其余记录项：

- **`identity` 与 `devCORS` 没有环境开关**（`identity/middleware.go:13`、`cmd/api/main.go:29`）。浏览器侧有 CORS 兜着（自定义头会触发预检失败），但任何非浏览器客户端只要带 `X-Canvas-Subject` 就能冒充任意用户、创建课程。另外首次见到某 subject 是"先查后建"（`:26-29`），并发首访会撞唯一键返回 500。
- **`UpdateMyState` 只传个人 DDL 会把完成状态重置为 todo**（`assignments/service.go:106-127`，空值默认 `todo` 并清空 `CompletedAt`）。前端目前两个字段一起发所以没炸，但接口本身是个坑。
- **`PutMeetings` 用字符串比较时间**（`courses/service.go:76`）：`"9:00" >= "10:00"` 因 `'9' > '1'` 判真，合法输入被拒；同时没有格式校验（见 P0-3）。
- **`RejectCandidate` 没有行锁**（`candidates.go:11-33`），而 `AcceptCandidate` 有。并发时"拒绝"会等锁、然后在 accept 提交后把已接受的候选改成 rejected，而作业已经建好了。
- **`GET /course-instances` 不筛 `ci.status='active'`**（`main.go:54`），而 `ListSchedule` 筛了。目前没有归档接口，所以暂时无影响。
- **`http.ListenAndServe` 的返回值被丢弃**（`main.go:290`），也没有 `ReadHeaderTimeout` 等超时。
- **`LoadDotEnv` 不剥离行内注释**（`shared/dotenv.go:23-28`）：`MODEL_API_KEY=sk-xxx # my key` 会把 ` # my key` 一起写进环境变量，症状是一个莫名的 401 —— 而因为 P0-1，你看不到"invalid api key"这句话。
- **通知扇出在候选行锁内串行 INSERT**（`notifications/service.go:34-39`）：几百人的课程会在持锁期间做几百次单行插入。当前规模可接受。
- **前端轮询没有上限**（`CourseSourcesView.vue:31`），配合 P0-4 会一直转下去。

---

## 4. 已验证无问题（含被推翻的怀疑）

- **`GET /course-instances/{id}/assignment-candidates` 的 `Table("... ac") + Joins + Find` 没有 bug。** 我一开始怀疑它会生成 `"assignment_candidates ac"."id"` 导致 SQL 报错，查 GORM `chainable_api.go:58` 的 `tableRegexp` 后确认：`Table()` 会从表名字符串里抽出别名，`Statement.Table` 实际是 `"ac"`，生成的是合法的 `SELECT "ac"."id",...`。**不要去"修"这一处。**
- **PG `time` 列映射到 Go `string` 不会报错。** 查 pgx v5 的 `stdlib/sql.go`：`time` OID(1083) 落到 `default` 分支，按 string 解码，回读到的是 `"09:00:00"`。唯一影响是 PUT `"09:00"` 后回读变成 `"09:00:00"` 的格式不一致（而 `course_meetings` 目前没有任何前端调用方）。
- **`joined_at` 的零值写入**已按上面的方式在 GORM 源码确认（P0/2.7），不是猜测。
- **jsonb 列读回正常**：`source_items.raw_text`/`assignment_candidates.evidence` 等读回无类型问题，`notifications.payload` 用 `json.RawMessage` 接能正常序列化成对象（前端类型对得上）。
- **`NOW()` 比较安全**：`official_due_at` 是 timestamptz，bucket 比较是绝对时刻，不受会话时区影响。
- **模型标签与迁移无列漂移**：`models.go` 的 tag 与三个迁移逐一对照过，无多余列/缺列（唯一例外是 2.7 提到的 `StartTime string` 与 `time` 列的类型对应）。
- **无 SQL 注入点**：路径参数走 `uuid.Parse`，查询条件全部参数绑定，`Order`/`bucket` 分支都是静态串。
- **`AcceptCandidate` 的防重是有效的**：候选行锁 + `assignments.source_candidate_id` 唯一约束 + `review_status` 前置检查，重复接受确实会被挡成 409，不会建出第二份作业。
- **候选替换策略是安全的**：只删 `pending`，已审核结果不会被重跑覆盖。

---

## 5. 修复顺序与进度

1. ~~**P0-1**（让错误可见）~~ —— 已修。
2. ~~**2.5 的时区归一**（happy path 上的静默错误）~~ —— 已修，时区统一为 UTC+8（见 2.5）。**连带产生的决定点**：`course_instances.timezone` 现在没有任何读取方了（ingestion 是唯一消费方，前端只在 `types.ts` 里声明、没渲染），要么按"全国统一 UTC+8"把它从迁移、DTO、前端类型里删掉，要么保留但注明仅供展示。我没有动它。
3. **P0-2**（prompt 封顶）—— 已修；**2.3 删 `RenderPreviews` 未做**，所以 2.2 的 45 秒共享预算、`pdftoppm` 无超时仍在。
4. ~~**P0-3**~~、~~**P0-4/P0-5**~~ —— 已修。
5. 其余 P1/P2 按需。

**P0 修完后的残留风险**（按重要性）：

- **P0-2 的封顶是静默截断**：超过 40000 字时被丢掉的页面只在 worker 日志里可见，审核者不知道"这份课件还有 30 页没看"。真要解决得让 `locate` 按信号强度排序取前 N 页，而不是按文档顺序吃满预算。
- **2.1 未修，所以重试仍不分类**：密钥错、额度不足这类 4xx 现在能在 `error_message` 里看到原因（P0-1），但仍会白跑 2 次重试。
- **2.3 未修，预览仍是纯浪费**：每个文件来源照旧渲染 PNG 再在终态删掉，`relay_extractor` 只发文字。
- **2.5 的"统一 UTC+8"是产品口径，不是代码推导**：如果将来出现非 UTC+8 的课程，`course_instances.timezone` 已经不再生效，需要在 `duetime.go` 里改回按课程取时区。

验证方式：起一次 worker，把 `MODEL_API_KEY` 改成错值跑一个文字来源。**实测结果**（2026-09-23）：UI 上显示

    暂时无法抽取作业候选：tokenrhythm model API returned 401 Unauthorized:
    {"code":"UNAUTHORIZED","message":"未认证或登录已过期","traceId":"trace_9143ab63-..."}

stderr 有 `ingestion retry ...` 与 `ingestion failed ...` 两行。P0-1 的目标（真因 + provider traceId 一路到界面）达成。

---

## 6. 端到端实测记录（2026-09-23，前端驱动）

环境：本机 Postgres + Redis + `go run` 起的 api/worker（均为当时最新代码）+ Vite dev server。靶子课程 `前端完整链路验证`。

通过的路径：

- **文字来源**：`queued → review_ready`，抽出 3 条候选，截止时间全部 `+08:00`。其中「10月15日 23:59」这类没写年份的被锚到 **2026**-10-15（离当前最近的一次）；带年份的原样保留；只给日期不给时刻的落到 00:00 且模型标了 `official_due_at` 存疑。
- **文件来源（PDF）**：上传 → `queued → review_ready`，同样 3 条候选；英文月份无年份（`November 5, 23:59`）也锚到 2026。
- **临时目录**：上传后目录存在（约 94KB），约 2 秒后随任务进终态被删空 —— 「原件只在处理期间保留」成立。
- **接受候选**：退出待审列表，作业出现在课程作业页，同时生成 `assignment_published` 通知（payload 含 title / assignment_id / 截止时间）。
- **拒绝候选**：退出待审列表，不产生作业。

新发现（本次实测才暴露，均已在同日修复）：

- **`max_attempts` 名不副实：配 3 只跑 2 次。** **状态：已修复（2026-09-23）。** 机制已确认：`service.go:214` 用 map 做 `Updates` 时会把 `attempts` 回写进内存里的 `job`（GORM `callbacks/update.go:156-160`，`case reflect.Struct` 下直接 `field.Set(..., stmt.ReflectValue, value)`），于是 `retryOrFail` 的 `j.Attempts+1` 把当前这一次算了两遍、提前一轮判定耗尽；DB 里的 `attempts` 本身没数错，错的是"还剩几次"的判断。修法：`Process` 改为显式 `job.Attempts++` 再写库（不再依赖那个副作用），`retryOrFail` 的判据与日志改用 `j.Attempts`。**实测**（错密钥）：三次执行，日志依次 `attempt=1/3`、`attempt=2/3`、`failed attempts=3`，落库 `attempts=3 / max_attempts=3`；正常路径一次成功时为 `attempts=1`。副作用：耗尽判定变慢，失败任务从 ~50 秒变成 ~1 分 45 秒才进 `failed`（asynq 退避，实测两次间隔约 30 秒、75 秒）。
- **候选接口的字段名是 PascalCase。** **状态：已修复（2026-09-23）。** `AssignmentCandidate`（`models.go:107`）没有 json tag，接口返回 `Title`/`OfficialDueAt`/`ReviewStatus`，而 `ingestion_jobs` 等模型有 tag、返回 snake_case；前端 `CourseCandidatesView.vue` 正好也在读 PascalCase，所以此前能跑。修法：给该模型补齐 json tag，前端那一份 `Candidate` 类型与模板同步改成 snake_case。**实测**：接口返回的键已是 `title`/`official_due_at`/`uncertain_fields` 等，页面 10 条候选的标题、截止、提交方式、待确认字段全部正常渲染。
- **环境依赖缺口（不是后端 bug，未改代码）**：本机没有 `soffice`，PPT/PPTX 来源只会走到 `ingestion.parse_failed`（提示「文件无法解析」）。部署清单/README 应写明这条外部依赖，否则用的人会以为是后端坏了。

同一个坑还有一片没扫（**未改**）：`SourceItem`、`User`、`Course`、`CourseInstance`、`Enrollment`、`Assignment`、`AssignmentUserState` 都没写 json tag，所以写接口返回的对象也是 PascalCase（例如创建来源的响应里 `source` 对象是 `SourceType`/`RawText`/`Status`）。前端目前不读这些字段（全量 grep 过，只有候选页读过 PascalCase），所以没露出来。要不要一次性统一，等你定 —— 那次改动得连前端一起对齐。

另外两处观察，暂不改：重试期间 UI 显示「当前阶段：extracting / 状态：queued」（`retryOrFail` 有意保留失败阶段），且此时 `error_message` 已经显示 —— 语义能解释，用户也正好能提前看到原因；`settleIfExhausted` 在 `fail()` 已落终态后仍把 cause 抛回 asynq，导致任务会多投递一次（`Process` 开头撞到 `status=failed` 直接返回 nil，是无害的空转）。

顺带排除一个怀疑：通知 `payload` 里写 2025-09-25、而作业是 2026-09-25，是发布快照与后续编辑的差异（该作业 `updated_at` 01:30 晚于通知创建 01:26），不是两处数据不一致。

遗留测试数据（都在 `前端完整链路验证` 课程里，可随时删）：3 条来自重复上传的重复候选、1 条 `failed` 的来源与任务。`前端完整链路验证`/`software-testing` 两门课里的早期作业（`实验一` 截止 2026-10-01 **23:00**、`实验二报告` 无截止）是旧代码跑出来的，也可清理。
