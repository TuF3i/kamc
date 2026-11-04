import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Alert, App, Button, Divider, Form, Input, Spin, Typography } from 'antd';
import { LockOutlined, LoginOutlined, UserOutlined } from '@ant-design/icons';
import { ApiError } from '@/api/http';
import { useAuthStore } from '@/stores/auth';

interface PasswordForm {
  username: string;
  password: string;
}

type LoginMode = 'sso' | 'password';

/** Mock 模式：脱离后端，登录直接进入本地模拟数据 */
const MOCK = import.meta.env.VITE_APP_MODE === 'mock';

export function Login() {
  const { message } = App.useApp();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [mode, setMode] = useState<LoginMode>(MOCK ? 'password' : 'sso');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const token = useAuthStore((s) => s.token);
  const claims = useAuthStore((s) => s.claims);
  const checked = useAuthStore((s) => s.checked);
  const restore = useAuthStore((s) => s.restore);
  const signInWithPassword = useAuthStore((s) => s.signInWithPassword);

  const rawRedirect = searchParams.get('redirect') || '/';
  const redirect =
    rawRedirect.startsWith('/') && !rawRedirect.startsWith('//') ? rawRedirect : '/';
  const expired = searchParams.get('expired') === '1';
  const loggedOut = searchParams.get('logged_out') === '1';

  useEffect(() => {
    if (token && !checked) {
      void restore();
    }
  }, [token, checked, restore]);

  useEffect(() => {
    if (token && claims) {
      navigate(redirect, { replace: true });
    }
  }, [token, claims, redirect, navigate]);

  const onPasswordLogin = async (values: PasswordForm) => {
    setSubmitting(true);
    setError(null);
    try {
      await signInWithPassword({
        username: values.username.trim(),
        password: values.password,
      });
      message.success('登录成功');
      navigate(redirect, { replace: true });
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

  if (token && !checked) {
    return (
      <div className="full-center">
        <Spin size="large" tip="正在校验登录状态…">
          <div style={{ padding: 48 }} />
        </Spin>
      </div>
    );
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <section className="login-left">
          <img src="/icon.png" alt="kamc" className="login-logo" />
          <div className="login-left-sub">Kubernetes Access Manager Center</div>
          <p className="login-left-desc">
            统一身份、集中授权、可审计的多集群 Kubernetes 访问入口
          </p>
        </section>

        <section className="login-right">
          <div className="login-card-head">
            <Typography.Title level={3}>登录访问管理中心</Typography.Title>
            <Typography.Text type="secondary">
              {MOCK ? 'Mock 模式：本地模拟数据，无需后端' : '使用组织身份登录，或使用账号密码'}
            </Typography.Text>
          </div>

          {expired ? (
            <Alert
              type="warning"
              showIcon
              message="登录已过期，请重新登录"
              style={{ marginBottom: 16 }}
            />
          ) : null}
          {loggedOut ? (
            <Alert
              type="success"
              showIcon
              message="已退出登录"
              style={{ marginBottom: 16 }}
            />
          ) : null}
          {error ? (
            <Alert
              type="error"
              showIcon
              message={error}
              closable
              onClose={() => setError(null)}
              style={{ marginBottom: 16 }}
            />
          ) : null}

          {mode === 'sso' && !MOCK ? (
            <>
              <Button
                type="primary"
                size="large"
                block
                icon={<LoginOutlined />}
                onClick={() => window.location.assign('/auth/login')}
              >
                使用 SSO 登录
              </Button>
              <Divider plain className="login-divider">
                或
              </Divider>
              <Button
                type="default"
                size="large"
                block
                className="login-outline-btn"
                icon={<UserOutlined />}
                onClick={() => setMode('password')}
              >
                使用用户名和密码
              </Button>
            </>
          ) : (
            <Form<PasswordForm>
              layout="vertical"
              onFinish={onPasswordLogin}
              initialValues={MOCK ? { username: 'sysadmin', password: 'sysadmin123' } : undefined}
            >
              <Form.Item
                name="username"
                label="用户名"
                rules={[{ required: true, message: '请输入用户名' }]}
              >
                <Input prefix={<UserOutlined />} placeholder="请输入用户名" autoComplete="username" />
              </Form.Item>
              <Form.Item
                name="password"
                label="密码"
                rules={[{ required: true, message: '请输入密码' }]}
              >
                <Input.Password
                  prefix={<LockOutlined />}
                  placeholder="请输入密码"
                  autoComplete="current-password"
                />
              </Form.Item>
              <Button
                type="default"
                size="large"
                block
                htmlType="submit"
                loading={submitting}
                className="login-outline-btn"
              >
                登录
              </Button>
              {MOCK ? null : (
                <Button type="link" block onClick={() => setMode('sso')}>
                  返回 SSO 登录
                </Button>
              )}
            </Form>
          )}

          {MOCK ? (
            <div className="login-card-foot">Mock 模式：数据为前端本地模拟，刷新后重置</div>
          ) : null}
        </section>
      </div>
    </div>
  );
}
