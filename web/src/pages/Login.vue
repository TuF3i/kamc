<script setup lang="ts">
import { ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import Banner from '@/components/Banner.vue';
import { ApiError } from '@/api/http';
import { useAuthStore } from '@/stores/auth';

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

const error = ref('');
const busy = ref(false);

// dev-token 直签（开发环境；生产环境该端点 404，提交时提示即可）
const showDev = ref(false);
const devSub = ref('admin');
const devEmail = ref('admin@example.com');
const devGroups = ref('am:admin');

function gotoTarget(): void {
  const target = (route.query.redirect as string) || '/';
  router.replace(target);
}

function ssoLogin(): void {
  // 后端 /auth/login 302 → Authentik → /auth/callback 桥接页写 localStorage 后回 '/'
  location.assign('/auth/login');
}

async function devLogin(): Promise<void> {
  if (!devSub.value.trim()) {
    error.value = '请填写用户标识（sub）';
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const groups = devGroups.value
      .split(/[,\s]+/)
      .map((g) => g.trim())
      .filter(Boolean);
    await auth.signinWithDev(devSub.value.trim(), devEmail.value.trim(), groups);
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

      <Banner v-if="error" type="error" closable @close="error = ''">{{ error }}</Banner>
      <Banner v-if="route.query.expired" type="warning">登录已过期，请重新登录</Banner>
      <Banner v-if="route.query.logged_out" type="info">已退出登录</Banner>

      <button class="btn btn-primary sso-btn" :disabled="busy" @click="ssoLogin">
        使用 SSO 登录（OIDC）
      </button>

      <div class="divider"><span>或</span></div>

      <button class="link-btn" type="button" @click="showDev = !showDev">
        {{ showDev ? '收起开发登录 ▴' : '开发直签登录（AM_DEV_ISSUE_TOKEN）▾' }}
      </button>

      <form v-if="showDev" class="dev-form" @submit.prevent="devLogin">
        <div class="field">
          <label class="field-label">用户标识（sub）<span class="req">*</span></label>
          <input v-model="devSub" class="input" placeholder="alice" />
        </div>
        <div class="field">
          <label class="field-label">邮箱</label>
          <input v-model="devEmail" class="input" placeholder="alice@example.com" />
        </div>
        <div class="field">
          <label class="field-label">组（逗号分隔；am:admin 为管理员组）</label>
          <input v-model="devGroups" class="input" placeholder="dev-team, am:admin" />
        </div>
        <button class="btn btn-primary" type="submit" :disabled="busy" style="width: 100%">
          {{ busy ? '签发中…' : '签发并登录' }}
        </button>
      </form>

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
    radial-gradient(1000px 500px at 80% -10%, rgba(69, 181, 164, 0.18), transparent),
    radial-gradient(900px 500px at 10% 110%, rgba(43, 125, 201, 0.2), transparent),
    $header-bg;
  padding: 24px;
}

.login-panel {
  width: 400px;
  max-width: 100%;
  background: #fff;
  border-radius: 10px;
  padding: 34px 32px 26px;
  box-shadow: 0 18px 60px rgba(0, 0, 0, 0.35);
}

.login-brand {
  text-align: center;
  margin-bottom: 22px;

  h1 {
    font-size: 19px;
    margin: 12px 0 4px;
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
  background: linear-gradient(135deg, #2b7dc9, #45b5a4);
  color: #fff;
  align-items: center;
  justify-content: center;
  font-size: 24px;
  font-weight: 700;
}

.sso-btn {
  width: 100%;
  padding: 10px;
  font-size: 14px;
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
  background: none;
  border: none;
  color: $brand;
  font-size: 13px;
  cursor: pointer;
  padding: 4px;

  &:hover {
    text-decoration: underline;
  }
}

.dev-form {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px dashed $border;
}

.login-foot {
  text-align: center;
  color: $text-muted;
  font-size: 12px;
  margin: 18px 0 0;
}
</style>
