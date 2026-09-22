<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
const route = useRoute();
const pageTitle = computed(() =>
    route.path.startsWith("/courses/")
        ? "课程详情"
        : ({
              "/": "首页",
              "/assignments": "我的作业",
              "/schedule": "我的课表",
              "/courses": "课程",
              "/notifications": "通知",
          }[route.path] ?? "Homework Canvas"),
);
</script>
<template>
    <div class="app-shell">
        <aside class="sidebar">
            <RouterLink class="brand" to="/"
                ><span class="brand-mark">H</span
                ><span
                    ><strong>Homework<br />Canvas</strong
                    ><small>学习事务工作台</small></span
                ></RouterLink
            >
            <nav class="side-nav" aria-label="主导航">
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
                    ><i>▤</i>课程</RouterLink
                >
                <RouterLink to="/notifications" active-class="active"
                    ><i>◉</i>通知</RouterLink
                >
            </nav>
            <div class="sidebar-tip">
                <b>作业数据是共享事实</b
                ><span>个人计划不会改动全班的官方截止时间。</span>
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
                ><span class="user-avatar">LD</span>
            </header>
            <main class="content"><RouterView /></main>
        </div>
    </div>
</template>
