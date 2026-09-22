<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "../api";
import { arrayOrEmpty } from "../types";
type Enrollment = {
    id: string;
    user_id: string;
    display_name: string;
    role: "maintainer" | "student";
    status: "active" | "left";
};
const props = defineProps<{ courseId: string }>(),
    items = ref<Enrollment[]>([]),
    loading = ref(false),
    error = ref("");
async function load() {
    loading.value = true;
    error.value = "";
    try {
        items.value = arrayOrEmpty(
            (
                await api<{ data: Enrollment[] | null }>(
                    "/course-instances/" + props.courseId + "/enrollments",
                )
            ).data,
        );
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}
async function update(item: Enrollment) {
    try {
        await api(
            "/course-instances/" + props.courseId + "/enrollments/" + item.id,
            {
                method: "PATCH",
                body: JSON.stringify({ role: item.role, status: item.status }),
            },
        );
        await load();
    } catch (e: any) {
        error.value = e.message;
    }
}
onMounted(load);
</script>
<template>
    <div>
        <h2>课程成员</h2>
        <p>维护者可调整角色或将成员设为离开；系统会保护最后一名维护者。</p>
        <p v-if="loading">正在加载…</p>
        <p v-if="error" class="error">{{ error }}</p>
        <article v-for="item in items" :key="item.id" class="card">
            <h3>{{ item.display_name }}</h3>
            <p>{{ item.user_id }}</p>
            <select v-model="item.role">
                <option value="student">学生</option>
                <option value="maintainer">维护者</option></select
            ><select v-model="item.status">
                <option value="active">有效成员</option>
                <option value="left">已离开</option></select
            ><button @click="update(item)">保存</button>
        </article>
    </div>
</template>
