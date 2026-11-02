<script setup lang="ts">
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
  <div class="tabs">
    <button
      v-for="t in tabs"
      :key="t.key"
      class="tab"
      :class="{ active: modelValue === t.key }"
      type="button"
      @click="$emit('update:modelValue', t.key)"
    >
      {{ t.label }}
    </button>
  </div>
</template>

<style lang="scss" scoped>
.tabs {
  display: flex;
  gap: 2px;
  border-bottom: 1px solid $border;
  margin-bottom: 16px;
}

.tab {
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  padding: 9px 16px;
  font-size: 13.5px;
  color: $text-muted;
  cursor: pointer;
  margin-bottom: -1px;

  &:hover {
    color: $brand;
  }

  &.active {
    color: $brand;
    border-bottom-color: $brand;
    font-weight: 600;
  }
}
</style>
