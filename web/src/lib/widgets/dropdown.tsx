import clsx from 'clsx';
import { For, Show, type JSX } from 'solid-js';
import { Popover } from './popover';

export interface MenuItem {
  key: string;
  label?: JSX.Element;
  icon?: JSX.Element;
  danger?: boolean;
  disabled?: boolean;
  type?: 'divider';
}

export interface MenuProps {
  items: MenuItem[];
  onClick?: (info: { key: string }) => void;
}

export interface DropdownProps {
  menu: MenuProps;
  trigger: JSX.Element;
  placement?: 'bottom-start' | 'bottom-end';
  class?: string;
}

export function Dropdown(props: DropdownProps) {
  return (
    <Popover
      trigger={props.trigger}
      placement={props.placement}
      minWidth={200}
      panelClass="p-1"
      class={props.class}
    >
      <div class="flex flex-col">
        <For each={props.menu.items}>
          {(item) => (
            <Show
              when={item.type !== 'divider'}
              fallback={<div class="menu-sep" />}
            >
              <button
                type="button"
                class={clsx('menu-item', item.danger && 'menu-item-danger')}
                disabled={item.disabled}
                onClick={() => props.menu.onClick?.({ key: item.key })}
              >
                <Show when={item.icon}>{item.icon}</Show>
                <span class="min-w-0 flex-1 truncate text-left">{item.label}</span>
              </button>
            </Show>
          )}
        </For>
      </div>
    </Popover>
  );
}
