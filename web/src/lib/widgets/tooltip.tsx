import { Show, createSignal, type JSX } from 'solid-js';
import { Portal } from 'solid-js/web';

export interface TooltipProps {
  title?: JSX.Element;
  children: JSX.Element;
  class?: string;
}

export function Tooltip(props: TooltipProps) {
  let anchor: HTMLSpanElement | undefined;
  const [open, setOpen] = createSignal(false);
  const [pos, setPos] = createSignal({ left: 0, top: 0 });

  const show = () => {
    if (!props.title || !anchor) return;
    const rect = anchor.getBoundingClientRect();
    setPos({ left: rect.left + rect.width / 2, top: rect.top - 8 });
    setOpen(true);
  };

  return (
    <>
      <span
        ref={anchor}
        class={props.class}
        onMouseEnter={show}
        onMouseLeave={() => setOpen(false)}
        onFocus={show}
        onBlur={() => setOpen(false)}
      >
        {props.children}
      </span>
      <Show when={open() && props.title}>
        <Portal mount={document.body}>
          <div
            class="tooltip"
            style={{
              left: `${pos().left}px`,
              top: `${pos().top}px`,
              transform: 'translate(-50%, -100%)',
            }}
            role="tooltip"
          >
            {props.title}
          </div>
        </Portal>
      </Show>
    </>
  );
}
