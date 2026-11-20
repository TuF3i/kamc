import clsx from 'clsx';
import { Show, type JSX } from 'solid-js';
import { ClusterOutlined } from '@/lib/icons';

export interface EmptyProps {
  description?: JSX.Element;
  class?: string;
  children?: JSX.Element;
}

export function Empty(props: EmptyProps) {
  return (
    <div class={clsx('empty-state', props.class)}>
      <ClusterOutlined class="h-7 w-7 opacity-30" />
      <div>{props.description ?? '暂无数据'}</div>
      <Show when={props.children}>{props.children}</Show>
    </div>
  );
}
