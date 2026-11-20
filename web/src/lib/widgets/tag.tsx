import clsx from 'clsx';
import { Show, type JSX } from 'solid-js';

export type TagLevel =
  | 'neutral'
  | 'primary'
  | 'info'
  | 'success'
  | 'warning'
  | 'error'
  | 'gold';

export interface TagProps {
  level?: TagLevel;
  class?: string;
  title?: string;
  icon?: JSX.Element;
  children?: JSX.Element;
}

/** 药丸标签：玻璃底 + 语义色圆点（参考示例 tag 结构，修正动态类问题）。 */
export function Tag(props: TagProps) {
  const level = () => props.level ?? 'neutral';

  return (
    <span class={clsx('tag', `tag-${level()}`, props.class)} title={props.title}>
      <span class="tag-content">
        <span class="tag-dot" />
        <Show when={props.icon}>{props.icon}</Show>
        <span class="min-w-0 truncate">{props.children}</span>
      </span>
    </span>
  );
}
