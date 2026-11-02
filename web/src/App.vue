<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useAuthStore } from '@/stores/auth';

const NAV_W = 232;
const NAV_W_COLLAPSED = 56;

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

const collapsed = ref(localStorage.getItem('kamc_nav_collapsed') === '1');
const userMenuOpen = ref(false);

const navWidth = computed(() => (collapsed.value ? NAV_W_COLLAPSED : NAV_W));

interface NavItem {
  label: string;
  to: string;
  icon: string;
  admin?: boolean;
}

// 分组导航（Rancher 侧栏形态）；icon 用内联 emoji 占位，避免引图标库
const groups = computed(() => {
  const all: { title: string; items: NavItem[] }[] = [
    {
      title: '',
      items: [{ label: '概览', to: '/', icon: '⌂' }],
    },
    {
      title: '资源',
      items: [
        { label: '集群', to: '/clusters', icon: '◈', admin: true },
        { label: '权限', to: '/access', icon: '⚿', admin: true },
      ],
    },
    {
      title: '自动化',
      items: [{ label: '绑定脚本', to: '/scripts', icon: 'λ', admin: true }],
    },
    {
      title: '运维',
      items: [{ label: '审计日志', to: '/audit', icon: '≡', admin: true }],
    },
  ];
  return all
    .map((g) => ({ ...g, items: g.items.filter((i) => !i.admin || auth.isAdmin) }))
    .filter((g) => g.items.length > 0);
});

function isActive(to: string): boolean {
  if (to === '/') return route.path === '/';
  return route.path === to || route.path.startsWith(to + '/');
}

function toggleNav(): void {
  collapsed.value = !collapsed.value;
  localStorage.setItem('kamc_nav_collapsed', collapsed.value ? '1' : '0');
}

async function onLogout(): Promise<void> {
  userMenuOpen.value = false;
  await auth.signout();
  router.push('/auth/login');
}
</script>

<template>
  <div class="shell">
    <header class="topbar">
      <div class="topbar-left">
        <button class="icon-btn" title="折叠/展开导航" @click="toggleNav">☰</button>
        <RouterLink to="/" class="brand">
          <span class="brand-mark">K</span>
          <span v-if="!collapsed" class="brand-name">kamc <small>访问管理中心</small></span>
        </RouterLink>
      </div>
      <div class="topbar-right">
        <div v-if="auth.claims" class="user-box" @click="userMenuOpen = !userMenuOpen">
          <span class="avatar">{{ (auth.claims.sub || '?')[0].toUpperCase() }}</span>
          <span class="user-name">{{ auth.claims.sub }}</span>
          <span class="caret">▾</span>
          <div v-if="userMenuOpen" class="user-menu" @click.stop>
            <div class="user-menu-head">
              <div>{{ auth.claims.sub }}</div>
              <div class="muted">{{ auth.claims.email || '—' }}</div>
            </div>
            <RouterLink to="/account" class="user-menu-item" @click="userMenuOpen = false">个人中心</RouterLink>
            <button class="user-menu-item danger" @click="onLogout">退出登录</button>
          </div>
        </div>
      </div>
    </header>

    <aside class="sidenav" :style="{ width: navWidth + 'px' }" @click="userMenuOpen = false">
      <nav v-for="g in groups" :key="g.title" class="nav-group">
        <div v-if="g.title && !collapsed" class="nav-title">{{ g.title }}</div>
        <RouterLink
          v-for="item in g.items"
          :key="item.to"
          :to="item.to"
          class="nav-item"
          :class="{ active: isActive(item.to) }"
          :title="collapsed ? item.label : undefined"
        >
          <span class="nav-icon">{{ item.icon }}</span>
          <span v-if="!collapsed">{{ item.label }}</span>
        </RouterLink>
      </nav>
    </aside>

    <main class="content" :style="{ marginLeft: navWidth + 'px', paddingTop: '52px' }">
      <RouterView />
    </main>
  </div>
</template>

<style lang="scss" scoped>
.shell {
  min-height: 100%;
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

.icon-btn {
  background: transparent;
  border: none;
  color: #c4cfda;
  font-size: 16px;
  cursor: pointer;
  padding: 6px 8px;
  border-radius: 4px;

  &:hover {
    background: rgba(255, 255, 255, 0.08);
    color: #fff;
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
  background: linear-gradient(135deg, #2b7dc9, #45b5a4);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 15px;
}

.brand-name {
  font-size: 15px;
  font-weight: 600;

  small {
    font-weight: 400;
    color: #9fb0c0;
    margin-left: 6px;
    font-size: 12px;
  }
}

.topbar-right {
  display: flex;
  align-items: center;
}

.user-box {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 5px 10px;
  border-radius: 4px;

  &:hover {
    background: rgba(255, 255, 255, 0.08);
  }
}

.avatar {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: #45b5a4;
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 600;
}

.user-name {
  font-size: 13px;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.caret {
  color: #9fb0c0;
  font-size: 11px;
}

.user-menu {
  position: absolute;
  top: 42px;
  right: 0;
  width: 220px;
  background: #fff;
  color: $text;
  border-radius: 6px;
  box-shadow: 0 6px 24px rgba(16, 28, 41, 0.18);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.user-menu-head {
  padding: 10px 14px;
  border-bottom: 1px solid $border;
  font-size: 13px;
  font-weight: 600;

  .muted {
    color: $text-muted;
    font-weight: 400;
    font-size: 12px;
    margin-top: 2px;
  }
}

.user-menu-item {
  padding: 9px 14px;
  font-size: 13px;
  text-align: left;
  background: none;
  border: none;
  cursor: pointer;
  color: $text;

  &:hover {
    background: $content-bg;
    text-decoration: none;
  }

  &.danger {
    color: $error;
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
  padding: 10px 0;
  transition: width 0.15s;
  z-index: 90;
}

.nav-group {
  margin-bottom: 14px;
}

.nav-title {
  padding: 6px 16px 4px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #6d7f90;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  color: $nav-fg;
  font-size: 13.5px;
  text-decoration: none;
  white-space: nowrap;

  &:hover {
    color: $nav-fg-active;
    background: rgba(255, 255, 255, 0.05);
    text-decoration: none;
  }

  &.active {
    color: $nav-fg-active;
    background: $nav-active-bg;
    box-shadow: inset 3px 0 0 $brand;
  }
}

.nav-icon {
  width: 18px;
  text-align: center;
  font-size: 14px;
  flex-shrink: 0;
}

.content {
  min-height: 100vh;
}
</style>
