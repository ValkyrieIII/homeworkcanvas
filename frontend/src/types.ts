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
export type Course = {
  id: string;
  course_name: string;
  term: string;
  teacher_name: string | null;
  timezone: string;
  my_role: "maintainer" | "student";
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
  kind: "assignment_published";
  payload: {
    assignment_id: string;
    course_instance_id: string;
    title: string;
    official_due_at: string | null;
  };
  read_at: string | null;
  created_at: string;
};
