<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import Banner from '@/components/Banner.vue';
import DataTable from '@/components/DataTable.vue';
import Modal from '@/components/Modal.vue';
import LabeledInput from '@/components/LabeledInput.vue';
import StatusBadge from '@/components/StatusBadge.vue';
import { deleteScript, listScripts, saveScript } from '@/api/scripts';
import type { AuthScript } from '@/api/types';
import { fmtTime } from '@/utils/format';
import { useGrowlStore } from '@/stores/growl';

const growl = useGrowlStore();
const router = useRouter();

const rows = ref<AuthScript[]>([]);
const loading = ref(true);
const error = ref('');

// 新建
const createOpen = ref(false);
const newName = ref('');
const creating = ref(false);

// 删除
const deleting = ref<AuthScript | null>(null);

const NAME_RE = /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$/;

async function load(): Promise<void> {
  loading.value = true;
  try {
    rows.value = await listScripts(true);
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

async function submitCreate(): Promise<void> {
  const name = newName.value.trim();
  if (!NAME_RE.test(name)) {
    growl.error('脚本名：字母/数字开头，可含 . _ -，最长 128');
    return;
  }
  creating.value = true;
  try {
    // 以示例模板 upsert，语法校验通过后进入编辑页
    await saveScript(name, TEMPLATE, true);
    growl.success(`脚本 ${name} 已创建`);
    createOpen.value = false;
    newName.value = '';
    router.push(`/scripts/${encodeURIComponent(name)}`);
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  } finally {
    creating.value = false;
  }
}

async function toggleEnabled(row: AuthScript): Promise<void> {
  try {
    await saveScript(row.name, row.content, !row.enabled);
    row.enabled = !row.enabled;
    growl.success(row.enabled ? `脚本 ${row.name} 已启用` : `脚本 ${row.name} 已禁用（登录不再执行，可用于止血）`);
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  }
}

async function submitDelete(): Promise<void> {
  if (!deleting.value) return;
  try {
    await deleteScript(deleting.value.name);
    growl.success(`脚本 ${deleting.value.name} 已删除`);
    deleting.value = null;
    await load();
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  }
}

function openEdit(row: unknown): void {
  router.push(`/scripts/${encodeURIComponent((row as AuthScript).name)}`);
}

onMounted(load);

const TEMPLATE = `-- 登录期权限绑定（按 Authentik 组映射 kamc 角色；fail-closed：报错即拒绝登录）
-- 可用桥接：kamc.list_clusters / cluster_exists / list_bindings / add_binding /
--   remove_binding / has_policy / add_policy / remove_policy / list_policies
for _, g in ipairs(claims.groups or {}) do
  if g == "dev-team" then
    if not kamc.has_policy("role:devs", "*", "dev/*", "get|list|watch") then
      kamc.add_policy("role:devs", "*", "dev/*", "get|list|watch")
    end
    kamc.add_binding(claims.sub, "role:devs", "*")
  end
end
`;
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h1>绑定脚本</h1>
        <div class="subtitle">
          Lua 脚本在每次登录 / Token 兑换时按名称排序执行，把登录 claims 计算为 Casbin 绑定与策略；
          <b>fail-closed</b>：脚本失败拒绝该次登录（403），禁用脚本即可恢复
        </div>
      </div>
      <button class="btn btn-primary" @click="createOpen = true">＋ 新建脚本</button>
    </div>

    <Banner v-if="error" type="error" closable @close="error = ''">{{ error }}</Banner>

    <DataTable
      :rows="rows"
      :cols="[
        { key: 'name', label: '脚本名（执行顺序）', sortable: true },
        { key: 'enabled', label: '状态', width: '110px' },
        { key: 'updated_at', label: '更新时间', sortable: true, width: '170px' },
      ]"
      row-key="name"
      :loading="loading"
      :search-keys="['name']"
      empty-text="尚无脚本 —— 登录时直接签发，不做绑定计算"
      @row-click="openEdit"
    >
      <template #col-name="{ row }">
        <span class="mono">{{ (row as AuthScript).name }}</span>
      </template>
      <template #col-enabled="{ row }">
        <StatusBadge :status="(row as AuthScript).enabled ? 'enabled' : 'disabled'" :label="(row as AuthScript).enabled ? '启用' : '禁用'" />
      </template>
      <template #col-updated_at="{ row }">
        {{ fmtTime((row as AuthScript).updated_at) }}
      </template>
      <template #actions="{ row }">
        <button class="btn btn-secondary btn-sm" @click="toggleEnabled(row as AuthScript)">
          {{ (row as AuthScript).enabled ? '禁用' : '启用' }}
        </button>
        <button class="btn btn-danger-ghost btn-sm" @click="deleting = row as AuthScript">删除</button>
      </template>
    </DataTable>

    <Modal :open="createOpen" title="新建绑定脚本" width="480px" @close="createOpen = false">
      <LabeledInput
        v-model="newName"
        label="脚本名"
        required
        mono
        placeholder="10-bind-devs"
        hint="按名称排序执行（如 10- 前缀控制顺序）；字母/数字开头，可含 . _ -"
      />
      <p class="muted-note">创建后进入编辑器，预填示例模板（按 Authentik 组绑定角色）。</p>
      <template #footer>
        <button class="btn btn-secondary" @click="createOpen = false">取消</button>
        <button class="btn btn-primary" :disabled="creating" @click="submitCreate">
          {{ creating ? '创建中…' : '创建' }}
        </button>
      </template>
    </Modal>

    <Modal :open="!!deleting" title="删除脚本" width="460px" @close="deleting = null">
      <p style="margin-top: 0">
        确认删除脚本 <b class="mono">{{ deleting?.name }}</b
        >？之后登录不再执行该脚本（已产生的绑定/策略保留）。
      </p>
      <template #footer>
        <button class="btn btn-secondary" @click="deleting = null">取消</button>
        <button class="btn btn-danger" @click="submitDelete">删除</button>
      </template>
    </Modal>
  </div>
</template>

<style scoped>
.muted-note {
  color: #76838f;
  font-size: 12.5px;
}
</style>
