<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "../api";
import { arrayOrEmpty, formatDate, type Notification } from "../types";
const items = ref<Notification[]>([]),
    loading = ref(false),
    error = ref("");
async function load() {
    loading.value = true;
    error.value = "";
    try {
        items.value = arrayOrEmpty(
            (await api<{ data: Notification[] | null }>("/me/notifications"))
                .data,
        );
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}
async function read(item: Notification) {
    if (item.read_at) return;
    try {
        await api(`/notifications/${item.id}/read`, { method: "POST" });
        item.read_at = new Date().toISOString();
    } catch (e: any) {
        error.value = e.message;
    }
}
onMounted(load);
</script>
<template>
    <section>
        <p class="eyebrow">通知</p>
        <h1>课程动态</h1>
        <p v-if="loading">正在加载…</p>
        <p v-if="error" class="error">{{ error }}</p>
        <p v-if="!loading && !error && !items.length" class="empty">
            暂无通知。
        </p>
        <article
            v-for="item in items"
            :key="item.id"
            class="card card-split"
            :class="{ unread: !item.read_at }"
        >
            <RouterLink
                class="card-split__link"
                :to="`/courses/${item.payload.course_instance_id}/assignments`"
            >
                <h3 v-if="item.kind === 'assignment_retracted'">作业已撤回：{{ item.payload.title }}</h3>
                <h3 v-else-if="item.kind === 'assignment_updated'">作业已修改：{{ item.payload.title }}</h3>
                <h3 v-else>发布了新作业：{{ item.payload.title }}</h3>
                <p v-if="item.kind !== 'assignment_retracted'">官方截止：{{ formatDate(item.payload.official_due_at) }}</p>
                <small>{{ formatDate(item.created_at) }}</small>
            </RouterLink>
            <div v-if="!item.read_at" class="card-split__actions">
                <button @click="read(item)">标为已读</button>
            </div>
        </article>
    </section>
</template>
