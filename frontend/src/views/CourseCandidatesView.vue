<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { api } from "../api";
import { arrayOrEmpty, formatDate, type PageResult } from "../types";
type Candidate = {
    id: string;
    course_name?: string;
    term?: string;
    source_type?: string;
    original_filename?: string | null;
    title: string;
    description: string;
    official_due_at: string | null;
    submission_method: string | null;
    uncertain_fields: string;
    raw_due_expression: string | null;
    evidence: string;
};
type Draft = { title: string; description: string; due: string; submission: string };
const props = defineProps<{ courseId?: string; embedded?: boolean }>(),
    items = ref<Candidate[]>([]),
    nextCursor = ref<string | null>(null),
    drafts = ref<Record<string, Draft>>({}),
    editing = ref<Record<string, boolean>>({}),
    loading = ref(false),
    saving = ref(false),
    error = ref("");
let loadRequest = 0;
function dateInput(value: string | null) {
    if (!value) return "";
    const d = new Date(value);
    const local = new Date(d.getTime() - d.getTimezoneOffset() * 60000);
    return local.toISOString().slice(0, 16);
}
function evidence(value: string) {
    try { return JSON.parse(value) as { page_number: number; text_excerpt: string }[]; }
    catch { return []; }
}
function uncertain(value: string) {
    try { return (JSON.parse(value) as string[]).join("、") || "无"; }
    catch { return value; }
}
function draftFor(c: Candidate): Draft {
    return {
        title: c.title,
        description: c.description,
        due: dateInput(c.official_due_at),
        submission: c.submission_method ?? "",
    };
}
function cancelEdit(c: Candidate) {
    drafts.value[c.id] = draftFor(c);
    editing.value[c.id] = false;
}
async function load(cursor?: string) {
    const request = ++loadRequest;
    const requestedCourse = props.courseId;
    loading.value = true;
    error.value = "";
    if (!cursor) {
        items.value = [];
        nextCursor.value = null;
    }
    try {
        const params = new URLSearchParams({ limit: "30" });
        if (requestedCourse) params.set("course_instance_id", requestedCourse);
        if (cursor) params.set("cursor", cursor);
        const page = await api<PageResult<Candidate>>(`/admin/assignment-candidates?${params}`);
        if (request !== loadRequest || props.courseId !== requestedCourse) return;
        const incoming = arrayOrEmpty(page.data);
        items.value = cursor ? [...items.value, ...incoming] : incoming;
        nextCursor.value = page.next_cursor;
        drafts.value = cursor
            ? { ...drafts.value, ...Object.fromEntries(incoming.map(c => [c.id, draftFor(c)])) }
            : Object.fromEntries(incoming.map(c => [c.id, draftFor(c)]));
    } catch (e: any) {
        if (request === loadRequest) error.value = e.message;
    } finally {
        if (request === loadRequest) loading.value = false;
    }
}
async function review(id: string, accepted: boolean) {
    if (saving.value) return;
    const draft = drafts.value[id];
    if (accepted && !draft?.title.trim()) {
        error.value = "作业标题不能为空";
        return;
    }
    saving.value = true;
    error.value = "";
    try {
        const body = accepted ? {
            edited: true,
            title: draft.title.trim(),
            description: draft.description,
            official_due_at: draft.due ? new Date(draft.due).toISOString() : null,
            submission_method: draft.submission.trim() || null,
        } : {};
        await api(
            `/assignment-candidates/${id}/${accepted ? "accept" : "reject"}`,
            { method: "POST", body: JSON.stringify(body) },
        );
        await load();
    } catch (e: any) {
        error.value = e.message;
    } finally {
        saving.value = false;
    }
}
watch(() => props.courseId, () => { void load(); });
onMounted(() => { void load(); });
</script>
<template>
    <div>
        <template v-if="!embedded">
            <p class="eyebrow">管理员端</p>
            <h1>待审核候选</h1>
        </template>
        <p>核对来源证据后，修改、发布或拒绝候选。</p>
        <p v-if="loading">正在加载…</p>
        <p v-if="error" class="error">{{ error }}</p>
        <p v-if="!loading && !error && !items.length" class="empty">
            没有待审核候选。
        </p>
        <article v-for="c in items" :key="c.id" class="card">
            <p v-if="c.course_name"><strong>{{ c.course_name }}</strong> · {{ c.term }}</p>
            <p>来源：{{ c.original_filename || (c.source_type === "text" ? "老师文字通知" : "附件") }}</p>
            <template v-if="editing[c.id] && drafts[c.id]">
                <h3>编辑候选作业</h3>
                <label :for="`title-${c.id}`">标题</label>
                <input :id="`title-${c.id}`" v-model="drafts[c.id].title" class="edit-input" />
                <label :for="`description-${c.id}`">作业说明</label>
                <textarea :id="`description-${c.id}`" v-model="drafts[c.id].description" />
                <label :for="`due-${c.id}`">官方截止时间（留空表示待确认）</label>
                <input :id="`due-${c.id}`" v-model="drafts[c.id].due" class="edit-input" type="datetime-local" />
                <label :for="`submission-${c.id}`">提交方式</label>
                <input :id="`submission-${c.id}`" v-model="drafts[c.id].submission" class="edit-input" />
            </template>
            <template v-else>
                <h3>{{ c.title }}</h3>
                <p>{{ c.description }}</p>
                <p>官方截止：{{ formatDate(c.official_due_at) }}</p>
                <p v-if="c.submission_method">提交方式：{{ c.submission_method }}</p>
            </template>
            <p v-if="c.raw_due_expression">原文截止描述：{{ c.raw_due_expression }}</p>
            <p class="uncertain">待确认字段：{{ uncertain(c.uncertain_fields) }}</p>
            <details><summary>查看来源证据</summary>
                <p v-for="(item, index) in evidence(c.evidence)" :key="index">
                    {{ item.page_number === 0 ? "老师文字" : `附件第 ${item.page_number} 页` }}：{{ item.text_excerpt }}
                </p>
            </details>
            <button v-if="!editing[c.id]" class="secondary" @click="editing[c.id] = true">编辑</button>
            <button v-else class="secondary" @click="cancelEdit(c)">取消编辑</button>
            <button :disabled="saving" @click="review(c.id, true)">确认发布</button
            ><button :disabled="saving" class="secondary" @click="review(c.id, false)">
                拒绝
            </button>
        </article>
        <button v-if="nextCursor" class="secondary" :disabled="loading" @click="load(nextCursor)">加载更多候选</button>
    </div>
</template>
