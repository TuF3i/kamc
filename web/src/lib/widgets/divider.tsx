import clsx from 'clsx';
import { Show, type JSX } from 'solid-js';

export interface DividerProps {
  children?: JSX.Element;
  vertical?: boolean;
  class?: string;
}

export function Divider(props: DividerProps) {
  if (props.vertical) {
    return <span class={clsx('divider divider-vertical', props.class)} />;
  }
  return (
    <Show
      when={props.children}
      fallback={<div class={clsx('divider', props.class)} />}
    >
      <div class={clsx('login-divider', props.class)}>
        <span>{props.children}</span>
      </div>
    </Show>
  );
}
