import {
  For,
  Show,
  createEffect,
  createSignal,
  type JSX,
} from 'solid-js';

export interface TabItem {
  key: string;
  label: JSX.Element;
  /** JSX 或惰性工厂；工厂在页签首次访问时才执行（保持 AntD 懒挂载语义）。 */
  children?: JSX.Element | (() => JSX.Element);
}

export interface TabsProps {
  items: TabItem[];
  activeKey?: string;
  defaultActiveKey?: string;
  onChange?: (key: string) => void;
  class?: string;
}

/** 页签：保留原 AntD Tabs 的懒挂载语义（首次访问后保持挂载）。 */
export function Tabs(props: TabsProps) {
  const first = props.activeKey ?? props.defaultActiveKey ?? props.items[0]?.key ?? '';
  const [current, setCurrent] = createSignal(first);
  const [visited, setVisited] = createSignal<string[]>([first]);

  createEffect(() => {
    if (props.activeKey !== undefined && props.activeKey !== current()) {
      setCurrent(props.activeKey);
      setVisited((list) =>
        list.includes(props.activeKey!) ? list : [...list, props.activeKey!],
      );
    }
  });

  const select = (key: string) => {
    setCurrent(key);
    setVisited((list) => (list.includes(key) ? list : [...list, key]));
    props.onChange?.(key);
  };

  return (
    <div class={props.class}>
      <div class="tabs" role="tablist">
        <For each={props.items}>
          {(item) => (
            <button
              type="button"
              role="tab"
              aria-selected={current() === item.key}
              data-active={current() === item.key ? 'true' : 'false'}
              class="tab"
              onClick={() => select(item.key)}
            >
              {item.label}
            </button>
          )}
        </For>
      </div>
      <For each={props.items}>
        {(item) => (
          <Show when={visited().includes(item.key) && item.children !== undefined}>
            <div
              class="tab-pane"
              role="tabpanel"
              hidden={current() !== item.key}
            >
              {typeof item.children === 'function'
                ? (item.children as () => JSX.Element)()
                : item.children}
            </div>
          </Show>
        )}
      </For>
    </div>
  );
}
