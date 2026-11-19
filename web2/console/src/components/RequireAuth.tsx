import { Show, onMount, createEffect, onCleanup, type JSX } from 'solid-js';
import { Navigate, useLocation, type Location } from '@solidjs/router';
import { auth } from '@/stores/auth';
import { FullScreenLoading } from '@/lib/widgets';

/** 当前 hash 路由路径（hash 路由下 window.location.pathname 恒为 /）。 */
function currentHashPath(): string {
  const hash = window.location.hash.replace(/^#/, '');
  return hash.split('?')[0] || '/';
}

function NoToken(props: { location: Location }) {
  if (currentHashPath() === '/login') {
    return <FullScreenLoading />;
  }
  const redirect = encodeURIComponent(`${props.location.pathname}${props.location.search}`);
  return <Navigate href={`/login?redirect=${redirect}`} />;
}

/** 需要登录：无 token 跳登录页；有 token 但未校验时先调用 /auth/me。 */
export function RequireAuth(props: { children: JSX.Element }) {
  const location = useLocation();

  onMount(() => {
    const onUnauthorized = () => auth.clearLocal();
    window.addEventListener('kamc:unauthorized', onUnauthorized);
    onCleanup(() => window.removeEventListener('kamc:unauthorized', onUnauthorized));
  });

  createEffect(() => {
    if (auth.token() && !auth.checked()) {
      void auth.restore();
    }
  });

  return (
    <Show when={auth.token()} fallback={<NoToken location={location} />}>
      <Show when={auth.checked()} fallback={<FullScreenLoading />}>
        <Show when={auth.claims()} fallback={<Navigate href="/login?expired=1" />}>
          {props.children}
        </Show>
      </Show>
    </Show>
  );
}

/** 管理员路由：非管理员回首页。 */
export function RequireAdmin(props: { children: JSX.Element }) {
  return (
    <Show when={auth.checked()} fallback={<FullScreenLoading />}>
      <Show when={auth.isAdmin()} fallback={<Navigate href="/" />}>
        {props.children}
      </Show>
    </Show>
  );
}
