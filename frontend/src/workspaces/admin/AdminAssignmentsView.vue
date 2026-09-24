<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api } from "../../api";
import AdminAssignmentForm from "../../components/AdminAssignmentForm.vue";
import LoadState from "../../components/LoadState.vue";
import SelectMenu from "../../components/SelectMenu.vue";
import { arrayOrEmpty, formatDate, type AdminAssignment, type Course, type PageResult, type SourceIssue } from "../../types";
import CourseCandidatesView from "../../views/CourseCandidatesView.vue";

const route = useRoute();
const router = useRouter();
const courses = ref<Course[]>([]);
const assignments = ref<AdminAssignment[]>([]);
const nextCursor = ref<string | null>(null);
const issues = ref<SourceIssue[]>([]);
const issueCursor = ref<string | null>(null);
const loading = ref(false);
const issueLoading = ref(false);
const busy = ref(false);
const error = ref("");
const issueError = ref("");
const message = ref("");
const showCreate = ref(false);
const editingId = ref<string | null>(null);
const retractTarget = ref<string | null>(null);
let assignmentRequest = 0;
let issueRequest = 0;
const tab = computed(() => route.query.tab === "published" || route.query.tab === "cancelled" ? route.query.tab : "pending");
const courseId = computed(() => typeof route.query.course === "string" ? route.query.course : "");
const courseOptions = computed(() => [
  { value: "", label: "全部课程" },
  ...courses.value.map((course) => ({ value: course.id, label: `${course.course_name} · ${course.term}` })),
]);

function setQuery(nextTab: string, nextCourse = courseId.value) {
  void router.push({ path: "/admin", query: { tab: nextTab === "pending" ? undefined : nextTab, course: nextCourse || undefined } });
}
async function loadCourses() {
  try {
    courses.value = arrayOrEmpty((await api<{ data: Course[] | null }>("/admin/course-instances")).data);
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "读取课程失败";
  }
}
async function loadAssignments(cursor?: string) {
  if (tab.value === "pending") return;
  const request = ++assignmentRequest;
  const requestedTab = tab.value;
  const requestedCourse = courseId.value;
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams({ status: requestedTab, limit: "30" });
    if (requestedCourse) params.set("course_instance_id", requestedCourse);
    if (cursor) params.set("cursor", cursor);
    const page = await api<PageResult<AdminAssignment>>(`/admin/assignments?${params}`);
    if (request !== assignmentRequest || tab.value !== requestedTab || courseId.value !== requestedCourse) return;
    assignments.value = cursor ? [...assignments.value, ...arrayOrEmpty(page.data)] : arrayOrEmpty(page.data);
    nextCursor.value = page.next_cursor;
  } catch (cause) {
    if (request === assignmentRequest) error.value = cause instanceof Error ? cause.message : "读取作业失败";
  } finally {
    if (request === assignmentRequest) loading.value = false;
  }
}
async function loadIssues(cursor?: string) {
  const request = ++issueRequest;
  const requestedCourse = courseId.value;
  issueLoading.value = true;
  issueError.value = "";
  try {
    const params = new URLSearchParams({ limit: "10" });
    if (requestedCourse) params.set("course_instance_id", requestedCourse);
    if (cursor) params.set("cursor", cursor);
    const page = await api<PageResult<SourceIssue>>(`/admin/source-issues?${params}`);
    if (request !== issueRequest || courseId.value !== requestedCourse) return;
    issues.value = cursor ? [...issues.value, ...arrayOrEmpty(page.data)] : arrayOrEmpty(page.data);
    issueCursor.value = page.next_cursor;
  } catch (cause) {
    if (request === issueRequest) issueError.value = cause instanceof Error ? cause.message : "读取来源处理反馈失败";
  } finally {
    if (request === issueRequest) issueLoading.value = false;
  }
}
async function saved() {
  message.value = editingId.value ? "共享作业已更新" : "共享作业已发布";
  editingId.value = null;
  showCreate.value = false;
  if (tab.value !== "published") {
    setQuery("published");
  } else {
    await loadAssignments();
  }
}
async function retract(item: AdminAssignment) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    await api(`/assignments/${item.id}/retract`, { method: "POST" });
    retractTarget.value = null;
    editingId.value = null;
    message.value = "作业已撤回，课程成员已收到通知";
    await loadAssignments();
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "撤回失败";
  } finally {
    busy.value = false;
  }
}
function sourceExcerpt(value: string | null) {
  if (!value) return "";
  try {
    const evidence = JSON.parse(value) as { page_number?: number; text_excerpt?: string }[];
    const first = evidence[0];
    if (!first?.text_excerpt) return "";
    return first.page_number ? `附件第 ${first.page_number} 页：${first.text_excerpt}` : `老师文字：${first.text_excerpt}`;
  } catch {
    return "";
  }
}
watch(() => [tab.value, courseId.value], () => {
  assignmentRequest++;
  loading.value = false;
  assignments.value = [];
  nextCursor.value = null;
  editingId.value = null;
  retractTarget.value = null;
  void loadAssignments();
});
watch(courseId, () => {
  issues.value = [];
  issueCursor.value = null;
  void loadIssues();
});
onMounted(() => { void loadCourses(); void loadAssignments(); void loadIssues(); });
</script>

<template>
  <section>
    <p class="eyebrow">管理员端</p>
    <h1>作业工作台</h1>
    <p>先处理全站作业，再按课程查看资料和来源。</p>
    <p v-if="message" class="notice" role="status">{{ message }}</p>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <div class="workbench-toolbar">
      <div class="filter-label">课程
        <SelectMenu
          :model-value="courseId"
          :options="courseOptions"
          label="课程"
          @update:model-value="(value) => setQuery(tab, String(value))"
        />
      </div>
      <button type="button" @click="showCreate = !showCreate">{{ showCreate ? "收起发布表单" : "手工发布作业" }}</button>
    </div>
    <AdminAssignmentForm v-if="showCreate" :courses="courses" @saved="saved" />
    <nav class="tabs" aria-label="作业工作台视图">
      <button v-for="item in [{ key: 'pending', label: '待审核' }, { key: 'published', label: '已发布' }, { key: 'cancelled', label: '已撤回' }]"
        :key="item.key" class="tab-button" :class="{ active: tab === item.key }" @click="setQuery(item.key)">{{ item.label }}</button>
    </nav>

    <CourseCandidatesView v-if="tab === 'pending'" :course-id="courseId || undefined" embedded />
    <template v-else>
      <LoadState :loading="loading" :error="''" :empty="!assignments.length && !error" :empty-text="tab === 'published' ? '暂无已发布作业。' : '暂无已撤回作业。'" />
      <article v-for="item in assignments" :key="item.id" class="card assignment-card admin-assignment-card" :class="item.status === 'cancelled' ? 'assignment-card--undated' : ''">
        <div class="assignment-card__top">
          <RouterLink :to="`/admin/courses/${item.course_instance_id}`">{{ item.course_name }} · {{ item.term }}</RouterLink>
          <span class="assignment-card__badge">{{ item.status === 'cancelled' ? '已撤回' : '已发布' }}</span>
        </div>
        <h3>{{ item.title }}</h3>
        <p v-if="item.description">{{ item.description }}</p>
        <p>官方截止：{{ formatDate(item.official_due_at) }}</p>
        <p v-if="item.submission_method">提交方式：{{ item.submission_method }}</p>
        <p v-if="item.source_type">来源：{{ item.original_filename || '老师文字通知' }}</p>
        <p v-if="sourceExcerpt(item.evidence)" class="source-note">证据：{{ sourceExcerpt(item.evidence) }}</p>
        <div v-if="item.status === 'published'" class="assignment-card__actions">
          <button class="secondary" type="button" @click="editingId = editingId === item.id ? null : item.id">修改</button>
          <button class="secondary" type="button" @click="retractTarget = item.id">撤回</button>
        </div>
        <div v-if="retractTarget === item.id" class="retract-confirm">
          <p>撤回后用户端不再显示这项作业，历史记录会保留。</p>
          <button :disabled="busy" type="button" @click="retract(item)">确认撤回</button>
          <button class="secondary" type="button" @click="retractTarget = null">取消</button>
        </div>
        <AdminAssignmentForm v-if="editingId === item.id" :key="item.id" :assignment="item" @saved="saved" @cancel="editingId = null" />
      </article>
      <button v-if="nextCursor" class="secondary" :disabled="loading" @click="loadAssignments(nextCursor)">加载更多作业</button>
    </template>

    <section class="source-issues">
      <h2>来源处理反馈</h2>
      <p>显示处理失败及超过 15 分钟仍未完成的来源。</p>
      <p v-if="issueLoading && !issues.length">正在加载处理反馈…</p>
      <p v-if="issueError" class="error" role="alert">{{ issueError }}</p>
      <p v-else-if="!issueLoading && !issues.length" class="empty">暂无需要处理的来源异常。</p>
      <article v-for="issue in issues" :key="issue.id" class="card source-issue">
        <RouterLink :to="`/admin/courses/${issue.course_instance_id}`">{{ issue.course_name }}</RouterLink>
        <strong>{{ issue.original_filename || '老师文字通知' }}</strong>
        <span>{{ issue.status === 'failed' ? '处理失败' : '处理积压' }} · 提交者：{{ issue.submitter_name }}</span>
        <p v-if="issue.error_message" class="error">{{ issue.error_message }}</p>
      </article>
      <button v-if="issueCursor" class="secondary" :disabled="issueLoading" @click="loadIssues(issueCursor)">加载更多反馈</button>
    </section>
  </section>
</template>
