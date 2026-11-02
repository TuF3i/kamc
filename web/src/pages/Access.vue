<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { ElMessageBox } from 'element-plus';
import Banner from '@/components/Banner.vue';
import DataTable from '@/components/DataTable.vue';
import Modal from '@/components/Modal.vue';
import LabeledInput from '@/components/LabeledInput.vue';
import { addBinding, addPolicy, listBindings, listPolicies, removeBinding, removePolicy, revokeUser } from '@/api/access';
import type { BindingRow, PolicyRow } from '@/api/types';
import { useGrowlStore } from '@/stores/growl';

const growl = useGrowlStore();

const tab = ref('bindings');
const tabs = [
  { key: 'bindings', label: '用户绑定（user → role）' },
  { key: 'policies', label: '角色策略（role → 权限）' },
];

const error = ref('');

// —— 绑定 ——
const bindings = ref<BindingRow[]>([]);
const bLoading = ref(false);
const bAddOpen = ref(false);
const bForm = ref({ user: '', role: 'role:', dom: '' });
const bDeleting = ref<BindingRow | null>(null);

// —— 策略 ——
const policies = ref<PolicyRow[]>([]);
const pLoading = ref(false);
const pAddOpen = ref(false);
const pForm = ref({ sub: 'role:', dom: '', obj: '', act: '' });
const pDeleting = ref<PolicyRow | null>(null);

// —— 踢出用户 ——
const revokeSub = ref('');
const revoking = ref(false);

async function loadBindings(): Promise<void> {
  bLoading.value = true;
  try {
    bindings.value = await listBindings();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    bLoading.value = false;
  }
}

async function loadPolicies(): Promise<void> {
  pLoading.value = true;
  try {
    policies.value = await listPolicies();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    pLoading.value = false;
  }
}

async function submitBinding(): Promise<void> {
  const { user, role, dom } = bForm.value;
  if (!user.trim() || !role.trim() || !dom.trim()) {
    growl.error('user / role / dom 均必填');
    return;
  }
  try {
    await addBinding({ user: user.trim(), role: role.trim(), dom: dom.trim() });
    growl.success('绑定已添加');
    bAddOpen.value = false;
    bForm.value = { user: '', role: 'role:', dom: '' };
    await loadBindings();
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  }
}

async function submitPolicy(): Promise<void> {
  const { sub, dom, obj, act } = pForm.value;
  if (!sub.trim() || !dom.trim() || !obj.trim() || !act.trim()) {
    growl.error('四项均必填');
    return;
  }
  try {
    await addPolicy({ sub: sub.trim(), dom: dom.trim(), obj: obj.trim(), act: act.trim() });
    growl.success('策略已添加');
    pAddOpen.value = false;
    pForm.value = { sub: 'role:', dom: '', obj: '', act: '' };
    await loadPolicies();
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  }
}

async function delBinding(): Promise<void> {
  if (!bDeleting.value) return;
  const [user, role, dom] = bDeleting.value;
  try {
    await removeBinding({ user, role, dom });
    growl.success('绑定已删除');
    bDeleting.value = null;
    await loadBindings();
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  }
}

async function delPolicy(): Promise<void> {
  if (!pDeleting.value) return;
  const [sub, dom, obj, act] = pDeleting.value;
  try {
    await removePolicy({ sub, dom, obj, act });
    growl.success('策略已删除');
    pDeleting.value = null;
    await loadPolicies();
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  }
}

async function doRevoke(): Promise<void> {
  const sub = revokeSub.value.trim();
  if (!sub) return;
  try {
    await ElMessageBox.confirm(
      `确认踢出用户「${sub}」？其此前签发的全部 Token 立即失效。`,
      '踢出用户',
      { type: 'warning' },
    );
  } catch {
    return;
  }
  revoking.value = true;
  try {
    await revokeUser(sub);
    growl.success(`用户 ${sub} 已踢出`);
    revokeSub.value = '';
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  } finally {
    revoking.value = false;
  }
}

onMounted(() => {
  loadBindings();
  loadPolicies();
});
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h1>权限管理</h1>
        <div class="subtitle">
          Casbin 模型：user --(绑定)--> role --(策略)--> 集群内资源；obj 语法
          <code class="mono">{ns}/{resource}</code> / <code class="mono">cluster/*</code> / <code class="mono">nonresource{path}</code>，act 为 verb 正则片段（锚定匹配）
        </div>
      </div>
    </div>

    <Banner v-if="error" type="error" closable @close="error = ''">{{ error }}</Banner>

    <el-tabs v-model="tab">
      <el-tab-pane label="用户绑定（user → role）" name="bindings">
        <div class="table-head-row">
          <span class="muted-note">共 {{ bindings.length }} 条绑定</span>
          <el-button type="primary" @click="bAddOpen = true">＋ 添加绑定</el-button>
        </div>
        <DataTable
          :rows="bindings"
          :cols="[
            { key: '0', label: '用户', sortable: true },
            { key: '1', label: '角色', sortable: true },
            { key: '2', label: '集群（dom）', sortable: true },
          ]"
          :loading="bLoading"
          :search-keys="['0', '1', '2']"
          empty-text="尚无绑定 —— 用户没有任何集群权限"
        >
          <template #actions="{ row }">
            <el-button size="small" type="danger" text @click="bDeleting = row as BindingRow">删除</el-button>
          </template>
        </DataTable>
      </el-tab-pane>

      <el-tab-pane label="角色策略（role → 权限）" name="policies">
        <div class="table-head-row">
          <span class="muted-note">共 {{ policies.length }} 条策略</span>
          <el-button type="primary" @click="pAddOpen = true">＋ 添加策略</el-button>
        </div>
        <DataTable
          :rows="policies"
          :cols="[
            { key: '0', label: '主体（role）', sortable: true },
            { key: '1', label: '集群（dom）', sortable: true },
            { key: '2', label: '资源（obj）' },
            { key: '3', label: '动词（act）', cellType: 'code' },
          ]"
          :loading="pLoading"
          :search-keys="['0', '1', '2', '3']"
          empty-text="尚无策略"
        >
          <template #actions="{ row }">
            <el-button size="small" type="danger" text @click="pDeleting = row as PolicyRow">删除</el-button>
          </template>
        </DataTable>
      </el-tab-pane>
    </el-tabs>

    <Card title="踢出用户" style="margin-top: 20px">
      <div class="revoke-row">
        <el-input v-model="revokeSub" style="max-width: 320px" placeholder="用户 sub（如 alice）" />
        <el-button type="danger" :loading="revoking" :disabled="!revokeSub.trim()" @click="doRevoke">
          立即吊销全部 Token
        </el-button>
        <span class="muted-note">epoch 递增：该用户已签发 Token 全部即时失效；同秒重新登录获得的新 Token 不受影响</span>
      </div>
    </Card>

    <!-- 添加绑定 -->
    <Modal :open="bAddOpen" title="添加绑定" width="480px" @close="bAddOpen = false">
      <LabeledInput v-model="bForm.user" label="用户（sub / Authentik 用户名）" required mono placeholder="alice" />
      <LabeledInput v-model="bForm.role" label="角色" required mono placeholder="role:developer" />
      <LabeledInput v-model="bForm.dom" label="集群（dom；* 为全部集群）" required mono placeholder="prod-bj" />
      <template #footer>
        <el-button @click="bAddOpen = false">取消</el-button>
        <el-button type="primary" @click="submitBinding">添加</el-button>
      </template>
    </Modal>

    <!-- 添加策略 -->
    <Modal :open="pAddOpen" title="添加策略" width="560px" @close="pAddOpen = false">
      <LabeledInput v-model="pForm.sub" label="主体（role）" required mono placeholder="role:developer" />
      <LabeledInput v-model="pForm.dom" label="集群（dom）" required mono placeholder="prod-bj" hint="支持 * 通配全部集群" />
      <LabeledInput
        v-model="pForm.obj"
        label="资源（obj）"
        required
        mono
        placeholder="dev/*"
        hint="语义资源串：{ns}/{resource}[/{subresource}]、cluster/{resource}、nonresource{path}；支持 * 通配"
      />
      <LabeledInput
        v-model="pForm.act"
        label="动词（act）"
        required
        mono
        placeholder="get|list|watch"
        hint="K8s verb 正则片段，匹配时锚定：delete 不会命中 deletecollection"
      />
      <template #footer>
        <el-button @click="pAddOpen = false">取消</el-button>
        <el-button type="primary" @click="submitPolicy">添加</el-button>
      </template>
    </Modal>

    <!-- 删除确认 -->
    <Modal :open="!!bDeleting" title="删除绑定" width="460px" @close="bDeleting = null">
      <p style="margin-top: 0">
        确认删除绑定
        <b class="mono">{{ bDeleting?.[0] }} → {{ bDeleting?.[1] }} @ {{ bDeleting?.[2] }}</b
        >？该用户将立即失去对应集群内该角色的全部权限。
      </p>
      <template #footer>
        <el-button @click="bDeleting = null">取消</el-button>
        <el-button type="danger" @click="delBinding">删除</el-button>
      </template>
    </Modal>

    <Modal :open="!!pDeleting" title="删除策略" width="520px" @close="pDeleting = null">
      <p style="margin-top: 0">确认删除策略：</p>
      <code class="mono policy-preview">{{ pDeleting?.join(', ') }}</code>
      <template #footer>
        <el-button @click="pDeleting = null">取消</el-button>
        <el-button type="danger" @click="delPolicy">删除</el-button>
      </template>
    </Modal>
  </div>
</template>

<style lang="scss" scoped>
.table-head-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}

.muted-note {
  color: $text-muted;
  font-size: 12.5px;
}

.revoke-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.policy-preview {
  display: block;
  background: var(--el-fill-color-light);
  border: 1px solid $border;
  border-radius: 4px;
  padding: 10px 12px;
  word-break: break-all;
}
</style>
