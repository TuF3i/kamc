<script setup lang="ts">
import { useId } from 'vue';

export interface SelectOption {
  label: string;
  value: string;
}

const selectId = useId();

withDefaults(
  defineProps<{
    label: string;
    modelValue: string;
    options: SelectOption[];
    required?: boolean;
    hint?: string;
    disabled?: boolean;
  }>(),
  { required: false, hint: '', disabled: false },
);

defineEmits<{ 'update:modelValue': [value: string] }>();
</script>

<template>
  <div class="field">
    <label class="field-label" :for="selectId">
      {{ label }}
      <span v-if="required" class="req">*</span>
    </label>
    <el-select
      :id="selectId"
      :model-value="modelValue"
      :disabled="disabled"
      :teleported="false"
      style="width: 100%"
      @update:model-value="$emit('update:modelValue', $event as string)"
    >
      <el-option v-for="o in options" :key="o.value" :label="o.label" :value="o.value" />
    </el-select>
    <div v-if="hint" class="field-hint">{{ hint }}</div>
  </div>
</template>

<style lang="scss" scoped>
.field {
  margin-bottom: 14px;
}

.field-label {
  display: block;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 5px;
  color: $text;
}

.req {
  color: var(--el-color-danger);
}

.field-hint {
  font-size: 12px;
  color: $text-muted;
  margin-top: 4px;
  line-height: 1.5;
}
</style>
