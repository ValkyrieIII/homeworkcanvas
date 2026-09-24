<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import { api } from "./api";
import type { CurrentUser } from "./types";
const route = useRoute();
const currentUser = ref<CurrentUser | null>(null);
const isAdminWorkspace = computed(() => route.path.startsWith("/admin"));
onMounted(async () => {
    try { currentUser.value = await api<CurrentUser>("/me"); } catch { currentUser.value = null; }
});
const pageTitle = computed(() =>
    route.path === "/admin/courses"
        ? "全站课程"
        : route.path === "/admin"
        ? "作业工作台"
        : route.path.startsWith("/admin/courses/")
        ? "课程管理"
        : route.path === "/admin/review"
        ? "候选审核"
        : route.path.startsWith("/courses/")
        ? "课程详情"
        : ({
              "/": "首页",
              "/assignments": "我的作业",
              "/schedule": "我的课表",
              "/courses": "课程目录",
              "/notifications": "通知",
          }[route.path] ?? "Homework Canvas"),
);
</script>
<template>
    <div class="app-shell">
        <aside class="sidebar">
            <RouterLink class="brand" :to="isAdminWorkspace ? '/admin' : '/'"
                ><span class="brand-mark">H</span
                ><span
                    ><strong>Homework<br />Canvas</strong
                    ><small>学习事务工作台</small></span
                ></RouterLink
            >
            <div v-if="currentUser?.role === 'admin'" class="workspace-switch" aria-label="切换工作台">
                <RouterLink to="/" :class="{ selected: !isAdminWorkspace }">用户端</RouterLink>
                <RouterLink to="/admin" :class="{ selected: isAdminWorkspace }">管理员端</RouterLink>
            </div>
            <nav v-if="!isAdminWorkspace" class="side-nav" aria-label="用户端导航">
                <RouterLink to="/" exact-active-class="active"
                    ><i>⌂</i>首页</RouterLink
                >
                <RouterLink to="/assignments" active-class="active"
                    ><i>✓</i>我的作业</RouterLink
                >
                <RouterLink to="/schedule" active-class="active"
                    ><i>▦</i>我的课表</RouterLink
                >
                <RouterLink to="/courses" active-class="active"
                    ><i>▤</i>课程目录</RouterLink
                >
                <RouterLink to="/notifications" active-class="active"
                    ><i>◉</i>通知</RouterLink
                >
            </nav>
            <nav v-else class="side-nav" aria-label="管理员端导航">
                <RouterLink to="/admin" exact-active-class="active"><i>✓</i>作业工作台</RouterLink>
                <RouterLink to="/admin/courses" active-class="active"><i>▤</i>课程管理</RouterLink>
            </nav>
            <div class="sidebar-tip">
                <template v-if="isAdminWorkspace">
                    <b>共享内容由管理员核对</b>
                    <span>这里维护全站课程和作业，个人学习安排请切换到用户端。</span>
                </template>
                <template v-else>
                    <b>作业数据是共享事实</b>
                    <span>个人计划不会改动全班的官方截止时间。</span>
                </template>
            </div>
        </aside>
        <div class="workspace">
            <header class="topbar">
                <span class="page-title">{{ pageTitle }}</span
                ><label class="search"
                    ><span>⌕</span
                    ><input
                        placeholder="搜索课程或作业（即将支持）"
                        disabled /></label
                ><span class="user-avatar" :title="currentUser?.display_name || ''">{{ currentUser?.display_name?.slice(0, 2) || "HC" }}</span>
            </header>
            <main class="content"><RouterView /></main>
        </div>
    </div>
</template>
