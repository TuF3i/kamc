import clsx from 'clsx';
import { For, Show, type JSX } from 'solid-js';

export interface DescriptionItem {
  label: JSX.Element;
  value: JSX.Element;
  span?: number;
}

export interface DescriptionsProps {
  items: DescriptionItem[];
  column?: number | { xs?: number; md?: number };
  bordered?: boolean;
  size?: 'small' | 'middle';
  class?: string;
}

export function Descriptions(props: DescriptionsProps) {
  const cols = () =>
    typeof props.column === 'number' ? props.column : (props.column?.md ?? 1);

  const rows = () => {
    const result: DescriptionItem[][] = [];
    let current: DescriptionItem[] = [];
    let used = 0;
    for (const item of props.items) {
      const span = Math.min(item.span ?? 1, cols());
      if (span >= cols()) {
        if (current.length) result.push(current);
        result.push([item]);
        current = [];
        used = 0;
        continue;
      }
      if (used + span > cols()) {
        result.push(current);
        current = [];
        used = 0;
      }
      current.push(item);
      used += span;
    }
    if (current.length) result.push(current);
    return result;
  };

  return (
    <div
      class={clsx('descriptions', !props.bordered && 'descriptions-plain', props.class)}
      style={{ '--desc-cols': String(cols()) }}
    >
      <For each={rows()}>
        {(row) => (
          <div class="descriptions-row">
            <For each={row}>
              {(item) => (
                <div class="desc-item" style={{ 'grid-column': `span ${item.span ?? 1}` }}>
                  <div class="descriptions-label">{item.label}</div>
                  <div class="descriptions-value">
                    <Show when={item.value !== undefined && item.value !== null && item.value !== ''} fallback="—">
                      {item.value}
                    </Show>
                  </div>
                </div>
              )}
            </For>
          </div>
        )}
      </For>
    </div>
  );
}
