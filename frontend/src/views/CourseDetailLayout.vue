<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { api } from "../api";
import { arrayOrEmpty, type Course } from "../types";
const props = defineProps<{ courseId: string }>(),
    course = ref<Course | null>(null);
onMounted(async () => {
    const r = await api<{ data: Course[] | null }>("/course-instances");
    course.value =
        arrayOrEmpty(r.data).find((x) => x.id === props.courseId) ?? null;
});
const base = computed(() => `/courses/${props.courseId}`);
const maintainer = computed(() => course.value?.my_role === "maintainer");
</script>
<template>
    <section>
        <RouterLink to="/courses" class="back">← 返回课程</RouterLink>
        <p class="eyebrow">课程详情</p>
        <h1>{{ course?.course_name || "课程" }}</h1>
        <p>
            {{ course?.term }}
            {{ course?.teacher_name ? `· ${course.teacher_name}` : "" }}
        </p>
        <nav class="tabs" aria-label="课程详情导航">
            <RouterLink :to="`${base}/assignments`" active-class="active"
                >作业</RouterLink
            ><RouterLink :to="`${base}/schedule`" active-class="active"
                >课表</RouterLink
            ><RouterLink :to="`${base}/sources`" active-class="active"
                >来源导入</RouterLink
            ><RouterLink
                v-if="maintainer"
                :to="`${base}/candidates`"
                active-class="active"
                >候选审核</RouterLink
            ><RouterLink
                v-if="maintainer"
                :to="base + '/members'"
                active-class="active"
                >成员管理</RouterLink
            >
        </nav>
        <RouterView />
    </section>
</template>
