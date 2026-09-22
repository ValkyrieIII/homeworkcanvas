<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { api } from "../api";
import { arrayOrEmpty, type Meeting } from "../types";
import LoadState from "../components/LoadState.vue";
const props = defineProps<{ courseId: string }>(),
    items = ref<Meeting[]>([]),
    loading = ref(false),
    error = ref("");
async function load() {
    loading.value = true;
    try {
        items.value = arrayOrEmpty(
            (await api<{ data: Meeting[] | null }>("/me/schedule")).data,
        ).filter((x) => x.course_instance_id === props.courseId);
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}
watch(() => props.courseId, load);
onMounted(load);
</script>
<template>
    <div>
        <h2>课程课表</h2>
        <LoadState
            :loading="loading"
            :error="error"
            :empty="!items.length"
            empty-text="该课程暂无上课安排。"
        />
        <article
            v-for="m in items"
            :key="`${m.weekday}-${m.start_time}`"
            class="card"
        >
            <h3>周{{ m.weekday }} {{ m.start_time }}–{{ m.end_time }}</h3>
            <p>{{ m.location || "地点待确认" }}</p>
        </article>
    </div>
</template>
