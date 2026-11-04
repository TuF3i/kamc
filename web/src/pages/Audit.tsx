import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  App,
  Button,
  Card,
  DatePicker,
  Form,
  Input,
  Select,
  Table,
  Tabs,
  Tag,
  Typography,
  type TableColumnsType,
} from 'antd';
import type { Dayjs } from 'dayjs';
import { EyeOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import { getAudit, listAudit, type AuditQuery } from '@/api/audit';
import type { AuditEvent } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { AuditDetailDrawer } from '@/components/AuditDetailDrawer';
import { AuditResultTag, ReasonTag } from '@/components/StatusTag';
import { fmtTime } from '@/utils/format';

type Kind = 'platform' | 'cluster';

interface FilterForm {
  user?: string;
  cluster?: string;
  event?: string;
  allowed?: 'all' | 'allow' | 'deny';
  range?: [Dayjs, Dayjs];
  limit: number;
}

/** 地理信息展示：国家 · 地区 · 城市（空 = —） */
function GeoCell({ record }: { record: AuditEvent }) {
  const parts = [record.geo_country, record.geo_region, record.geo_city].filter(Boolean);
  if (parts.length === 0) return <Typography.Text type="secondary">—</Typography.Text>;
  return <span>{parts.join(' · ')}</span>;
}

/** 对端 IP：主 IP + 转发链提示 */
function IPCell({ record }: { record: AuditEvent }) {
  if (!record.source_ip) return <Typography.Text type="secondary">—</Typography.Text>;
  return (
    <span>
      <code className="mono">{record.source_ip}</code>
      {record.forwarded_for ? (
        <Typography.Text type="secondary" style={{ marginLeft: 6, fontSize: 11 }}>
          xff
        </Typography.Text>
      ) : null}
    </span>
  );
}

/** 固定的右侧详情列：点击后从侧面打开事件全文。 */
function detailColumn(onOpen: (record: AuditEvent) => void): TableColumnsType<AuditEvent>[number] {
  return {
    title: '',
    key: 'detail',
    width: 88,
    fixed: 'right',
    render: (_, record) => (
      <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => onOpen(record)}>
        详情
      </Button>
    ),
  };
}

/** 平台日志表：登录/Token/下载/管理动作。 */
function PlatformTable({
  events,
  loading,
  onOpen,
}: {
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
      render: (v: string) => fmtTime(v),
    },
    {
      title: '事件',
      dataIndex: 'event',
      key: 'event',
      width: 190,
      render: (v: string) => <code className="mono strong">{v || '—'}</code>,
    },
    {
      title: '用户',
      dataIndex: 'user',
      key: 'user',
      width: 150,
      render: (v: string) => (v ? <code className="mono">{v}</code> : '—'),
    },
    {
      title: '组',
      dataIndex: 'groups',
      key: 'groups',
      width: 160,
      render: (v?: string) =>
        v ? v.split(',').slice(0, 2).map((g) => <Tag key={g}>{g}</Tag>) : <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: '对端 IP',
      key: 'ip',
      width: 160,
      render: (_, r) => <IPCell record={r} />,
    },
    {
      title: '地理信息',
      key: 'geo',
      width: 180,
      render: (_, r) => <GeoCell record={r} />,
    },
    {
      title: '结果',
      dataIndex: 'allowed',
      key: 'allowed',
      width: 80,
      render: (allowed: boolean) => <AuditResultTag allowed={allowed} />,
    },
    {
      title: '说明',
      dataIndex: 'reason',
      key: 'reason',
      ellipsis: true,
      render: (reason: string) => <ReasonTag reason={reason} />,
    },
    detailColumn(onOpen),
  ];

  return (
    <Card className="soft-card" styles={{ body: { padding: 0 } }}>
      <Table<AuditEvent>
        rowKey="id"
        columns={columns}
        dataSource={events}
        loading={loading}
        pagination={{ pageSize: 20, showSizeChanger: true, pageSizeOptions: [20, 50, 100], showTotal: (t) => `共 ${t} 条平台事件` }}
        scroll={{ x: 1200 }}
      />
    </Card>
  );
}

/** 集群日志表：数据面 API 操作（资源/动作/状态 + 原始请求响应）。 */
function ClusterTable({
  events,
  loading,
  onOpen,
}: {
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
      render: (v: string) => fmtTime(v),
    },
    {
      title: '用户',
      dataIndex: 'user',
      key: 'user',
      width: 150,
      render: (v: string) => <code className="mono strong">{v || '—'}</code>,
    },
    {
      title: '集群',
      dataIndex: 'cluster',
      key: 'cluster',
      width: 130,
      render: (v: string) => (v ? <code className="mono">{v}</code> : '—'),
    },
    {
      title: '动作',
      dataIndex: 'verb',
      key: 'verb',
      width: 110,
      render: (v: string) => (v ? <Tag color="blue">{v}</Tag> : '—'),
    },
    {
      title: '资源',
      key: 'object',
      ellipsis: true,
      render: (_, r) => <code className="mono">{r.object || r.path || '—'}</code>,
    },
    {
      title: '状态',
      dataIndex: 'status_code',
      key: 'status',
      width: 80,
      render: (v?: number) => {
        if (!v) return '—';
        const color = v >= 500 ? 'error' : v >= 400 ? 'warning' : 'success';
        return <Tag color={color}>{v}</Tag>;
      },
    },
    {
      title: '结果',
      dataIndex: 'allowed',
      key: 'allowed',
      width: 80,
      render: (allowed: boolean) => <AuditResultTag allowed={allowed} />,
    },
    {
      title: '对端 IP / 地理',
      key: 'ip',
      width: 220,
      render: (_, r) => (
        <div>
          <IPCell record={r} />
          <div style={{ fontSize: 12, color: 'var(--kamc-muted)' }}>
            <GeoCell record={r} />
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
      render: (v?: number) => (v ? `${v} ms` : '—'),
    },
    detailColumn(onOpen),
  ];

  return (
    <Card className="soft-card" styles={{ body: { padding: 0 } }}>
      <Table<AuditEvent>
        rowKey="id"
        columns={columns}
        dataSource={events}
        loading={loading}
        pagination={{ pageSize: 20, showSizeChanger: true, pageSizeOptions: [20, 50, 100], showTotal: (t) => `共 ${t} 条集群操作` }}
        scroll={{ x: 1400 }}
      />
    </Card>
  );
}

export function Audit() {
  const { message } = App.useApp();
  const [kind, setKind] = useState<Kind>('platform');
  const [events, setEvents] = useState<Record<Kind, AuditEvent[]>>({ platform: [], cluster: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [detail, setDetail] = useState<AuditEvent | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [form] = Form.useForm<FilterForm>();

  const load = useCallback(async (target: Kind, query: AuditQuery = {}) => {
    setLoading(true);
    setError(null);
    try {
      const list = await listAudit({ ...query, kind: target, limit: query.limit ?? 200, summary: true });
      setEvents((prev) => ({ ...prev, [target]: list }));
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载审计日志失败');
    } finally {
      setLoading(false);
    }
  }, []);

  const buildQuery = (values: FilterForm): AuditQuery => ({
    user: values.user?.trim() || undefined,
    cluster: values.cluster?.trim() || undefined,
    event: values.event?.trim() || undefined,
    allowed: values.allowed === 'allow' ? true : values.allowed === 'deny' ? false : undefined,
    from: values.range?.[0]?.startOf('day').toISOString(),
    to: values.range?.[1]?.endOf('day').toISOString(),
    limit: values.limit ?? 200,
  });

  useEffect(() => {
    void load('platform', { limit: 200 });
    void load('cluster', { limit: 200 });
  }, [load]);

  const search = async () => {
    await load(kind, buildQuery(form.getFieldsValue()));
  };

  const openDetail = async (record: AuditEvent) => {
    setDetail(record);
    setDetailLoading(true);
    try {
      setDetail(await getAudit(record.id));
    } catch (err) {
      message.error(err instanceof Error ? err.message : '加载审计详情失败');
    } finally {
      setDetailLoading(false);
    }
  };

  return (
    <div>
      <PageHeader
        title="审计日志"
        subtitle="平台事件走 stdio 并落库；集群操作走审计库，可经 syslog 导出到远程日志服务器。"
        extra={
          <Button
            icon={<ReloadOutlined />}
            onClick={() => void load(kind, buildQuery(form.getFieldsValue()))}
          >
            刷新
          </Button>
        }
      />

      {error ? (
        <Alert
          type="error"
          showIcon
          message="审计日志加载失败"
          description={error}
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" onClick={() => void load(kind, { limit: 200 })}>
              重试
            </Button>
          }
        />
      ) : null}

      <Tabs
        activeKey={kind}
        onChange={(key) => setKind(key as Kind)}
        items={[
          { key: 'platform', label: '平台日志', children: null },
          { key: 'cluster', label: '集群日志', children: null },
        ]}
      />

      <Card className="soft-card filter-card" style={{ marginBottom: 16 }}>
        <Form<FilterForm>
          form={form}
          layout="inline"
          initialValues={{ limit: 200, allowed: 'all' }}
          onFinish={() => void search()}
          className="audit-filter"
        >
          <Form.Item name="user" label="用户">
            <Input placeholder="精确匹配 sub" allowClear className="mono" />
          </Form.Item>
          <Form.Item name="cluster" label="集群">
            <Input placeholder="精确匹配集群 ID" allowClear className="mono" />
          </Form.Item>
          <Form.Item name="event" label="事件">
            <Input placeholder="如 k8s-api" allowClear className="mono" />
          </Form.Item>
          <Form.Item name="allowed" label="结果">
            <Select
              style={{ width: 100 }}
              options={[
                { value: 'all', label: '全部' },
                { value: 'allow', label: '允许' },
                { value: 'deny', label: '拒绝' },
              ]}
            />
          </Form.Item>
          <Form.Item name="range" label="时间">
            <DatePicker.RangePicker showTime={false} allowClear />
          </Form.Item>
          <Form.Item name="limit" label="条数">
            <Select
              style={{ width: 110 }}
              options={[100, 200, 500, 1000].map((n) => ({ value: n, label: `${n} 条` }))}
            />
          </Form.Item>
          <Form.Item>
            <Button type="primary" icon={<SearchOutlined />} htmlType="submit" loading={loading}>
              查询
            </Button>
          </Form.Item>
        </Form>
      </Card>

      {kind === 'platform' ? (
        <PlatformTable events={events.platform} loading={loading} onOpen={openDetail} />
      ) : (
        <ClusterTable events={events.cluster} loading={loading} onOpen={openDetail} />
      )}

      <AuditDetailDrawer event={detail} loading={detailLoading} onClose={() => setDetail(null)} />
    </div>
  );
}
