import clsx from 'clsx';
import {
  Show,
  createEffect,
  createSignal,
  onCleanup,
  type JSX,
} from 'solid-js';
import { Portal } from 'solid-js/web';

export interface PopoverProps {
  trigger: JSX.Element;
  children: JSX.Element;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  placement?: 'bottom-start' | 'bottom-end';
  class?: string;
  panelClass?: string;
  /** 面板最小宽度；默认 192px。 */
  minWidth?: number;
}

export function Popover(props: PopoverProps) {
  let wrap: HTMLSpanElement | undefined;
  let panel: HTMLDivElement | undefined;
  const [innerOpen, setInnerOpen] = createSignal(false);
  const [pos, setPos] = createSignal({ left: 0, top: 0 });

  const isOpen = () => (props.open === undefined ? innerOpen() : props.open);

  const setOpen = (value: boolean) => {
    if (props.open === undefined) setInnerOpen(value);
    props.onOpenChange?.(value);
  };

  const place = () => {
    if (!wrap) return;
    const rect = wrap.getBoundingClientRect();
    const width = Math.max(props.minWidth ?? 192, rect.width);
    const left =
      props.placement === 'bottom-end'
        ? Math.max(8, Math.min(rect.right - width, window.innerWidth - width - 8))
        : Math.max(8, Math.min(rect.left, window.innerWidth - width - 8));
    setPos({ left, top: rect.bottom + 6 });
  };

  createEffect(() => {
    if (!isOpen()) return;
    place();

    const onDown = (event: MouseEvent) => {
      const target = event.target as Node;
      if (wrap?.contains(target)) return;
      if (panel?.contains(target)) return;
      setOpen(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    window.addEventListener('resize', place);
    window.addEventListener('scroll', place, true);
    onCleanup(() => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', place);
      window.removeEventListener('scroll', place, true);
    });
  });

  return (
    <>
      <span
        ref={wrap}
        class={clsx('inline-flex', props.class)}
        onClick={() => setOpen(!isOpen())}
      >
        {props.trigger}
      </span>
      <Show when={isOpen()}>
        <Portal mount={document.body}>
          <div
            ref={panel}
            data-popover-panel
            class={clsx('popover fixed z-[70]', props.panelClass)}
            style={{ left: `${pos().left}px`, top: `${pos().top}px` }}
          >
            {props.children}
          </div>
        </Portal>
      </Show>
    </>
  );
}
