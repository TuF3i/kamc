import { useCallback, useEffect, useState } from 'react';
import { Alert, Button, Card, Descriptions, Skeleton, Typography } from 'antd';
import {
  ClockCircleOutlined,
  CodeOutlined,
  ThunderboltOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { getPlatformInfo } from '@/api/platform';
import type { PlatformInfo } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { fmtBytes, fmtUptime } from '@/utils/format';

function masked(value?: string | null): string {
  return value ? value : '—';
}

export function Platform() {
  const [info, setInfo] = useState<PlatformInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setInfo(await getPlatformInfo());
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载平台信息失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <div>
      <PageHeader
        title="平台信息"
        subtitle="平台配置快照与运行时状态；密钥与明文密码已脱敏，仅管理员可见。"
        extra={
          <Button icon={<ReloadOutlined />} onClick={() => void load()} loading={loading}>
            刷新
          </Button>
        }
      />

      {error ? (
        <Alert
          type="error"
          showIcon
          message="平台信息加载失败"
          description={error}
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" onClick={() => void load()}>
              重试
            </Button>
          }
        />
      ) : null}

      {loading && !info ? (
        <Card className="soft-card">
          <Skeleton active paragraph={{ rows: 12 }} />
        </Card>
      ) : info ? (
        <div className="tab-stack">
          <Card title="平台设置" className="soft-card">
            <Descriptions column={{ xs: 1, md: 2 }} size="middle">
              <Descriptions.Item label="平台版本">
                <code className="mono strong">
                  {info.version.version} ({info.version.commit})
                </code>
              </Descriptions.Item>
              <Descriptions.Item label="监听地址">
                <code className="mono">{masked(info.settings.listen_addr)}</code>
              </Descriptions.Item>
              <Descriptions.Item label="外部访问地址">
                <code className="mono">{masked(info.settings.external_url)}</code>
              </Descriptions.Item>
              <Descriptions.Item label="管理员用户组">
                <code className="mono">{masked(info.settings.admin_group)}</code>
              </Descriptions.Item>
              <Descriptions.Item label="账号密码登录">
                {info.settings.password_login ? (
                  <Typography.Text>
                    已启用（内置账号 <code className="mono">{info.settings.password_user}</code>）
                  </Typography.Text>
                ) : (
                  <Typography.Text type="secondary">已关闭</Typography.Text>
                )}
              </Descriptions.Item>
              <Descriptions.Item label="Robot Token 有效期">
                {fmtUptime(info.settings.robot_token_ttl)}
              </Descriptions.Item>
              <Descriptions.Item label="Agent 镜像" span={2}>
                <code className="mono strong">{info.settings.agent_image}</code>
                <Typography.Text type="secondary" style={{ marginLeft: 8 }}>
                  仓库由 AM_AGENT_IMAGE_REPO 设置，tag 随平台版本
                </Typography.Text>
              </Descriptions.Item>
            </Descriptions>
          </Card>

          <Card title="SSO 接入" className="soft-card">
            {info.sso.enabled ? (
              <Descriptions column={{ xs: 1, md: 2 }} size="middle">
                <Descriptions.Item label="状态">
                  <Typography.Text type="success">已启用</Typography.Text>
                </Descriptions.Item>
                <Descriptions.Item label="Issuer">
                  <code className="mono">{masked(info.sso.issuer)}</code>
                </Descriptions.Item>
                <Descriptions.Item label="Client ID">
                  <code className="mono">{masked(info.sso.client_id)}</code>
                </Descriptions.Item>
                <Descriptions.Item label="回调地址">
                  <code className="mono">{masked(info.sso.redirect_url)}</code>
                </Descriptions.Item>
              </Descriptions>
            ) : (
              <Alert
                type="info"
                showIcon
                message="SSO 未配置"
                description="设置 OIDC_ISSUER / OIDC_CLIENT_ID / OIDC_CLIENT_SECRET / OIDC_REDIRECT_URL 后启用 Authentik SSO 登录；当前仅内置账号密码登录可用。"
              />
            )}
          </Card>

          <Card title="数据库" className="soft-card">
            <Descriptions column={{ xs: 1, md: 2 }} size="middle">
              <Descriptions.Item label="连接串（已脱敏）" span={2}>
                <code className="mono">{masked(info.database.dsn)}</code>
              </Descriptions.Item>
              <Descriptions.Item label="连接池上限">
                {info.database.max_open_connections || '—'}
              </Descriptions.Item>
              <Descriptions.Item label="当前连接">
                在用 {info.database.in_use} / 空闲 {info.database.idle}（共{' '}
                {info.database.open_connections}）
              </Descriptions.Item>
              <Descriptions.Item label="等待请求数">
                {info.database.wait_count}
              </Descriptions.Item>
            </Descriptions>
          </Card>

          <Card title="缓存（Redis）" className="soft-card">
            {info.cache.enabled ? (
              <Descriptions column={{ xs: 1, md: 2 }} size="middle">
                <Descriptions.Item label="地址">
                  <code className="mono">{masked(info.cache.addrs)}</code>
                </Descriptions.Item>
                <Descriptions.Item label="逻辑库">DB {info.cache.db}</Descriptions.Item>
              </Descriptions>
            ) : (
              <Alert
                type="info"
                showIcon
                message="Redis 未配置"
                description="设置 REDIS_ADDR / REDIS_ADDRESSES 后启用：跨副本策略同步、Token 吊销共享、分布式限流与隧道跨 Pod 路由依赖 Redis；未配置时以上能力退化为单副本语义。"
              />
            )}
          </Card>

          <Card title="Authentik API" className="soft-card">
            {info.authentik.configured ? (
              <Descriptions column={{ xs: 1, md: 2 }} size="middle">
                <Descriptions.Item label="API 地址">
                  <code className="mono">{masked(info.authentik.api_url)}</code>
                </Descriptions.Item>
                <Descriptions.Item label="Token">
                  <Typography.Text type="success">已设置（不展示）</Typography.Text>
                </Descriptions.Item>
                <Descriptions.Item label="用户标识字段">
                  <code className="mono">{masked(info.authentik.user_sub_field)}</code>
                </Descriptions.Item>
                <Descriptions.Item label="目录缓存 TTL">
                  {info.authentik.cache_ttl_seconds} 秒
                </Descriptions.Item>
              </Descriptions>
            ) : (
              <Alert
                type="info"
                showIcon
                message="Authentik 目录未配置"
                description="设置 AUTHENTIK_API_URL / AUTHENTIK_API_TOKEN 后，策略编辑器可下拉选择 SSO 组织与用户。"
              />
            )}
          </Card>

          <Card title="Go 运行时" className="soft-card">
            <div className="runtime-grid">
              <div className="runtime-metric">
                <div className="runtime-icon">
                  <CodeOutlined />
                </div>
                <div className="runtime-label">Go 版本</div>
                <div className="runtime-value mono">{info.runtime.go_version}</div>
              </div>
              <div className="runtime-metric">
                <div className="runtime-icon">
                  <ClockCircleOutlined />
                </div>
                <div className="runtime-label">运行时长</div>
                <div className="runtime-value">{fmtUptime(info.runtime.uptime_seconds)}</div>
              </div>
              <div className="runtime-metric">
                <div className="runtime-icon">
                  <ThunderboltOutlined />
                </div>
                <div className="runtime-label">Goroutines</div>
                <div className="runtime-value">{info.runtime.goroutines}</div>
              </div>
              <div className="runtime-metric">
                <div className="runtime-icon">
                  <CodeOutlined />
                </div>
                <div className="runtime-label">CPU 核数</div>
                <div className="runtime-value">{info.runtime.num_cpu}</div>
              </div>
            </div>
            <Descriptions column={{ xs: 1, md: 2 }} size="middle" style={{ marginTop: 16 }}>
              <Descriptions.Item label="堆内存（在用）">
                {fmtBytes(info.runtime.heap_alloc_bytes)}
              </Descriptions.Item>
              <Descriptions.Item label="进程内存（Sys）">
                {fmtBytes(info.runtime.sys_bytes)}
              </Descriptions.Item>
              <Descriptions.Item label="累计分配">
                {fmtBytes(info.runtime.total_alloc_bytes)}
              </Descriptions.Item>
              <Descriptions.Item label="GC 次数">{info.runtime.num_gc}</Descriptions.Item>
            </Descriptions>
          </Card>
        </div>
      ) : null}
    </div>
  );
}
