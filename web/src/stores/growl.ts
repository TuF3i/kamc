import { ElMessage } from 'element-plus';
import { defineStore } from 'pinia';

export type GrowlType = 'success' | 'error' | 'info' | 'warning';

/**
 * 操作反馈：底层 Element ElMessage（右上角消息条）。
 * 保持既有 push/success/error 接口，页面代码无感迁移。
 */
export const useGrowlStore = defineStore('growl', {
  actions: {
    push(type: GrowlType, message: string): void {
      ElMessage({ type, message, grouping: true, showClose: true, duration: type === 'error' ? 6000 : 4000 });
    },
    success(message: string): void {
      this.push('success', message);
    },
    error(message: string): void {
      this.push('error', message);
    },
    info(message: string): void {
      this.push('info', message);
    },
    warning(message: string): void {
      this.push('warning', message);
    },
  },
});
