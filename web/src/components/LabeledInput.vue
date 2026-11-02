<script setup lang="ts">
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
    <label class="field-label">
      {{ label }}
      <span v-if="required" class="req">*</span>
    </label>
    <input
      class="input"
      :class="{ mono }"
      :type="type"
      :value="modelValue"
      :placeholder="placeholder"
      :required="required"
      :disabled="disabled"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
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
