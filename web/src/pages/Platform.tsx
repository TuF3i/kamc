import { Show, createMemo, createSignal, onMount } from 'solid-js';
import { getPlatformInfo } from '@/api/platform';
import type { PlatformInfo } from '@/api/types';
import { fmtBytes, fmtUptime } from '@/utils/format';
import { Alert, Button, Card, Skeleton, Table, type TableColumnsType } from '@/lib/widgets';
import { PageHeader } from '@/components/PageHeader';
import { ReloadOutlined } from '@/lib/icons';

interface InfoRow {
  key: string;
  group: string;
  setting: string;
  value: string;
}

/** 平台信息：单一表格（键→值），密钥与明文密码不出现在响应中。 */
export default function Platform() {
  const [info, setInfo] = createSignal<PlatformInfo | null>(null);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      setInfo(await getPlatformInfo());
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载平台信息失败');
    } finally {
      setLoading(false);
    }
  };

  onMount(() => void load());

  const rows = createMemo<InfoRow[]>(() => {
    const current = info();
    const result: InfoRow[] = [];
    const push = (
      group: string,
      setting: string,
      value: string | number | undefined | null,
    ) => {
      result.push({
        key: `${group}.${setting}`,
        group,
        setting,
        value: value === undefined || value === null || value === '' ? '—' : String(value),
      });
    };
    if (!current) return result;
    push('平台设置', 'version', `${current.version.version} (${current.version.commit})`);
    push('平台设置', 'listen_addr', current.settings.listen_addr);
    push('平台设置', 'external_url', current.settings.external_url);
    push('平台设置', 'admin_group', current.settings.admin_group);
    push(
      '平台设置',
      'password_login',
      current.settings.password_login ? `已启用（${current.settings.password_user}）` : '已关闭',
    );
    push('平台设置', 'robot_token_ttl', fmtUptime(current.settings.robot_token_ttl));
    push('平台设置', 'agent_image', current.settings.agent_image);
    push('SSO 接入', 'enabled', current.sso.enabled ? '已启用' : '未配置');
    if (current.sso.enabled) {
      push('SSO 接入', 'issuer', current.sso.issuer);
      push('SSO 接入', 'client_id', current.sso.client_id);
      push('SSO 接入', 'redirect_url', current.sso.redirect_url);
    }
    push('数据库', 'dsn', current.database.dsn);
    push('数据库', 'max_open_connections', current.database.max_open_connections);
    push(
      '数据库',
      'connections',
      `在用 ${current.database.in_use} / 空闲 ${current.database.idle} / 共 ${current.database.open_connections}`,
    );
    push('数据库', 'wait_count', current.database.wait_count);
    push('缓存（Redis）', 'enabled', current.cache.enabled ? '已启用' : '未配置');
    if (current.cache.enabled) {
      push('缓存（Redis）', 'addrs', current.cache.addrs);
      push('缓存（Redis）', 'db', current.cache.db);
    }
    push('Go 运行时', 'go_version', current.runtime.go_version);
    push('Go 运行时', 'uptime', fmtUptime(current.runtime.uptime_seconds));
    push('Go 运行时', 'goroutines', current.runtime.goroutines);
    push('Go 运行时', 'num_cpu', current.runtime.num_cpu);
    push('Go 运行时', 'heap_alloc', fmtBytes(current.runtime.heap_alloc_bytes));
    push('Go 运行时', 'sys', fmtBytes(current.runtime.sys_bytes));
    push('Go 运行时', 'num_gc', current.runtime.num_gc);
    return result;
  });

  const columns: TableColumnsType<InfoRow> = [
    {
      title: '分组',
      dataIndex: 'group',
      key: 'group',
      width: 160,
      onCell: (_record, index) => {
        const list = rows();
        const first = list[index];
        const isFirstOfGroup =
          first && list.findIndex((row) => row.group === first.group) === index;
        return isFirstOfGroup
          ? { rowSpan: list.filter((row) => row.group === first.group).length }
          : { rowSpan: 0 };
      },
      render: (group) => <strong>{String(group)}</strong>,
    },
    {
      title: '配置',
      dataIndex: 'setting',
      key: 'setting',
      width: 220,
      render: (setting) => <code class="mono">{String(setting)}</code>,
    },
    {
      title: '值',
      dataIndex: 'value',
      key: 'value',
      render: (value) => {
        const text = String(value);
        if (text === '—') return <span class="muted">—</span>;
        if (text.includes(' ') || text.includes('/')) {
          return <code class="mono wrap-anywhere">{text}</code>;
        }
        return text;
      },
    },
  ];

  return (
    <div>
      <PageHeader
        title="平台信息"
        subtitle="平台配置快照与运行时状态"
        extra={
          <Button
            icon={<ReloadOutlined class="h-4 w-4" />}
            onClick={() => void load()}
            loading={loading()}
          >
            刷新
          </Button>
        }
      />

      <Show when={error()}>
        <Alert
          type="error"
          message="平台信息加载失败"
          description={error() ?? undefined}
          class="mb-4"
          action={
            <Button size="small" onClick={() => void load()}>
              重试
            </Button>
          }
        />
      </Show>

      <Card class="soft-card" contentClass="p-0">
        <Show
          when={!(loading() && !info())}
          fallback={
            <div class="p-6">
              <Skeleton paragraph={{ rows: 14 }} />
            </div>
          }
        >
          <Table<InfoRow>
            rowKey="key"
            columns={columns}
            dataSource={rows()}
            pagination={false}
            bordered
          />
        </Show>
      </Card>
    </div>
  );
}
