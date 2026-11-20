import clsx from 'clsx';
import { Show, type JSX } from 'solid-js';
import { DismissIcon, ErrorIcon, InfoIcon, SuccessIcon, WarningIcon } from '@/lib/icons';

export type AlertType = 'info' | 'success' | 'warning' | 'error';

export interface AlertProps {
  type?: AlertType;
  showIcon?: boolean;
  message?: JSX.Element;
  description?: JSX.Element;
  action?: JSX.Element;
  closable?: boolean;
  onClose?: () => void;
  class?: string;
  style?: JSX.CSSProperties;
}

export function Alert(props: AlertProps) {
  const type = () => props.type ?? 'info';
  return (
    <div class={clsx('alert', `alert-${type()}`, props.class)} style={props.style} role="alert">
      <Show when={props.showIcon !== false}>
        <Show when={type() === 'info'}>
          <InfoIcon class="alert-icon" />
        </Show>
        <Show when={type() === 'success'}>
          <SuccessIcon class="alert-icon" />
        </Show>
        <Show when={type() === 'warning'}>
          <WarningIcon class="alert-icon" />
        </Show>
        <Show when={type() === 'error'}>
          <ErrorIcon class="alert-icon" />
        </Show>
      </Show>
      <div class="alert-body">
        <Show when={props.message}>
          <div class="alert-title">{props.message}</div>
        </Show>
        <Show when={props.description}>
          <div class="alert-desc">{props.description}</div>
        </Show>
      </div>
      <Show when={props.action}>
        <div class="alert-action">{props.action}</div>
      </Show>
      <Show when={props.closable}>
        <button
          type="button"
          class="icon-btn -mr-1 -mt-1 h-6 w-6"
          aria-label="关闭"
          onClick={() => props.onClose?.()}
        >
          <DismissIcon class="h-3.5 w-3.5" />
        </button>
      </Show>
    </div>
  );
}
