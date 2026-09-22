<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api } from "../api";
import { arrayOrEmpty, type Assignment } from "../types";
import AssignmentCard from "../components/AssignmentCard.vue";
import LoadState from "../components/LoadState.vue";
const route = useRoute(),
    router = useRouter(),
    items = ref<Assignment[]>([]),
    loading = ref(false),
    error = ref("");
const buckets = [
    ["upcoming", "即将截止"],
    ["overdue", "已逾期"],
    ["undated", "待确认"],
    ["all", "全部"],
] as const;
const bucket = () =>
    typeof route.query.bucket === "string" &&
    buckets.some(([key]) => key === route.query.bucket)
        ? route.query.bucket
        : "upcoming";
async function load() {
    loading.value = true;
    error.value = "";
    try {
        items.value = arrayOrEmpty(
            (
                await api<{ data: Assignment[] | null }>(
                    `/me/assignments?bucket=${bucket()}`,
                )
            ).data,
        );
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}
async function toggle(a: Assignment) {
    try {
        await api(`/assignments/${a.id}/my-state`, {
            method: "PUT",
            body: JSON.stringify({
                personal_due_at: a.personal_due_at,
                completion_status:
                    a.completion_status === "completed" ? "todo" : "completed",
            }),
        });
        await load();
    } catch (e: any) {
        error.value = e.message;
    }
}
watch(() => route.query.bucket, load);
onMounted(load);
</script>
<template>
    <section>
        <div class="page-heading">
            <div>
                <p class="eyebrow">我的作业</p>
                <h1>按官方截止时间分组</h1>
            </div>
        </div>
        <div class="tabs">
            <RouterLink
                v-for="[key, label] in buckets"
                :key="key"
                :to="{ path: '/assignments', query: { bucket: key } }"
                :class="{ active: bucket() === key }"
                >{{ label }}</RouterLink
            >
        </div>
        <LoadState
            :loading="loading"
            :error="error"
            :empty="!items.length"
            empty-text="这个分组没有作业。"
        /><AssignmentCard
            v-for="item in items"
            :key="item.id"
            :assignment="item"
            @toggle="toggle"
        />
    </section>
</template>
