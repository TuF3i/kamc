import clsx from 'clsx';
import { Show, createEffect, onCleanup, type JSX } from 'solid-js';
import { Portal } from 'solid-js/web';
import { DismissIcon } from '@/lib/icons';

export interface ModalProps {
  open: boolean;
  title?: JSX.Element;
  onClose: () => void;
  footer?: JSX.Element | null;
  width?: number;
  class?: string;
  bodyClass?: string;
  children?: JSX.Element;
  maskClosable?: boolean;
}

export function Modal(props: ModalProps) {
  let panel: HTMLDivElement | undefined;

  createEffect(() => {
    if (!props.open) return;
    const previous = document.activeElement as HTMLElement | null;
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKey = (event: KeyboardEvent) => {
      // 嵌套的 Select/Popover/日期面板应先消费 Escape，避免连同弹窗一起关闭。
      const nested = document.querySelector('.select-popup, .date-popup, .popover');
      if (event.key === 'Escape' && !nested) props.onClose();
    };
    document.addEventListener('keydown', onKey);
    queueMicrotask(() => panel?.focus());
    onCleanup(() => {
      document.body.style.overflow = prevOverflow;
      document.removeEventListener('keydown', onKey);
      previous?.focus?.();
    });
  });

  return (
    <Show when={props.open}>
      <Portal mount={document.body}>
        <div
          class="overlay"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget && props.maskClosable !== false) {
              props.onClose();
            }
          }}
        >
          <div
            ref={panel}
            class={clsx('modal', props.class)}
            style={props.width ? { width: `${props.width}px` } : undefined}
            role="dialog"
            aria-modal="true"
            tabindex={-1}
          >
            <Show when={props.title}>
              <div class="modal-head">
                <div class="modal-title">{props.title}</div>
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
            <div class={clsx('modal-body', props.bodyClass)}>{props.children}</div>
            <Show when={props.footer !== null}>
              <div class="modal-foot">{props.footer}</div>
            </Show>
          </div>
        </div>
      </Portal>
    </Show>
  );
}
