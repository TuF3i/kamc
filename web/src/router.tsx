import { lazy, Suspense, type ReactNode } from 'react';
import { createBrowserRouter } from 'react-router-dom';
import { FullScreenLoading, RequireAdmin, RequireAuth } from '@/components/RequireAuth';
import { AppLayout } from '@/layouts/AppLayout';
import { Login } from '@/pages/Login';

const Dashboard = lazy(() => import('@/pages/Dashboard').then((m) => ({ default: m.Dashboard })));
const Clusters = lazy(() => import('@/pages/Clusters').then((m) => ({ default: m.Clusters })));
const ClusterDetail = lazy(() =>
  import('@/pages/ClusterDetail').then((m) => ({ default: m.ClusterDetail })),
);
const Access = lazy(() => import('@/pages/Access').then((m) => ({ default: m.Access })));
const Audit = lazy(() => import('@/pages/Audit').then((m) => ({ default: m.Audit })));
const Account = lazy(() => import('@/pages/Account').then((m) => ({ default: m.Account })));
const Kubeconfig = lazy(() =>
  import('@/pages/Kubeconfig').then((m) => ({ default: m.Kubeconfig })),
);
const Robots = lazy(() => import('@/pages/Robots').then((m) => ({ default: m.Robots })));
const Platform = lazy(() => import('@/pages/Platform').then((m) => ({ default: m.Platform })));
const NotFound = lazy(() => import('@/pages/NotFound').then((m) => ({ default: m.NotFound })));

function page(node: ReactNode): ReactNode {
  return <Suspense fallback={<FullScreenLoading tip="正在加载页面…" />}>{node}</Suspense>;
}

export const router = createBrowserRouter([
  { path: '/login', element: <Login /> },
  {
    path: '/',
    element: (
      <RequireAuth>
        <AppLayout />
      </RequireAuth>
    ),
    children: [
      { index: true, element: page(<Dashboard />) },
      {
        path: 'clusters',
        element: page(
          <RequireAdmin>
            <Clusters />
          </RequireAdmin>,
        ),
      },
      {
        path: 'clusters/:id',
        element: page(
          <RequireAdmin>
            <ClusterDetail />
          </RequireAdmin>,
        ),
      },
      {
        path: 'access',
        element: page(
          <RequireAdmin>
            <Access />
          </RequireAdmin>,
        ),
      },
      {
        path: 'audit',
        element: page(
          <RequireAdmin>
            <Audit />
          </RequireAdmin>,
        ),
      },
      {
        path: 'robots',
        element: page(
          <RequireAdmin>
            <Robots />
          </RequireAdmin>,
        ),
      },
      { path: 'kubeconfig', element: page(<Kubeconfig />) },
      {
        path: 'platform',
        element: page(
          <RequireAdmin>
            <Platform />
          </RequireAdmin>,
        ),
      },
      { path: 'account', element: page(<Account />) },
      { path: '*', element: page(<NotFound />) },
    ],
  },
]);
