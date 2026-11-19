import clsx from 'clsx';
import { For, type JSX } from 'solid-js';

export interface SegmentedOption {
  label: JSX.Element;
  value: string;
}

export interface SegmentedProps {
  value?: string;
  options: SegmentedOption[];
  onChange?: (value: string) => void;
  block?: boolean;
  class?: string;
  disabled?: boolean;
}

export function Segmented(props: SegmentedProps) {
  return (
    <div
      class={clsx(
        'inline-flex rounded-lg border border-layer-content/10 bg-layer-content/5 p-0.5',
        props.block && 'flex w-full',
        props.class,
      )}
      role="tablist"
    >
      <For each={props.options}>
        {(option) => (
          <button
            type="button"
            role="tab"
            aria-selected={props.value === option.value}
            disabled={props.disabled}
            class={clsx(
              'inline-flex min-h-7 flex-1 cursor-pointer items-center justify-center whitespace-nowrap rounded-md border border-transparent px-2.5 text-xs font-bold outline-none transition-colors',
              props.value === option.value
                ? 'border-layer-content/15 bg-layer/80 text-primary'
                : 'text-layer-content/55 hover:text-layer-content/80',
              props.disabled && 'cursor-not-allowed opacity-50',
            )}
            onClick={() => props.onChange?.(option.value)}
          >
            {option.label}
          </button>
        )}
      </For>
    </div>
  );
}
