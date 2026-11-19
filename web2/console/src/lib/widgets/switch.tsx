import clsx from 'clsx';
import { Show } from 'solid-js';

export interface SwitchProps {
  checked?: boolean;
  onChange?: (checked: boolean) => void;
  disabled?: boolean;
  checkedChildren?: string;
  unCheckedChildren?: string;
  class?: string;
}

export function Switch(props: SwitchProps) {
  const checked = () => Boolean(props.checked);
  return (
    <span class={clsx('inline-flex items-center gap-2', props.class)}>
      <button
        type="button"
        role="switch"
        aria-checked={checked() ? 'true' : 'false'}
        disabled={props.disabled}
        class={clsx(
          'relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border border-transparent outline-none transition-colors duration-200',
          checked() ? 'bg-primary' : 'bg-layer-content/20',
          props.disabled && 'cursor-not-allowed opacity-50',
        )}
        onClick={() => {
          if (!props.disabled) props.onChange?.(!checked());
        }}
        onKeyDown={(event) => {
          if (event.key === ' ' || event.key === 'Enter') {
            event.preventDefault();
            if (!props.disabled) props.onChange?.(!checked());
          }
        }}
      >
        <span
          class={clsx(
            'inline-block h-4.5 w-4.5 transform rounded-full bg-white transition-transform duration-200',
            checked() ? 'translate-x-[22px]' : 'translate-x-0.5',
          )}
          style={{ width: '18px', height: '18px' }}
        />
      </button>
      <Show when={props.checkedChildren || props.unCheckedChildren}>
        <span class="text-xs text-layer-content/60">
          {checked() ? props.checkedChildren : props.unCheckedChildren}
        </span>
      </Show>
    </span>
  );
}
