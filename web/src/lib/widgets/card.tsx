import clsx from 'clsx';
import { Show, type JSX } from 'solid-js';

export interface CardProps {
  title?: JSX.Element;
  extra?: JSX.Element;
  class?: string;
  contentClass?: string;
  style?: JSX.CSSProperties;
  onClick?: (event: MouseEvent) => void;
  children?: JSX.Element;
}

/** 玻璃卡片：外层 .card + 内层 .card-content，可选标题栏。 */
export function Card(props: CardProps) {
  const interactive = () => Boolean(props.onClick);

  return (
    <div
      class={clsx('card flex-col', interactive() && 'card-interactive', props.class)}
      style={props.style}
      onClick={props.onClick}
    >
      <Show when={props.title || props.extra}>
        <div class="flex min-h-[50px] items-center justify-between gap-3 border-b border-layer-content/10 px-5 py-2.5 font-bold">
          <div class="min-w-0">{props.title}</div>
          <Show when={props.extra}>
            <div class="flex shrink-0 flex-wrap items-center gap-2">{props.extra}</div>
          </Show>
        </div>
      </Show>
      <div class={clsx('card-content', props.contentClass ?? 'p-5')}>{props.children}</div>
    </div>
  );
}
