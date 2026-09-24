<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from "vue";
import { api } from "../api";
type Job = {
    id: string;
    stage: string;
    status: string;
    error_message?: string | null;
};
type Source = {
    id: string;
    course_instance_id: string;
    source_type: string;
    original_filename: string | null;
    status: string;
    stage: string;
    error_message: string | null;
};
const props = defineProps<{ courseId: string }>();
const statusLabel: Record<string, string> = {
    queued: "排队中", processing: "处理中", review_ready: "待管理员审核",
    completed: "已完成", failed: "处理失败",
};
const text = ref(""),
    file = ref<File | null>(null),
    fileInput = ref<HTMLInputElement | null>(null),
    message = ref(""),
    error = ref(""),
    sending = ref(false),
    job = ref<Job | null>(null),
    sources = ref<Source[]>([]);
async function loadSources() {
    try {
        const result = await api<{ data: Source[] | null }>("/me/sources");
        sources.value = (result.data ?? []).filter(item => item.course_instance_id === props.courseId);
    } catch (e: any) { error.value = e.message; }
}
let timer: number | undefined;
function stopPolling() {
    if (timer) {
        window.clearInterval(timer);
        timer = undefined;
    }
}
async function refreshJob() {
    if (!job.value) return;
    try {
        job.value = await api<Job>(`/ingestion-jobs/${job.value.id}`);
        if (["review_ready", "completed", "failed"].includes(job.value.status)) {
            stopPolling();
            await loadSources();
        }
    } catch (e: any) {
        error.value = e.message;
        stopPolling();
    }
}
function watchJob(value: Job) {
    job.value = value;
    stopPolling();
    timer = window.setInterval(refreshJob, 1800);
    void refreshJob();
}
async function submit() {
    if (!text.value.trim() && !file.value) return;
    sending.value = true;
    message.value = "";
    error.value = "";
    try {
        let r: { job: Job; reused?: boolean };
        if (file.value) {
            const body = new FormData();
            body.append("file", file.value);
            body.append("text", text.value.trim());
            r = await api<{ job: Job; reused: boolean }>(
                `/course-instances/${props.courseId}/sources/file`,
                { method: "POST", body },
            );
        } else {
            r = await api<{ job: Job }>(
                `/course-instances/${props.courseId}/sources/text`,
                { method: "POST", body: JSON.stringify({ text: text.value.trim() }) },
            );
        }
        text.value = "";
        file.value = null;
        if (fileInput.value) fileInput.value.value = "";
        message.value = r.reused
            ? "这组文字和附件此前已导入，已显示原有处理结果。"
            : "已提交一条导入任务，文字和附件会一起交给解析流程。";
        watchJob(r.job);
        await loadSources();
    } catch (e: any) {
        error.value = e.message;
    } finally {
        sending.value = false;
    }
}
onUnmounted(stopPolling);
onMounted(loadSources);
watch(() => props.courseId, loadSources);
</script>
<template>
    <div>
        <h2>来源导入</h2>
        <p>导入只会生成待审核候选；原始文件和页面预览在处理结束后自动删除。</p>
        <p v-if="error" class="error">{{ error }}</p>
        <p v-if="message" class="notice">{{ message }}</p>
        <section class="card admin-form">
            <h3>导入老师通知</h3>
            <p>可填写文字、选择附件，或两者一起提交。附件支持 PDF / PPT / PPTX，单个不超过 25MB。</p>
            <label for="source-message">通知文字</label>
            <textarea
                id="source-message"
                v-model="text"
                placeholder="例如：下周三前提交实验一报告，发到课程平台。"
            />
            <label for="source-file">附件（可选）</label>
            <input
                id="source-file"
                ref="fileInput"
                type="file"
                accept=".pdf,.ppt,.pptx"
                @change="
                    file =
                        ($event.target as HTMLInputElement).files?.[0] ?? null
                "
            /><button :disabled="sending || (!file && !text.trim())" @click="submit">
                {{ sending ? "正在提交…" : "导入并生成候选" }}
            </button>
        </section>
        <section v-if="job" class="card">
            <h3>处理进度</h3>
            <p>处理状态：{{ statusLabel[job.status] || job.status }}</p>
            <p v-if="job.error_message" class="error">
                {{ job.error_message }}
            </p>
            <p v-else-if="job.status === 'review_ready'" class="notice">
                候选已准备好，等待管理员核对发布。
            </p>
        </section>
        <section class="card">
            <h3>我的提交记录</h3>
            <p v-if="!sources.length">暂无提交记录。</p>
            <p v-for="source in sources" :key="source.id">
                {{ source.original_filename || "文字通知" }} · {{ statusLabel[source.status] || source.status }}
                <span v-if="source.error_message" class="error">{{ source.error_message }}</span>
            </p>
        </section>
    </div>
</template>
