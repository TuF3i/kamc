<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import Banner from '@/components/Banner.vue';
import Card from '@/components/Card.vue';
import CodeEditor from '@/components/CodeEditor.vue';
import LabeledInput from '@/components/LabeledInput.vue';
import StatusBadge from '@/components/StatusBadge.vue';
import { deleteScript, executeScript, getScript, saveScript } from '@/api/scripts';
import type { AuthScript, ScriptExecuteResult } from '@/api/types';
import { fmtTime } from '@/utils/format';
import { useGrowlStore } from '@/stores/growl';

const route = useRoute();
const router = useRouter();
const growl = useGrowlStore();

const name = computed(() => String(route.params.name));

const original = ref<AuthScript | null>(null);
const content = ref('');
const enabled = ref(true);
const loadErr = ref('');
const saving = ref(false);

// dry-run 面板
const drySub = ref('');
const dryEmail = ref('');
const dryGroups = ref('');
const dryResult = ref<ScriptExecuteResult | null>(null);
const dryErr = ref('');
const dryRunning = ref(false);
const dryRealMode = ref(false); // 关闭 dry-run 将真实落地（危险操作）

async function load(): Promise<void> {
  try {
    original.value = await getScript(name.value);
    content.value = original.value.content;
    enabled.value = original.value.enabled;
  } catch (e) {
    loadErr.value = e instanceof Error ? e.message : String(e);
  }
}

const dirty = computed(
  () => original.value !== null && (content.value !== original.value.content || enabled.value !== original.value.enabled),
);

async function save(): Promise<void> {
  saving.value = true;
  try {
    const saved = await saveScript(name.value, content.value, enabled.value);
    original.value = saved;
    growl.success('已保存（Lua 语法校验通过）');
  } catch (e) {
    growl.error(e instanceof Error ? e.message : '保存失败');
  } finally {
    saving.value = false;
  }
}

async function runDry(): Promise<void> {
  dryErr.value = '';
  dryResult.value = null;
  dryRunning.value = true;
  try {
    dryResult.value = await executeScript(name.value, {
      dryRun: !dryRealMode.value,
      sub: drySub.value.trim() || undefined,
      email: dryEmail.value.trim() || undefined,
      groups: dryGroups.value
        .split(/[,\s]+/)
        .map((g) => g.trim())
        .filter(Boolean),
    });
    growl.success(dryResult.value.dry_run ? 'dry-run 执行成功（未落地）' : '执行成功（已真实落地）');
  } catch (e) {
    dryErr.value = e instanceof Error ? e.message : String(e);
  } finally {
    dryRunning.value = false;
  }
}

async function remove(): Promise<void> {
  if (!confirm(`确认删除脚本 ${name.value}？`)) return;
  try {
    await deleteScript(name.value);
    growl.success('脚本已删除');
    router.push('/scripts');
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  }
}

onMounted(load);
</script>

<template>
  <div class="page">
    <div class="crumbs">
      <RouterLink to="/scripts">绑定脚本</RouterLink>
      <span>/</span>
      <span class="mono">{{ name }}</span>
    </div>
    <div class="page-header">
      <div>
        <h1>
          <span class="mono">{{ name }}</span>
          <StatusBadge
            :status="enabled ? 'enabled' : 'offline'"
            :label="enabled ? '启用' : '禁用'"
            class="head-badge"
          />
        </h1>
        <div class="subtitle" v-if="original">更新于 {{ fmtTime(original.updated_at) }}</div>
      </div>
      <div style="display: flex; gap: 12px; align-items: center">
        <el-checkbox v-model="enabled">启用（登录时执行）</el-checkbox>
        <el-button type="danger" text @click="remove">删除</el-button>
        <el-button type="primary" :disabled="!dirty" :loading="saving" @click="save">
          {{ dirty ? '保存' : '已保存' }}
        </el-button>
      </div>
    </div>

    <Banner v-if="loadErr" type="error">{{ loadErr }}</Banner>

    <Card :pad="false">
      <div style="padding: 12px 14px 4px">
        <CodeEditor v-model="content" language="lua" height="380px" />
      </div>
      <div class="editor-foot">
        可用桥接：<code class="mono">list_clusters / cluster_exists / list_bindings / add_binding / remove_binding /
        has_policy / add_policy / remove_policy / list_policies</code>
        （调用前缀 <code class="mono">kamc.</code>）；入参 <code class="mono">claims = {sub, email, groups}</code>；
        执行超时 2s，任何错误拒绝本次登录
      </div>
    </Card>

    <Card title="试运行（dry-run 验证闭环）">
      <p class="hint-p">
        注入 mock claims 执行脚本；dry-run 模式下桥接只记录动作不落地——保存前验证脚本行为的标准方式。
      </p>
      <div class="dry-form">
        <div class="dry-input">
          <LabeledInput v-model="drySub" label="sub（缺省=调用者）" mono placeholder="alice" />
        </div>
        <div class="dry-input">
          <LabeledInput v-model="dryEmail" label="email" placeholder="alice@example.com" />
        </div>
        <div class="dry-input">
          <LabeledInput v-model="dryGroups" label="groups（逗号分隔）" placeholder="dev-team, qa-team" />
        </div>
        <div class="dry-run-col">
          <el-checkbox v-model="dryRealMode" class="danger-check">真实执行（落地绑定/策略）</el-checkbox>
          <el-button type="primary" :loading="dryRunning" @click="runDry">
            {{ dryRealMode ? '执行（真实落地）' : '执行（dry-run）' }}
          </el-button>
        </div>
      </div>

      <Banner v-if="dryErr" type="error">{{ dryErr }}</Banner>

      <div v-if="dryResult" class="dry-result">
        <div class="dry-meta">
          <StatusBadge
            :status="dryResult.dry_run ? 'info' : 'success'"
            :label="dryResult.dry_run ? 'dry-run' : '已落地'"
          />
          <span v-if="dryResult.actions" class="muted-note">动作 {{ dryResult.actions.length }} 条</span>
        </div>
        <el-table v-if="dryResult.actions?.length" :data="dryResult.actions" size="small" stripe>
          <el-table-column label="桥接 API" width="180">
            <template #default="{ row }"><code class="mono">{{ (row as { api: string }).api }}</code></template>
          </el-table-column>
          <el-table-column label="参数">
            <template #default="{ row }"><code class="mono">{{ (row as { args: string[] }).args.join(' , ') }}</code></template>
          </el-table-column>
        </el-table>
        <p v-else class="muted-note" style="margin: 10px 0 0">脚本执行成功，但未产生任何绑定/策略动作。</p>
      </div>
    </Card>
  </div>
</template>

<style lang="scss" scoped>
.head-badge {
  margin-left: 10px;
  vertical-align: 3px;
}

.editor-foot {
  padding: 10px 14px;
  color: $text-muted;
  font-size: 12px;
  line-height: 1.8;
  border-top: 1px solid $border;
}

.hint-p {
  color: $text-muted;
  font-size: 13px;
  margin: 0 0 12px;
  line-height: 1.7;
}

.dry-form {
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
  align-items: flex-start;
}

.dry-input {
  flex: 1;
  min-width: 180px;
}

.dry-run-col {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 4px;
}

.danger-check {
  :deep(.el-checkbox__label) {
    color: var(--el-color-danger);
    font-size: 12.5px;
  }
}

.dry-result {
  margin-top: 14px;
  border-top: 1px dashed $border;
  padding-top: 12px;
}

.dry-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}

.muted-note {
  color: $text-muted;
  font-size: 12.5px;
}
</style>
