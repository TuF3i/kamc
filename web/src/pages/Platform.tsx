import { useCallback, useEffect, useState } from 'react';
import { Alert, Button, Card, Skeleton, Table, Typography, type TableColumnsType } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { getPlatformInfo } from '@/api/platform';
import type { PlatformInfo } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { fmtBytes, fmtUptime } from '@/utils/format';

interface InfoRow {
  key: string;
  group: string;
  setting: string;
  value: string;
}

/** 平台信息：单一表格（建→值），密钥与明文密码不出现在响应中。 */
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

  const rows: InfoRow[] = [];
  const push = (group: string, setting: string, value: string | number | undefined | null) => {
    rows.push({
      key: `${group}.${setting}`,
      group,
      setting,
      value: value === undefined || value === null || value === '' ? '—' : String(value),
    });
  };

  if (info) {
    push('平台设置', 'version', `${info.version.version} (${info.version.commit})`);
    push('平台设置', 'listen_addr', info.settings.listen_addr);
    push('平台设置', 'external_url', info.settings.external_url);
    push('平台设置', 'admin_group', info.settings.admin_group);
    push('平台设置', 'password_login', info.settings.password_login ? `已启用（${info.settings.password_user}）` : '已关闭');
    push('平台设置', 'robot_token_ttl', fmtUptime(info.settings.robot_token_ttl));
    push('平台设置', 'agent_image', info.settings.agent_image);
    push('SSO 接入', 'enabled', info.sso.enabled ? '已启用' : '未配置');
    if (info.sso.enabled) {
      push('SSO 接入', 'issuer', info.sso.issuer);
      push('SSO 接入', 'client_id', info.sso.client_id);
      push('SSO 接入', 'redirect_url', info.sso.redirect_url);
    }
    push('数据库', 'dsn', info.database.dsn);
    push('数据库', 'max_open_connections', info.database.max_open_connections);
    push('数据库', 'connections', `在用 ${info.database.in_use} / 空闲 ${info.database.idle} / 共 ${info.database.open_connections}`);
    push('数据库', 'wait_count', info.database.wait_count);
    push('缓存（Redis）', 'enabled', info.cache.enabled ? '已启用' : '未配置');
    if (info.cache.enabled) {
      push('缓存（Redis）', 'addrs', info.cache.addrs);
      push('缓存（Redis）', 'db', info.cache.db);
    }
    push('Authentik API', 'configured', info.authentik.configured ? '已配置' : '未配置');
    if (info.authentik.configured) {
      push('Authentik API', 'api_url', info.authentik.api_url);
      push('Authentik API', 'user_sub_field', info.authentik.user_sub_field);
      push('Authentik API', 'cache_ttl_seconds', info.authentik.cache_ttl_seconds);
    }
    push('Go 运行时', 'go_version', info.runtime.go_version);
    push('Go 运行时', 'uptime', fmtUptime(info.runtime.uptime_seconds));
    push('Go 运行时', 'goroutines', info.runtime.goroutines);
    push('Go 运行时', 'num_cpu', info.runtime.num_cpu);
    push('Go 运行时', 'heap_alloc', fmtBytes(info.runtime.heap_alloc_bytes));
    push('Go 运行时', 'sys', fmtBytes(info.runtime.sys_bytes));
    push('Go 运行时', 'num_gc', info.runtime.num_gc);
  }

  const columns: TableColumnsType<InfoRow> = [
    {
      title: '分组',
      dataIndex: 'group',
      key: 'group',
      width: 160,
      onCell: (_, index) => {
        // 分组列纵向合并：同组首行显示，其余被覆盖
        const first = rows[index ?? -1];
        const isFirstOfGroup = first && rows.findIndex((r) => r.group === first.group) === index;
        return isFirstOfGroup ? { rowSpan: rows.filter((r) => r.group === first.group).length } : { rowSpan: 0 };
      },
      render: (group: string) => <strong>{group}</strong>,
    },
    {
      title: '配置',
      dataIndex: 'setting',
      key: 'setting',
      width: 220,
      render: (setting: string) => <code className="mono">{setting}</code>,
    },
    {
      title: '值',
      dataIndex: 'value',
      key: 'value',
      render: (value: string) => {
        if (value === '—') return <Typography.Text type="secondary">—</Typography.Text>;
        if (value.includes(' ') || value.includes('/')) {
          return <code className="mono">{value}</code>;
        }
        return value;
      },
    },
  ];

  return (
    <div>
      <PageHeader
        title="平台信息"
        subtitle="平台配置快照与运行时状态。"
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

      <Card className="soft-card" styles={{ body: { padding: 0 } }}>
        {loading && !info ? (
          <div style={{ padding: 24 }}>
            <Skeleton active paragraph={{ rows: 14 }} />
          </div>
        ) : (
          <Table<InfoRow>
            rowKey="key"
            columns={columns}
            dataSource={rows}
            pagination={false}
            size="middle"
            bordered
          />
        )}
      </Card>
    </div>
  );
}
