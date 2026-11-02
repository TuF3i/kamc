<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import Card from '@/components/Card.vue';
import LabeledInput from '@/components/LabeledInput.vue';
import LabeledSelect from '@/components/LabeledSelect.vue';
import { downloadKubeconfig, listClusters } from '@/api/clusters';
import { useAuthStore } from '@/stores/auth';
import { useGrowlStore } from '@/stores/growl';
import { fmtExpiresIn, fmtTime } from '@/utils/format';

const auth = useAuthStore();
const growl = useGrowlStore();

// 管理员：从集群列表选择；普通用户：手动输入集群 ID（clusters 列表是管理面端点）
const clusterOptions = ref<{ label: string; value: string }[]>([]);
const kubeCluster = ref('');
const kubeMode = ref<'exec' | 'static'>('exec');
const kubeBusy = ref(false);

const expiresIn = computed(() => (auth.claims ? fmtExpiresIn(auth.claims.exp) : ''));

async function doDownload(): Promise<void> {
  if (!kubeCluster.value.trim()) {
    growl.warning('请填写集群 ID');
    return;
  }
  kubeBusy.value = true;
  try {
    await downloadKubeconfig(kubeCluster.value.trim(), kubeMode.value);
    growl.success(`集群 ${kubeCluster.value} 的 kubeconfig 已下载`);
  } catch (e) {
    growl.error(e instanceof Error ? e.message : String(e));
  } finally {
    kubeBusy.value = false;
  }
}

async function doLogout(): Promise<void> {
  await auth.signout(); // 内部整页跳转，无需再 push
}

onMounted(async () => {
  if (auth.isAdmin) {
    try {
      const cs = await listClusters();
      clusterOptions.value = cs.map((c) => ({ label: `${c.name}（${c.id}）`, value: c.id }));
      if (cs.length) kubeCluster.value = cs[0].id;
    } catch {
      // 非致命
    }
  }
});
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h1>个人中心</h1>
        <div class="subtitle">会话身份与自服务操作</div>
      </div>
    </div>

    <Card title="当前身份（平台 JWT claims）">
      <dl class="kv-list">
        <dt>用户（sub）</dt>
        <dd class="mono">{{ auth.claims?.sub }}</dd>
        <dt>邮箱</dt>
        <dd>{{ auth.claims?.email || '—' }}</dd>
        <dt>组</dt>
        <dd>
          <span v-for="g in auth.claims?.groups || []" :key="g" class="group-chip">{{ g }}</span>
          <span v-if="!auth.claims?.groups?.length">—</span>
        </dd>
        <dt>签发时间</dt>
        <dd>{{ auth.claims ? fmtTime(new Date(auth.claims.iat * 1000).toISOString()) : '—' }}</dd>
        <dt>过期时间</dt>
        <dd>
          {{ auth.claims ? fmtTime(new Date(auth.claims.exp * 1000).toISOString()) : '—' }}
          <span class="muted-note">（剩余 {{ expiresIn }}，过期后请重新登录）</span>
        </dd>
        <dt>管理员</dt>
        <dd>{{ auth.isAdmin ? '是（am-admin 组）' : '否' }}</dd>
        <dt>jti</dt>
        <dd class="mono">{{ auth.claims?.jti }}</dd>
      </dl>
    </Card>

    <Card title="kubeconfig 下载（自服务）">
      <p class="hint-p">
        产物只承载你自己的身份；数据面每次请求仍由 Casbin 资源级鉴权。推荐 exec 模式：
        凭证由 <code class="mono">k8s-am-login</code> 自动续期，不会过期。
      </p>
      <div class="kube-row">
        <div style="flex: 1; min-width: 220px">
          <LabeledSelect
            v-if="auth.isAdmin && clusterOptions.length"
            v-model="kubeCluster"
            label="集群"
            :options="clusterOptions"
          />
          <LabeledInput v-else v-model="kubeCluster" label="集群 ID" required mono placeholder="prod-bj" hint="向管理员索取集群 ID" />
        </div>
        <div style="width: 300px">
          <LabeledSelect
            v-model="kubeMode"
            label="模式"
            :options="[
              { label: 'exec（推荐，凭证自动续期）', value: 'exec' },
              { label: 'static（嵌入当前 JWT，1h 过期）', value: 'static' },
            ]"
          />
        </div>
        <div style="padding-top: 24px">
          <button class="btn btn-primary" :disabled="kubeBusy" @click="doDownload">
            {{ kubeBusy ? '生成中…' : '下载' }}
          </button>
        </div>
      </div>
    </Card>

    <Card title="会话">
      <button class="btn btn-danger" @click="doLogout">退出登录</button>
      <span class="muted-note" style="margin-left: 12px">
        当前 Token 的 jti 进入黑名单，立即失效；不影响其他已登录会话
      </span>
    </Card>
  </div>
</template>

<style lang="scss" scoped>
.kube-row {
  display: flex;
  gap: 14px;
  flex-wrap: wrap;
}

.group-chip {
  display: inline-block;
  background: rgba(43, 125, 201, 0.09);
  color: #1e5d99;
  border-radius: 10px;
  padding: 1px 10px;
  font-size: 12px;
  margin: 0 6px 6px 0;
}

.muted-note {
  color: $text-muted;
  font-size: 12.5px;
}

.hint-p {
  color: $text-muted;
  font-size: 13px;
  line-height: 1.7;
  margin: 0 0 12px;
}
</style>
