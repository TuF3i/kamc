<script setup lang="ts">
import { onMounted, ref } from 'vue';
import Banner from '@/components/Banner.vue';
import DataTable from '@/components/DataTable.vue';
import LabeledInput from '@/components/LabeledInput.vue';
import LabeledSelect from '@/components/LabeledSelect.vue';
import StatusBadge from '@/components/StatusBadge.vue';
import { listAudit } from '@/api/audit';
import type { AuditEvent } from '@/api/types';
import { fmtTime } from '@/utils/format';

const rows = ref<AuditEvent[]>([]);
const loading = ref(true);
const error = ref('');

const fUser = ref('');
const fCluster = ref('');
const fLimit = ref('200');

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    rows.value = await listAudit({
      user: fUser.value.trim() || undefined,
      cluster: fCluster.value.trim() || undefined,
      limit: Number(fLimit.value),
    });
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h1>审计日志</h1>
        <div class="subtitle">数据面访问与登录事件（时间倒序，含拒绝原因；异步批量落库）</div>
      </div>
      <button class="btn btn-secondary" :disabled="loading" @click="load">↻ 刷新</button>
    </div>

    <div class="filter-bar">
      <div style="width: 220px">
        <LabeledInput v-model="fUser" label="用户" placeholder="精确匹配 sub" @keyup.enter="load" />
      </div>
      <div style="width: 220px">
        <LabeledInput v-model="fCluster" label="集群" placeholder="集群 ID" @keyup.enter="load" />
      </div>
      <div style="width: 140px">
        <LabeledSelect
          v-model="fLimit"
          label="条数"
          :options="[
            { label: '100', value: '100' },
            { label: '200', value: '200' },
            { label: '500', value: '500' },
            { label: '1000', value: '1000' },
          ]"
        />
      </div>
      <div style="padding-top: 24px">
        <button class="btn btn-primary" :disabled="loading" @click="load">{{ loading ? '查询中…' : '查询' }}</button>
      </div>
    </div>

    <Banner v-if="error" type="error" closable @close="error = ''">{{ error }}</Banner>

    <DataTable
      :rows="rows"
      :cols="[
        { key: 'timestamp', label: '时间', sortable: true, width: '170px' },
        { key: 'user', label: '用户', sortable: true },
        { key: 'cluster', label: '集群', sortable: true, width: '120px' },
        { key: 'verb', label: '动词', width: '110px' },
        { key: 'path', label: '资源' },
        { key: 'allowed', label: '结果', width: '90px' },
        { key: 'reason', label: '原因', width: '140px' },
        { key: 'latency_ms', label: '耗时', sortable: true, width: '80px', align: 'right' },
      ]"
      row-key="id"
      :loading="loading"
      :search-keys="['user', 'cluster', 'path', 'verb', 'reason']"
      search-placeholder="过滤（客户端）"
      empty-text="暂无审计事件"
    >
      <template #col-timestamp="{ row }">
        <span :title="fmtTime((row as AuditEvent).timestamp)">{{ fmtTime((row as AuditEvent).timestamp) }}</span>
      </template>
      <template #col-user="{ row }">
        <span class="mono">{{ (row as AuditEvent).user || '—' }}</span>
      </template>
      <template #col-path="{ row }">
        <span class="mono">{{ (row as AuditEvent).path }}</span>
      </template>
      <template #col-allowed="{ row }">
        <StatusBadge :status="(row as AuditEvent).allowed ? 'allowed' : 'denied'" :label="(row as AuditEvent).allowed ? '允许' : '拒绝'" />
      </template>
      <template #col-reason="{ row }">
        <span class="mono reason">{{ (row as AuditEvent).reason }}</span>
      </template>
    </DataTable>
  </div>
</template>

<style lang="scss" scoped>
.filter-bar {
  display: flex;
  gap: 14px;
  margin-bottom: 16px;
  flex-wrap: wrap;
  align-items: flex-start;
}

.reason {
  font-size: 12px;
  color: $text-muted;
}
</style>
