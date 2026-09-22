<script setup lang="ts">
import type { Assignment } from "../types";
import { formatDate } from "../types";
defineProps<{ assignment: Assignment }>();
defineEmits<{ toggle: [assignment: Assignment] }>();
</script>
<template>
    <article class="card assignment-card">
        <small>{{ assignment.course_name }} · {{ assignment.term }}</small>
        <h3>{{ assignment.title }}</h3>
        <p>官方截止：{{ formatDate(assignment.official_due_at) }}</p>
        <p v-if="assignment.personal_due_at" class="personal-due">
            个人计划：{{ formatDate(assignment.personal_due_at) }}
        </p>
        <p class="status">
            状态：{{
                assignment.completion_status === "completed" ? "已完成" : "待办"
            }}
        </p>
        <button @click="$emit('toggle', assignment)">
            {{
                assignment.completion_status === "completed"
                    ? "恢复待办"
                    : "标记完成"
            }}
        </button>
    </article>
</template>
