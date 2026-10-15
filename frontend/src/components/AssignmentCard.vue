<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import type { Assignment } from "../types";
import { formatDate } from "../types";

const props = defineProps<{ assignment: Assignment }>();
const emit = defineEmits<{
    toggle: [assignment: Assignment];
    plan: [assignment: Assignment, value: string | null];
}>();
const editingPlan = ref(false);
const planValue = ref("");
function editPlan() {
    const value = props.assignment.personal_due_at;
    if (value) {
        const date = new Date(value);
        planValue.value = new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
    } else {
        planValue.value = "";
    }
    editingPlan.value = true;
}
function savePlan() {
    emit("plan", props.assignment, planValue.value ? new Date(planValue.value).toISOString() : null);
    editingPlan.value = false;
}

const now = ref(Date.now());
let clock: ReturnType<typeof setInterval> | undefined;
onMounted(() => {
    clock = setInterval(() => { now.value = Date.now(); }, 60_000);
});
onUnmounted(() => clearInterval(clock));

// 官方逾期只看官方截止；临近提醒则优先使用个人计划时间。
const urgency = computed(() => {
    const a = props.assignment;
    if (a.completion_status === "completed") {
        return { tone: "completed", label: "已完成" };
    }
    const official = a.official_due_at && Date.parse(a.official_due_at);
    if (official && official < now.value) {
        return { tone: "overdue", label: "官方已逾期" };
    }
    const due = a.effective_due_at && Date.parse(a.effective_due_at);
    if (!due || Number.isNaN(due)) {
        return { tone: "undated", label: "截止待确认" };
    }
    if (due < now.value) {
        return { tone: "urgent", label: "个人计划已过" };
    }
    if (due - now.value <= 24 * 60 * 60 * 1000) {
        return { tone: "urgent", label: "24 小时内" };
    }
    if (due - now.value <= 3 * 24 * 60 * 60 * 1000) {
        return { tone: "soon", label: "3 天内" };
    }
    return { tone: "normal", label: "待办" };
});

// 提交方式可以是说明文字，也可以包含网址；只有 HTTP(S) 才生成外链。
const submissionUrlText = computed(() =>
    props.assignment.submission_method?.match(/https?:\/\/[^\s<>"'，。；、）)]+/i)?.[0] ?? null,
);
const submissionLink = computed(() => {
    if (!submissionUrlText.value) return null;
    try {
        const url = new URL(submissionUrlText.value);
        return url.protocol === "http:" || url.protocol === "https:" ? url.href : null;
    } catch {
        return null;
    }
});
const submissionNote = computed(() => {
    const method = props.assignment.submission_method?.trim() ?? "";
    if (!submissionLink.value) return method;
    return method.replace(submissionUrlText.value ?? "", "").replace(/^[：:，,\s]+|[：:，,\s]+$/g, "");
});
</script>

<template>
    <article class="card assignment-card" :class="`assignment-card--${urgency.tone}`">
        <div class="assignment-card__top">
            <small>{{ assignment.course_name }} · {{ assignment.term }}</small>
            <span class="assignment-card__badge">{{ urgency.label }}</span>
        </div>
        <h3>{{ assignment.title }}</h3>
        <p v-if="assignment.description.trim()" class="assignment-card__description">
            {{ assignment.description }}
        </p>
        <div class="assignment-card__details">
            <p>官方截止：{{ formatDate(assignment.official_due_at) }}</p>
            <p v-if="assignment.personal_due_at" class="personal-due">
                个人计划：{{ formatDate(assignment.personal_due_at) }}
            </p>
            <p v-if="submissionNote">提交方式：{{ submissionNote }}</p>
        </div>
        <div class="assignment-card__actions">
            <a v-if="submissionLink" :href="submissionLink" target="_blank" rel="noopener noreferrer">
                打开提交链接 ↗
            </a>
            <button class="secondary" @click="$emit('toggle', assignment)">
                {{ assignment.completion_status === "completed" ? "恢复待办" : "标记完成" }}
            </button>
            <button class="secondary" @click="editPlan">设置个人计划</button>
        </div>
        <form v-if="editingPlan" class="plan-editor" @submit.prevent="savePlan">
            <label>个人计划截止时间<input v-model="planValue" type="datetime-local" /></label>
            <button type="submit">保存计划</button>
            <button class="secondary" type="button" @click="editingPlan = false">取消</button>
            <p>留空后沿用官方截止时间。</p>
        </form>
    </article>
</template>
