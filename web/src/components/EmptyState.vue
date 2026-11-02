<script setup lang="ts">
defineProps<{
  icon?: string;
  title: string;
  desc?: string;
}>();
</script>

<template>
  <div class="empty-state">
    <div class="empty-icon">{{ icon ?? '◌' }}</div>
    <div class="empty-title">{{ title }}</div>
    <div v-if="desc" class="empty-desc">{{ desc }}</div>
    <div class="empty-actions"><slot /></div>
  </div>
</template>

<style lang="scss" scoped>
.empty-state {
  text-align: center;
  padding: 56px 20px;
  color: $text-muted;
}

.empty-icon {
  font-size: 34px;
  margin-bottom: 12px;
  opacity: 0.5;
}

.empty-title {
  font-size: 15px;
  font-weight: 600;
  color: $text;
  margin-bottom: 4px;
}

.empty-desc {
  font-size: 13px;
  max-width: 420px;
  margin: 0 auto;
  line-height: 1.6;
}

.empty-actions {
  margin-top: 14px;
}
</style>
