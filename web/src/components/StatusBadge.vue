<script setup lang="ts">
import { computed } from 'vue';

// 状态徽章：el-tag 按语义着色（denied/unknown 归 danger/default）
const props = withDefaults(
  defineProps<{
    status: string;
    label?: string;
  }>(),
  { label: '' },
);

const tagType = computed((): 'success' | 'warning' | 'danger' | 'info' | 'primary' => {
  const s = props.status.toLowerCase();
  if (s.startsWith('online') || s.startsWith('active') || s.startsWith('enabled') || s.startsWith('allowed') || s.startsWith('success'))
    return 'success';
  if (s.startsWith('pending') || s.startsWith('rotating') || s.startsWith('warning')) return 'warning';
  if (s.startsWith('denied')) return 'danger';
  if (s.startsWith('offline')) return 'info';
  return 'info';
});

const effect = computed((): 'light' => 'light');
const round = true;

const text = computed(() => props.label || props.status);
</script>

<template>
  <el-tag :type="tagType" :effect="effect" :round="round" size="small" disable-transitions>
    {{ text }}
  </el-tag>
</template>
