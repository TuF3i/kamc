<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import Banner from '@/components/Banner.vue';
import DataTable from '@/components/DataTable.vue';
import Modal from '@/components/Modal.vue';
import StatusBadge from '@/components/StatusBadge.vue';
import LabeledInput from '@/components/LabeledInput.vue';
import { createCluster, deleteCluster, listClusters } from '@/api/clusters';
import type { Cluster } from '@/api/types';
import { fmtTime } from '@/utils/format';
import { useGrowlStore } from '@/stores/growl';

const growl = useGrowlStore();
const router = useRouter();

const rows = ref<Cluster[]>([]);
const loading = ref(true);
const error = ref('');

// 创建弹窗
const createOpen = ref(false);
const form = ref({ id: '', name: '', desc: '' });
const creating = ref(false);

// 删除确认
const deleting = ref<Cluster | null>(null);
const deleteConfirmText = ref('');
const deletingBusy = ref(false);

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    rows.value = await listClusters();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

async function submitCreate(): Promise<void> {
  if (!form.value.id.trim() || !form.value.name.trim()) {
    growl.error('集群 ID 与名称必填');
    return;
  }
  creating.value = true;
  try {
    await createCluster({
      id: form.value.id.trim(),
      name: form.value.name.trim(),
      desc: form.value.desc.trim() || undefined,
    });
    growl.success(`集群 ${form.value.id} 已注册`);
    createOpen.value = false;
    form.value = { id: '', name: '', desc: '' };
    await load();
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  } finally {
    creating.value = false;
  }
}

async function submitDelete(): Promise<void> {
  if (!deleting.value) return;
  deletingBusy.value = true;
  try {
    await deleteCluster(deleting.value.id);
    growl.success(`集群 ${deleting.value.id} 已注销`);
    deleting.value = null;
    deleteConfirmText.value = '';
    await load();
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  } finally {
    deletingBusy.value = false;
  }
}

function openDetail(row: unknown): void {
  const c = row as Cluster;
  router.push(`/clusters/${encodeURIComponent(c.id)}`);
}

onMounted(load);
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h1>集群</h1>
        <div class="subtitle">目标集群注册与管理；接入状态来自反向隧道</div>
      </div>
      <button class="btn btn-primary" @click="createOpen = true">＋ 注册集群</button>
    </div>

    <Banner v-if="error" type="error" closable @close="error = ''">{{ error }}</Banner>

    <DataTable
      :rows="rows"
      :cols="[
        { key: 'id', label: '集群 ID', sortable: true },
        { key: 'name', label: '名称', sortable: true },
        { key: 'desc', label: '描述' },
        { key: 'status', label: '接入状态', sortable: true, width: '110px' },
        { key: 'created_at', label: '注册时间', sortable: true, width: '170px' },
      ]"
      row-key="id"
      :loading="loading"
      :search-keys="['id', 'name', 'desc']"
      empty-text="尚无注册集群"
      @row-click="openDetail"
    >
      <template #col-status="{ row }">
        <StatusBadge :status="(row as Cluster).status" />
      </template>
      <template #col-created_at="{ row }">
        {{ fmtTime((row as Cluster).created_at) }}
      </template>
      <template #actions="{ row }">
        <RouterLink class="btn btn-secondary btn-sm" :to="`/clusters/${encodeURIComponent((row as Cluster).id)}`">
          详情
        </RouterLink>
        <button class="btn btn-danger-ghost btn-sm" @click="deleting = row as Cluster">注销</button>
      </template>
    </DataTable>

    <Modal :open="createOpen" title="注册集群" @close="createOpen = false">
      <LabeledInput
        v-model="form.id"
        label="集群 ID"
        required
        mono
        hint="唯一标识，同时作为 Casbin dom（如 prod-bj）；注册后不可修改"
      />
      <LabeledInput v-model="form.name" label="名称" required hint="展示名（如 北京生产集群）" />
      <LabeledInput v-model="form.desc" label="描述" hint="可选" />
      <template #footer>
        <button class="btn btn-secondary" @click="createOpen = false">取消</button>
        <button class="btn btn-primary" :disabled="creating" @click="submitCreate">
          {{ creating ? '注册中…' : '注册' }}
        </button>
      </template>
    </Modal>

    <Modal :open="!!deleting" title="注销集群" width="500px" @close="deleting = null; deleteConfirmText = ''">
      <p style="margin-top: 0">
        即将注销集群 <b class="mono">{{ deleting?.id }}</b
        >。仅删除集群记录本身：已签发的 Agent Token 与 Casbin 策略/绑定<b>不会自动清理</b>，
        如需彻底回收请分别在集群详情与权限页删除。
      </p>
      <p>输入集群 ID 以确认：<code class="mono">{{ deleting?.id }}</code></p>
      <input v-model="deleteConfirmText" class="input mono" placeholder="输入集群 ID" />
      <template #footer>
        <button class="btn btn-secondary" @click="deleting = null; deleteConfirmText = ''">取消</button>
        <button
          class="btn btn-danger"
          :disabled="deleteConfirmText !== deleting?.id || deletingBusy"
          @click="submitDelete"
        >
          {{ deletingBusy ? '注销中…' : '确认注销' }}
        </button>
      </template>
    </Modal>
  </div>
</template>
