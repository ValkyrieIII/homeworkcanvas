<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { api } from "../../api";
import { arrayOrEmpty, formatDate, type Course } from "../../types";
import AdminAssignmentForm from "../../components/AdminAssignmentForm.vue";
import SelectMenu from "../../components/SelectMenu.vue";

type Meeting = { id?: string; weekday: number; start_time: string; end_time: string; location: string | null };
type Assignment = {
    id: string;
    title: string;
    description: string;
    official_due_at: string | null;
    submission_method: string | null;
    status: "published" | "cancelled";
};
type Member = { id: string; display_name: string; user_id: string; status: "active" | "left" };
type Source = { id: string; submitter_name: string; original_filename: string | null; source_type: string; status: string; stage: string; error_message: string | null };
const props = defineProps<{ courseId: string }>();
const course = ref<Course | null>(null);
const meetings = ref<Meeting[]>([]);
const assignments = ref<Assignment[]>([]);
const members = ref<Member[]>([]);
const sources = ref<Source[]>([]);
const error = ref("");
const message = ref("");
const busy = ref(false);
const name = ref("");
const term = ref("");
const teacher = ref("");
const status = ref<"active" | "archived">("active");
const editingAssignment = ref<string | null>(null);
const retractTarget = ref<string | null>(null);
const sourceStatus: Record<string, string> = {
    queued: "排队中", processing: "处理中", review_ready: "待审核",
    completed: "已完成", failed: "处理失败",
};
const statusOptions = [
    { value: "active", label: "开放" },
    { value: "archived", label: "归档" },
];
const weekdayOptions = Array.from({ length: 7 }, (_, index) => ({ value: index + 1, label: `周${index + 1}` }));

function setStatus(value: string | number) {
    if (value === "active" || value === "archived") status.value = value;
}

async function load() {
    error.value = "";
    try {
        const [courseResult, meetingResult, assignmentResult, memberResult, sourceResult] = await Promise.all([
            api<{ data: Course[] | null }>("/admin/course-instances"),
            api<{ data: Meeting[] | null }>(`/course-instances/${props.courseId}/meetings`),
            api<{ data: Assignment[] | null }>(`/course-instances/${props.courseId}/assignments`),
            api<{ data: Member[] | null }>(`/course-instances/${props.courseId}/enrollments`),
            api<{ data: Source[] | null }>(`/course-instances/${props.courseId}/sources`),
        ]);
        course.value = arrayOrEmpty(courseResult.data).find(item => item.id === props.courseId) ?? null;
        meetings.value = arrayOrEmpty(meetingResult.data).map(item => ({ ...item, start_time: item.start_time.slice(0, 5), end_time: item.end_time.slice(0, 5) }));
        assignments.value = arrayOrEmpty(assignmentResult.data);
        members.value = arrayOrEmpty(memberResult.data);
        sources.value = arrayOrEmpty(sourceResult.data);
        if (course.value) {
            name.value = course.value.course_name;
            term.value = course.value.term;
            teacher.value = course.value.teacher_name ?? "";
            status.value = course.value.status;
        }
    } catch (e: any) {
        error.value = e.message;
    }
}
async function saveCourse() {
    if (!course.value || busy.value) return;
    busy.value = true; error.value = ""; message.value = "";
    try {
        await api(`/course-instances/${props.courseId}`, {
            method: "PATCH",
            body: JSON.stringify({ course_name: name.value.trim(), term: term.value.trim(), teacher_name: teacher.value.trim() || null, timezone: course.value.timezone, status: status.value }),
        });
        message.value = "课程资料已保存";
        await load();
    } catch (e: any) { error.value = e.message; }
    finally { busy.value = false; }
}
function addMeeting() {
    meetings.value.push({ weekday: 1, start_time: "08:00", end_time: "09:40", location: null });
}
async function saveMeetings() {
    if (busy.value) return;
    busy.value = true; error.value = ""; message.value = "";
    try {
        await api(`/course-instances/${props.courseId}/meetings`, {
            method: "PUT", body: JSON.stringify(meetings.value.map(({ weekday, start_time, end_time, location }) => ({ weekday, start_time, end_time, location }))),
        });
        message.value = "课表已保存";
        await load();
    } catch (e: any) { error.value = e.message; }
    finally { busy.value = false; }
}
async function assignmentSaved() {
    message.value = editingAssignment.value ? "共享作业已更新" : "共享作业已发布";
    editingAssignment.value = null;
    await load();
}
async function retractAssignment(item: Assignment) {
    if (busy.value || item.status !== "published") return;
    busy.value = true;
    error.value = "";
    message.value = "";
    try {
        await api(`/assignments/${item.id}/retract`, { method: "POST" });
        if (editingAssignment.value === item.id) editingAssignment.value = null;
        retractTarget.value = null;
        message.value = "作业已撤回，用户端将不再显示，并已通知课程成员";
        await load();
    } catch (e: any) {
        error.value = e.message;
    } finally {
        busy.value = false;
    }
}
watch(() => props.courseId, load);
onMounted(load);
</script>

<template>
    <section>
        <RouterLink to="/admin/courses" class="back">← 返回全站课程</RouterLink>
        <p class="eyebrow">管理员端 · 课程</p>
        <h1>{{ course?.course_name || "课程详情" }}</h1>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <p v-if="message" class="notice" role="status">{{ message }}</p>
        <template v-if="course">
            <form class="card admin-form" @submit.prevent="saveCourse">
                <h2>课程资料</h2>
                <p>课程名称属于课程定义；同一课程在其他学期的实例会同步显示新名称。</p>
                <div class="form-grid">
                    <label>课程名称<input v-model="name" required /></label>
                    <label>学期<input v-model="term" required /></label>
                    <label>任课教师<input v-model="teacher" /></label>
                    <div class="form-field">状态
                        <SelectMenu
                            :model-value="status"
                            :options="statusOptions"
                            label="状态"
                            @update:model-value="setStatus"
                        />
                    </div>
                </div>
                <button :disabled="busy || !name.trim() || !term.trim()">保存课程资料</button>
            </form>
            <section class="card admin-form">
                <h2>上课安排</h2>
                <p v-if="!meetings.length">暂无上课安排。</p>
                <div v-for="(item, index) in meetings" :key="index" class="meeting-row">
                    <div class="form-field">星期
                        <SelectMenu
                            :model-value="item.weekday"
                            :options="weekdayOptions"
                            label="星期"
                            @update:model-value="(value) => (item.weekday = Number(value))"
                        />
                    </div>
                    <label>开始<input v-model="item.start_time" type="time" required /></label>
                    <label>结束<input v-model="item.end_time" type="time" required /></label>
                    <label>地点<input v-model="item.location" /></label>
                    <button class="secondary" type="button" @click="meetings.splice(index, 1)">移除</button>
                </div>
                <button class="secondary" type="button" @click="addMeeting">添加时间</button>
                <button :disabled="busy" type="button" @click="saveMeetings">保存课表</button>
            </section>
            <section class="card admin-form">
                <h2>共享作业</h2>
                <p>管理员核对后发布；截止时间来自老师通知，个人计划由学生自己维护。</p>
                <article v-for="item in assignments" :key="item.id" class="admin-list-item">
                    <strong>{{ item.title }}</strong>
                    <span>{{ item.status === "cancelled" ? "已撤回" : `官方截止：${formatDate(item.official_due_at)}` }}</span>
                    <template v-if="item.status === 'published'">
                        <button class="secondary" type="button" @click="editingAssignment = item.id">修改</button>
                        <button class="secondary" type="button" @click="retractTarget = item.id">撤回</button>
                        <div v-if="retractTarget === item.id" class="retract-confirm">
                            <p>撤回后用户端不再显示这项作业，历史记录会保留。</p>
                            <button :disabled="busy" type="button" @click="retractAssignment(item)">确认撤回</button>
                            <button class="secondary" type="button" @click="retractTarget = null">取消</button>
                        </div>
                    </template>
                </article>
                <p v-if="!assignments.length">暂无共享作业。</p>
            </section>
            <AdminAssignmentForm
                :key="editingAssignment || 'new'"
                :course-id="courseId"
                :assignment="assignments.find(item => item.id === editingAssignment)"
                @saved="assignmentSaved"
                @cancel="editingAssignment = null"
            />
            <section class="card admin-form">
                <h2>来源与处理进度</h2>
                <p v-if="!sources.length">暂无用户提交的来源。</p>
                <div v-for="source in sources" :key="source.id" class="admin-list-item">
                    <strong>{{ source.original_filename || "文字通知" }}</strong>
                    <span>提交者：{{ source.submitter_name }}</span>
                    <span>{{ sourceStatus[source.status] || source.status }}</span>
                    <span v-if="source.error_message" class="error">{{ source.error_message }}</span>
                </div>
                <RouterLink to="/admin?tab=pending">核对待审核候选 →</RouterLink>
            </section>
            <section class="card admin-form">
                <h2>课程成员</h2>
                <p v-if="!members.length">暂无参与成员。</p>
                <div v-for="member in members" :key="member.id" class="admin-list-item">
                    <strong>{{ member.display_name }}</strong>
                    <span>{{ member.status === "active" ? "已加入" : "已退出" }}</span>
                </div>
            </section>
        </template>
    </section>
</template>
