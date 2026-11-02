<script setup lang="ts">
// 对话框：Element el-dialog 封装（open prop 驱动，close 事件回传）
withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    width?: string;
  }>(),
  { title: '', width: '520px' },
);

const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <el-dialog
    :model-value="open"
    :title="title"
    :width="width"
    :close-on-click-modal="false"
    @update:model-value="(v: boolean) => !v && emit('close')"
    @close="emit('close')"
  >
    <slot />
    <template v-if="$slots.footer" #footer>
      <slot name="footer" />
    </template>
  </el-dialog>
</template>
