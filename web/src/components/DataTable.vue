<script setup lang="ts">
import { computed, ref } from 'vue';

export interface Column {
  /** 字段键（行对象属性名；嵌套用 'a.b'） */
  key: string;
  label: string;
  sortable?: boolean;
  width?: string;
  align?: 'left' | 'right';
  /** 原样渲染（不转义），配合受控内容使用 */
  raw?: boolean;
}

const props = withDefaults(
  defineProps<{
    rows: unknown[];
    cols: Column[];
    /** 行唯一键（属性名或取值函数） */
    rowKey?: string | ((row: unknown) => string | number);
    loading?: boolean;
    /** 提供即启用客户端搜索（在前几列文本中 includes 匹配） */
    searchKeys?: string[];
    searchPlaceholder?: string;
    emptyText?: string;
  }>(),
  {
    rowKey: '',
    loading: false,
    searchPlaceholder: '搜索',
    emptyText: '暂无数据',
  },
);

const emit = defineEmits<{ 'row-click': [row: unknown] }>();

const search = ref('');
const sortKey = ref('');
const sortAsc = ref(true);

function field(row: unknown, path: string): unknown {
  let v: unknown = row;
  for (const seg of path.split('.')) {
    if (v == null || typeof v !== 'object') return v;
    v = (v as Record<string, unknown>)[seg];
  }
  return v;
}

function keyOf(row: unknown, idx: number): string | number {
  if (typeof props.rowKey === 'function') return props.rowKey(row);
  if (props.rowKey) return String(field(row, props.rowKey) ?? idx);
  return idx;
}

const view = computed(() => {
  let out = props.rows;
  const q = search.value.trim().toLowerCase();
  if (q && props.searchKeys?.length) {
    out = out.filter((r) =>
      props.searchKeys!.some((k) => String(field(r, k) ?? '').toLowerCase().includes(q)),
    );
  }
  if (sortKey.value) {
    const dir = sortAsc.value ? 1 : -1;
    const key = sortKey.value;
    out = [...out].sort((a, b) => {
      const va = field(a, key);
      const vb = field(b, key);
      if (va == null) return -dir;
      if (vb == null) return dir;
      if (typeof va === 'number' && typeof vb === 'number') return (va - vb) * dir;
      return String(va).localeCompare(String(vb), 'zh-CN') * dir;
    });
  }
  return out;
});

function toggleSort(col: Column): void {
  if (!col.sortable) return;
  if (sortKey.value === col.key) {
    if (sortAsc.value) {
      sortAsc.value = false;
    } else {
      sortKey.value = '';
    }
  } else {
    sortKey.value = col.key;
    sortAsc.value = true;
  }
}
</script>

<template>
  <div class="table-wrap">
    <div v-if="searchKeys?.length || $slots.toolbar" class="table-toolbar">
      <input
        v-if="searchKeys?.length"
        v-model="search"
        class="input search"
        type="search"
        :placeholder="searchPlaceholder"
      />
      <slot name="toolbar" />
    </div>

    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th
              v-for="col in cols"
              :key="col.key"
              :style="{ width: col.width, textAlign: col.align ?? 'left' }"
              :class="{ sortable: col.sortable, sorted: sortKey === col.key }"
              @click="toggleSort(col)"
            >
              {{ col.label }}
              <span v-if="col.sortable" class="sort-mark">
                {{ sortKey === col.key ? (sortAsc ? '↑' : '↓') : '⇅' }}
              </span>
            </th>
            <th v-if="$slots.actions" class="actions-col">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td :colspan="cols.length + ($slots.actions ? 1 : 0)" class="empty">加载中…</td>
          </tr>
          <tr v-else-if="!view.length">
            <td :colspan="cols.length + ($slots.actions ? 1 : 0)" class="empty">{{ emptyText }}</td>
          </tr>
          <tr
            v-for="(row, i) in view"
            :key="keyOf(row, i)"
            :class="{ clickable: $attrs['onRowClick'] !== undefined }"
            @click="emit('row-click', row)"
          >
            <td v-for="col in cols" :key="col.key" :style="{ textAlign: col.align ?? 'left' }">
              <slot :name="'col-' + col.key" :row="row" :value="field(row, col.key)">
                {{ field(row, col.key) ?? '—' }}
              </slot>
            </td>
            <td v-if="$slots.actions" class="actions-col" @click.stop>
              <slot name="actions" :row="row" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.table-wrap {
  background: $card-bg;
  border: 1px solid $border;
  border-radius: 6px;
  overflow: hidden;
}

.table-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-bottom: 1px solid $border;

  .search {
    max-width: 260px;
  }
}

.table-scroll {
  overflow-x: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

th {
  text-align: left;
  padding: 9px 14px;
  background: #f7f9fb;
  border-bottom: 1px solid $border;
  color: $text-muted;
  font-weight: 600;
  font-size: 12px;
  white-space: nowrap;
  user-select: none;

  &.sortable {
    cursor: pointer;

    &:hover {
      color: $brand;
    }
  }

  &.sorted {
    color: $brand;
  }
}

.sort-mark {
  font-size: 10px;
  margin-left: 2px;
  opacity: 0.8;
}

td {
  padding: 9px 14px;
  border-bottom: 1px solid #edf1f5;
  vertical-align: middle;
  word-break: break-all;
}

tr:last-child td {
  border-bottom: none;
}

tbody tr.clickable {
  cursor: pointer;

  &:hover {
    background: #f6f9fd;
  }
}

.empty {
  text-align: center;
  color: $text-muted;
  padding: 32px 0 !important;
}

.actions-col {
  white-space: nowrap;
  width: 1%;
}
</style>
