<script setup lang="ts">
defineProps<{
  loading: boolean
  error: string
  empty: boolean
  emptyText: string
}>()
</script>
<template><p v-if="loading">正在加载…</p><p v-else-if="error" class="error">{{error}}</p><p v-else-if="empty" class="empty">{{emptyText}}</p></template>
