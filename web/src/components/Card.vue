<script setup lang="ts">
defineProps<{
  title?: string;
  pad?: boolean;
}>();
</script>

<template>
  <section class="card" :class="{ pad: pad !== false }">
    <header v-if="title || $slots.actions" class="card-head">
      <h3>{{ title }}</h3>
      <div class="card-actions"><slot name="actions" /></div>
    </header>
    <slot />
  </section>
</template>

<style lang="scss" scoped>
.card {
  background: $card-bg;
  border: 1px solid $border;
  border-radius: 6px;
  box-shadow: $shadow;
  margin-bottom: 16px;

  &.pad {
    padding: 16px;
  }
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;

  h3 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }
}

.card:has(.card-head) {
  .card-actions {
    display: flex;
    gap: 8px;
    align-items: center;
  }
}
</style>
