<script setup lang="ts">
import { ref } from 'vue';
import { useGrowlStore } from '@/stores/growl';

const props = defineProps<{
  text: string;
  /** 复制成功提示（缺省不提示） */
  tip?: string;
}>();

const growl = useGrowlStore();
const copied = ref(false);

async function copy(): Promise<void> {
  try {
    await navigator.clipboard.writeText(props.text);
  } catch {
    // 非安全上下文回退
    const ta = document.createElement('textarea');
    ta.value = props.text;
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    ta.remove();
  }
  copied.value = true;
  if (props.tip) growl.success(props.tip);
  setTimeout(() => (copied.value = false), 1200);
}
</script>

<template>
  <button class="copy-btn" type="button" :title="copied ? '已复制' : '复制'" @click="copy">
    {{ copied ? '✓' : '⧉' }}
  </button>
</template>

<style lang="scss" scoped>
.copy-btn {
  background: none;
  border: none;
  cursor: pointer;
  color: $text-muted;
  font-size: 14px;
  padding: 2px 6px;
  border-radius: 3px;
  line-height: 1.2;

  &:hover {
    color: $brand;
    background: rgba(43, 125, 201, 0.08);
  }
}
</style>
