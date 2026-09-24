<script setup lang="ts">
import { ref, watch } from "vue";
import { api } from "../api";
import type { AdminAssignment, Course } from "../types";

type EditableAssignment = Pick<AdminAssignment, "id" | "title" | "description" | "official_due_at" | "submission_method">;
const props = defineProps<{
  assignment?: EditableAssignment | null;
  courseId?: string;
  courses?: Course[];
}>();
const emit = defineEmits<{ saved: []; cancel: [] }>();
const selectedCourse = ref("");
const title = ref("");
const description = ref("");
const due = ref("");
const submission = ref("");
const busy = ref(false);
const error = ref("");

function localDate(value: string | null | undefined) {
  if (!value) return "";
  const date = new Date(value);
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}
function reset() {
  selectedCourse.value = props.courseId ?? "";
  title.value = props.assignment?.title ?? "";
  description.value = props.assignment?.description ?? "";
  due.value = localDate(props.assignment?.official_due_at);
  submission.value = props.assignment?.submission_method ?? "";
  error.value = "";
}
watch(() => [props.assignment, props.courseId], reset, { immediate: true });

async function save() {
  const courseId = props.courseId || selectedCourse.value;
  if (busy.value || !title.value.trim() || (!props.assignment && !courseId)) return;
  busy.value = true;
  error.value = "";
  try {
    await api(props.assignment ? `/assignments/${props.assignment.id}` : `/course-instances/${courseId}/assignments`, {
      method: props.assignment ? "PATCH" : "POST",
      body: JSON.stringify({
        title: title.value.trim(),
        description: description.value,
        official_due_at: due.value ? new Date(due.value).toISOString() : null,
        submission_method: submission.value.trim() || null,
      }),
    });
    reset();
    emit("saved");
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "保存作业失败";
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <form class="card admin-form" @submit.prevent="save">
    <h2>{{ assignment ? "修改共享作业" : "手工发布作业" }}</h2>
    <p>官方截止时间来自老师通知，个人计划由学生自行维护。</p>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <div class="form-grid">
      <label v-if="!assignment && !courseId">所属课程
        <select v-model="selectedCourse" required>
          <option value="">请选择课程</option>
          <option v-for="course in courses" :key="course.id" :value="course.id">{{ course.course_name }} · {{ course.term }}</option>
        </select>
      </label>
      <label>标题<input v-model="title" required /></label>
      <label>官方截止<input v-model="due" type="datetime-local" /></label>
      <label>提交方式<input v-model="submission" /></label>
    </div>
    <label>说明<textarea v-model="description" /></label>
    <div class="assignment-form-actions">
      <button :disabled="busy || !title.trim() || (!assignment && !courseId && !selectedCourse)">{{ busy ? "保存中…" : assignment ? "保存修改" : "发布作业" }}</button>
      <button v-if="assignment" class="secondary" type="button" @click="emit('cancel')">取消编辑</button>
    </div>
  </form>
</template>
