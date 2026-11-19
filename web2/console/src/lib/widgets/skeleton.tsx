import clsx from 'clsx';
import { For, Show } from 'solid-js';

export interface SkeletonProps {
  active?: boolean;
  avatar?: boolean;
  paragraph?: { rows?: number } | boolean;
  rows?: number;
  class?: string;
}

const WIDTHS = ['100%', '92%', '78%', '96%', '85%', '70%'];

export function Skeleton(props: SkeletonProps) {
  const rows = () => {
    if (typeof props.paragraph === 'object') return props.paragraph.rows ?? 3;
    if (props.paragraph === false) return 0;
    return props.rows ?? 3;
  };

  return (
    <div class={clsx('flex flex-col gap-3', props.class)} aria-hidden="true">
      <div class="flex items-center gap-3">
        <Show when={props.avatar}>
          <span class="skeleton skeleton-circle h-10 w-10 shrink-0" />
        </Show>
        <div class="flex min-w-0 flex-1 flex-col gap-2">
          <span class="skeleton skeleton-text" style={{ width: '42%' }} />
          <span class="skeleton skeleton-text" style={{ width: '68%' }} />
        </div>
      </div>
      <Show when={rows() > 0}>
        <div class="flex flex-col gap-2.5">
          <For each={Array.from({ length: rows() })}>
            {(_, index) => (
              <span
                class="skeleton skeleton-text"
                style={{ width: WIDTHS[index() % WIDTHS.length] }}
              />
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}
