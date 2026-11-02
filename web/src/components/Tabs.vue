<script setup lang="ts">
// 页签：Element el-tabs 封装（v-model 双向）
withDefaults(
  defineProps<{
    tabs: { key: string; label: string }[];
    modelValue: string;
  }>(),
  { modelValue: '' },
);

defineEmits<{ 'update:modelValue': [key: string] }>();
</script>

<template>
  <el-tabs class="tabs" :model-value="modelValue" @update:model-value="(k: string) => $emit('update:modelValue', k)">
    <el-tab-pane v-for="t in tabs" :key="t.key" :name="t.key">
      <template #label>{{ t.label }}</template>
    </el-tab-pane>
  </el-tabs>
</template>

<style lang="scss" scoped>
.tabs {
  margin-bottom: 16px;

  :deep(.el-tabs__header) {
    margin-bottom: 0;
  }
}
</style>
