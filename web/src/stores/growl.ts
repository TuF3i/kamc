import { defineStore } from 'pinia';

export type GrowlType = 'success' | 'error' | 'info' | 'warning';

export interface Growl {
  id: number;
  type: GrowlType;
  message: string;
}

let nextId = 1;

/** 右上角Growl 通知（Rancher 风格的操作反馈） */
export const useGrowlStore = defineStore('growl', {
  state: () => ({
    items: [] as Growl[],
  }),

  actions: {
    push(type: GrowlType, message: string, ttl = 4000): void {
      const id = nextId++;
      this.items.push({ id, type, message });
      if (ttl > 0) {
        setTimeout(() => this.dismiss(id), ttl);
      }
    },
    success(message: string): void {
      this.push('success', message);
    },
    error(message: string): void {
      this.push('error', message, 6000);
    },
    info(message: string): void {
      this.push('info', message);
    },
    warning(message: string): void {
      this.push('warning', message, 6000);
    },
    dismiss(id: number): void {
      this.items = this.items.filter((g) => g.id !== id);
    },
  },
});
