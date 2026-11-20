import clsx from 'clsx';
import {
  For,
  Show,
  createMemo,
  createSignal,
  type JSX,
} from 'solid-js';
import { FilterIcon, SpinnerIcon } from '@/lib/icons';
import { Popover } from './popover';

export const SELECTION_ALL = 'SELECT_ALL';
export const SELECTION_INVERT = 'SELECT_INVERT';
export const SELECTION_NONE = 'SELECT_NONE';

export type TableKey = string | number;

export interface TableColumn<T> {
  key?: string;
  title?: JSX.Element;
  dataIndex?: string | string[];
  render?: (value: unknown, record: T, index: number) => JSX.Element;
  width?: number | string;
  align?: 'left' | 'center' | 'right';
  sorter?: (a: T, b: T) => number;
  defaultSortOrder?: 'ascend' | 'descend';
  filters?: { text: string; value: string | number }[];
  onFilter?: (value: string | number, record: T) => boolean;
  filterMultiple?: boolean;
  fixed?: 'right';
  ellipsis?: boolean;
  className?: string;
  onCell?: (record: T, index: number) => { rowSpan?: number; colSpan?: number };
}

export type TableColumnsType<T> = TableColumn<T>[];

export interface TablePagination {
  current?: number;
  pageSize?: number;
  total?: number;
  showSizeChanger?: boolean;
  pageSizeOptions?: (number | string)[];
  showTotal?: (total: number, range: [number, number]) => JSX.Element;
  onChange?: (page: number, pageSize: number) => void;
}

export interface TableRowSelection<T> {
  selectedRowKeys: TableKey[];
  onChange?: (keys: TableKey[], rows: T[]) => void;
  selections?: (string | { key: string; text: string })[];
  preserveSelectedRowKeys?: boolean;
  getCheckboxProps?: (record: T) => { disabled?: boolean };
  columnWidth?: number;
}

export interface TableProps<T> {
  columns: TableColumn<T>[];
  dataSource: T[];
  rowKey?: string | ((record: T, index: number) => TableKey);
  pagination?: false | TablePagination;
  rowSelection?: TableRowSelection<T>;
  scroll?: { x?: number | string; y?: number | string };
  onRow?: (record: T, index: number) => {
    onClick?: (event: MouseEvent) => void;
  };
  locale?: { emptyText?: JSX.Element };
  size?: 'small' | 'middle' | 'large';
  bordered?: boolean;
  class?: string;
  loading?: boolean;
}

function getPath(record: unknown, path?: string | string[]): unknown {
  if (!path) return undefined;
  const parts = Array.isArray(path) ? path : path.split('.');
  let value: unknown = record;
  for (const part of parts) {
    if (value === null || value === undefined) return undefined;
    value = (value as Record<string, unknown>)[part];
  }
  return value;
}

function pageNumbers(current: number, total: number): (number | '...')[] {
  if (total <= 7) return Array.from({ length: total }, (_, index) => index + 1);
  const result: (number | '...')[] = [1];
  const start = Math.max(2, current - 1);
  const end = Math.min(total - 1, current + 1);
  if (start > 2) result.push('...');
  for (let page = start; page <= end; page += 1) result.push(page);
  if (end < total - 1) result.push('...');
  result.push(total);
  return result;
}

interface SortState {
  key: string;
  order: 'ascend' | 'descend';
}

function initialSort<T>(columns: TableColumn<T>[]): SortState | null {
  const index = columns.findIndex((column) => column.defaultSortOrder);
  if (index < 0) return null;
  const column = columns[index];
  return {
    key: column.key ?? String(column.dataIndex ?? index),
    order: column.defaultSortOrder!,
  };
}

function TableImpl<T extends object>(props: TableProps<T>) {
  const [sort, setSort] = createSignal<SortState | null>(initialSort(props.columns));
  const [filters, setFilters] = createSignal<Record<string, (string | number)[]>>({});
  const [page, setPage] = createSignal(props.pagination ? (props.pagination.current ?? 1) : 1);
  const [pageSize, setPageSize] = createSignal(
    props.pagination ? (props.pagination.pageSize ?? 10) : 10,
  );
  const [filterOpen, setFilterOpen] = createSignal<string | null>(null);

  const colKey = (column: TableColumn<T>, index: number) =>
    column.key ?? (Array.isArray(column.dataIndex)
      ? column.dataIndex.join('.')
      : column.dataIndex) ?? `col-${index}`;

  const filteredRows = createMemo(() => {
    const active = filters();
    let rows = props.dataSource;
    for (const [key, values] of Object.entries(active)) {
      if (!values.length) continue;
      const index = props.columns.findIndex((column, i) => colKey(column, i) === key);
      if (index < 0) continue;
      const column = props.columns[index];
      rows = rows.filter((record) => {
        if (column.onFilter) {
          return values.some((value) => column.onFilter!(value, record));
        }
        const value = String(getPath(record, column.dataIndex) ?? '');
        return values.some((item) => String(item) === value);
      });
    }
    return rows;
  });

  const sortedRows = createMemo(() => {
    const state = sort();
    if (!state) return filteredRows();
    const index = props.columns.findIndex((column, i) => colKey(column, i) === state.key);
    if (index < 0) return filteredRows();
    const column = props.columns[index];
    if (!column.sorter) return filteredRows();
    const rows = [...filteredRows()];
    rows.sort((a, b) => {
      const result = column.sorter!(a, b);
      return state.order === 'ascend' ? result : -result;
    });
    return rows;
  });

  const pagination = () => (props.pagination === false ? null : (props.pagination ?? {}));
  const total = () => pagination()?.total ?? sortedRows().length;
  const totalPages = () => Math.max(1, Math.ceil(total() / pageSize()));
  const pageRows = createMemo(() => {
    if (props.pagination === false) return sortedRows();
    const start = (page() - 1) * pageSize();
    return sortedRows().slice(start, start + pageSize());
  });

  const rowKey = (record: T, index: number): TableKey => {
    const key = props.rowKey ?? 'key';
    if (typeof key === 'function') return key(record, index);
    const value = (record as Record<string, unknown>)[key];
    return typeof value === 'string' || typeof value === 'number' ? value : index;
  };

  const selectedKeys = () => props.rowSelection?.selectedRowKeys ?? [];
  const selectedSet = createMemo(() => new Set(selectedKeys().map(String)));
  const isSelected = (key: TableKey) => selectedSet().has(String(key));

  const emitSelection = (keys: TableKey[]) => {
    const allRows = sortedRows();
    const rows = allRows.filter((record, index) => keys.includes(rowKey(record, index)));
    props.rowSelection?.onChange?.(keys, rows);
  };

  const toggleRow = (record: T, index: number) => {
    const key = rowKey(record, index);
    const keys = isSelected(key)
      ? selectedKeys().filter((item) => String(item) !== String(key))
      : [...selectedKeys(), key];
    emitSelection(keys);
  };

  const allSelectable = createMemo(() =>
    sortedRows()
      .map((record, index) => rowKey(record, index))
      .filter((_key, index) => {
        const record = sortedRows()[index];
        return !props.rowSelection?.getCheckboxProps?.(record).disabled;
      }),
  );

  const headerChecked = () =>
    allSelectable().length > 0 && allSelectable().every((key) => isSelected(key));
  const headerIndeterminate = () =>
    !headerChecked() && allSelectable().some((key) => isSelected(key));

  const selectionAction = (action: string) => {
    const keys = allSelectable();
    if (action === SELECTION_ALL) emitSelection(keys);
    else if (action === SELECTION_NONE) emitSelection([]);
    else if (action === SELECTION_INVERT) {
      emitSelection(selectedKeys().filter((key) => !keys.some((item) => String(item) === String(key))).concat(keys.filter((key) => !isSelected(key))));
    }
  };

  const changePage = (next: number) => {
    const target = Math.min(Math.max(1, next), totalPages());
    setPage(target);
    pagination()?.onChange?.(target, pageSize());
  };

  const changePageSize = (next: number) => {
    setPageSize(next);
    setPage(1);
    pagination()?.onChange?.(1, next);
  };

  const selectionCount = () => (props.rowSelection ? 1 : 0);

  return (
    <div class={clsx('table-shell', props.class)}>
      <div class={clsx('table-scroll', props.bordered && 'rounded-lg border border-layer-content/10')}>
        <table
          class="data-table"
          style={
            props.scroll?.x
              ? { 'min-width': typeof props.scroll.x === 'number' ? `${props.scroll.x}px` : props.scroll.x }
              : undefined
          }
        >
          <thead>
            <tr>
              <Show when={props.rowSelection}>
                <th style={{ width: `${props.rowSelection?.columnWidth ?? 40}px` }}>
                  <div class="flex items-center gap-1">
                    <input
                      type="checkbox"
                      class="table-check"
                      checked={headerChecked()}
                      data-indeterminate={headerIndeterminate() ? 'true' : 'false'}
                      onChange={() => {
                        if (headerChecked()) emitSelection([]);
                        else emitSelection(allSelectable());
                      }}
                    />
                    <Show when={props.rowSelection?.selections?.length}>
                      <Popover
                        minWidth={128}
                        panelClass="w-32 p-1"
                        trigger={
                          <button
                            type="button"
                            class="cursor-pointer border-none bg-transparent px-0.5 text-[9px] text-layer-content/40 outline-none"
                            aria-label="选择操作"
                          >
                            ▾
                          </button>
                        }
                      >
                        <div class="flex flex-col">
                          <For each={props.rowSelection?.selections}>
                            {(selection) => {
                              const key = typeof selection === 'string' ? selection : selection.key;
                              const text =
                                typeof selection === 'string'
                                  ? key === SELECTION_ALL
                                    ? '全选'
                                    : key === SELECTION_INVERT
                                      ? '反选'
                                      : '清空'
                                  : selection.text;
                              return (
                                <button
                                  type="button"
                                  class="menu-item"
                                  onClick={() => selectionAction(key)}
                                >
                                  {text}
                                </button>
                              );
                            }}
                          </For>
                        </div>
                      </Popover>
                    </Show>
                  </div>
                </th>
              </Show>
              <For each={props.columns}>
                {(column, index) => {
                  const key = colKey(column, index());
                  const sorted = () => sort()?.key === key;
                  return (
                    <th
                      class={clsx(
                        column.align === 'right' && 'cell-right',
                        column.align === 'center' && 'cell-center',
                        column.fixed === 'right' && 'cell-sticky-right',
                        column.className,
                      )}
                      style={column.width ? { width: typeof column.width === 'number' ? `${column.width}px` : column.width } : undefined}
                    >
                      <div class="flex items-center gap-1.5">
                        <Show
                          when={column.sorter}
                          fallback={<span>{column.title}</span>}
                        >
                          <button
                            type="button"
                            class="table-sort"
                            data-active={sorted() ? 'true' : 'false'}
                            onClick={() => {
                              const current = sort();
                              if (!current || current.key !== key) {
                                setSort({ key, order: 'ascend' });
                              } else if (current.order === 'ascend') {
                                setSort({ key, order: 'descend' });
                              } else {
                                setSort(null);
                              }
                            }}
                          >
                            <span>{column.title}</span>
                            <span class="text-[9px]">
                              {sorted() ? (sort()!.order === 'ascend' ? '▲' : '▼') : '⇅'}
                            </span>
                          </button>
                        </Show>
                        <Show when={column.filters?.length}>
                          <Popover
                            open={filterOpen() === key}
                            onOpenChange={(open) => setFilterOpen(open ? key : null)}
                            minWidth={192}
                            panelClass="w-48 p-2"
                            trigger={
                              <button
                                type="button"
                                class={clsx(
                                  'inline-flex cursor-pointer border-none bg-transparent p-0 outline-none',
                                  (filters()[key]?.length ?? 0) > 0
                                    ? 'text-primary'
                                    : 'text-layer-content/40',
                                )}
                                aria-label="筛选"
                              >
                                <FilterIcon class="h-3.5 w-3.5" />
                              </button>
                            }
                          >
                            <div class="flex flex-col">
                              <For each={column.filters}>
                                {(option) => {
                                  const values = () => filters()[key] ?? [];
                                  const checked = () => values().some((value) => String(value) === String(option.value));
                                  return (
                                    <label class="filter-option">
                                      <input
                                        type={column.filterMultiple === false ? 'radio' : 'checkbox'}
                                        class="table-check"
                                        checked={checked()}
                                        onChange={() => {
                                          if (column.filterMultiple === false) {
                                            setFilters({ ...filters(), [key]: checked() ? [] : [option.value] });
                                            return;
                                          }
                                          const next = checked()
                                            ? values().filter((value) => String(value) !== String(option.value))
                                            : [...values(), option.value];
                                          setFilters({ ...filters(), [key]: next });
                                        }}
                                      />
                                      <span class="min-w-0 truncate">{option.text}</span>
                                    </label>
                                  );
                                }}
                              </For>
                              <div class="mt-1 flex items-center justify-end gap-1 border-t border-layer-content/10 pt-2">
                                <button
                                  type="button"
                                  class="btn btn-ghost btn-sm"
                                  onClick={() => setFilters({ ...filters(), [key]: [] })}
                                >
                                  重置
                                </button>
                                <button
                                  type="button"
                                  class="btn btn-primary btn-sm"
                                  onClick={() => setFilterOpen(null)}
                                >
                                  确定
                                </button>
                              </div>
                            </div>
                          </Popover>
                        </Show>
                      </div>
                    </th>
                  );
                }}
              </For>
            </tr>
          </thead>
          <tbody>
            <Show
              when={pageRows().length > 0}
              fallback={
                <tr>
                  <td class="table-empty" colSpan={props.columns.length + selectionCount()}>
                    {props.locale?.emptyText ?? '暂无数据'}
                  </td>
                </tr>
              }
            >
              <For each={pageRows()}>
                {(record, index) => {
                  const absoluteIndex = () => (page() - 1) * pageSize() + index();
                  const key = () => rowKey(record, absoluteIndex());
                  const rowProps = () => props.onRow?.(record, absoluteIndex());
                  return (
                    <tr
                      class={clsx(
                        isSelected(key()) && 'table-row-selected',
                        rowProps()?.onClick && 'table-row-clickable',
                      )}
                      onClick={(event) => rowProps()?.onClick?.(event)}
                    >
                      <Show when={props.rowSelection}>
                        <td onClick={(event) => event.stopPropagation()}>
                          <input
                            type="checkbox"
                            class="table-check"
                            checked={isSelected(key())}
                            disabled={props.rowSelection?.getCheckboxProps?.(record).disabled}
                            onChange={() => toggleRow(record, absoluteIndex())}
                          />
                        </td>
                      </Show>
                      <For each={props.columns}>
                        {(column) => {
                          const cell = column.onCell?.(record, absoluteIndex()) ?? {};
                          const value = getPath(record, column.dataIndex);
                          return (
                            <Show when={cell.rowSpan !== 0}>
                              <td
                                class={clsx(
                                  column.align === 'right' && 'cell-right',
                                  column.align === 'center' && 'cell-center',
                                  column.fixed === 'right' && 'cell-sticky-right',
                                  column.className,
                                )}
                                rowSpan={cell.rowSpan && cell.rowSpan > 1 ? cell.rowSpan : undefined}
                                colSpan={cell.colSpan && cell.colSpan > 1 ? cell.colSpan : undefined}
                              >
                                <Show
                                  when={column.ellipsis}
                                  fallback={
                                    column.render
                                      ? column.render(value, record, absoluteIndex())
                                      : (value as JSX.Element)
                                  }
                                >
                                  <span
                                    class="cell-ellipsis"
                                    title={typeof value === 'string' ? value : undefined}
                                  >
                                    {column.render
                                      ? column.render(value, record, absoluteIndex())
                                      : (value as JSX.Element)}
                                  </span>
                                </Show>
                              </td>
                            </Show>
                          );
                        }}
                      </For>
                    </tr>
                  );
                }}
              </For>
            </Show>
          </tbody>
        </table>
      </div>

      <Show when={pagination()}>
        <div class="pager">
          <div>
            {pagination()?.showTotal?.(total(), [
              total() === 0 ? 0 : (page() - 1) * pageSize() + 1,
              Math.min(page() * pageSize(), total()),
            ])}
          </div>
          <div class="flex flex-wrap items-center gap-3">
            <Show when={pagination()?.showSizeChanger}>
              <label class="pager-size">
                <select
                  class="pager-select"
                  value={String(pageSize())}
                  onChange={(event) => changePageSize(Number(event.currentTarget.value))}
                >
                  <For each={pagination()?.pageSizeOptions ?? [10, 20, 50]}>
                    {(option) => <option value={String(option)}>{option} 条/页</option>}
                  </For>
                </select>
              </label>
            </Show>
            <div class="pager-pages">
              <button
                type="button"
                class="pager-btn"
                disabled={page() <= 1}
                onClick={() => changePage(page() - 1)}
              >
                ‹
              </button>
              <For each={pageNumbers(page(), totalPages())}>
                {(item) =>
                  item === '...' ? (
                    <span class="px-1 text-layer-content/40">…</span>
                  ) : (
                    <button
                      type="button"
                      class="pager-btn"
                      data-active={item === page() ? 'true' : 'false'}
                      onClick={() => changePage(item)}
                    >
                      {item}
                    </button>
                  )
                }
              </For>
              <button
                type="button"
                class="pager-btn"
                disabled={page() >= totalPages()}
                onClick={() => changePage(page() + 1)}
              >
                ›
              </button>
            </div>
          </div>
        </div>
      </Show>

      <Show when={props.loading}>
        <div class="absolute inset-0 z-10 grid place-items-center bg-layer/40">
          <SpinnerIcon class="h-6 w-6 animate-spin text-primary" />
        </div>
      </Show>
    </div>
  );
}

export const Table = Object.assign(TableImpl, {
  SELECTION_ALL,
  SELECTION_INVERT,
  SELECTION_NONE,
});
