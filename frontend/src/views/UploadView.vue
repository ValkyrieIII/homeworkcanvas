<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { api } from "../api";
import { formatDate } from "../types";

type Task = {
    id: string;
    original_filename: string | null;
    source_type: string;
    status: string;
    stage: string;
    error_message: string | null;
    created_at: string;
};
const file = ref<File | null>(null);
const fileInput = ref<HTMLInputElement | null>(null);
const note = ref("");
const busy = ref(false);
const dragging = ref(false);
const error = ref("");
const notice = ref("");
const tasks = ref<Task[]>([]);
const loading = ref(false);
const loaded = ref(false);
const refreshError = ref("");
const statusLabels: Record<string, string> = {
    queued: "排队中", processing: "处理中", review_ready: "提取完成 · 待确认",
    completed: "已完成", failed: "处理失败",
};
const stageLabels: Record<string, string> = {
    queued: "等待处理", parsing: "读取材料", locating: "定位作业信息",
    extracting: "提取作业", validating: "检查提取结果",
    review_ready: "提取完成", completed: "处理完成", failed: "处理已停止",
};
const isActive = (task: Task) => ["queued", "processing"].includes(task.status);
const activeCount = computed(() => tasks.value.filter(isActive).length);
const orderedTasks = computed(() => [...tasks.value].sort((a, b) =>
    Number(isActive(b)) - Number(isActive(a)) || Date.parse(b.created_at) - Date.parse(a.created_at),
));
let timer: number | undefined;
let disposed = false;
const controller = new AbortController();

// 页面只保留一条轮询链；离开后取消请求，避免旧响应重新启动轮询。
async function refresh() {
    if (loading.value || disposed) return;
    window.clearTimeout(timer);
    loading.value = true;
    try {
        const result = await api<{ data: Task[] | null }>("/me/sources", { signal: controller.signal });
        if (disposed) return;
        tasks.value = result.data ?? [];
        loaded.value = true;
        refreshError.value = "";
    } catch (e) {
        if (!disposed) refreshError.value = e instanceof Error ? e.message : "读取任务失败";
    } finally {
        loading.value = false;
        if (!disposed) timer = window.setTimeout(refresh, 5000);
    }
}
function pick(files: FileList | null) {
    if (busy.value) return;
    const selected = files?.[0];
    if (!selected) return;
    error.value = "";
    notice.value = "";
    if (selected.size === 0 || selected.size > 25 * 1024 * 1024) {
        file.value = null;
        error.value = "请选择非空且不超过 25 MB 的文件。";
        return;
    }
    file.value = selected;
}
async function upload() {
    if (!file.value || busy.value) return;
    busy.value = true;
    error.value = "";
    notice.value = "";
    try {
        const form = new FormData();
        form.append("file", file.value);
        if (note.value.trim()) form.append("text", note.value.trim());
        const result = await api<{ reused?: boolean }>("/agent/inputs", {
            method: "POST", body: form,
        });
        if (disposed) return;
        notice.value = result.reused ? "这份材料已有处理记录，可在下方查看。" : "上传成功，可在下方查看任务状态。";
        file.value = null;
        note.value = "";
        if (fileInput.value) fileInput.value.value = "";
        await refresh();
    } catch (e) {
        if (!disposed) error.value = e instanceof Error ? e.message : "上传失败，请重试";
    } finally {
        busy.value = false;
    }
}
onMounted(refresh);
onUnmounted(() => {
    disposed = true;
    window.clearTimeout(timer);
    controller.abort();
});
</script>

<template>
    <section>
        <h1>上传材料</h1>
        <p class="upload-intro">上传课件或作业说明，自动整理其中的作业信息。</p>
        <form class="card upload-form" @submit.prevent="upload">
            <label class="dropzone" :class="{ dragging }"
                @dragover.prevent="dragging = true" @dragleave.prevent="dragging = false"
                @drop.prevent="dragging = false; pick(($event as DragEvent).dataTransfer?.files ?? null)">
                <input ref="fileInput" type="file" :disabled="busy"
                    accept=".pdf,.ppt,.pptx,.docx,.doc,image/*"
                    @change="pick(($event.target as HTMLInputElement).files)" />
                <strong>{{ file ? file.name : "拖文件到这里，或点击选择" }}</strong>
                <small>PDF、PPT、Word 或图片 · 单个文件不超过 25 MB</small>
            </label>
            <label class="form-field">补充说明（可选）
                <textarea v-model="note" :disabled="busy" rows="2" placeholder="例如：这是第三次课的课件"></textarea>
            </label>
            <p v-if="error" class="error" role="alert">{{ error }}</p>
            <p v-if="notice" class="notice" role="status">{{ notice }}</p>
            <button :disabled="!file || busy">{{ busy ? "上传中…" : "上传并整理" }}</button>
        </form>
        <section class="upload-tasks" aria-labelledby="tasks-heading">
            <div class="tasks-heading">
                <div><h2 id="tasks-heading">上传任务</h2><p>{{ activeCount ? `${activeCount} 个任务正在处理` : "查看材料的处理状态" }} · 每 5 秒更新</p></div>
                <button class="secondary" :disabled="loading" @click="refresh">{{ loading ? "更新中…" : "刷新" }}</button>
            </div>
            <p v-if="refreshError" class="error" role="alert">状态更新失败：{{ refreshError }}。将自动重试，已有记录可能不是最新状态。</p>
            <p v-if="!loaded && loading">正在读取任务…</p>
            <p v-else-if="loaded && !tasks.length" class="empty">还没有上传记录，上传后会在这里显示。</p>
            <article v-for="task in orderedTasks" :key="task.id" class="card upload-task">
                <div class="task-title"><h3>{{ task.original_filename || (task.source_type === "agent" ? "Agent 投递" : "文字材料") }}</h3>
                    <span class="task-status" :class="{ 'task-status--active': isActive(task), 'task-status--failed': task.status === 'failed' }">{{ statusLabels[task.status] || "状态待更新" }}</span>
                </div>
                <p>提交于 {{ formatDate(task.created_at) }}</p>
                <p v-if="isActive(task)">{{ task.status === 'queued' ? "等待处理，请稍候" : (stageLabels[task.stage] || "正在整理材料") }}</p>
                <p v-if="task.error_message" class="error">{{ task.error_message }}</p>
                <p v-if="task.status === 'failed'">可重新选择原文件上传。</p>
                <p v-if="task.status === 'review_ready'">材料已整理为候选，尚未加入 DDL。</p>
            </article>
        </section>
    </section>
</template>
