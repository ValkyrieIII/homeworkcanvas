<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { api } from "../api";
import { arrayOrEmpty, formatDate } from "../types";
type Candidate = {
    id: string;
    title: string;
    description: string;
    official_due_at: string | null;
    submission_method: string | null;
    uncertain_fields: string;
};
const props = defineProps<{ courseId: string }>(),
    items = ref<Candidate[]>([]),
    loading = ref(false),
    error = ref("");
async function load() {
    loading.value = true;
    error.value = "";
    try {
        items.value = arrayOrEmpty(
            (
                await api<{ data: Candidate[] | null }>(
                    `/course-instances/${props.courseId}/assignment-candidates?status=pending`,
                )
            ).data,
        );
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}
async function review(id: string, accepted: boolean) {
    try {
        await api(
            `/assignment-candidates/${id}/${accepted ? "accept" : "reject"}`,
            { method: "POST", body: JSON.stringify({}) },
        );
        await load();
    } catch (e: any) {
        error.value = e.message;
    }
}
watch(() => props.courseId, load);
onMounted(load);
</script>
<template>
    <div>
        <h2>候选审核</h2>
        <p v-if="loading">正在加载…</p>
        <p v-if="error" class="error">{{ error }}</p>
        <p v-if="!loading && !error && !items.length" class="empty">
            没有待审核候选。
        </p>
        <article v-for="c in items" :key="c.id" class="card">
            <h3>{{ c.title }}</h3>
            <p>{{ c.description }}</p>
            <p>官方截止：{{ formatDate(c.official_due_at) }}</p>
            <p v-if="c.submission_method">提交方式：{{ c.submission_method }}</p>
            <p class="uncertain">待确认字段：{{ c.uncertain_fields }}</p>
            <button @click="review(c.id, true)">确认发布</button
            ><button class="secondary" @click="review(c.id, false)">
                拒绝
            </button>
        </article>
    </div>
</template>
