<script setup lang="ts">
import { useGrowlStore } from '@/stores/growl';

const growl = useGrowlStore();

const icons: Record<string, string> = { success: '✓', error: '✕', info: 'ℹ', warning: '⚠' };
</script>

<template>
  <Teleport to="body">
    <div class="growl-stack">
      <TransitionGroup name="growl">
        <div v-for="g in growl.items" :key="g.id" class="growl" :class="g.type" @click="growl.dismiss(g.id)">
          <span class="growl-icon">{{ icons[g.type] }}</span>
          <span class="growl-msg">{{ g.message }}</span>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style lang="scss" scoped>
.growl-stack {
  position: fixed;
  top: 62px;
  right: 18px;
  z-index: 300;
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 380px;
}

.growl {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 14px;
  border-radius: 5px;
  background: #fff;
  box-shadow: 0 4px 20px rgba(13, 22, 33, 0.18);
  font-size: 13px;
  cursor: pointer;
  border-left: 3px solid;

  &.success {
    border-color: $success;
    .growl-icon { color: $success; }
  }

  &.error {
    border-color: $error;
    .growl-icon { color: $error; }
  }

  &.info {
    border-color: $info;
    .growl-icon { color: $info; }
  }

  &.warning {
    border-color: $warning;
    .growl-icon { color: $warning; }
  }
}

.growl-icon {
  font-weight: 700;
  flex-shrink: 0;
}

.growl-msg {
  word-break: break-word;
}

.growl-enter-active,
.growl-leave-active {
  transition: all 0.18s;
}

.growl-enter-from,
.growl-leave-to {
  opacity: 0;
  transform: translateX(16px);
}
</style>
