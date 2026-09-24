export type Assignment = {
  id: string;
  course_instance_id: string;
  course_name: string;
  term: string;
  title: string;
  description: string;
  official_due_at: string | null;
  submission_method: string | null;
  personal_due_at: string | null;
  effective_due_at: string | null;
  completion_status: "todo" | "completed";
};
export type PageResult<T> = { data: T[] | null; next_cursor: string | null };
export type AdminAssignment = {
  id: string;
  course_instance_id: string;
  course_name: string;
  term: string;
  title: string;
  description: string;
  official_due_at: string | null;
  submission_method: string | null;
  status: "published" | "cancelled";
  source_type: string | null;
  original_filename: string | null;
  evidence: string | null;
};
export type SourceIssue = {
  id: string;
  course_instance_id: string;
  course_name: string;
  original_filename: string | null;
  submitter_name: string;
  status: string;
  stage: string;
  error_message: string | null;
  created_at: string;
};
export type Course = {
  id: string;
  course_name: string;
  term: string;
  teacher_name: string | null;
  timezone: string;
  status: "active" | "archived";
  joined: boolean;
};
export type CurrentUser = {
  id: string;
  display_name: string;
  role: "admin" | "user";
};
export type Meeting = {
  course_instance_id: string;
  course_name: string;
  term: string;
  weekday: number;
  start_time: string;
  end_time: string;
  location: string | null;
};
export const formatDate = (value: string | null) =>
  value
    ? new Intl.DateTimeFormat("zh-CN", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(new Date(value))
    : "待确认";
export const arrayOrEmpty = <T>(value: T[] | null | undefined): T[] =>
  value ?? [];
export type Notification = {
  id: string;
  kind: "assignment_published" | "assignment_updated" | "assignment_retracted";
  payload: {
    assignment_id: string;
    course_instance_id: string;
    title: string;
    official_due_at: string | null;
  };
  read_at: string | null;
  created_at: string;
};
