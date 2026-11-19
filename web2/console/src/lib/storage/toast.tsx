import { For, createSignal } from 'solid-js';
import { Portal } from 'solid-js/web';
import clsx from 'clsx';

export type ToastLevel = 'info' | 'success' | 'warning' | 'error';

interface ToastData {
  id: number;
  level: ToastLevel;
  text: string;
}

const ICONS: Record<ToastLevel, string> = {
  info: 'icon-[fluent--info-16-regular]',
  success: 'icon-[fluent--checkmark-circle-16-regular]',
  warning: 'icon-[fluent--warning-16-regular]',
  error: 'icon-[fluent--error-circle-16-regular]',
};

const COLORS: Record<ToastLevel, string> = {
  info: 'text-info',
  success: 'text-success',
  warning: 'text-warning',
  error: 'text-error',
};

let nextId = 1;
const [toasts, setToasts] = createSignal<ToastData[]>([]);

function add(level: ToastLevel, text: string, duration = 4000): void {
  const id = nextId++;
  setToasts((list) => [...list, { id, level, text }]);
  window.setTimeout(() => {
    setToasts((list) => list.filter((item) => item.id !== id));
  }, duration);
}

export const toast = {
  info: (text: string, duration?: number) => add('info', text, duration),
  success: (text: string, duration?: number) => add('success', text, duration),
  warning: (text: string, duration?: number) => add('warning', text, duration),
  error: (text: string, duration?: number) => add('error', text, duration),
};

/** 右下角堆叠提示（参考示例 ToastContainer 同款布局）。 */
export function Toasts() {
  return (
    <Portal mount={document.body}>
      <div class="toast-container" aria-live="polite">
        <For each={toasts()}>
          {(item) => (
            <div class="toast">
              <div class="toast-content">
                <span
                  class={clsx(
                    'h-4 w-4 shrink-0',
                    ICONS[item.level],
                    COLORS[item.level],
                  )}
                />
                <span class="min-w-0 flex-1 break-words">{item.text}</span>
              </div>
            </div>
          )}
        </For>
      </div>
    </Portal>
  );
}
