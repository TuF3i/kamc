import clsx from 'clsx';
import {
  For,
  Show,
  createEffect,
  createMemo,
  createSignal,
  onCleanup,
  type JSX,
} from 'solid-js';
import { Portal } from 'solid-js/web';
import { ChevronUpIcon, DismissIcon, DownOutlined, SpinnerIcon } from '@/lib/icons';

export interface SelectOption {
  label: JSX.Element | string;
  value: string | number;
  disabled?: boolean;
  options?: SelectOption[];
}

export interface SelectProps {
  value?: string | number | (string | number)[];
  onChange?: (value: unknown, option: unknown) => void;
  options?: SelectOption[];
  placeholder?: string;
  showSearch?: boolean;
  allowClear?: boolean;
  filterOption?: boolean | ((input: string, option: SelectOption) => boolean);
  optionFilterProp?: 'label' | 'value';
  mode?: 'multiple' | 'tags';
  maxTagCount?: number | 'responsive';
  disabled?: boolean;
  loading?: boolean;
  notFoundContent?: JSX.Element;
  onSearch?: (value: string) => void;
  class?: string;
  style?: JSX.CSSProperties;
  size?: 'small' | 'middle';
  id?: string;
}

function flatten(options?: SelectOption[]): SelectOption[] {
  const result: SelectOption[] = [];
  for (const option of options ?? []) {
    if (option.options) result.push(...flatten(option.options));
    else result.push(option);
  }
  return result;
}

function isSelectedValue(
  value: string | number,
  selected: (string | number)[],
): boolean {
  return selected.some((item) => String(item) === String(value));
}

export function Select(props: SelectProps) {
  let trigger: HTMLDivElement | undefined;
  let panel: HTMLDivElement | undefined;
  let search: HTMLInputElement | undefined;

  const [open, setOpen] = createSignal(false);
  const [query, setQuery] = createSignal('');
  const [focus, setFocus] = createSignal(-1);
  const [pos, setPos] = createSignal({ left: 0, top: 0, width: 0 });

  const multiple = () => props.mode === 'multiple' || props.mode === 'tags';
  const selected = createMemo<(string | number)[]>(() => {
    if (multiple()) {
      return Array.isArray(props.value) ? props.value : [];
    }
    return props.value === undefined || props.value === null
      ? []
      : [props.value as string | number];
  });
  const flat = createMemo(() => flatten(props.options));
  const labelOf = (value: string | number): JSX.Element => {
    const option = flat().find((item) => String(item.value) === String(value));
    return option?.label ?? String(value);
  };

  const visibleOptions = createMemo<SelectOption[]>(() => {
    const text = query().trim().toLowerCase();
    const options = props.options ?? [];
    if (!text || (props.filterOption === false && props.onSearch)) return options;
    if (props.filterOption === false && !props.onSearch) return options;
    const match = (option: SelectOption) => {
      if (props.filterOption && typeof props.filterOption === 'function') {
        return props.filterOption(text, option);
      }
      const source =
        props.optionFilterProp === 'value'
          ? String(option.value)
          : typeof option.label === 'string'
            ? option.label
            : String(option.value);
      return source.toLowerCase().includes(text);
    };
    const walk = (list: SelectOption[]): SelectOption[] => {
      const result: SelectOption[] = [];
      for (const option of list) {
        if (option.options) {
          const children = walk(option.options);
          if (children.length) result.push({ ...option, options: children });
        } else if (match(option)) {
          result.push(option);
        }
      }
      return result;
    };
    return walk(options);
  });

  const flatVisible = createMemo(() => flatten(visibleOptions()).filter((item) => !item.disabled));

  const place = () => {
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    setPos({ left: rect.left, top: rect.bottom + 4, width: rect.width });
  };

  const close = () => {
    setOpen(false);
    setQuery('');
    setFocus(-1);
  };

  const emit = (values: (string | number)[]) => {
    if (multiple()) {
      props.onChange?.(
        values,
        values.map((value) => flat().find((item) => String(item.value) === String(value))),
      );
    } else {
      const value = values[0];
      props.onChange?.(
        value,
        flat().find((item) => String(item.value) === String(value)),
      );
    }
  };

  const choose = (option: SelectOption) => {
    if (option.disabled) return;
    if (multiple()) {
      const values = selected();
      emit(
        isSelectedValue(option.value, values)
          ? values.filter((value) => String(value) !== String(option.value))
          : [...values, option.value],
      );
    } else {
      emit([option.value]);
      close();
    }
  };

  const removeValue = (value: string | number) => {
    emit(selected().filter((item) => String(item) !== String(value)));
  };

  const clear = () => {
    props.onChange?.(multiple() ? [] : undefined, multiple() ? [] : undefined);
    setQuery('');
  };

  const toggleOpen = () => {
    if (props.disabled) return;
    if (open()) {
      close();
      return;
    }
    place();
    setOpen(true);
    queueMicrotask(() => search?.focus());
  };

  createEffect(() => {
    if (!open()) return;
    const onDown = (event: MouseEvent) => {
      const target = event.target as Node;
      if (trigger?.contains(target) || panel?.contains(target)) return;
      close();
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close();
    };
    const onMove = () => place();
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    window.addEventListener('resize', onMove);
    window.addEventListener('scroll', onMove, true);
    onCleanup(() => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', onMove);
      window.removeEventListener('scroll', onMove, true);
    });
  });

  const onKeyDown = (event: KeyboardEvent) => {
    const items = flatVisible();
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setFocus((index) => Math.min(items.length - 1, index + 1));
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      setFocus((index) => Math.max(0, index - 1));
    } else if (event.key === 'Enter') {
      event.preventDefault();
      if (props.mode === 'tags' && query().trim()) {
        const value = query().trim();
        if (!isSelectedValue(value, selected())) emit([...selected(), value]);
        setQuery('');
        props.onSearch?.('');
        return;
      }
      const item = items[focus()];
      if (item) choose(item);
    } else if (event.key === 'Backspace' && multiple() && !query().trim() && selected().length) {
      removeValue(selected()[selected().length - 1]);
    }
  };

  const showSearch = () => Boolean(props.showSearch || props.mode === 'tags' || props.onSearch);

  return (
    <>
      <div
        ref={trigger}
        id={props.id}
        class={clsx('select-trigger', props.size === 'small' && 'input-sm', props.class)}
        style={props.style}
        data-open={open() ? 'true' : 'false'}
        data-disabled={props.disabled ? 'true' : 'false'}
        role="combobox"
        aria-expanded={open()}
        tabindex={props.disabled ? -1 : 0}
        onClick={toggleOpen}
        onKeyDown={(event) => {
          if (!open() && (event.key === 'Enter' || event.key === ' ' || event.key === 'ArrowDown')) {
            event.preventDefault();
            toggleOpen();
            return;
          }
          onKeyDown(event);
        }}
      >
        <Show
          when={multiple()}
          fallback={
            <Show
              when={selected().length}
              fallback={<span class="select-placeholder">{props.placeholder}</span>}
            >
              <span class="select-value">{labelOf(selected()[0])}</span>
            </Show>
          }
        >
          <Show
            when={selected().length}
            fallback={<span class="select-placeholder">{props.placeholder}</span>}
          >
            <span class="select-tags">
              <For
                each={
                  props.maxTagCount && typeof props.maxTagCount === 'number'
                    ? selected().slice(0, props.maxTagCount)
                    : selected()
                }
              >
                {(value) => (
                  <span class="select-tag">
                    <span class="truncate">{labelOf(value)}</span>
                    <button
                      type="button"
                      class="select-tag-x"
                      aria-label={`移除 ${String(labelOf(value))}`}
                      onClick={(event) => {
                        event.stopPropagation();
                        removeValue(value);
                      }}
                    >
                      <DismissIcon class="h-3 w-3" />
                    </button>
                  </span>
                )}
              </For>
              <Show
                when={
                  typeof props.maxTagCount === 'number' &&
                  selected().length > props.maxTagCount
                }
              >
                <span class="select-tag">
                  +{selected().length - (props.maxTagCount as number)}
                </span>
              </Show>
            </span>
          </Show>
        </Show>
        <Show when={props.allowClear && selected().length > 0 && !props.disabled}>
          <span
            class="select-clear"
            role="button"
            aria-label="清除"
            onClick={(event) => {
              event.stopPropagation();
              clear();
            }}
          >
            <DismissIcon class="h-3 w-3" />
          </span>
        </Show>
        <span class="select-chevron">
          <Show when={open()} fallback={<DownOutlined class="h-4 w-4" />}>
            <ChevronUpIcon class="h-4 w-4" />
          </Show>
        </span>
      </div>

      <Show when={open()}>
        <Portal mount={document.body}>
          <div
            ref={panel}
            class="select-popup"
            style={{
              left: `${pos().left}px`,
              top: `${pos().top}px`,
              width: `${pos().width}px`,
            }}
          >
            <Show when={showSearch()}>
              <input
                ref={search}
                class="select-search"
                value={query()}
                placeholder="搜索…"
                onInput={(event) => {
                  setQuery(event.currentTarget.value);
                  setFocus(-1);
                  props.onSearch?.(event.currentTarget.value);
                }}
                onKeyDown={onKeyDown}
              />
            </Show>
            <div class="select-list" role="listbox">
              <Show
                when={flatVisible().length || visibleOptions().length}
                fallback={
                  <div class="select-empty">
                    {props.loading ? (
                      <span class="inline-flex items-center gap-2">
                        <SpinnerIcon class="h-3.5 w-3.5 animate-spin" />
                        加载中…
                      </span>
                    ) : (
                      (props.notFoundContent ?? '暂无数据')
                    )}
                  </div>
                }
              >
                <For each={visibleOptions()}>
                  {(option) => (
                    <Show
                      when={option.options}
                      fallback={
                        <button
                          type="button"
                          class="select-option"
                          data-selected={isSelectedValue(option.value, selected()) ? 'true' : 'false'}
                          data-disabled={option.disabled ? 'true' : 'false'}
                          data-focus={flatVisible()[focus()] === option ? 'true' : 'false'}
                          role="option"
                          aria-selected={isSelectedValue(option.value, selected())}
                          onClick={() => choose(option)}
                        >
                          <span class="min-w-0 flex-1 truncate">{option.label}</span>
                          <Show when={isSelectedValue(option.value, selected())}>
                            <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
                          </Show>
                        </button>
                      }
                    >
                      <div class="select-group">{option.label}</div>
                      <For each={option.options}>
                        {(child) => (
                          <button
                            type="button"
                            class="select-option"
                            data-selected={isSelectedValue(child.value, selected()) ? 'true' : 'false'}
                            data-disabled={child.disabled ? 'true' : 'false'}
                            data-focus={flatVisible()[focus()] === child ? 'true' : 'false'}
                            role="option"
                            aria-selected={isSelectedValue(child.value, selected())}
                            onClick={() => choose(child)}
                          >
                            <span class="min-w-0 flex-1 truncate">{child.label}</span>
                            <Show when={isSelectedValue(child.value, selected())}>
                              <span class="h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
                            </Show>
                          </button>
                        )}
                      </For>
                    </Show>
                  )}
                </For>
              </Show>
            </div>
          </div>
        </Portal>
      </Show>
    </>
  );
}
