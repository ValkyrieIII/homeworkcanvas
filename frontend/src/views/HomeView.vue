<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "../api";
import { arrayOrEmpty, type Assignment } from "../types";
import AssignmentCard from "../components/AssignmentCard.vue";
import LoadState from "../components/LoadState.vue";
import { useAssignmentState } from "../composables/useAssignmentState";
const upcoming = ref<Assignment[]>([]),
    overdue = ref<Assignment[]>([]),
    undated = ref<Assignment[]>([]),
    loading = ref(false),
    error = ref("");
async function load() {
    loading.value = true;
    error.value = "";
    try {
        const [u, o, d] = await Promise.all([
            api<{ data: Assignment[] | null }>(
                "/me/assignments?bucket=upcoming",
            ),
            api<{ data: Assignment[] | null }>(
                "/me/assignments?bucket=overdue",
            ),
            api<{ data: Assignment[] | null }>(
                "/me/assignments?bucket=undated",
            ),
        ]);
        upcoming.value = arrayOrEmpty(u.data).filter(item => item.completion_status === "todo").slice(0, 3);
        overdue.value = arrayOrEmpty(o.data).filter(item => item.completion_status === "todo").slice(0, 3);
        undated.value = arrayOrEmpty(d.data).filter(item => item.completion_status === "todo").slice(0, 3);
    } catch (e: any) {
        error.value = e.message;
    } finally {
        loading.value = false;
    }
}
const { actionError, toggle, savePlan } = useAssignmentState(load);
onMounted(load);
</script>
<template>
    <section>
        <p class="eyebrow">首页</p>
        <h1>最近要做什么</h1>
        <RouterLink to="/assignments">查看全部作业 →</RouterLink>
        <p v-if="actionError" class="error" role="alert">{{ actionError }}</p>
        <LoadState
            :loading="loading"
            :error="error"
            :empty="false"
            empty-text=""
        /><template v-if="!loading && !error"
            ><h2>临近个人 DDL</h2>
            <AssignmentCard
                v-for="a in upcoming"
                :key="a.id"
                :assignment="a"
                @toggle="toggle"
                @plan="savePlan"
            />
            <p v-if="!upcoming.length" class="empty">暂无即将截止的作业。</p>
            <h2>官方已逾期</h2>
            <AssignmentCard
                v-for="a in overdue"
                :key="a.id"
                :assignment="a"
                @toggle="toggle"
                @plan="savePlan"
            />
            <p v-if="!overdue.length" class="empty">暂无官方逾期作业。</p>
            <h2>截止时间待确认</h2>
            <AssignmentCard
                v-for="a in undated"
                :key="a.id"
                :assignment="a"
                @toggle="toggle"
                @plan="savePlan"
            />
            <p v-if="!undated.length" class="empty">
                暂无待确认作业。
            </p>
        </template>
    </section>
</template>
