<script setup lang="ts">
defineProps<{
  title: string;
  desc?: string;
}>();
</script>

<template>
  <el-empty :image-size="88">
    <template #description>
      <div class="empty-title">{{ title }}</div>
      <div v-if="desc" class="empty-sub">{{ desc }}</div>
    </template>
    <slot />
  </el-empty>
</template>

<style lang="scss" scoped>
.empty-title {
  font-size: 14px;
  font-weight: 600;
  color: $text;
}

.empty-sub {
  font-size: 12.5px;
  color: $text-muted;
  max-width: 420px;
  margin: 4px auto 0;
  line-height: 1.6;
}
</style>
