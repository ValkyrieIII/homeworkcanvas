<script setup lang="ts">
import { onUnmounted, ref } from "vue";
import { api } from "../api";
type Job = {
    id: string;
    stage: string;
    status: string;
    error_message?: string | null;
};
const props = defineProps<{ courseId: string }>();
const text = ref(""),
    file = ref<File | null>(null),
    message = ref(""),
    error = ref(""),
    sending = ref(false),
    job = ref<Job | null>(null);
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
        if (["review_ready", "completed", "failed"].includes(job.value.status))
            stopPolling();
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
async function submitText() {
    if (!text.value.trim()) return;
    sending.value = true;
    message.value = "";
    error.value = "";
    try {
        const r = await api<any>(
            `/course-instances/${props.courseId}/sources/text`,
            { method: "POST", body: JSON.stringify({ text: text.value }) },
        );
        text.value = "";
        message.value = "已创建文字处理任务。";
        watchJob(r.job);
    } catch (e: any) {
        error.value = e.message;
    } finally {
        sending.value = false;
    }
}
async function submitFile() {
    if (!file.value) return;
    sending.value = true;
    message.value = "";
    error.value = "";
    try {
        const body = new FormData();
        body.append("file", file.value);
        const r = await api<any>(
            `/course-instances/${props.courseId}/sources/file`,
            { method: "POST", body },
        );
        file.value = null;
        message.value = "文件已进入处理队列，原件只会在处理期间临时保留。";
        watchJob(r.job);
    } catch (e: any) {
        error.value = e.message;
    } finally {
        sending.value = false;
    }
}
onUnmounted(stopPolling);
</script>
<template>
    <div class="source-import">
        <h2>来源导入</h2>
        <p>导入只会生成待审核候选；原始文件和页面预览在处理结束后自动删除。</p>
        <p v-if="error" class="error">{{ error }}</p>
        <p v-if="message" class="notice">{{ message }}</p>
        <section class="card">
            <h3>老师文字消息</h3>
            <textarea
                v-model="text"
                placeholder="例如：下周三前提交实验一报告，发到课程平台。"
            /><button :disabled="sending || !text.trim()" @click="submitText">
                {{ sending ? "正在提交…" : "提交文字" }}
            </button>
        </section>
        <section class="card">
            <h3>PDF / PPT / PPTX</h3>
            <p>单个文件不超过 25MB。</p>
            <input
                type="file"
                accept=".pdf,.ppt,.pptx"
                @change="
                    file =
                        ($event.target as HTMLInputElement).files?.[0] ?? null
                "
            /><button :disabled="sending || !file" @click="submitFile">
                {{ sending ? "正在上传…" : "上传并处理" }}
            </button>
        </section>
        <section v-if="job" class="card job-card">
            <h3>处理进度</h3>
            <p>当前阶段：{{ job.stage }}</p>
            <p>状态：{{ job.status }}</p>
            <p v-if="job.error_message" class="error">
                {{ job.error_message }}
            </p>
            <p v-else-if="job.status === 'review_ready'" class="notice">
                候选已准备好，请由课程维护者前往“候选审核”确认。
            </p>
        </section>
    </div>
</template>
