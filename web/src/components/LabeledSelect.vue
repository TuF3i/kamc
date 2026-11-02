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
    <select
      :id="selectId"
      class="select"
      :value="modelValue"
      :required="required"
      :disabled="disabled"
      @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
    >
      <option v-for="o in options" :key="o.value" :value="o.value">{{ o.label }}</option>
    </select>
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
}

.req {
  color: $error;
}

.field-hint {
  font-size: 12px;
  color: $text-muted;
  margin-top: 4px;
  line-height: 1.5;
}
</style>
