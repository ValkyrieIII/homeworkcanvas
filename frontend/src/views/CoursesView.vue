<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { api } from "../api";
import { arrayOrEmpty, type Course } from "../types";
import LoadState from "../components/LoadState.vue";

const courses = ref<Course[]>([]);
const loading = ref(false);
const busy = ref<string | null>(null);
const error = ref("");
const mineOnly = ref(false);
const visibleCourses = computed(() => mineOnly.value ? courses.value.filter(item => item.joined) : courses.value);

async function load() {
    loading.value = true;
    error.value = "";
    try {
        courses.value = arrayOrEmpty((await api<{ data: Course[] | null }>("/course-instances")).data);
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}

async function setJoined(course: Course) {
    busy.value = course.id;
    error.value = "";
    try {
        await api(`/course-instances/${course.id}/enrollment`, {
            method: course.joined ? "DELETE" : "PUT",
        });
        await load();
    } catch (e: any) {
        error.value = e.message;
    } finally {
        busy.value = null;
    }
}

onMounted(load);
</script>

<template>
    <section>
        <p class="eyebrow">课程目录</p>
        <h1>选择参与的课程</h1>
        <p>加入后，课程作业和课表会出现在你的个人视图中。</p>
        <div class="tabs" aria-label="课程筛选">
            <button class="tab-button" :class="{ active: !mineOnly }" @click="mineOnly = false">全部课程</button>
            <button class="tab-button" :class="{ active: mineOnly }" @click="mineOnly = true">我已加入</button>
        </div>
        <LoadState :loading="loading" :error="error" :empty="!visibleCourses.length" :empty-text="mineOnly ? '尚未加入课程。' : '暂无可加入课程。'" />
        <article v-for="course in visibleCourses" :key="course.id" class="card course-link">
            <small>{{ course.term }} · {{ course.joined ? "已加入" : "未加入" }}</small>
            <h3>{{ course.course_name }}</h3>
            <p>{{ course.teacher_name || "教师待确认" }}</p>
            <div class="course-actions">
                <RouterLink v-if="course.joined" :to="`/courses/${course.id}/assignments`">查看课程</RouterLink>
                <button :disabled="busy === course.id" :class="{ secondary: course.joined }" @click="setJoined(course)">
                    {{ course.joined ? "退出课程" : "加入课程" }}
                </button>
            </div>
        </article>
    </section>
</template>

<style scoped>
.course-actions { display: flex; align-items: center; gap: 12px; }
</style>
