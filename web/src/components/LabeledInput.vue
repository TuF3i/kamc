<script setup lang="ts">
// 表单字段：Element el-form-item + 对应控件（label 唯一键由表单上下文处理）
import { useId } from 'vue';

const inputId = useId();

withDefaults(
  defineProps<{
    label: string;
    modelValue: string;
    type?: string;
    placeholder?: string;
    required?: boolean;
    hint?: string;
    mono?: boolean;
    disabled?: boolean;
  }>(),
  { type: 'text', placeholder: '', required: false, hint: '', mono: false, disabled: false },
);

defineEmits<{ 'update:modelValue': [value: string] }>();
</script>

<template>
  <div class="field">
    <label class="field-label" :for="inputId">
      {{ label }}
      <span v-if="required" class="req">*</span>
    </label>
    <el-input
      :id="inputId"
      :model-value="modelValue"
      :type="type === 'password' ? 'password' : 'text'"
      :placeholder="placeholder"
      :disabled="disabled"
      :class="{ mono }"
      :rows="type === 'textarea' ? 3 : undefined"
      @update:model-value="$emit('update:modelValue', $event)"
    />
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

.mono :deep(input),
.mono :deep(textarea) {
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12.5px;
}
</style>
