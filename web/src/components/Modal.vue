<script setup lang="ts">
withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    width?: string;
  }>(),
  { title: '', width: '520px' },
);

const emit = defineEmits<{ close: [] }>();

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') emit('close');
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="modal-backdrop" @click.self="emit('close')" @keydown="onKeydown">
      <div class="modal" :style="{ width }" role="dialog" aria-modal="true">
        <header class="modal-head">
          <h3>{{ title }}</h3>
          <button class="modal-close" aria-label="关闭" @click="emit('close')">✕</button>
        </header>
        <div class="modal-body">
          <slot />
        </div>
        <footer v-if="$slots.footer" class="modal-foot">
          <slot name="footer" />
        </footer>
      </div>
    </div>
  </Teleport>
</template>

<style lang="scss" scoped>
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(13, 22, 33, 0.5);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 8vh 16px 16px;
  z-index: 200;
}

.modal {
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 12px 48px rgba(13, 22, 33, 0.3);
  max-width: 100%;
  max-height: 84vh;
  display: flex;
  flex-direction: column;
  animation: pop 0.14s ease-out;
}

@keyframes pop {
  from {
    transform: translateY(8px);
    opacity: 0;
  }
}

.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid $border;

  h3 {
    margin: 0;
    font-size: 15px;
  }
}

.modal-close {
  background: none;
  border: none;
  cursor: pointer;
  color: $text-muted;
  font-size: 13px;
  padding: 4px;

  &:hover {
    color: $text;
  }
}

.modal-body {
  padding: 18px;
  overflow-y: auto;
}

.modal-foot {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 18px;
  border-top: 1px solid $border;
}
</style>
