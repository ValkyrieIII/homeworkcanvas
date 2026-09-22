<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "../api";
import { arrayOrEmpty, type Meeting } from "../types";
import LoadState from "../components/LoadState.vue";
const items = ref<Meeting[]>([]),
    loading = ref(false),
    error = ref("");
async function load() {
    loading.value = true;
    try {
        items.value = arrayOrEmpty(
            (await api<{ data: Meeting[] | null }>("/me/schedule")).data,
        );
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}
onMounted(load);
</script>
<template>
    <section>
        <p class="eyebrow">我的课表</p>
        <h1>本周课程安排</h1>
        <LoadState
            :loading="loading"
            :error="error"
            :empty="!items.length"
            empty-text="暂无上课安排。"
        />
        <article
            v-for="m in items"
            :key="`${m.course_instance_id}-${m.weekday}-${m.start_time}`"
            class="card"
        >
            <small>{{ m.course_name }} · {{ m.term }}</small>
            <h3>周{{ m.weekday }} {{ m.start_time }}–{{ m.end_time }}</h3>
            <p>{{ m.location || "地点待确认" }}</p>
        </article>
    </section>
</template>
