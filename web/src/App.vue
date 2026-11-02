<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRoute } from 'vue-router';
import { useAuthStore } from '@/stores/auth';
import {
  Odometer,
  Monitor,
  Lock,
  MagicStick,
  Document,
  ArrowDown,
  Fold,
  Expand,
  UserFilled,
} from '@element-plus/icons-vue';

const route = useRoute();
const auth = useAuthStore();

const isPublic = computed(() => !!route.meta.public);
const collapsed = ref(localStorage.getItem('kamc_nav_collapsed') === '1');
const navWidth = computed(() => (collapsed.value ? '64px' : '232px'));

const activeMenu = computed(() => {
  if (route.path === '/') return '/';
  const hit = ['/clusters', '/access', '/scripts', '/audit'].find(
    (p) => route.path === p || route.path.startsWith(p + '/'),
  );
  return hit ?? '';
});

interface NavItem {
  label: string;
  to: string;
  icon: typeof Monitor;
  admin?: boolean;
}

const groups = computed(() => {
  const all: { title: string; items: NavItem[] }[] = [
    { title: '', items: [{ label: '概览', to: '/', icon: Odometer }] },
    {
      title: '资源',
      items: [
        { label: '集群', to: '/clusters', icon: Monitor, admin: true },
        { label: '权限', to: '/access', icon: Lock, admin: true },
      ],
    },
    { title: '自动化', items: [{ label: '绑定脚本', to: '/scripts', icon: MagicStick, admin: true }] },
    { title: '运维', items: [{ label: '审计日志', to: '/audit', icon: Document, admin: true }] },
  ];
  return all
    .map((g) => ({ ...g, items: g.items.filter((i) => !i.admin || auth.isAdmin) }))
    .filter((g) => g.items.length > 0);
});

function toggleNav(): void {
  collapsed.value = !collapsed.value;
  localStorage.setItem('kamc_nav_collapsed', collapsed.value ? '1' : '0');
}

async function onLogout(): Promise<void> {
  await auth.signout(); // 内部整页跳转
}
</script>

<template>
  <!-- 公开路由（登录页）裸渲染，不套应用壳 -->
  <RouterView v-if="isPublic" />

  <div v-else class="shell">
    <header class="topbar">
      <div class="topbar-left">
        <el-button text class="fold-btn" :icon="collapsed ? Expand : Fold" @click="toggleNav" />
        <RouterLink to="/" class="brand">
          <span class="brand-mark">K</span>
          <span v-if="!collapsed" class="brand-name">kamc <small>访问管理中心</small></span>
        </RouterLink>
      </div>
      <div class="topbar-right">
        <el-dropdown v-if="auth.claims" trigger="click">
          <span class="user-box">
            <el-avatar :size="26" class="avatar">{{ (auth.claims.sub || '?')[0].toUpperCase() }}</el-avatar>
            <span class="user-name">{{ auth.claims.sub }}</span>
            <el-icon class="caret"><ArrowDown /></el-icon>
          </span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item disabled>
                <div class="dd-identity">
                  <div class="dd-sub">{{ auth.claims.sub }}</div>
                  <div class="dd-email">{{ auth.claims.email || '—' }}</div>
                </div>
              </el-dropdown-item>
              <el-dropdown-item divided :icon="UserFilled" @click="$router.push('/account')">个人中心</el-dropdown-item>
              <el-dropdown-item divided class="dd-danger" @click="onLogout">退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </header>

    <aside class="sidenav" :style="{ width: navWidth }">
      <el-menu
        :default-active="activeMenu"
        :collapse="collapsed"
        :collapse-transition="false"
        router
        class="side-menu"
      >
        <template v-for="g in groups" :key="g.title">
          <div v-if="g.title && !collapsed" class="nav-title">{{ g.title }}</div>
          <el-menu-item v-for="item in g.items" :key="item.to" :index="item.to">
            <el-icon><component :is="item.icon" /></el-icon>
            <template #title>{{ item.label }}</template>
          </el-menu-item>
        </template>
      </el-menu>
    </aside>

    <main class="content" :style="{ marginLeft: navWidth, paddingTop: '52px' }">
      <RouterView />
    </main>
  </div>
</template>

<style lang="scss" scoped>
.shell {
  min-height: 100vh;
}

.topbar {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: 52px;
  background: $header-bg;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px;
  z-index: 100;
}

.topbar-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.fold-btn {
  color: #b8c6d6;

  &:hover {
    color: #fff;
    background: rgba(255, 255, 255, 0.08);
  }
}

.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #fff;
  text-decoration: none;

  &:hover {
    text-decoration: none;
  }
}

.brand-mark {
  width: 26px;
  height: 26px;
  border-radius: 6px;
  background: linear-gradient(135deg, #4a89ca, #4d9e9b);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 15px;
  color: #fff;
}

.brand-name {
  font-size: 15px;
  font-weight: 600;

  small {
    font-weight: 400;
    color: #8ba0b8;
    margin-left: 6px;
    font-size: 12px;
  }
}

.topbar-right {
  display: flex;
  align-items: center;
}

.user-box {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 4px 10px;
  border-radius: 4px;
  outline: none;

  &:hover {
    background: rgba(255, 255, 255, 0.08);
  }
}

.avatar {
  background: #4d9e9b;
  font-size: 13px;
  font-weight: 600;
}

.user-name {
  font-size: 13px;
  color: #fff;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.caret {
  color: #8ba0b8;
  font-size: 12px;
}

.dd-identity {
  .dd-sub {
    font-weight: 600;
  }
  .dd-email {
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }
}

.dd-danger {
  color: var(--el-color-danger) !important;

  &:hover {
    color: var(--el-color-danger) !important;
    background: rgba(194, 85, 67, 0.08) !important;
  }
}

.sidenav {
  position: fixed;
  top: 52px;
  bottom: 0;
  left: 0;
  background: $nav-bg;
  overflow-y: auto;
  overflow-x: hidden;
  transition: width 0.15s;
  z-index: 90;
}

.side-menu {
  border-right: none;
  background: transparent;

  &.el-menu {
    background: transparent;
  }

  :deep(.el-menu-item) {
    color: $nav-fg;
    height: 44px;
    margin: 2px 8px;
    border-radius: 4px;

    .el-icon {
      color: $nav-fg;
    }

    &:hover {
      background: rgba(255, 255, 255, 0.05);
      color: #fff;

      .el-icon {
        color: #fff;
      }
    }

    &.is-active {
      background: $nav-active-bg;
      color: $nav-fg-active;

      .el-icon {
        color: $nav-fg-active;
      }
    }
  }
}

.nav-title {
  padding: 12px 20px 4px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #5f748c;
}

.content {
  min-height: 100vh;
}
</style>
