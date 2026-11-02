<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Monitor, CircleCheckFilled, WarningFilled } from '@element-plus/icons-vue';
import Card from '@/components/Card.vue';
import StatusBadge from '@/components/StatusBadge.vue';
import { listClusters } from '@/api/clusters';
import { version } from '@/api/auth';
import type { Cluster, VersionInfo } from '@/api/types';
import { useAuthStore } from '@/stores/auth';

const auth = useAuthStore();
const clusters = ref<Cluster[]>([]);
const ver = ref<VersionInfo | null>(null);
const loaded = ref(false);

onMounted(async () => {
  if (!auth.isAdmin) {
    loaded.value = true;
    return;
  }
  try {
    [clusters.value, ver.value] = await Promise.all([listClusters(), version().catch(() => null)]);
  } catch {
    // 探测已过，此处失败不阻塞概览
  } finally {
    loaded.value = true;
  }
});
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div>
        <h1>概览</h1>
        <div class="subtitle">欢迎回来，{{ auth.claims?.sub }}</div>
      </div>
    </div>

    <template v-if="auth.isAdmin">
      <el-row :gutter="16" class="stat-row">
        <el-col :span="6" :xs="12">
          <div class="stat">
            <div class="stat-num">{{ clusters.length }}</div>
            <div class="stat-label">已注册集群</div>
          </div>
        </el-col>
        <el-col :span="6" :xs="12">
          <div class="stat">
            <div class="stat-num online">{{ clusters.filter((c) => c.status === 'online').length }}</div>
            <div class="stat-label"><el-icon><CircleCheckFilled /></el-icon> 隧道在线</div>
          </div>
        </el-col>
        <el-col :span="6" :xs="12">
          <div class="stat">
            <div class="stat-num warn">{{ clusters.filter((c) => c.status === 'pending').length }}</div>
            <div class="stat-label"><el-icon><WarningFilled /></el-icon> 待接入</div>
          </div>
        </el-col>
        <el-col v-if="ver" :span="6" :xs="12">
          <div class="stat">
            <div class="stat-num ver">{{ ver.version }}</div>
            <div class="stat-label">平台版本（{{ ver.commit }}）</div>
          </div>
        </el-col>
      </el-row>

      <Card title="集群">
        <template #actions>
          <RouterLink to="/clusters"><el-button size="small" text type="primary">全部集群 →</el-button></RouterLink>
        </template>
        <div v-if="!loaded" v-loading="true" style="height: 80px" />
        <template v-else-if="clusters.length">
          <el-row :gutter="12" class="cluster-grid">
            <el-col v-for="c in clusters" :key="c.id" :xs="24" :sm="12" :md="8" :lg="6">
              <RouterLink :to="`/clusters/${encodeURIComponent(c.id)}`" class="cluster-card">
                <div class="cluster-head">
                  <span class="cluster-name">{{ c.name }}</span>
                  <StatusBadge :status="c.status" />
                </div>
                <div class="cluster-id mono">{{ c.id }}</div>
                <div class="cluster-desc">{{ c.desc || '—' }}</div>
              </RouterLink>
            </el-col>
          </el-row>
        </template>
        <el-empty v-else description="尚无注册集群 —— 前往「集群」页注册第一个目标集群" :image-size="72" />
      </Card>
    </template>

    <template v-else>
      <Card title="快速开始">
        <ol class="steps">
          <li>在<b>个人中心</b>按集群 ID 下载 kubeconfig（推荐 exec 模式，凭证不过期）。</li>
          <li>用 <code class="mono">kubectl --kubeconfig</code> 或 <code class="mono">k8s-am-login</code> 直接访问目标集群；数据面请求经平台 Casbin 资源级鉴权。</li>
          <li>如需开通权限，联系管理员配置角色绑定（或由登录期绑定脚本自动分配）。</li>
        </ol>
      </Card>
      <Card title="我的身份">
        <dl class="kv-list">
          <dt>用户</dt>
          <dd class="mono">{{ auth.claims?.sub }}</dd>
          <dt>邮箱</dt>
          <dd>{{ auth.claims?.email || '—' }}</dd>
          <dt>组</dt>
          <dd>
            <el-tag v-for="g in auth.claims?.groups || []" :key="g" size="small" class="group-chip" disable-transitions>
              {{ g }}
            </el-tag>
            <span v-if="!auth.claims?.groups?.length">—</span>
          </dd>
        </dl>
      </Card>
    </template>
  </div>
</template>

<style lang="scss" scoped>
.stat-row {
  margin-bottom: 16px;
}

.stat {
  background: $card-bg;
  border: 1px solid $border;
  border-radius: 6px;
  padding: 14px 18px;
  box-shadow: $shadow;
  height: 100%;
}

.stat-num {
  font-size: 26px;
  font-weight: 700;
  color: $text;

  &.online { color: var(--el-color-success); }
  &.warn { color: var(--el-color-warning); }
  &.ver { font-size: 18px; padding: 4px 0; }
}

.stat-label {
  color: $text-muted;
  font-size: 12.5px;
  margin-top: 2px;
  display: flex;
  align-items: center;
  gap: 4px;
}

.cluster-card {
  display: block;
  border: 1px solid $border;
  border-radius: 6px;
  padding: 12px 14px;
  margin-bottom: 12px;
  text-decoration: none;
  color: $text;
  transition: border-color 0.12s, box-shadow 0.12s;

  &:hover {
    border-color: var(--el-color-primary);
    box-shadow: var(--el-box-shadow-light);
    text-decoration: none;
  }
}

.cluster-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.cluster-name {
  font-weight: 600;
  font-size: 14px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cluster-id {
  color: $text-muted;
  font-size: 12px;
  margin: 4px 0 6px;
}

.cluster-desc {
  font-size: 12.5px;
  color: $text-muted;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cluster-grid {
  // el-row gutter 负 margin 会让卡片网格撑破父容器产生横向滚动
  overflow: hidden;
  padding-bottom: 2px;
}

.steps {
  margin: 0;
  padding-left: 18px;
  line-height: 2;
  font-size: 13.5px;
}

.group-chip {
  margin: 0 6px 6px 0;
}
</style>
