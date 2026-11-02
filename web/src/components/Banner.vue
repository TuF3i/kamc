<script setup lang="ts">
// 警示条：Element el-alert 封装（slot 内容进 default，closable 透传）
withDefaults(
  defineProps<{
    type?: 'error' | 'warning' | 'info' | 'success';
    closable?: boolean;
  }>(),
  { type: 'info', closable: false },
);

const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <el-alert class="banner" :type="type" :closable="closable" show-icon @close="emit('close')">
    <slot />
  </el-alert>
</template>

<style lang="scss" scoped>
.banner {
  margin-bottom: 14px;

  :deep(.el-alert__content) {
    width: 100%;
  }

  :deep(pre) {
    margin: 4px 0 0;
    white-space: pre-wrap;
    font-size: 12px;
    font-family: Consolas, monospace;
  }
}
</style>
