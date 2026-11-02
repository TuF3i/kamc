<script setup lang="ts">
import { computed, ref } from 'vue';

/** 单元格渲染类型（配置驱动，不做跨层插槽透传） */
export type CellType =
  /** 默认：原样文本（null → —） */
  | 'text'
  /** StatusBadge（值须为状态串；label 从 labelMap 取或原样） */
  | 'status'
  /** fmtTime 时间格式化 */
  | 'time'
  /** 等宽 code 展示 */
  | 'code';

export interface Column {
  /** 字段键（行对象属性名；嵌套用 'a.b'；数组索引用 '0'/'1'…） */
  key: string;
  label: string;
  sortable?: boolean;
  width?: string | number;
  align?: 'left' | 'center' | 'right';
  cellType?: CellType;
  /** status 徽章的显示文案映射（值前缀匹配） */
  labelMap?: Record<string, string>;
}

const props = withDefaults(
  defineProps<{
    rows: unknown[];
    cols: Column[];
    /** 行唯一键（属性名或取值函数） */
    rowKey?: string | ((row: unknown) => string | number);
    loading?: boolean;
    /** 提供即启用客户端搜索（在指定列文本中 includes 匹配） */
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

function field(row: unknown, path: string): unknown {
  let v: unknown = row;
  for (const seg of path.split('.')) {
    if (v == null || typeof v !== 'object') return v;
    v = (v as Record<string, unknown>)[seg];
  }
  return v;
}

// 中文locale排序（el-table 内建排序，比较函数按列生成）
function makeSorter(path: string) {
  return (a: unknown, b: unknown) => {
    const va = field(a, path);
    const vb = field(b, path);
    if (va == null) return -1;
    if (vb == null) return 1;
    if (typeof va === 'number' && typeof vb === 'number') return va - vb;
    return String(va).localeCompare(String(vb), 'zh-CN');
  };
}

const view = computed(() => {
  const q = search.value.trim().toLowerCase();
  if (!q || !props.searchKeys?.length) return props.rows;
  return props.rows.filter((r) =>
    props.searchKeys!.some((k) => String(field(r, k) ?? '').toLowerCase().includes(q)),
  );
});

const tagType = (raw: string): 'success' | 'warning' | 'danger' | 'info' => {
  const s = raw.toLowerCase();
  if (['online', 'active', 'enabled', 'allowed', 'success', 'true'].some((p) => s.startsWith(p))) return 'success';
  if (['pending', 'rotating', 'warning'].some((p) => s.startsWith(p))) return 'warning';
  if (s.startsWith('denied') || s.startsWith('false')) return 'danger';
  return 'info';
};

function badgeLabel(col: Column, raw: string): string {
  if (col.labelMap) {
    for (const [prefix, label] of Object.entries(col.labelMap)) {
      if (raw.toLowerCase().startsWith(prefix.toLowerCase())) return label;
    }
  }
  return raw;
}

/** status 列统一转字符串（boolean → allowed/denied 语义由 labelMap 决定） */
function statusText(col: Column, v: unknown): string {
  if (typeof v === 'boolean') return v ? 'allowed' : 'denied';
  return String(v ?? '');
}

import { fmtTime } from '@/utils/format';

function onRowClick(row: unknown): void {
  emit('row-click', row);
}
</script>

<template>
  <div class="table-wrap">
    <div v-if="searchKeys?.length || $slots.toolbar" class="table-toolbar">
      <el-input
        v-if="searchKeys?.length"
        v-model="search"
        class="search"
        :placeholder="searchPlaceholder"
        clearable
        style="max-width: 260px"
      />
      <slot name="toolbar" />
    </div>

    <el-table :data="view" v-loading="loading" stripe @row-click="onRowClick">
      <el-table-column
        v-for="col in cols"
        :key="col.key"
        :prop="col.key"
        :label="col.label"
        :width="col.width"
        :align="col.align ?? 'left'"
        :sortable="!!col.sortable"
        :sort-method="col.sortable ? makeSorter(col.key) : undefined"
      >
        <template #default="{ row }">
          <template v-if="(col.cellType ?? 'text') === 'status'">
            <el-tag
              :type="tagType(statusText(col, field(row, col.key)))"
              effect="light"
              round
              size="small"
              disable-transitions
            >
              {{ badgeLabel(col, statusText(col, field(row, col.key))) }}
            </el-tag>
          </template>
          <template v-else-if="col.cellType === 'time'">
            {{ fmtTime(String(field(row, col.key) ?? '')) }}
          </template>
          <template v-else-if="col.cellType === 'code'">
            <code class="mono cell-code">{{ field(row, col.key) ?? '—' }}</code>
          </template>
          <template v-else>
            {{ field(row, col.key) ?? '—' }}
          </template>
        </template>
      </el-table-column>
      <!--
        操作列是唯一的插槽：el-table 的 cells 不做父插槽透传（嵌套作用域插槽
        经 el-table-column 缓冲会在 patch 期触发 "parentNode of null" 崩溃，
        见 component/DataTable 注释与二分记录）。操作按钮经 slotProps 原样传出。
      -->
      <el-table-column v-if="$slots.actions" label="操作" :width="160">
        <template #default="{ row, $index }">
          <div class="row-actions" @click.stop>
            <slot name="actions" :row="row" :index="$index" />
          </div>
        </template>
      </el-table-column>
      <template #empty>
        <el-empty :description="emptyText" :image-size="72" />
      </template>
    </el-table>
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
}

:deep(.el-table) {
  --el-table-header-bg-color: #f2f5f9;
}

.cell-code {
  font-size: 12px;
}

.row-actions {
  display: flex;
  gap: 6px;
  align-items: center;
  white-space: nowrap;
}
</style>
