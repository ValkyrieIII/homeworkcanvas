# 数据库表设计

> 参考：[领域模型](01-domain-model.md) · [总架构](../architecture.md)  
> 数据库：PostgreSQL；迁移：goose；访问层：GORM

## 1. 通用约定

- 主键使用 UUID；`created_at`、`updated_at` 使用 `timestamptz`。
- 时间点一律使用 `timestamptz`；`CourseInstance.timezone` 保存课程默认时区。
- 枚举首版使用受 `CHECK` 约束的文本字段，避免数据库枚举的演进成本。
- 业务删除优先改状态；不对 CourseInstance、Enrollment、Assignment 做物理删除。
- 文件二进制不进入 PostgreSQL；临时路径只在任务执行期间使用，清理后置空。

## 2. 表与字段

### users

| 字段 | 类型/约束 | 说明 |
| --- | --- | --- |
| id | uuid PK | Canvas 内部标识 |
| external_subject | text UNIQUE NOT NULL | UniSSO 稳定用户标识 |
| display_name | text NOT NULL | 展示名 |
| email | text NULL | 可用时同步，不作为登录依据 |
| status | text NOT NULL | `active / disabled` |
| created_at, updated_at | timestamptz | 审计时间 |

### courses 与 course_instances

`courses`：`id`、`code NULL`、`name`、时间字段。`code` 在非空时唯一。

`course_instances`：

| 字段 | 类型/约束 | 说明 |
| --- | --- | --- |
| id | uuid PK | 课程实例 |
| course_id | uuid FK courses | 课程定义 |
| term | text NOT NULL | 如 `2026-fall` |
| teacher_name | text NULL | 展示信息 |
| timezone | text NOT NULL | 默认 `Asia/Shanghai` |
| status | text NOT NULL | `active / archived` |
| created_by | uuid FK users | 创建维护者 |

不以“课程名 + 教师 + 学期”建立强唯一约束，避免真实场景中的同名平行课程被错误合并。

### enrollments 与 course_meetings

`enrollments`：`id`、`user_id` FK、`course_instance_id` FK、`role`、`status`、`joined_at`、时间字段；`UNIQUE(user_id, course_instance_id)`。

`course_meetings`：`id`、`course_instance_id` FK、`weekday smallint`、`start_time time`、`end_time time`、`location text NULL`、`valid_from date NULL`、`valid_to date NULL`、时间字段。

约束：`weekday BETWEEN 1 AND 7`、`start_time < end_time`、有效日期范围合法。

### assignments 与 assignment_user_states

`assignments`：

| 字段 | 类型/约束 | 说明 |
| --- | --- | --- |
| id | uuid PK | 共享作业 |
| course_instance_id | uuid FK | 所属课程实例 |
| title | text NOT NULL | 标题 |
| description | text NOT NULL DEFAULT '' | 作业说明 |
| official_due_at | timestamptz NULL | 官方截止时间 |
| submission_method | text NULL | 提交方式 |
| status | text NOT NULL | `published / cancelled` |
| source_candidate_id | uuid NULL UNIQUE | 被接受的候选；手工创建时为空 |
| created_by | uuid FK users | 创建/确认人 |

`assignment_user_states`：`id`、`assignment_id` FK、`user_id` FK、`personal_due_at timestamptz NULL`、`completion_status`、`completed_at timestamptz NULL`、时间字段；`UNIQUE(assignment_id, user_id)`。

约束：`completion_status IN ('todo', 'completed')`；状态为 `completed` 时 `completed_at` 非空，反之为空。

### source_items、ingestion_jobs 与 assignment_candidates

`source_items`：

| 字段 | 类型/约束 | 说明 |
| --- | --- | --- |
| id | uuid PK | 来源标识 |
| course_instance_id | uuid FK | 上传时确定归属 |
| submitted_by | uuid FK users | 提交者 |
| source_type | text NOT NULL | `text / file` |
| raw_text | text NULL | 仅文字消息保存原文 |
| original_filename | text NULL | 文件元数据 |
| mime_type | text NULL | 文件元数据 |
| byte_size | bigint NULL | 文件元数据 |
| checksum | text NULL | 内容校验值 |
| status | text NOT NULL | `queued / processing / review_ready / failed / completed` |

`ingestion_jobs`：`id`、`source_item_id` FK、`stage`、`status`、`attempts`、`max_attempts`、`queue_task_id`、`temporary_locator NULL`、`error_code NULL`、`error_message NULL`、`started_at NULL`、`finished_at NULL`、时间字段。

`assignment_candidates`：`id`、`source_item_id` FK、`ordinal smallint`、`title`、`description`、`official_due_at NULL`、`raw_due_expression NULL`、`submission_method NULL`、`evidence jsonb`、`field_confidence jsonb`、`uncertain_fields jsonb`、`review_status`、`reviewed_by NULL`、`reviewed_at NULL`、`review_note NULL`、时间字段。

约束：

- `UNIQUE(source_item_id, ordinal)`，同一任务重试以同一序号覆盖候选；
- `review_status IN ('pending', 'accepted', 'rejected')`；
- 候选被接受后，由 `assignments.source_candidate_id` 反查对应 Assignment，避免双方重复保存外键。

`evidence` 为数组，每项含 `page_number`、`text_excerpt`；不写入页面图片或原文件。

## 3. 外键策略

- Course、CourseInstance、Assignment：`RESTRICT`，防止删除破坏历史。
- Enrollment、CourseMeeting、AssignmentUserState：跟随父对象的逻辑状态，不依赖级联物理删除。
- SourceItem、Job、Candidate：保留审计记录；原始临时文件的清理不影响外键。

## 4. 索引

| 表 | 索引 | 支持查询 |
| --- | --- | --- |
| enrollments | `(user_id, status, course_instance_id)` | 用户可访问课程及课表 |
| course_meetings | `(course_instance_id, weekday)` | 按课程生成课表 |
| assignments | `(course_instance_id, official_due_at, id)` | 单课程作业排序 |
| assignment_user_states | `(user_id, assignment_id)` | 用户个人状态连接 |
| assignment_user_states | `(user_id, personal_due_at, assignment_id)` | 个人 DDL 排序辅助 |
| source_items | `(course_instance_id, created_at DESC)` | 课程来源列表 |
| ingestion_jobs | `(status, created_at)` | 队列补偿扫描与任务页 |
| assignment_candidates | `(source_item_id, review_status, ordinal)` | 审核列表 |

跨课程作业列表由 `enrollments → assignments LEFT JOIN assignment_user_states` 生成，以 `COALESCE(personal_due_at, official_due_at), assignment.id` 排序。首版先用上述索引观察；没有真实慢查询前不增加表达式索引。

## 5. 事务边界

- 创建 CourseInstance：实例、创建者的 maintainer Enrollment 在同一事务提交。
- 接受 Candidate：锁定候选，创建 Assignment 并写入 `assignments.source_candidate_id`，再将候选标为 `accepted`，同一事务完成。
- 修改个人状态：只允许当前用户的 `(assignment_id, user_id)` upsert。
- 创建 SourceItem 与 IngestionJob：同一事务落库；提交后再投递 Asynq。投递失败时保留 `queued` 任务，由补偿扫描重新投递。
