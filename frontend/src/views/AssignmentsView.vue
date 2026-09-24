<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api } from "../api";
import { arrayOrEmpty, type Assignment, type Course } from "../types";
import AssignmentCard from "../components/AssignmentCard.vue";
import LoadState from "../components/LoadState.vue";
import { useAssignmentState } from "../composables/useAssignmentState";
const route = useRoute(),
    router = useRouter(),
    items = ref<Assignment[]>([]),
    courses = ref<Course[]>([]),
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
const courseId = () => typeof route.query.course === "string" ? route.query.course : "";
async function loadCourses() {
    try {
        courses.value = arrayOrEmpty((await api<{ data: Course[] | null }>("/me/course-instances")).data);
    } catch (e) {
        error.value = e instanceof Error ? e.message : "读取课程失败";
    }
}
async function load() {
    loading.value = true;
    error.value = "";
    try {
        items.value = arrayOrEmpty(
            (
                await api<{ data: Assignment[] | null }>(
                    `/me/assignments?${new URLSearchParams({ bucket: bucket(), ...(courseId() ? { course_instance_id: courseId() } : {}) })}`,
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
watch(() => [route.query.bucket, route.query.course], load);
onMounted(() => { void loadCourses(); void load(); });
</script>
<template>
    <section>
        <div class="page-heading">
            <div>
                <p class="eyebrow">我的作业</p>
                <h1>跨课程作业</h1>
            </div>
        </div>
        <label class="filter-label">课程
            <select :value="courseId()" @change="router.push({ path: '/assignments', query: { bucket: bucket(), course: ($event.target as HTMLSelectElement).value || undefined } })">
                <option value="">全部已加入课程</option>
                <option v-for="course in courses" :key="course.id" :value="course.id">{{ course.course_name }} · {{ course.term }}</option>
            </select>
        </label>
        <div class="tabs">
            <RouterLink
                v-for="[key, label] in buckets"
                :key="key"
                :to="{ path: '/assignments', query: { bucket: key, course: courseId() || undefined } }"
                :class="{ active: bucket() === key }"
                >{{ label }}</RouterLink
            >
        </div>
        <p v-if="actionError" class="error" role="alert">{{ actionError }}</p>
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
            @plan="savePlan"
        />
    </section>
</template>
