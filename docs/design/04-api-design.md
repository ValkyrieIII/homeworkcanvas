# API 设计

> 参考：[后端模块与接口设计](03-backend-modules.md) · [领域模型](01-domain-model.md)  
> 风格：JSON REST；身份从 UniSSO 上下文获得，不接收客户端传入的 user_id。

## 1. 通用约定

- 路径前缀：`/api/v1`。
- 成功响应直接返回资源或 `{ "data": ... }`；列表返回 `{ "data": [], "next_cursor": null }`。
- 错误响应统一为：

```json
{
  "error": {
    "code": "courses.not_maintainer",
    "message": "你没有维护该课程的权限",
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
| `GET /course-instances` | 已登录 | `term`、`status` 可选 | 当前用户已加入的课程实例 |
| `POST /course-instances` | 已登录 | course 名称/代码、term、教师、timezone | 新实例；创建者自动成为 maintainer |
| `GET /course-instances/{id}` | active member | 无 | 课程、成员数、Meeting 摘要 |
| `PATCH /course-instances/{id}` | maintainer | 名称、教师、状态 | 更新后实例 |
| `POST /course-instances/{id}/enrollments` | maintainer | `user_id`、`role` | 新 Enrollment |
| `PATCH /course-instances/{id}/enrollments/{enrollmentId}` | maintainer | role/status | 更新后的 Enrollment |
| `PUT /course-instances/{id}/meetings` | maintainer | 完整 Meeting 列表 | 替换后的列表 |
| `GET /me/schedule` | 已登录 | `from`、`to` | 由 Enrollment + Meeting 动态生成的课表项 |

`POST /course-instances` 中，如果 Course 不存在则同时创建 Course；若调用方传入已有 `course_id`，则复用它。课程选择和人员搜索依赖主站可提供的用户目录能力，不在 Canvas 重建。

## 3. 作业与个人状态

### 我的作业

`GET /me/assignments?bucket=upcoming&limit=30&cursor=...`

`bucket` 为：

- `overdue`：官方 `official_due_at < now` 且未取消；
- `upcoming`：有官方截止时间且未官方逾期；
- `undated`：官方截止时间为空；
- `all`：默认，供非分组场景使用。

每项返回：

```json
{
  "id": "uuid",
  "course_instance": { "id": "uuid", "name": "软件测试", "term": "2026-fall" },
  "title": "实验报告",
  "description": "...",
  "official_due_at": "2026-10-01T23:59:00+08:00",
  "personal_due_at": "2026-09-29T20:00:00+08:00",
  "effective_due_at": "2026-09-29T20:00:00+08:00",
  "is_official_overdue": false,
  "completion_status": "todo"
}
```

排序键固定为 `effective_due_at ASC NULLS LAST, assignment_id ASC`。`bucket` 由后端计算，前端不能自行以本机字符串判断逾期。

### 课程共享作业与个人 DDL

| 方法与路径 | 权限 | 说明 |
| --- | --- | --- |
| `GET /course-instances/{id}/assignments` | active member | 单课程共享作业及当前用户状态 |
| `POST /course-instances/{id}/assignments` | maintainer | 手工创建共享作业 |
| `PATCH /assignments/{id}` | maintainer | 更新官方字段或取消作业 |
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
| `GET /ingestion-jobs/{id}` | 来源提交者或 maintainer | 查看状态、阶段、错误与候选数量 |
| `GET /course-instances/{id}/assignment-candidates` | maintainer | 按 `pending / accepted / rejected` 查询候选 |
| `POST /assignment-candidates/{id}/accept` | maintainer | 审核页传 `edited: true` 和完整的标题、说明、截止时间、提交方式；允许将可空字段清空，原子更新候选并发布 |
| `POST /assignment-candidates/{id}/reject` | maintainer | 可选审核说明 |

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
