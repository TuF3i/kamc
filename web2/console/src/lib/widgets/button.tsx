import clsx from 'clsx';
import { Show, splitProps, type JSX } from 'solid-js';
import { SpinnerIcon } from '@/lib/icons';

export type ButtonType =
  | 'default'
  | 'primary'
  | 'ghost'
  | 'text'
  | 'link'
  | 'dashed';

export interface ButtonProps
  extends Omit<JSX.ButtonHTMLAttributes<HTMLButtonElement>, 'type'> {
  type?: ButtonType;
  htmlType?: 'button' | 'submit' | 'reset';
  size?: 'small' | 'middle' | 'large';
  block?: boolean;
  loading?: boolean;
  danger?: boolean;
  icon?: JSX.Element;
}

/** 参考示例按钮：玻璃底、圆角、focus 外描边；sm 用于密集工具栏。 */
export function Button(props: ButtonProps) {
  const [local, rest] = splitProps(props, [
    'type',
    'htmlType',
    'size',
    'block',
    'loading',
    'danger',
    'icon',
    'class',
    'children',
    'disabled',
  ]);

  const type = () => local.type ?? 'default';
  const isGhost = () => ['ghost', 'text', 'link', 'dashed'].includes(type());

  const classes = () => {
    const ghost = isGhost();
    return clsx(
      'btn',
      ghost
        ? 'btn-ghost'
        : local.danger
          ? 'btn-error'
          : type() === 'primary'
            ? 'btn-primary'
            : null,
      local.danger && ghost && 'text-error',
      type() === 'link' && !local.danger && 'text-primary underline-offset-2 hover:underline',
      type() === 'dashed' && 'border-dashed border-layer-content/25',
      local.size === 'small' && 'btn-sm',
      local.block && 'btn-block',
      local.class,
    );
  };

  return (
    <button
      {...rest}
      type={local.htmlType ?? 'button'}
      class={classes()}
      disabled={local.disabled || local.loading}
      aria-busy={local.loading ? 'true' : undefined}
    >
      <Show
        when={local.loading}
        fallback={local.icon}
      >
        <SpinnerIcon class="h-4 w-4 animate-spin" />
      </Show>
      {local.children}
    </button>
  );
}
