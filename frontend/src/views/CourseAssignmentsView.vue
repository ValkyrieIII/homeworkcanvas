<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { api } from "../api";
import { arrayOrEmpty, type Assignment } from "../types";
import AssignmentCard from "../components/AssignmentCard.vue";
import LoadState from "../components/LoadState.vue";
import { useAssignmentState } from "../composables/useAssignmentState";
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
                    `/me/assignments?bucket=all&course_instance_id=${props.courseId}`,
                )
            ).data,
        );
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}
const { actionError, toggle, savePlan } = useAssignmentState(load);
watch(() => props.courseId, load);
onMounted(load);
</script>
<template>
    <div>
        <h2>共享作业</h2>
        <p v-if="actionError" class="error" role="alert">{{ actionError }}</p>
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
            @plan="savePlan"
        />
    </div>
</template>
