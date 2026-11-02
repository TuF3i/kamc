<script setup lang="ts">
// 内容卡片：Element el-card 轻封装（head 插槽带操作区）
defineProps<{
  title?: string;
  pad?: boolean;
}>();
</script>

<template>
  <el-card class="card" :class="{ 'no-pad': pad === false }" shadow="never">
    <template v-if="title || $slots.actions" #header>
      <div class="card-head">
        <h3>{{ title }}</h3>
        <div class="card-actions"><slot name="actions" /></div>
      </div>
    </template>
    <slot />
  </el-card>
</template>

<style lang="scss" scoped>
.card {
  margin-bottom: 16px;

  &.no-pad :deep(.el-card__body) {
    padding: 0;
  }
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;

  h3 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }
}

.card-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}
</style>
