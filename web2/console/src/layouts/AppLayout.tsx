import { For, Show, createMemo, type Component, type JSX } from 'solid-js';
import { useLocation, useNavigate } from '@solidjs/router';
import {
  ClusterOutlined,
  CloudDownloadOutlined,
  DashboardOutlined,
  FileSearchOutlined,
  InfoCircleOutlined,
  LogoutOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  UserOutlined,
  type IconProps,
} from '@/lib/icons';
import { Avatar, Popover, Tag } from '@/lib/widgets';
import { ThemeToggle } from '@/lib/blocks/theme-toggle';
import { auth } from '@/stores/auth';

interface NavItem {
  key: string;
  label: string;
  icon: Component<IconProps>;
}

const ADMIN_NAV: NavItem[] = [
  { key: '/', label: '概览', icon: DashboardOutlined },
  { key: '/clusters', label: '集群', icon: ClusterOutlined },
  { key: '/access', label: '权限', icon: SafetyCertificateOutlined },
  { key: '/robots', label: '机器人', icon: RobotOutlined },
  { key: '/audit', label: '审计日志', icon: FileSearchOutlined },
  { key: '/kubeconfig', label: 'Kubeconfig', icon: CloudDownloadOutlined },
  { key: '/platform', label: '平台信息', icon: InfoCircleOutlined },
  { key: '/account', label: '个人中心', icon: UserOutlined },
];

const USER_NAV: NavItem[] = [
  { key: '/kubeconfig', label: 'Kubeconfig', icon: CloudDownloadOutlined },
  { key: '/account', label: '个人中心', icon: UserOutlined },
];

function activeKey(pathname: string): string {
  if (pathname.startsWith('/clusters')) return '/clusters';
  if (pathname.startsWith('/access')) return '/access';
  if (pathname.startsWith('/audit')) return '/audit';
  if (pathname.startsWith('/robots')) return '/robots';
  if (pathname.startsWith('/kubeconfig')) return '/kubeconfig';
  if (pathname.startsWith('/platform')) return '/platform';
  if (pathname.startsWith('/account')) return '/account';
  return '/';
}

export function AppLayout(props: { children?: JSX.Element }) {
  const location = useLocation();
  const navigate = useNavigate();

  const navItems = createMemo(() => (auth.isAdmin() ? ADMIN_NAV : USER_NAV));
  const current = createMemo(() => activeKey(location.pathname));
  const displayName = createMemo(
    () => auth.profile()?.nickname || auth.claims()?.sub || '未登录',
  );
  const avatarChar = createMemo(() => displayName().slice(0, 1).toUpperCase());
  const home = createMemo(() => (auth.isAdmin() ? '/' : '/kubeconfig'));

  return (
    <div class="app-shell">
      <header class="floating-nav">
        <button
          type="button"
          class="nav-panel nav-logo"
          onClick={() => navigate(home())}
          aria-label="kamc 首页"
          title="kamc"
        >
          <img src="/icon-mark.png" alt="kamc" class="brand-logo" />
        </button>

        <nav class="nav-panel nav-tabs" aria-label="主导航">
          <For each={navItems()}>
            {(item) => (
              <button
                type="button"
                class={`nav-tab${current() === item.key ? ' active' : ''}`}
                onClick={() => navigate(item.key)}
                aria-current={current() === item.key ? 'page' : undefined}
              >
                {item.icon({ class: 'h-[15px] w-[15px]' })}
                <span>{item.label}</span>
              </button>
            )}
          </For>
        </nav>

        <Popover
          class="justify-self-end ml-auto"
          placement="bottom-end"
          minWidth={248}
          panelClass="w-64 p-2"
          trigger={
            <button type="button" class="nav-panel nav-user">
              <Avatar size={30} src={auth.profile()?.avatar_url || undefined}>
                {avatarChar()}
              </Avatar>
              <span class="user-name">{displayName()}</span>
              <Show when={auth.isAdmin()}>
                <Tag level="primary" class="admin-tag">
                  管理员
                </Tag>
              </Show>
              <span class="user-chevron">▾</span>
            </button>
          }
        >
          <div class="flex items-center gap-2.5 px-2 py-2">
            <Avatar size={36} src={auth.profile()?.avatar_url || undefined}>
              {avatarChar()}
            </Avatar>
            <div class="min-w-0 flex-1">
              <div class="truncate text-[13px] font-bold">{displayName()}</div>
              <div class="truncate text-[11px] text-layer-content/50">
                {auth.profile()?.email || auth.claims()?.email || '未设置邮箱'}
              </div>
            </div>
          </div>
          <div class="menu-sep" />
          <div class="flex items-center justify-between gap-3 px-2 py-2">
            <span class="text-xs text-layer-content/50">主题</span>
            <div class="w-28">
              <ThemeToggle />
            </div>
          </div>
          <div class="menu-sep" />
          <button
            type="button"
            class="menu-item menu-item-danger"
            onClick={() => void auth.signOut()}
          >
            {LogoutOutlined({ class: 'h-4 w-4' })}
            <span>退出登录</span>
          </button>
        </Popover>
      </header>

      <main class="app-content">
        <div class="app-content-inner">
          {props.children}
        </div>
        <div class="app-footer-text">Kubernetes Access Manager Center</div>
      </main>
    </div>
  );
}
