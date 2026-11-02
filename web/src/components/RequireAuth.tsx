import type { ReactNode } from 'react';
import { useEffect } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { Spin } from 'antd';
import { useAuthStore } from '@/stores/auth';

export function FullScreenLoading({ tip = '正在校验登录状态…' }: { tip?: string }) {
  return (
    <div className="full-center">
      <Spin size="large" tip={tip}>
        <div style={{ padding: 48 }} />
      </Spin>
    </div>
  );
}

/** 需要登录：无 token 跳登录页；有 token 但未校验时先调用 /auth/me。 */
export function RequireAuth({ children }: { children: ReactNode }) {
  const token = useAuthStore((s) => s.token);
  const claims = useAuthStore((s) => s.claims);
  const checked = useAuthStore((s) => s.checked);
  const restore = useAuthStore((s) => s.restore);
  const location = useLocation();

  useEffect(() => {
    if (token && !checked) {
      void restore();
    }
  }, [token, checked, restore]);

  if (!token) {
    const redirect = encodeURIComponent(`${location.pathname}${location.search}`);
    return <Navigate to={`/login?redirect=${redirect}`} replace />;
  }
  if (!checked) {
    return <FullScreenLoading />;
  }
  if (!claims) {
    // /auth/me 失败（网络/服务异常）：不要渲染一个没有身份的应用壳
    return <Navigate to="/login?expired=1" replace />;
  }
  return <>{children}</>;
}

/** 管理员路由：非管理员回首页。 */
export function RequireAdmin({ children }: { children: ReactNode }) {
  const checked = useAuthStore((s) => s.checked);
  const isAdmin = useAuthStore((s) => s.isAdmin);

  if (!checked) {
    return <FullScreenLoading />;
  }
  if (!isAdmin) {
    return <Navigate to="/" replace />;
  }
  return <>{children}</>;
}
