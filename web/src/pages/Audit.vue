<script setup lang="ts">
import { onMounted, ref } from 'vue';
import Banner from '@/components/Banner.vue';
import DataTable from '@/components/DataTable.vue';
import LabeledInput from '@/components/LabeledInput.vue';
import LabeledSelect from '@/components/LabeledSelect.vue';
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
      <el-button :loading="loading" @click="load">↻ 刷新</el-button>
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
        <el-button type="primary" :loading="loading" @click="load">查询</el-button>
      </div>
    </div>

    <Banner v-if="error" type="error" closable @close="error = ''">{{ error }}</Banner>

    <DataTable
      :rows="rows"
      :cols="[
        { key: 'timestamp', label: '时间', sortable: true, width: '170px', cellType: 'time' },
        { key: 'user', label: '用户', sortable: true, cellType: 'code' },
        { key: 'cluster', label: '集群', sortable: true, width: '120px' },
        { key: 'verb', label: '动词', width: '110px' },
        { key: 'path', label: '资源', cellType: 'code' },
        { key: 'allowed', label: '结果', width: '90px', cellType: 'status',
          labelMap: { allowed: '允许', denied: '拒绝' } },
        { key: 'reason', label: '原因', width: '150px', cellType: 'code' },
        { key: 'latency_ms', label: '耗时', sortable: true, width: '80px', align: 'right' },
      ]"
      row-key="id"
      :loading="loading"
      :search-keys="['user', 'cluster', 'path', 'verb', 'reason']"
      search-placeholder="过滤（客户端）"
      empty-text="暂无审计事件"
    >
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
