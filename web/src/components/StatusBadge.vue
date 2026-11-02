<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    status: string;
    /** 显示文本，缺省用 status 原文 */
    label?: string;
  }>(),
  { label: '' },
);

// 归一化：denied-casbin → denied；未知状态一律中性灰
const kind = computed(() => {
  const s = props.status.toLowerCase();
  for (const key of ['online', 'active', 'enabled', 'allowed', 'success', 'offline', 'pending', 'rotating', 'denied']) {
    if (s.startsWith(key)) return key;
  }
  return 'other';
});

const text = computed(() => props.label || props.status);
</script>

<template>
  <span class="status-badge" :class="kind">
    <span class="dot" />
    {{ text }}
  </span>
</template>

<style lang="scss" scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  line-height: 20px;
  padding: 0 8px;
  border-radius: 10px;
  background: rgba(138, 151, 163, 0.12);
  color: #5c6b7a;
  white-space: nowrap;
}

.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
  flex-shrink: 0;
}

.online,
.active,
.enabled,
.allowed,
.success {
  background: rgba(46, 158, 91, 0.12);
  color: #20713f;
}

.pending,
.rotating {
  background: rgba(208, 135, 0, 0.12);
  color: #96660a;
}

.offline {
  background: rgba(138, 151, 163, 0.14);
  color: #5c6b7a;
}

.denied {
  background: rgba(212, 74, 50, 0.1);
  color: #a33a26;
}
</style>
