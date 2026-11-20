import { Suspense, lazy, type Component, type JSX } from 'solid-js';
import { HashRouter, Route } from '@solidjs/router';
import { AppLayout } from '@/layouts/AppLayout';
import { RequireAdmin, RequireAuth } from '@/components/RequireAuth';
import { FullScreenBoot } from '@/lib/widgets';

const Login = lazy(() => import('@/pages/Login'));
const Dashboard = lazy(() => import('@/pages/Dashboard'));
const Clusters = lazy(() => import('@/pages/Clusters'));
const ClusterDetail = lazy(() => import('@/pages/ClusterDetail'));
const Access = lazy(() => import('@/pages/Access'));
const Audit = lazy(() => import('@/pages/Audit'));
const Robots = lazy(() => import('@/pages/Robots'));
const Kubeconfig = lazy(() => import('@/pages/Kubeconfig'));
const Platform = lazy(() => import('@/pages/Platform'));
const Account = lazy(() => import('@/pages/Account'));
const NotFound = lazy(() => import('@/pages/NotFound'));

function ProtectedLayout(props: { children?: JSX.Element }) {
  return (
    <RequireAuth>
      <AppLayout>{props.children}</AppLayout>
    </RequireAuth>
  );
}

function adminPage(Page: Component) {
  return () => (
    <RequireAdmin>
      <Page />
    </RequireAdmin>
  );
}

export default function App() {
  return (
    <Suspense fallback={<FullScreenBoot tip="正在加载页面…" />}>
      <HashRouter>
        <Route path="/login" component={Login} />
        <Route path="/" component={ProtectedLayout}>
          <Route path="/" component={Dashboard} />
          <Route path="/clusters" component={adminPage(Clusters)} />
          <Route path="/clusters/:id" component={adminPage(ClusterDetail)} />
          <Route path="/access" component={adminPage(Access)} />
          <Route path="/audit" component={adminPage(Audit)} />
          <Route path="/robots" component={adminPage(Robots)} />
          <Route path="/kubeconfig" component={Kubeconfig} />
          <Route path="/platform" component={adminPage(Platform)} />
          <Route path="/account" component={Account} />
          <Route path="*" component={NotFound} />
        </Route>
      </HashRouter>
    </Suspense>
  );
}
