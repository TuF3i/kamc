import {
  For,
  Show,
  createSignal,
  onMount,
} from 'solid-js';
import type { Dayjs } from 'dayjs';
import { getAudit, listAudit, type AuditQuery } from '@/api/audit';
import type { AuditEvent } from '@/api/types';
import { toast } from '@/lib/storage/toast';
import {
  Alert,
  Button,
  Card,
  DateRange,
  Input,
  Select,
  Table,
  Tabs,
  Tag,
  type TableColumnsType,
} from '@/lib/widgets';
import { PageHeader } from '@/components/PageHeader';
import { AuditDetailDrawer } from '@/components/AuditDetailDrawer';
import { AuditResultTag, ReasonTag } from '@/components/StatusTag';
import { fmtTime } from '@/utils/format';
import { EyeOutlined, ReloadOutlined, SearchOutlined } from '@/lib/icons';

type Kind = 'platform' | 'cluster';

type AllowedFilter = 'all' | 'allow' | 'deny';

/** 地理信息展示：国家 · 地区 · 城市（空 = —） */
function GeoCell(props: { record: AuditEvent }) {
  const parts = () =>
    [props.record.geo_country, props.record.geo_region, props.record.geo_city].filter(
      Boolean,
    );
  return (
    <Show when={parts().length > 0} fallback={<span class="muted">—</span>}>
      <span>{parts().join(' · ')}</span>
    </Show>
  );
}

/** 对端 IP：主 IP + 转发链提示 */
function IPCell(props: { record: AuditEvent }) {
  return (
    <Show when={props.record.source_ip} fallback={<span class="muted">—</span>}>
      <span>
        <code class="mono">{props.record.source_ip}</code>
        <Show when={props.record.forwarded_for}>
          <span class="ml-1.5 text-[11px] text-layer-content/50">xff</span>
        </Show>
      </span>
    </Show>
  );
}

/** 固定的右侧详情列：点击后从侧面打开事件全文。 */
function detailColumn(
  onOpen: (record: AuditEvent) => void,
): TableColumnsType<AuditEvent>[number] {
  return {
    title: '',
    key: 'detail',
    width: 88,
    fixed: 'right',
    render: (_value, record) => (
      <Button
        type="link"
        size="small"
        icon={<EyeOutlined class="h-3.5 w-3.5" />}
        onClick={() => onOpen(record)}
      >
        详情
      </Button>
    ),
  };
}

/** 平台日志表：登录/Token/下载/管理动作。 */
function PlatformTable(props: {
  events: AuditEvent[];
  loading: boolean;
  onOpen: (record: AuditEvent) => void;
}) {
  const columns: TableColumnsType<AuditEvent> = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 170,
      sorter: (a, b) => a.timestamp.localeCompare(b.timestamp),
      defaultSortOrder: 'descend',
      render: (value) => fmtTime(String(value)),
    },
    {
      title: '事件',
      dataIndex: 'event',
      key: 'event',
      width: 190,
      render: (value) => <code class="mono strong">{String(value ?? '') || '—'}</code>,
    },
    {
      title: '用户',
      dataIndex: 'user',
      key: 'user',
      width: 150,
      render: (value) => (value ? <code class="mono">{String(value)}</code> : '—'),
    },
    {
      title: '组',
      dataIndex: 'groups',
      key: 'groups',
      width: 160,
      render: (value) =>
        value ? (
          <span class="flex flex-wrap gap-1">
            <For each={String(value).split(',').slice(0, 2)}>
              {(group) => <Tag>{group}</Tag>}
            </For>
          </span>
        ) : (
          <span class="muted">—</span>
        ),
    },
    {
      title: '对端 IP',
      key: 'ip',
      width: 160,
      render: (_value, record) => <IPCell record={record} />,
    },
    {
      title: '地理信息',
      key: 'geo',
      width: 180,
      render: (_value, record) => <GeoCell record={record} />,
    },
    {
      title: '结果',
      dataIndex: 'allowed',
      key: 'allowed',
      width: 80,
      render: (_value, record) => <AuditResultTag allowed={record.allowed} />,
    },
    {
      title: '说明',
      dataIndex: 'reason',
      key: 'reason',
      ellipsis: true,
      render: (value) => <ReasonTag reason={String(value ?? '')} />,
    },
    detailColumn(props.onOpen),
  ];

  return (
    <Card class="soft-card" contentClass="p-0">
      <Table<AuditEvent>
        rowKey="id"
        columns={columns}
        dataSource={props.events}
        loading={props.loading}
        pagination={{
          pageSize: 20,
          showSizeChanger: true,
          pageSizeOptions: [20, 50, 100],
          showTotal: (total) => `共 ${total} 条平台事件`,
        }}
        scroll={{ x: 1200 }}
      />
    </Card>
  );
}

/** 集群日志表：数据面 API 操作（资源/动作/状态 + 原始请求响应）。 */
function ClusterTable(props: {
  events: AuditEvent[];
  loading: boolean;
  onOpen: (record: AuditEvent) => void;
}) {
  const columns: TableColumnsType<AuditEvent> = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 170,
      sorter: (a, b) => a.timestamp.localeCompare(b.timestamp),
      defaultSortOrder: 'descend',
      render: (value) => fmtTime(String(value)),
    },
    {
      title: '用户',
      dataIndex: 'user',
      key: 'user',
      width: 150,
      render: (value) =>
        value ? (
          <code class="mono strong audit-actor" title={String(value)}>
            {String(value)}
          </code>
        ) : (
          '—'
        ),
    },
    {
      title: '集群',
      dataIndex: 'cluster',
      key: 'cluster',
      width: 130,
      render: (value) => (value ? <code class="mono">{String(value)}</code> : '—'),
    },
    {
      title: '动作',
      dataIndex: 'verb',
      key: 'verb',
      width: 110,
      render: (value) =>
        value ? <Tag level="primary">{String(value)}</Tag> : '—',
    },
    {
      title: '资源',
      key: 'object',
      ellipsis: true,
      render: (_value, record) => (
        <code class="mono">{record.object || record.path || '—'}</code>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status_code',
      key: 'status',
      width: 80,
      render: (value) => {
        const code = Number(value);
        if (!code) return '—';
        return (
          <Tag level={code >= 500 ? 'error' : code >= 400 ? 'warning' : 'success'}>
            {code}
          </Tag>
        );
      },
    },
    {
      title: '结果',
      dataIndex: 'allowed',
      key: 'allowed',
      width: 80,
      render: (_value, record) => <AuditResultTag allowed={record.allowed} />,
    },
    {
      title: '对端 IP / 地理',
      key: 'ip',
      width: 220,
      render: (_value, record) => (
        <div>
          <IPCell record={record} />
          <div class="text-[12px] text-layer-content/50">
            <GeoCell record={record} />
          </div>
        </div>
      ),
    },
    {
      title: '耗时',
      dataIndex: 'latency_ms',
      key: 'latency_ms',
      width: 90,
      align: 'right',
      sorter: (a, b) => (a.latency_ms ?? 0) - (b.latency_ms ?? 0),
      render: (value) => (value ? `${value} ms` : '—'),
    },
    detailColumn(props.onOpen),
  ];

  return (
    <Card class="soft-card" contentClass="p-0">
      <Table<AuditEvent>
        rowKey="id"
        columns={columns}
        dataSource={props.events}
        loading={props.loading}
        pagination={{
          pageSize: 20,
          showSizeChanger: true,
          pageSizeOptions: [20, 50, 100],
          showTotal: (total) => `共 ${total} 条集群操作`,
        }}
        scroll={{ x: 1400 }}
      />
    </Card>
  );
}

export default function Audit() {
  const [kind, setKind] = createSignal<Kind>('platform');
  const [events, setEvents] = createSignal<Record<Kind, AuditEvent[]>>({
    platform: [],
    cluster: [],
  });
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [detail, setDetail] = createSignal<AuditEvent | null>(null);
  const [detailLoading, setDetailLoading] = createSignal(false);

  const [user, setUser] = createSignal('');
  const [cluster, setCluster] = createSignal('');
  const [event, setEvent] = createSignal('');
  const [allowed, setAllowed] = createSignal<AllowedFilter>('all');
  const [range, setRange] = createSignal<[Dayjs, Dayjs] | null>(null);
  const [limit, setLimit] = createSignal(200);

  const load = async (target: Kind, query: AuditQuery = {}) => {
    setLoading(true);
    setError(null);
    try {
      const list = await listAudit({
        ...query,
        kind: target,
        limit: query.limit ?? 200,
        summary: true,
      });
      setEvents((prev) => ({ ...prev, [target]: list }));
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载审计日志失败');
    } finally {
      setLoading(false);
    }
  };

  const buildQuery = (): AuditQuery => ({
    user: user().trim() || undefined,
    cluster: cluster().trim() || undefined,
    event: event().trim() || undefined,
    allowed:
      allowed() === 'allow' ? true : allowed() === 'deny' ? false : undefined,
    from: range()?.[0]?.startOf('day').toISOString(),
    to: range()?.[1]?.endOf('day').toISOString(),
    limit: limit(),
  });

  onMount(() => {
    void load('platform', { limit: 200 });
    void load('cluster', { limit: 200 });
  });

  const search = async () => {
    await load(kind(), buildQuery());
  };

  const openDetail = async (record: AuditEvent) => {
    setDetail(record);
    setDetailLoading(true);
    try {
      setDetail(await getAudit(record.id));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '加载审计详情失败');
    } finally {
      setDetailLoading(false);
    }
  };

  return (
    <div>
      <PageHeader
        title="审计日志"
        subtitle="平台与集群操作的完整审计记录"
        extra={
          <Button
            icon={<ReloadOutlined class="h-4 w-4" />}
            onClick={() => void load(kind(), buildQuery())}
          >
            刷新
          </Button>
        }
      />

      <Show when={error()}>
        <Alert
          type="error"
          message="审计日志加载失败"
          description={error() ?? undefined}
          class="mb-4"
          action={
            <Button size="small" onClick={() => void load(kind(), { limit: 200 })}>
              重试
            </Button>
          }
        />
      </Show>

      <Tabs
        activeKey={kind()}
        onChange={(key) => setKind(key as Kind)}
        items={[
          { key: 'platform', label: '平台日志' },
          { key: 'cluster', label: '集群日志' },
        ]}
      />

      <Card class="soft-card filter-card mt-4">
        <form
          class="flex flex-wrap items-end gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            void search();
          }}
        >
          <label class="flex items-center gap-2">
            <span class="text-[13px] text-layer-content/60">用户</span>
            <Input
              value={user()}
              onValueChange={setUser}
              placeholder="精确匹配 sub"
              allowClear
              class="mono w-[160px]"
            />
          </label>
          <label class="flex items-center gap-2">
            <span class="text-[13px] text-layer-content/60">集群</span>
            <Input
              value={cluster()}
              onValueChange={setCluster}
              placeholder="精确匹配集群 ID"
              allowClear
              class="mono w-[170px]"
            />
          </label>
          <label class="flex items-center gap-2">
            <span class="text-[13px] text-layer-content/60">事件</span>
            <Input
              value={event()}
              onValueChange={setEvent}
              placeholder="如 k8s-api"
              allowClear
              class="mono w-[150px]"
            />
          </label>
          <label class="flex items-center gap-2">
            <span class="text-[13px] text-layer-content/60">结果</span>
            <Select
              class="w-[100px]"
              value={allowed()}
              options={[
                { value: 'all', label: '全部' },
                { value: 'allow', label: '允许' },
                { value: 'deny', label: '拒绝' },
              ]}
              onChange={(value) => setAllowed(String(value) as AllowedFilter)}
            />
          </label>
          <label class="flex items-center gap-2">
            <span class="text-[13px] text-layer-content/60">时间</span>
            <DateRange
              class="w-[250px]"
              value={range()}
              onChange={(value) => setRange(value)}
            />
          </label>
          <label class="flex items-center gap-2">
            <span class="text-[13px] text-layer-content/60">条数</span>
            <Select
              class="w-[110px]"
              value={limit()}
              options={[100, 200, 500, 1000].map((value) => ({
                value,
                label: `${value} 条`,
              }))}
              onChange={(value) => setLimit(Number(value))}
            />
          </label>
          <Button
            type="primary"
            icon={<SearchOutlined class="h-4 w-4" />}
            htmlType="submit"
            loading={loading()}
          >
            查询
          </Button>
        </form>
      </Card>

      <Show
        when={kind() === 'platform'}
        fallback={
          <ClusterTable events={events().cluster} loading={loading()} onOpen={openDetail} />
        }
      >
        <PlatformTable events={events().platform} loading={loading()} onOpen={openDetail} />
      </Show>

      <AuditDetailDrawer
        event={detail()}
        loading={detailLoading()}
        onClose={() => setDetail(null)}
      />
    </div>
  );
}
