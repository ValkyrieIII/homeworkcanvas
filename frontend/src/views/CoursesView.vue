<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "../api";
import { arrayOrEmpty, type Course } from "../types";
import LoadState from "../components/LoadState.vue";
const items = ref<Course[]>([]),
    loading = ref(false),
    error = ref("");
async function load() {
    loading.value = true;
    try {
        items.value = arrayOrEmpty(
            (await api<{ data: Course[] | null }>("/course-instances")).data,
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
        <p class="eyebrow">课程</p>
        <h1>我已加入的课程</h1>
        <LoadState
            :loading="loading"
            :error="error"
            :empty="!items.length"
            empty-text="暂未加入课程。"
        /><RouterLink
            v-for="course in items"
            :key="course.id"
            :to="`/courses/${course.id}/assignments`"
            class="card course-link"
            ><small
                >{{ course.term }} ·
                {{
                    course.my_role === "maintainer" ? "课程维护者" : "学生"
                }}</small
            >
            <h3>{{ course.course_name }}</h3>
            <p>{{ course.teacher_name || "教师待确认" }}</p></RouterLink
        >
    </section>
</template>
