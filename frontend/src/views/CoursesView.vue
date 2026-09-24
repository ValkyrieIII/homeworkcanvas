<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { api } from "../api";
import { arrayOrEmpty, type Course } from "../types";
import LoadState from "../components/LoadState.vue";
const router = useRouter();
const items = ref<Course[]>([]),
    loading = ref(false),
    error = ref(""),
    showCreate = ref(false),
    courseName = ref(""),
    term = ref(""),
    teacherName = ref(""),
    creating = ref(false),
    createError = ref("");
async function load() {
    loading.value = true;
    error.value = "";
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
async function createCourse() {
    const name = courseName.value.trim();
    const courseTerm = term.value.trim();
    if (!name || !courseTerm || creating.value) return;
    creating.value = true;
    createError.value = "";
    try {
        const created = await api<{ id: string }>("/course-instances", {
            method: "POST",
            body: JSON.stringify({
                course_name: name,
                term: courseTerm,
                teacher_name: teacherName.value.trim() || null,
            }),
        });
        await router.push(`/courses/${created.id}/assignments`);
    } catch (e: any) {
        createError.value = e.message;
    } finally {
        creating.value = false;
    }
}
onMounted(load);
</script>
<template>
    <section>
        <p class="eyebrow">课程</p>
        <div class="course-heading">
            <h1>我已加入的课程</h1>
            <button
                type="button"
                :aria-expanded="showCreate"
                aria-controls="create-course-form"
                @click="showCreate = !showCreate"
            >
                {{ showCreate ? "收起" : "+ 创建课程" }}
            </button>
        </div>
        <form
            v-if="showCreate"
            id="create-course-form"
            class="card create-course-form"
            @submit.prevent="createCourse"
        >
            <h2>创建课程</h2>
            <p>创建后，你会成为这门课程的维护者。</p>
            <div class="course-fields">
                <label>
                    课程名称 <span aria-hidden="true">*</span>
                    <input v-model="courseName" required placeholder="例如：软件测试" />
                </label>
                <label>
                    学期 <span aria-hidden="true">*</span>
                    <input v-model="term" required placeholder="例如：2026 秋季学期" />
                </label>
                <label>
                    任课教师（选填）
                    <input v-model="teacherName" placeholder="教师姓名" />
                </label>
            </div>
            <p v-if="createError" class="error" role="alert">{{ createError }}</p>
            <button type="submit" :disabled="creating || !courseName.trim() || !term.trim()">
                {{ creating ? "创建中…" : "创建并进入课程" }}
            </button>
        </form>
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
<style scoped>
.course-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
}
.course-heading h1 {
    margin-bottom: 12px;
}
.course-heading button {
    margin: 0 0 12px;
    white-space: nowrap;
}
.create-course-form {
    margin: 4px 0 20px;
}
.create-course-form h2 {
    margin: 0 0 8px;
}
.create-course-form > p:not(.error) {
    margin: 0 0 18px;
    color: #637084;
    font-size: 14px;
}
.course-fields {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 14px;
    margin-bottom: 15px;
}
.course-fields label {
    display: grid;
    gap: 7px;
    color: #3c4b61;
    font-size: 14px;
    font-weight: 700;
}
.course-fields input {
    width: 100%;
    min-width: 0;
    padding: 10px 12px;
    border: 1px solid #ccd5e1;
    border-radius: 7px;
    font: inherit;
    font-weight: 400;
    outline-color: #6e9dcf;
}
@media (max-width: 650px) {
    .course-fields {
        grid-template-columns: 1fr;
    }
}
</style>
