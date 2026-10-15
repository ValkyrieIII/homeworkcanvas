<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { api } from "../api";
import { arrayOrEmpty, type Course } from "../types";
const props = defineProps<{ courseId: string }>(),
    course = ref<Course | null>(null),
    loaded = ref(false),
    error = ref("");
async function load() {
    loaded.value = false;
    course.value = null;
    error.value = "";
    try {
        const r = await api<{ data: Course[] | null }>("/me/course-instances");
        course.value = arrayOrEmpty(r.data).find((x) => x.id === props.courseId) ?? null;
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loaded.value = true;
    }
}
watch(() => props.courseId, load, { immediate: true });
const base = computed(() => `/courses/${props.courseId}`);
</script>
<template>
    <section>
        <RouterLink to="/courses" class="back">← 返回课程</RouterLink>
        <p v-if="error" class="error">{{ error }}</p>
        <p v-else-if="loaded && !course">你尚未加入这门课程，请先从课程目录选择。</p>
        <template v-if="course">
        <p class="eyebrow">课程详情</p>
        <h1>{{ course.course_name }}</h1>
        <p>
            {{ course?.term }}
            {{ course?.teacher_name ? `· ${course.teacher_name}` : "" }}
        </p>
        <nav class="tabs" aria-label="课程详情导航">
            <RouterLink :to="`${base}/assignments`" active-class="active"
                >作业</RouterLink
            ><RouterLink :to="`${base}/sources`" active-class="active"
                >来源导入</RouterLink
            >
        </nav>
        <RouterView />
        </template>
    </section>
</template>
