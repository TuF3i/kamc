<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import Banner from '@/components/Banner.vue';
import DataTable from '@/components/DataTable.vue';
import Modal from '@/components/Modal.vue';
import LabeledInput from '@/components/LabeledInput.vue';
import { createCluster, deleteCluster, listClusters } from '@/api/clusters';
import type { Cluster } from '@/api/types';
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
  router.push(`/clusters/${encodeURIComponent((row as Cluster).id)}`);
}

function openDelete(row: unknown): void {
  deleting.value = row as Cluster;
  deleteConfirmText.value = '';
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
      <el-button type="primary" @click="createOpen = true">＋ 注册集群</el-button>
    </div>

    <Banner v-if="error" type="error" closable @close="error = ''">{{ error }}</Banner>

    <DataTable
      :rows="rows"
      :cols="[
        { key: 'id', label: '集群 ID', sortable: true },
        { key: 'name', label: '名称', sortable: true },
        { key: 'desc', label: '描述' },
        { key: 'status', label: '接入状态', sortable: true, width: '110px', cellType: 'status' },
        { key: 'created_at', label: '注册时间', sortable: true, width: '170px', cellType: 'time' },
      ]"
      row-key="id"
      :loading="loading"
      :search-keys="['id', 'name', 'desc']"
      empty-text="尚无注册集群"
      @row-click="openDetail"
    >
      <template #actions="{ row }">
        <el-button size="small" @click.stop="openDetail(row)">详情</el-button>
        <el-button size="small" type="danger" text @click.stop="openDelete(row)">注销</el-button>
      </template>
    </DataTable>

    <Modal :open="createOpen" title="注册集群" width="480px" @close="createOpen = false">
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
        <el-button @click="createOpen = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">注册</el-button>
      </template>
    </Modal>

    <Modal :open="!!deleting" title="注销集群" width="500px" @close="deleting = null">
      <p style="margin-top: 0">
        即将注销集群 <b class="mono">{{ deleting?.id }}</b
        >。仅删除集群记录本身：已签发的 Agent Token 与 Casbin 策略/绑定<b>不会自动清理</b>，
        如需彻底回收请分别在集群详情与权限页删除。
      </p>
      <p>输入集群 ID 以确认：<code class="mono">{{ deleting?.id }}</code></p>
      <el-input v-model="deleteConfirmText" class="mono" placeholder="输入集群 ID" />
      <template #footer>
        <el-button @click="deleting = null">取消</el-button>
        <el-button
          type="danger"
          :disabled="deleteConfirmText !== deleting?.id || deletingBusy"
          :loading="deletingBusy"
          @click="submitDelete"
        >
          确认注销
        </el-button>
      </template>
    </Modal>
  </div>
</template>
