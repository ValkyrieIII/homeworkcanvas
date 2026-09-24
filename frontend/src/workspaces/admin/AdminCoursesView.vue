<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { api } from "../../api";
import { arrayOrEmpty, type Course } from "../../types";
import LoadState from "../../components/LoadState.vue";

const router = useRouter();
const courses = ref<Course[]>([]);
const loading = ref(false);
const saving = ref(false);
const error = ref("");
const name = ref("");
const term = ref("");
const teacher = ref("");

async function load() {
    loading.value = true;
    error.value = "";
    try {
        courses.value = arrayOrEmpty((await api<{ data: Course[] | null }>("/admin/course-instances")).data);
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}
async function create() {
    if (saving.value) return;
    saving.value = true;
    error.value = "";
    try {
        const result = await api<{ id: string }>("/course-instances", {
            method: "POST",
            body: JSON.stringify({ course_name: name.value.trim(), term: term.value.trim(), teacher_name: teacher.value.trim() || null }),
        });
        await router.push(`/admin/courses/${result.id}`);
    } catch (e: any) {
        error.value = e.message;
    } finally {
        saving.value = false;
    }
}
onMounted(load);
</script>

<template>
    <section>
        <p class="eyebrow">管理员端</p>
        <h1>全站课程</h1>
        <p>维护共享课程资料和上课安排。用户可从课程目录自主加入。</p>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <form class="card admin-form" @submit.prevent="create">
            <h2>创建课程</h2>
            <div class="form-grid">
                <label>课程名称<input v-model="name" required placeholder="例如：软件测试" /></label>
                <label>学期<input v-model="term" required placeholder="例如：2026 秋季学期" /></label>
                <label>任课教师<input v-model="teacher" placeholder="选填" /></label>
            </div>
            <button type="submit" :disabled="saving || !name.trim() || !term.trim()">{{ saving ? "创建中…" : "创建课程" }}</button>
        </form>
        <LoadState :loading="loading" :error="''" :empty="!courses.length" empty-text="暂无课程。" />
        <RouterLink v-for="course in courses" :key="course.id" :to="`/admin/courses/${course.id}`" class="card course-link">
            <small>{{ course.term }} · {{ course.status === "active" ? "开放" : "已归档" }}</small>
            <h3>{{ course.course_name }}</h3>
            <p>{{ course.teacher_name || "教师待确认" }}</p>
        </RouterLink>
    </section>
</template>
