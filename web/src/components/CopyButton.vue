<script setup lang="ts">
import { ref } from 'vue';
import { CopyDocument, Check } from '@element-plus/icons-vue';
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
  <el-button text size="small" class="copy-btn" :title="copied ? '已复制' : '复制'" @click="copy">
    <el-icon><component :is="copied ? Check : CopyDocument" /></el-icon>
  </el-button>
</template>

<style lang="scss" scoped>
.copy-btn {
  padding: 4px 6px;
  color: var(--el-text-color-secondary);
}
</style>
