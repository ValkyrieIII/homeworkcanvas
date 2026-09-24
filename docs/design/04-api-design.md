# API 设计

> 参考：[后端模块与接口设计](03-backend-modules.md) · [领域模型](01-domain-model.md)  
> 风格：JSON REST；当前本地请求头身份适配器仅供开发，目标为可信登录上下文；个人操作不接收客户端传入的 user_id。
> 状态：作业优先视图所需的跨课程管理查询已实现。

## 1. 通用约定

- 路径前缀：`/api/v1`。
- 成功响应直接返回资源或 `{ "data": ... }`；列表返回 `{ "data": [], "next_cursor": null }`。
- 错误响应统一为：

```json
{
  "error": {
    "code": "courses.not_admin",
    "message": "需要管理员身份",
    "details": {}
  }
}
```

- `400` 参数错误；`401` 未认证；`403` 已认证但无权访问；`404` 资源不存在或对当前用户不可见；`409` 状态冲突；`422` 文件或业务内容无法处理；`429` 触发限流；`500` 未分类服务错误。
- 日期时间使用 RFC 3339，例如 `2026-09-21T23:00:00+08:00`。
- `limit` 默认 30、最大 100；游标为服务端生成的不透明字符串。

## 2. 课程与选课

| 方法与路径 | 权限 | 请求重点 | 响应重点 |
| --- | --- | --- | --- |
| `GET /course-instances` | 当前用户 | 无 | 可选课程目录及本人是否加入 |
| `GET /me/course-instances` | 当前用户 | 无 | 当前用户已加入的课程实例 |
| `PUT /course-instances/{id}/enrollment` | 当前用户 | 无 | 自行加入课程 |
| `DELETE /course-instances/{id}/enrollment` | 当前用户 | 无 | 自行退出课程 |
| `GET /admin/course-instances` | 全站管理员 | 无 | 全站课程列表，供维护资料使用 |
| `POST /course-instances` | 全站管理员 | course 名称/代码、term、教师、timezone | 新课程实例；不自动加入课程 |
| `PATCH /course-instances/{id}` | 全站管理员 | 名称、教师、状态 | 更新后实例 |
| `GET /course-instances/{id}/enrollments` | 全站管理员 | 无 | 课程成员列表；不代替用户选课 |
| `GET /course-instances/{id}/meetings` | 全站管理员 | 无 | 课程上课安排 |
| `PUT /course-instances/{id}/meetings` | 全站管理员 | 完整 Meeting 列表 | 替换后的列表 |
| `GET /me/schedule` | 已登录 | `from`、`to` | 由 Enrollment + Meeting 动态生成的课表项 |

`POST /course-instances` 中，如果 Course 不存在则同时创建 Course；若调用方传入已有 `course_id`，则复用它。全站管理员类型不来自 Enrollment；旧课程维护者不会自动升级。

## 3. 作业与个人状态

### 我的作业

`GET /me/assignments?bucket=upcoming&course_instance_id=...`

`bucket` 为：

- `overdue`：官方 `official_due_at < now` 且未取消；
- `upcoming`：有官方截止时间且未官方逾期；
- `undated`：官方截止时间为空；
- `all`：默认，供非分组场景使用。

`course_instance_id` 可选，仅缩小当前用户已加入课程的作业范围。当前个人作业接口返回完整列表，尚未实现上文通用分页约定。

每项返回：

```json
{
  "id": "uuid",
  "course_instance_id": "uuid",
  "course_name": "软件测试",
  "term": "2026-fall",
  "title": "实验报告",
  "description": "...",
  "official_due_at": "2026-10-01T23:59:00+08:00",
  "personal_due_at": "2026-09-29T20:00:00+08:00",
  "effective_due_at": "2026-09-29T20:00:00+08:00",
  "completion_status": "todo"
}
```

排序键固定为 `effective_due_at ASC NULLS LAST, assignment_id ASC`。`bucket` 由后端计算，前端不能自行以本机字符串判断逾期。

### 跨课程管理、课程共享作业与个人 DDL

作业是管理员端的默认视图。需提供跨全站课程的共享作业查询，返回课程名称、作业状态、官方截止时间和来源线索，并支持按状态与课程缩小范围；不能依赖逐门请求课程作业后在前端拼接。

| 方法与路径 | 权限 | 说明 |
| --- | --- | --- |
| `GET /admin/assignments` | 全站管理员 | `status=published/cancelled`、`course_instance_id`、`limit`、`cursor` | 跨课程共享作业列表，返回课程和来源线索 |
| `GET /course-instances/{id}/assignments` | 全站管理员 | 无 | 单课程共享作业，供课程详情辅助查看 |
| `POST /course-instances/{id}/assignments` | 全站管理员 | 标题、说明、官方截止、提交方式 | 手工创建共享作业 |
| `PATCH /assignments/{id}` | 全站管理员 | 完整共享字段 | 修改已发布作业 |
| `POST /assignments/{id}/retract` | 全站管理员 | 无 | 撤回已发布作业，保留记录并通知成员 |
| `PUT /assignments/{id}/my-state` | active member | 写入个人 DDL、完成状态；仅当前用户 |

个人状态请求：

```json
{
  "personal_due_at": "2026-09-29T20:00:00+08:00",
  "completion_status": "todo"
}
```

传 `personal_due_at: null` 表示恢复使用官方截止时间。无权更新其他用户状态。

## 4. 来源、任务与候选

| 方法与路径 | 权限 | 说明 |
| --- | --- | --- |
| `POST /course-instances/{id}/sources/text` | active member | 提交文字消息；返回 SourceItem 和 Job |
| `POST /course-instances/{id}/sources/file` | active member | `multipart/form-data` 上传 PDF/PPT/PPTX，可同时传 `text` 表示同一条老师通知；新任务返回 `202 Accepted`，相同文件与文字返回 `200 OK` 与原有 Source/Job，响应含 `reused` |
| `GET /me/sources` | 当前用户 | 无 | 本人的来源处理记录 |
| `GET /course-instances/{id}/sources` | 全站管理员 | 无 | 该课程来源与处理进度 |
| `GET /ingestion-jobs/{id}` | 来源提交者或全站管理员 | 无 | 查看状态、阶段和错误 |
| `GET /admin/assignment-candidates` | 全站管理员 | `course_instance_id`、`limit`、`cursor` | 跨课程待审核候选列表 |
| `GET /admin/source-issues` | 全站管理员 | `course_instance_id`、`limit`、`cursor` | 处理失败或超过 15 分钟未完成的来源 |
| `GET /course-instances/{id}/assignment-candidates` | 全站管理员 | `status` | 单课程候选列表 |
| `POST /assignment-candidates/{id}/accept` | 全站管理员 | 审核页传 `edited: true` 和完整的标题、说明、截止时间、提交方式；允许将可空字段清空，原子更新候选并发布 |
| `POST /assignment-candidates/{id}/reject` | 全站管理员 | 可选审核说明 |

文字来源请求：

```json
{ "text": "下周三前提交实验一报告，发到课程平台。" }
```

候选审核页面只返回页码和文字证据，不返回已删除的原文件或页面图片。

## 5. 幂等与并发

- 文件上传按 `(课程实例、提交者、文件内容 SHA-256)` 去重；已有任务未失败时返回原有 Source/Job，失败后允许重新上传。不同用户和不同课程互不复用。当前文字来源与显式 `Idempotency-Key` 请求头尚未实现幂等。
- `accept` 使用候选行锁；重复接受返回 `409 assignments.candidate_not_pending`，不会创建第二份作业。
- `PUT /my-state` 是幂等 upsert；最后一次成功写入为准。
- 所有修改接口返回最新资源版本；首版不引入通用乐观锁，只有 Candidate 审核使用事务锁定。
