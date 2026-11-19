import clsx from 'clsx';
import { Show, createEffect, onCleanup, type JSX } from 'solid-js';
import { Portal } from 'solid-js/web';
import { DismissIcon } from '@/lib/icons';

export interface DrawerProps {
  open: boolean;
  title?: JSX.Element;
  onClose: () => void;
  width?: number;
  footer?: JSX.Element;
  class?: string;
  bodyClass?: string;
  children?: JSX.Element;
}

export function Drawer(props: DrawerProps) {
  createEffect(() => {
    if (!props.open) return;
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKey = (event: KeyboardEvent) => {
      // 嵌套的 Select/Popover/日期面板应先消费 Escape，避免连同弹窗一起关闭。
      const nested = document.querySelector('.select-popup, .date-popup, .popover');
      if (event.key === 'Escape' && !nested) props.onClose();
    };
    document.addEventListener('keydown', onKey);
    onCleanup(() => {
      document.body.style.overflow = prevOverflow;
      document.removeEventListener('keydown', onKey);
    });
  });

  return (
    <Show when={props.open}>
      <Portal mount={document.body}>
        <div
          class="drawer-overlay"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) props.onClose();
          }}
        />
        <aside
          class={clsx('drawer', props.class)}
          style={props.width ? { width: `${props.width}px` } : undefined}
          role="dialog"
          aria-modal="true"
        >
          <Show when={props.title}>
            <div class="drawer-head">
              <div class="drawer-title">{props.title}</div>
              <button
                type="button"
                class="icon-btn"
                aria-label="关闭"
                onClick={() => props.onClose()}
              >
                <DismissIcon class="h-4 w-4" />
              </button>
            </div>
          </Show>
          <div class={clsx('drawer-body', props.bodyClass)}>{props.children}</div>
          <Show when={props.footer}>
            <div class="drawer-foot">{props.footer}</div>
          </Show>
        </aside>
      </Portal>
    </Show>
  );
}
