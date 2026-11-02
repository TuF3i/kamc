<script setup lang="ts">
import { ref } from 'vue';
import { useRoute } from 'vue-router';
import { ElAlert, ElButton, ElForm, ElFormItem, ElInput } from 'element-plus';
import { ApiError } from '@/api/http';
import { useAuthStore } from '@/stores/auth';

const route = useRoute();
const auth = useAuthStore();

const error = ref('');
const busy = ref(false);

// dev-token 直签（仅后端 AM_DEV_ISSUE_TOKEN=1 时可用，否则 404）
const showDev = ref(false);
const devForm = ref({
  sub: 'admin',
  email: 'admin@example.com',
  groups: 'am:admin',
});

function gotoTarget(): void {
  const target = (route.query.redirect as string) || '/';
  location.assign(target); // dev 登录统一硬跳，避免残留 SPA 状态
}

function ssoLogin(): void {
  // 后端 /auth/login 302 → Authentik → /auth/callback 桥接页写 localStorage 后回 '/'
  location.assign('/auth/login');
}

async function devLogin(): Promise<void> {
  if (!devForm.value.sub.trim()) {
    error.value = '请填写用户标识（sub）';
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const groups = devForm.value.groups
      .split(/[,\s]+/)
      .map((g) => g.trim())
      .filter(Boolean);
    await auth.signinWithDev(devForm.value.sub.trim(), devForm.value.email.trim(), groups);
    gotoTarget();
  } catch (e) {
    if (e instanceof ApiError && e.httpStatus === 404) {
      error.value = '开发直签未启用：需要平台以 AM_DEV_ISSUE_TOKEN=1 启动';
    } else {
      error.value = e instanceof Error ? e.message : String(e);
    }
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="login-page">
    <div class="login-panel">
      <div class="login-brand">
        <span class="brand-mark">K</span>
        <h1>kamc 访问管理中心</h1>
        <p>Kubernetes 多集群访问管理 · SSO 登录</p>
      </div>

      <ElAlert v-if="error" type="error" show-icon :title="error" class="login-alert" @close="error = ''" />
      <ElAlert v-if="route.query.expired" type="warning" show-icon title="登录已过期，请重新登录" class="login-alert" />
      <ElAlert v-if="route.query.logged_out" type="info" show-icon title="已退出登录" class="login-alert" />

      <ElButton type="primary" size="large" class="sso-btn" :disabled="busy" @click="ssoLogin">
        使用 SSO 登录（OIDC）
      </ElButton>

      <div class="divider"><span>或</span></div>

      <el-button text type="primary" class="link-btn" @click="showDev = !showDev">
        {{ showDev ? '收起开发登录 ▴' : '开发直签登录（AM_DEV_ISSUE_TOKEN）▾' }}
      </el-button>

      <ElForm v-if="showDev" label-position="top" class="dev-form" @submit.prevent="devLogin">
        <ElFormItem label="用户标识（sub）" required>
          <ElInput v-model="devForm.sub" placeholder="alice" />
        </ElFormItem>
        <ElFormItem label="邮箱">
          <ElInput v-model="devForm.email" placeholder="alice@example.com" />
        </ElFormItem>
        <ElFormItem label="组（逗号分隔；am:admin 为管理员组）">
          <ElInput v-model="devForm.groups" placeholder="dev-team, am:admin" />
        </ElFormItem>
        <ElButton type="primary" class="submit-btn" :loading="busy" @click="devLogin">
          {{ busy ? '签发中…' : '签发并登录' }}
        </ElButton>
      </ElForm>

      <p class="login-foot">平台 JWT 1 小时有效 · 登出即时吊销</p>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background:
    radial-gradient(1000px 500px at 80% -10%, rgba(77, 158, 155, 0.16), transparent),
    radial-gradient(900px 500px at 10% 110%, rgba(74, 137, 202, 0.22), transparent),
    $header-bg;
  padding: 24px;
}

.login-panel {
  width: 400px;
  max-width: 100%;
  background: #fff;
  border-radius: 10px;
  padding: 34px 32px 26px;
  box-shadow: 0 18px 60px rgba(15, 25, 40, 0.4);
}

.login-brand {
  text-align: center;
  margin-bottom: 22px;

  h1 {
    font-size: 19px;
    margin: 12px 0 4px;
    color: $text;
  }

  p {
    color: $text-muted;
    font-size: 13px;
    margin: 0;
  }
}

.brand-mark {
  display: inline-flex;
  width: 46px;
  height: 46px;
  border-radius: 12px;
  background: linear-gradient(135deg, #4a89ca, #4d9e9b);
  color: #fff;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  font-weight: 700;
}

.login-alert {
  margin-bottom: 12px;
}

.sso-btn {
  width: 100%;
}

.divider {
  display: flex;
  align-items: center;
  gap: 12px;
  color: $text-muted;
  font-size: 12px;
  margin: 18px 0 10px;

  &::before,
  &::after {
    content: '';
    flex: 1;
    height: 1px;
    background: $border;
  }
}

.link-btn {
  display: block;
  margin: 0 auto;
}

.dev-form {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed $border;
}

.submit-btn {
  width: 100%;
}

.login-foot {
  text-align: center;
  color: $text-muted;
  font-size: 12px;
  margin: 18px 0 0;
}
</style>
