<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import Banner from '@/components/Banner.vue';
import Card from '@/components/Card.vue';
import CopyButton from '@/components/CopyButton.vue';
import Modal from '@/components/Modal.vue';
import StatusBadge from '@/components/StatusBadge.vue';
import Tabs from '@/components/Tabs.vue';
import ExecTerminal from '@/components/ExecTerminal.vue';
import { downloadKubeconfig, getCluster, issueToken, listTokens, revokeToken } from '@/api/clusters';
import type { Cluster, IssuedToken, TokenDTO } from '@/api/types';
import { fmtTime } from '@/utils/format';
import { useGrowlStore } from '@/stores/growl';

const route = useRoute();
const router = useRouter();
const growl = useGrowlStore();

const clusterId = computed(() => String(route.params.id));
const tab = ref('overview');
const tabs = [
  { key: 'overview', label: '概况' },
  { key: 'tokens', label: 'Agent Token' },
  { key: 'terminal', label: '终端' },
  { key: 'kubeconfig', label: 'kubeconfig' },
];

const cluster = ref<Cluster | null>(null);
const error = ref('');

// —— Token 管理 ——
const tokens = ref<TokenDTO[]>([]);
const tokensLoading = ref(false);
const issuing = ref<'register' | 'longterm' | null>(null);
const issued = ref<IssuedToken | null>(null);

async function loadTokens(): Promise<void> {
  tokensLoading.value = true;
  try {
    tokens.value = await listTokens(clusterId.value);
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  } finally {
    tokensLoading.value = false;
  }
}

async function doIssue(kind: 'register' | 'longterm'): Promise<void> {
  issuing.value = kind;
  try {
    issued.value = await issueToken(clusterId.value, kind);
    await loadTokens();
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  } finally {
    issuing.value = null;
  }
}

async function doRevoke(kind: 'register' | 'longterm'): Promise<void> {
  if (!confirm(`确认吊销 ${kind} Token？立即失效，无宽限期。`)) return;
  try {
    await revokeToken(clusterId.value, kind);
    growl.success(`${kind} Token 已吊销`);
    await loadTokens();
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  }
}

// —— kubeconfig ——
const kubeMode = ref<'exec' | 'static'>('exec');
const kubeBusy = ref(false);

async function doDownload(): Promise<void> {
  kubeBusy.value = true;
  try {
    await downloadKubeconfig(clusterId.value, kubeMode.value);
    growl.success('kubeconfig 已下载');
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  } finally {
    kubeBusy.value = false;
  }
}

onMounted(async () => {
  try {
    cluster.value = await getCluster(clusterId.value);
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  }
});
</script>

<template>
  <div class="page">
    <div class="crumbs">
      <RouterLink to="/clusters">集群</RouterLink>
      <span>/</span>
      <span class="mono">{{ clusterId }}</span>
    </div>
    <div class="page-header">
      <div>
        <h1>
          {{ cluster?.name || clusterId }}
          <StatusBadge v-if="cluster" :status="cluster.status" style="margin-left: 8px; vertical-align: 3px" />
        </h1>
        <div class="subtitle mono">{{ clusterId }}</div>
      </div>
    </div>

    <Banner v-if="error" type="error">{{ error }}</Banner>

    <Tabs v-model="tab" :tabs="tabs" />

    <!-- 概况 -->
    <template v-if="tab === 'overview'">
      <Card title="集群信息">
        <dl class="kv-list">
          <dt>集群 ID</dt>
          <dd class="mono">{{ cluster?.id }}</dd>
          <dt>名称</dt>
          <dd>{{ cluster?.name }}</dd>
          <dt>描述</dt>
          <dd>{{ cluster?.desc || '—' }}</dd>
          <dt>接入状态</dt>
          <dd><StatusBadge v-if="cluster" :status="cluster.status" /></dd>
          <dt>注册时间</dt>
          <dd>{{ fmtTime(cluster?.created_at) }}</dd>
          <dt>更新时间</dt>
          <dd>{{ fmtTime(cluster?.updated_at) }}</dd>
        </dl>
      </Card>
      <Card title="接入说明">
        <p class="hint-p">
          Agent 侧以注册 Token 完成一次性交换后，使用长期 Token 建立
          <code class="mono">/tunnel</code> 反向隧道即可上线；数据面请求由平台经隧道转发并注入平台身份。
          Agent 部署清单见 <code class="mono">deploy/manifests.yaml</code>。
        </p>
      </Card>
    </template>

    <!-- Agent Token -->
    <template v-else-if="tab === 'tokens'">
      <div class="page-header">
        <div class="subtitle" style="margin: 0">
          平台只存哈希；明文仅在签发响应中出现一次，请立即保存。
        </div>
        <div style="display: flex; gap: 8px">
          <button class="btn btn-secondary" :disabled="issuing !== null" @click="doIssue('register')">
            {{ issuing === 'register' ? '签发中…' : '签发注册 Token（1h 一次性）' }}
          </button>
          <button class="btn btn-primary" :disabled="issuing !== null" @click="doIssue('longterm')">
            {{ issuing === 'longterm' ? '签发中…' : '签发/轮换长期 Token' }}
          </button>
        </div>
      </div>
      <Card :pad="false">
        <div style="padding: 14px">
          <table class="plain-table">
            <thead>
              <tr>
                <th>类型</th>
                <th>状态</th>
                <th>过期时间</th>
                <th>创建时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="tokensLoading">
                <td colspan="5" class="muted">加载中…</td>
              </tr>
              <tr v-else-if="!tokens.length">
                <td colspan="5" class="muted">尚无 Token —— 签发注册 Token 供 Agent 首次接入</td>
              </tr>
              <tr v-for="(t, i) in tokens" :key="i">
                <td>{{ t.kind === 'register' ? '注册（一次性）' : '长期' }}</td>
                <td><StatusBadge :status="t.status" /></td>
                <td>{{ t.expires_at ? fmtTime(t.expires_at) : '长期有效' }}</td>
                <td>{{ fmtTime(t.created_at) }}</td>
                <td>
                  <button class="btn btn-danger-ghost btn-sm" @click="doRevoke(t.kind)">吊销</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </Card>

      <Modal :open="!!issued" title="Token 已签发（仅此一次）" width="640px" @close="issued = null">
        <Banner type="warning">关闭本弹窗后明文不可再获取，平台只保存 SHA256 哈希。</Banner>
        <div class="token-box">
          <code class="mono">{{ issued?.token }}</code>
          <CopyButton :text="issued?.token || ''" tip="Token 已复制" />
        </div>
        <p v-if="issued?.kind === 'register'" class="hint-p">
          Agent 以该 Token 调用 <code class="mono">POST /api/v1/agent/register</code> 一次性换取长期 Token。
        </p>
        <p v-else-if="issued?.grace_ends_at" class="hint-p">
          轮换语义：旧 Token 宽限期至 <b>{{ fmtTime(issued.grace_ends_at) }}</b
          >（24h），期间新旧并存。
        </p>
        <template #footer>
          <button class="btn btn-primary" @click="issued = null">我已保存</button>
        </template>
      </Modal>
    </template>

    <!-- 终端 -->
    <template v-else-if="tab === 'terminal'">
      <ExecTerminal :cluster-id="clusterId" />
    </template>

    <!-- kubeconfig -->
    <template v-else-if="tab === 'kubeconfig'">
      <Card title="生成 kubeconfig">
        <p class="hint-p">
          产物只承载你自己的身份；数据面授权仍由请求时的 Casbin 策略决定。
          server 指向平台 <code class="mono">/proxy/{{ clusterId }}</code>。
        </p>
        <div style="display: flex; gap: 10px; align-items: center; flex-wrap: wrap">
          <label class="radio">
            <input v-model="kubeMode" type="radio" value="exec" />
            exec 模式（推荐：写 k8s-am-login 命令行，凭证自动续期）
          </label>
          <label class="radio">
            <input v-model="kubeMode" type="radio" value="static" />
            static 模式（嵌入当前 JWT，1 小时过期）
          </label>
        </div>
        <div style="margin-top: 14px">
          <button class="btn btn-primary" :disabled="kubeBusy" @click="doDownload">
            {{ kubeBusy ? '生成中…' : '下载 kubeconfig' }}
          </button>
        </div>
      </Card>
      <Card title="使用方式">
        <pre class="usage-pre">kubectl --kubeconfig ./{{ clusterId }}-kubeconfig.yaml get pods -n dev</pre>
        <p class="hint-p">
          exec 模式依赖 <code class="mono">k8s-am-login</code> 在 PATH 中（OIDC 设备流程登录，凭证缓存于
          <code class="mono">~/.config/kamc</code>）；static 模式可直接使用但 1 小时后需重新下载。
        </p>
      </Card>
    </template>
  </div>
</template>

<style lang="scss" scoped>
.plain-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;

  th {
    text-align: left;
    padding: 8px 12px;
    background: #f7f9fb;
    border-bottom: 1px solid $border;
    color: $text-muted;
    font-size: 12px;
  }

  td {
    padding: 8px 12px;
    border-bottom: 1px solid #edf1f5;
  }

  .muted {
    color: $text-muted;
    text-align: center;
    padding: 24px 0;
  }
}

.token-box {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  background: #f4f6f9;
  border: 1px solid $border;
  border-radius: 4px;
  padding: 10px 12px;
  word-break: break-all;
  font-size: 12.5px;
}

.hint-p {
  color: $text-muted;
  font-size: 13px;
  line-height: 1.7;
  margin: 8px 0;
}

.radio {
  font-size: 13px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.usage-pre {
  background: #1a222d;
  color: #cde3f5;
  border-radius: 6px;
  padding: 12px 14px;
  font-size: 12.5px;
  overflow-x: auto;
  margin: 0;
}
</style>
