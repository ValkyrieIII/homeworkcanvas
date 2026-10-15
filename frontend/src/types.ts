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
  term_year: number;
  term_season: "春" | "秋";
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
export const formatDate = (value: string | null) =>
  value
    ? new Intl.DateTimeFormat("zh-CN", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(new Date(value))
    : "待确认";
export const arrayOrEmpty = <T>(value: T[] | null | undefined): T[] =>
  value ?? [];
// 学期在数据层是「年份 + 季节」两个字段，展示串由它拼出来，不再靠字符串排序。
const AUTUMN_START_MONTH = 7;
export type TermValue = { year: number; season: "春" | "秋" };
export const formatTerm = (value: TermValue) => `${value.year}${value.season}季学期`;
// 下拉选项的 value 是 "2026-秋" 这样的传输格式，提交前解析回结构化字段。
export const parseTermValue = (value: string): { term_year: number; term_season: "春" | "秋" } => {
  const [year, season] = value.split("-");
  return { term_year: Number(year), term_season: season as "春" | "秋" };
};
export function termOptions(
  existing: { term_year: number; term_season: "春" | "秋" }[],
  now = new Date(),
): { value: string; label: string }[] {
  // 学期序号：偶数=春季、奇数=秋季，除以 2 取整就是年份。
  const current =
    now.getFullYear() * 2 + (now.getMonth() + 1 >= AUTUMN_START_MONTH ? 1 : 0);
  const candidates: TermValue[] = existing.map((item) => ({
    year: item.term_year,
    season: item.term_season,
  }));
  for (let offset = 0; offset < 4; offset++) {
    const index = current + offset;
    candidates.push({ year: Math.floor(index / 2), season: index % 2 ? "秋" : "春" });
  }
  const seen = new Set<string>();
  return candidates
    .filter((item) => {
      const key = `${item.year}-${item.season}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .sort((a, b) => b.year - a.year || (a.season === b.season ? 0 : a.season === "秋" ? -1 : 1))
    .map((item) => ({ value: `${item.year}-${item.season}`, label: formatTerm(item) }));
}
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
