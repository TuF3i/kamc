import { useMemo, type ReactNode } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { Avatar, Dropdown, Tag, type MenuProps } from 'antd';
import {
  ClusterOutlined,
  CloudDownloadOutlined,
  DashboardOutlined,
  DownOutlined,
  FileSearchOutlined,
  LogoutOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { useAuthStore } from '@/stores/auth';

interface NavItem {
  key: string;
  label: string;
  icon: ReactNode;
}

const ADMIN_NAV: NavItem[] = [
  { key: '/', label: '概览', icon: <DashboardOutlined /> },
  { key: '/clusters', label: '集群', icon: <ClusterOutlined /> },
  { key: '/access', label: '权限', icon: <SafetyCertificateOutlined /> },
  { key: '/robots', label: '机器人', icon: <RobotOutlined /> },
  { key: '/audit', label: '审计日志', icon: <FileSearchOutlined /> },
  { key: '/kubeconfig', label: 'Kubeconfig', icon: <CloudDownloadOutlined /> },
  { key: '/account', label: '个人中心', icon: <UserOutlined /> },
];

const USER_NAV: NavItem[] = [
  { key: '/kubeconfig', label: 'Kubeconfig', icon: <CloudDownloadOutlined /> },
  { key: '/account', label: '个人中心', icon: <UserOutlined /> },
];

function activeKey(pathname: string): string {
  if (pathname.startsWith('/clusters')) return '/clusters';
  if (pathname.startsWith('/access')) return '/access';
  if (pathname.startsWith('/audit')) return '/audit';
  if (pathname.startsWith('/robots')) return '/robots';
  if (pathname.startsWith('/kubeconfig')) return '/kubeconfig';
  if (pathname.startsWith('/account')) return '/account';
  return '/';
}

export function AppLayout() {
  const location = useLocation();
  const navigate = useNavigate();
  const claims = useAuthStore((s) => s.claims);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const signOut = useAuthStore((s) => s.signOut);

  const navItems = useMemo(() => (isAdmin ? ADMIN_NAV : USER_NAV), [isAdmin]);
  const current = activeKey(location.pathname);

  const userMenu: MenuProps = {
    items: [
      { key: 'logout', icon: <LogoutOutlined />, label: '退出登录', danger: true },
    ],
    onClick: ({ key }) => {
      if (key === 'logout') {
        void signOut();
      }
    },
  };

  const displayName = claims?.sub ?? '未登录';
  const avatarChar = displayName.slice(0, 1).toUpperCase();
  const home = isAdmin ? '/' : '/kubeconfig';

  return (
    <div className="app-shell">
      <header className="floating-nav">
        <button
          type="button"
          className="nav-panel nav-logo"
          onClick={() => navigate(home)}
          aria-label="kamc 首页"
          title="kamc"
        >
          <img src="/icon-mark.png" alt="kamc" className="brand-logo" />
        </button>

        <nav className="nav-panel nav-tabs" aria-label="主导航">
          {navItems.map((item) => (
            <button
              key={item.key}
              type="button"
              className={`nav-tab${current === item.key ? ' active' : ''}`}
              onClick={() => navigate(item.key)}
              aria-current={current === item.key ? 'page' : undefined}
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}
        </nav>

        <Dropdown menu={userMenu} trigger={['click']} placement="bottomRight">
          <button type="button" className="nav-panel nav-user">
            <Avatar size={30} className="user-avatar">
              {avatarChar}
            </Avatar>
            <span className="user-name">{displayName}</span>
            {isAdmin ? <Tag color="blue" className="admin-tag">管理员</Tag> : null}
            <DownOutlined className="user-chevron" />
          </button>
        </Dropdown>
      </header>

      <main className="app-content">
        <div className="app-content-inner" key={location.pathname}>
          <Outlet />
        </div>
        <div className="app-footer-text">kamc · Kubernetes Access Manager Center</div>
      </main>
    </div>
  );
}
