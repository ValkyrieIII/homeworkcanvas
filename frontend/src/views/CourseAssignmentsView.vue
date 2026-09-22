<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { api } from "../api";
import { arrayOrEmpty, type Assignment } from "../types";
import AssignmentCard from "../components/AssignmentCard.vue";
import LoadState from "../components/LoadState.vue";
const props = defineProps<{ courseId: string }>(),
    items = ref<Assignment[]>([]),
    loading = ref(false),
    error = ref("");
async function load() {
    loading.value = true;
    try {
        items.value = arrayOrEmpty(
            (
                await api<{ data: Assignment[] | null }>(
                    "/me/assignments?bucket=all",
                )
            ).data,
        ).filter((x) => x.course_instance_id === props.courseId);
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}
async function toggle(a: Assignment) {
    await api(`/assignments/${a.id}/my-state`, {
        method: "PUT",
        body: JSON.stringify({
            personal_due_at: a.personal_due_at,
            completion_status:
                a.completion_status === "completed" ? "todo" : "completed",
        }),
    });
    await load();
}
watch(() => props.courseId, load);
onMounted(load);
</script>
<template>
    <div>
        <h2>共享作业</h2>
        <LoadState
            :loading="loading"
            :error="error"
            :empty="!items.length"
            empty-text="该课程暂无作业。"
        /><AssignmentCard
            v-for="a in items"
            :key="a.id"
            :assignment="a"
            @toggle="toggle"
        />
    </div>
</template>
