import { Show, createEffect, createMemo, createSignal, onMount } from 'solid-js';
import { useNavigate, useSearchParams } from '@solidjs/router';
import { ApiError } from '@/api/http';
import { auth } from '@/stores/auth';
import { toast } from '@/lib/storage/toast';
import {
  Alert,
  Button,
  Divider,
  Field,
  FullScreenLoading,
  Input,
  PasswordInput,
  Segmented,
} from '@/lib/widgets';
import { LockOutlined, LoginOutlined, UserOutlined } from '@/lib/icons';

type LoginMode = 'sso' | 'password';

function param(value: string | string[] | undefined): string {
  if (Array.isArray(value)) return value[0] ?? '';
  return value ?? '';
}

/** Mock 模式：脱离后端，登录直接进入本地模拟数据 */
const MOCK = import.meta.env.VITE_APP_MODE === 'mock';

function readMockIdentity(): string {
  try {
    return localStorage.getItem('kamc_mock_user') || 'sysadmin';
  } catch {
    return 'sysadmin';
  }
}

export default function Login() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const initialMock = readMockIdentity();

  const [mode, setMode] = createSignal<LoginMode>(MOCK ? 'password' : 'sso');
  const [mockIdentity, setMockIdentity] = createSignal(initialMock);
  const [username, setUsername] = createSignal(MOCK ? initialMock : '');
  const [password, setPassword] = createSignal(
    MOCK ? (initialMock === 'sysadmin' ? 'sysadmin123' : 'mock') : '',
  );
  const [fieldErrors, setFieldErrors] = createSignal<{ username?: string; password?: string }>({});
  const [submitting, setSubmitting] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [sessionNotice, setSessionNotice] = createSignal({
    expired: false,
    loggedOut: false,
    passwordChanged: false,
  });

  const rawRedirect = createMemo(() => param(searchParams.redirect) || '/');
  const redirect = createMemo(() => {
    const value = rawRedirect();
    return value.startsWith('/') && !value.startsWith('//') ? value : '/';
  });
  const expired = createMemo(
    () => param(searchParams.expired) === '1' || sessionNotice().expired,
  );
  const loggedOut = createMemo(
    () => param(searchParams.logged_out) === '1' || sessionNotice().loggedOut,
  );
  const passwordChanged = createMemo(
    () =>
      param(searchParams.password_changed) === '1' ||
      sessionNotice().passwordChanged,
  );

  onMount(() => {
    try {
      const expiredFlag = sessionStorage.getItem('kamc:expired') === '1';
      const logoutFlag = sessionStorage.getItem('kamc:logged_out') === '1';
      const pwdFlag = sessionStorage.getItem('kamc:password_changed') === '1';
      if (expiredFlag) sessionStorage.removeItem('kamc:expired');
      if (logoutFlag) sessionStorage.removeItem('kamc:logged_out');
      if (pwdFlag) sessionStorage.removeItem('kamc:password_changed');
      if (expiredFlag || logoutFlag || pwdFlag) {
        setSessionNotice({ expired: expiredFlag, loggedOut: logoutFlag, passwordChanged: pwdFlag });
      }
    } catch {
      // 隐私模式下 sessionStorage 不可用
    }
  });

  createEffect(() => {
    if (auth.token() && !auth.checked()) {
      void auth.restore();
    }
  });

  createEffect(() => {
    if (auth.token() && auth.claims()) {
      navigate(redirect(), { replace: true });
    }
  });

  const onPasswordLogin = async (event: SubmitEvent) => {
    event.preventDefault();
    const errors: { username?: string; password?: string } = {};
    if (!username().trim()) errors.username = '请输入用户名';
    if (!password()) errors.password = '请输入密码';
    setFieldErrors(errors);
    if (errors.username || errors.password) return;

    setSubmitting(true);
    setError(null);
    try {
      await auth.signInWithPassword({
        username: username().trim(),
        password: password(),
      });
      toast.success('登录成功');
      navigate(redirect(), { replace: true });
    } catch (err) {
      if (err instanceof ApiError && err.httpStatus === 404) {
        setError('账号密码登录未启用。');
      } else if (err instanceof ApiError && err.httpStatus === 401) {
        setError('用户名或密码错误。');
      } else {
        setError(err instanceof Error ? err.message : '登录失败');
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Show
      when={auth.token() && !auth.checked()}
      fallback={
        <div class="login-page">
          <div class="login-card">
            <section class="login-left">
              <img src="/icon.png" alt="kamc" class="login-logo" />
              <div class="login-left-sub">Kubernetes Access Manager Center</div>
              <p class="login-left-desc">
                统一身份、集中授权、可审计的多集群 Kubernetes 访问入口
              </p>
            </section>

            <section class="login-right">
              <div class="login-card-head">
                <h1 class="login-heading">登录访问管理中心</h1>
                <div class="login-sub">
                  {MOCK ? 'Mock 模式：本地模拟数据，无需后端' : '使用组织身份登录，或使用账号密码'}
                </div>
              </div>

              <Show when={expired()}>
                <Alert
                  type="warning"
                  message="登录已过期，请重新登录"
                  class="login-notice"
                />
              </Show>
              <Show when={loggedOut()}>
                <Alert type="success" message="已退出登录" class="login-notice" />
              </Show>
              <Show when={passwordChanged()}>
                <Alert
                  type="success"
                  message="密码已修改，请使用新密码重新登录"
                  class="login-notice"
                />
              </Show>
              <Show when={error()}>
                <Alert
                  type="error"
                  message={error()}
                  closable
                  onClose={() => setError(null)}
                  class="login-notice"
                />
              </Show>

              <Show
                when={mode() === 'sso' && !MOCK}
                fallback={
                  <form onSubmit={(event) => void onPasswordLogin(event)}>
                    <Show when={MOCK}>
                      <div class="login-mock-switch">
                        <span class="text-[13px] text-layer-content/50">Mock 身份</span>
                        <Segmented
                          value={mockIdentity()}
                          options={[
                            { label: '管理员', value: 'sysadmin' },
                            { label: '普通用户 alice', value: 'alice' },
                          ]}
                          onChange={(value) => {
                            setMockIdentity(value);
                            setError(null);
                            setUsername(value);
                            setPassword(value === 'sysadmin' ? 'sysadmin123' : 'mock');
                            setFieldErrors({});
                          }}
                        />
                      </div>
                    </Show>
                    <Field
                      label="用户名"
                      required
                      error={fieldErrors().username}
                      for="login-username"
                      class="mb-4"
                    >
                      <Input
                        id="login-username"
                        value={username()}
                        onValueChange={setUsername}
                        prefix={<UserOutlined class="h-4 w-4" />}
                        placeholder="请输入用户名"
                        autocomplete="username"
                        status={fieldErrors().username ? 'error' : undefined}
                      />
                    </Field>
                    <Field
                      label="密码"
                      required
                      error={fieldErrors().password}
                      for="login-password"
                      class="mb-4"
                    >
                      <PasswordInput
                        id="login-password"
                        value={password()}
                        onValueChange={setPassword}
                        prefix={<LockOutlined class="h-4 w-4" />}
                        placeholder="请输入密码"
                        autocomplete="current-password"
                        status={fieldErrors().password ? 'error' : undefined}
                      />
                    </Field>
                    <Button
                      type="ghost"
                      block
                      htmlType="submit"
                      loading={submitting()}
                      class="login-outline-btn"
                    >
                      登录
                    </Button>
                    <Show when={!MOCK}>
                      <Button type="link" block onClick={() => setMode('sso')}>
                        返回 SSO 登录
                      </Button>
                    </Show>
                  </form>
                }
              >
                <Button
                  type="primary"
                  block
                  icon={<LoginOutlined class="h-4 w-4" />}
                  onClick={() => window.location.assign('/auth/login')}
                >
                  使用 SSO 登录
                </Button>
                <Divider>或</Divider>
                <Button
                  type="ghost"
                  block
                  class="login-outline-btn"
                  icon={<UserOutlined class="h-4 w-4" />}
                  onClick={() => setMode('password')}
                >
                  使用用户名和密码
                </Button>
              </Show>

              <Show when={MOCK}>
                <div class="login-card-foot">Mock 模式：数据为前端本地模拟，刷新后重置</div>
              </Show>
            </section>
          </div>
        </div>
      }
    >
      <FullScreenLoading tip="正在校验登录状态…" />
    </Show>
  );
}
