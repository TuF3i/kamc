<script setup lang="ts">
export type BannerType = 'error' | 'warning' | 'info' | 'success';

withDefaults(
  defineProps<{
    type?: BannerType;
    closable?: boolean;
  }>(),
  { type: 'info', closable: false },
);

const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <div class="banner" :class="type" role="alert">
    <span class="banner-icon">
      {{ type === 'error' ? '✕' : type === 'warning' ? '⚠' : type === 'success' ? '✓' : 'ℹ' }}
    </span>
    <div class="banner-body"><slot /></div>
    <button v-if="closable" class="banner-close" aria-label="关闭" @click="emit('close')">✕</button>
  </div>
</template>

<style lang="scss" scoped>
.banner {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 14px;
  border-radius: 4px;
  font-size: 13px;
  line-height: 20px;
  margin-bottom: 14px;
  border: 1px solid transparent;

  &.error {
    background: rgba(212, 74, 50, 0.08);
    border-color: rgba(212, 74, 50, 0.35);
    color: #a33a26;
    .banner-icon { color: $error; }
  }

  &.warning {
    background: rgba(208, 135, 0, 0.08);
    border-color: rgba(208, 135, 0, 0.35);
    color: #96660a;
    .banner-icon { color: $warning; }
  }

  &.info {
    background: rgba(43, 125, 201, 0.07);
    border-color: rgba(43, 125, 201, 0.3);
    color: #1e5d99;
    .banner-icon { color: $info; }
  }

  &.success {
    background: rgba(46, 158, 91, 0.08);
    border-color: rgba(46, 158, 91, 0.35);
    color: #227643;
    .banner-icon { color: $success; }
  }
}

.banner-icon {
  flex-shrink: 0;
  font-weight: 700;
}

.banner-body {
  flex: 1;
  word-break: break-word;

  :deep(pre) {
    margin: 6px 0 0;
    white-space: pre-wrap;
    font-size: 12px;
    font-family: Consolas, monospace;
  }
}

.banner-close {
  background: none;
  border: none;
  cursor: pointer;
  color: inherit;
  opacity: 0.6;
  font-size: 12px;
  padding: 2px;

  &:hover {
    opacity: 1;
  }
}
</style>
