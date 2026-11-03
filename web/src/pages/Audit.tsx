import { useCallback, useEffect, useState, type ReactNode } from 'react';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Form,
  Input,
  Select,
  Table,
  Tabs,
  Tag,
  Typography,
  type TableColumnsType,
} from 'antd';
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import { listAudit, type AuditQuery } from '@/api/audit';
import type { AuditEvent } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { CopyButton } from '@/components/CopyButton';
import { AuditResultTag, ReasonTag } from '@/components/StatusTag';
import { fmtTime } from '@/utils/format';

type Kind = 'platform' | 'cluster';

interface FilterForm {
  user?: string;
  cluster?: string;
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

function metaItems(record: AuditEvent) {
  return [
    <Descriptions.Item label="事件 ID" key="id">{record.id}</Descriptions.Item>,
    <Descriptions.Item label="事件" key="event"><code className="mono">{record.event || '—'}</code></Descriptions.Item>,
    <Descriptions.Item label="用户" key="user"><code className="mono">{record.user || '—'}</code></Descriptions.Item>,
    <Descriptions.Item label="组" key="groups">
      {record.groups ? record.groups.split(',').map((g) => <Tag key={g}>{g}</Tag>) : '—'}
    </Descriptions.Item>,
    <Descriptions.Item label="对端 IP" key="ip"><code className="mono">{record.source_ip || '—'}</code></Descriptions.Item>,
    <Descriptions.Item label="X-Forwarded-For" key="xff">
      <code className="mono">{record.forwarded_for || '—'}</code>
    </Descriptions.Item>,
    <Descriptions.Item label="地理信息" key="geo"><GeoCell record={record} /></Descriptions.Item>,
    <Descriptions.Item label="User-Agent" key="ua">
      <span style={{ wordBreak: 'break-all' }}>{record.user_agent || '—'}</span>
    </Descriptions.Item>,
    <Descriptions.Item label="方法" key="method">{record.method || '—'}</Descriptions.Item>,
    <Descriptions.Item label="耗时" key="lat">
      {record.latency_ms ? `${record.latency_ms} ms` : '—'}
    </Descriptions.Item>,
  ];
}

/** 平台日志表：登录/Token/下载/管理动作。 */
function PlatformTable({ events, loading }: { events: AuditEvent[]; loading: boolean }) {
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
        expandable={{
          expandedRowRender: (record) => (
            <Descriptions size="small" column={{ xs: 1, md: 2 }} bordered>
              {metaItems(record)}
              <Descriptions.Item label="路径" span={2}>
                <code className="mono">{record.path || '—'}</code>
              </Descriptions.Item>
            </Descriptions>
          ),
        }}
      />
    </Card>
  );
}

/** 请求/响应体块：限高滚动 + 强制换行（避免长 JSON 撑爆表格）+ 复制。 */
/** JSON 行内语法高亮：key / 字符串 / 数字 / 布尔 分色 */
const JSON_TOKEN = /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g;

function highlightJSONLine(line: string, rowKey: number): ReactNode {
  const nodes: ReactNode[] = [];
  let last = 0;
  let seq = 0;
  JSON_TOKEN.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = JSON_TOKEN.exec(line)) !== null) {
    if (m.index > last) nodes.push(line.slice(last, m.index));
    if (m[1] !== undefined) {
      const isKey = m[2] !== undefined;
      nodes.push(
        <span key={`s${rowKey}-${seq++}`} className={isKey ? 'json-key' : 'json-string'}>
          {m[1]}
        </span>,
      );
      if (isKey) nodes.push(m[2]);
    } else if (m[3] !== undefined) {
      nodes.push(<span key={`b${rowKey}-${seq++}`} className="json-bool">{m[3]}</span>);
    } else {
      nodes.push(<span key={`n${rowKey}-${seq++}`} className="json-number">{m[4]}</span>);
    }
    last = JSON_TOKEN.lastIndex;
  }
  if (last < line.length) nodes.push(line.slice(last));
  return <div key={rowKey}>{nodes.length ? nodes : ' '}</div>;
}

/** 完整 HTTP 报文高亮：起始行 / 头名值 / JSON body 分别着色。 */
function HighlightedDump({ text }: { text: string }) {
  const sep = text.indexOf('\r\n\r\n');
  const head = sep >= 0 ? text.slice(0, sep) : text;
  const body = sep >= 0 ? text.slice(sep + 4) : '';
  const headLines = head.split(/\r?\n/);
  return (
    <>
      {headLines.map((line, i) => {
        if (i === 0) {
          return <div key={`h${i}`} className="http-start">{line || ' '}</div>;
        }
        const idx = line.indexOf(':');
        if (idx > 0) {
          return (
            <div key={`h${i}`} className="http-header">
              <span className="http-header-key">{line.slice(0, idx)}</span>
              <span className="http-header-value">{line.slice(idx)}</span>
            </div>
          );
        }
        return <div key={`h${i}`} className={line.startsWith('…') ? 'dim' : undefined}>{line || ' '}</div>;
      })}
      {sep >= 0
        ? body.split(/\r?\n/).map((line, i) =>
            line.startsWith('(') || line.startsWith('…') ? (
              <div key={`j${i}`} className="dim">{line || ' '}</div>
            ) : (
              highlightJSONLine(line, i)
            ),
          )
        : null}
    </>
  );
}

function AuditBody({ title, body }: { title: string; body?: string }) {
  if (!body) {
    return (
      <div style={{ marginTop: 12, color: 'var(--kamc-muted)', fontSize: 12 }}>
        {title}：（空）
      </div>
    );
  }
  return (
    <div style={{ marginTop: 12 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
        <Typography.Text type="secondary">
          {title}（{body.length} 字符）
        </Typography.Text>
        <CopyButton text={body} label="复制" size="small" />
      </div>
      <pre className="code-block audit-body">
        <HighlightedDump text={body} />
      </pre>
    </div>
  );
}

/** 集群日志表：数据面 API 操作（资源/动作/状态 + 原始请求响应）。 */
function ClusterTable({ events, loading }: { events: AuditEvent[]; loading: boolean }) {
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
        expandable={{
          expandedRowRender: (record) => (
            <div className="audit-expand">
              <Descriptions size="small" column={{ xs: 1, md: 2 }} bordered>
                {metaItems(record)}
                <Descriptions.Item label="原始请求" span={2}>
                  <code className="mono" style={{ wordBreak: 'break-all' }}>{record.path || '—'}</code>
                </Descriptions.Item>
                <Descriptions.Item label="鉴权对象" span={2}>
                  <code className="mono">{record.object || '—'}</code>
                </Descriptions.Item>
                <Descriptions.Item label="拒绝/放行原因" span={2}>
                  <ReasonTag reason={record.reason} />
                </Descriptions.Item>
              </Descriptions>
              <AuditBody title="请求报文" body={record.request_body} />
              <AuditBody title="响应报文" body={record.response_body} />
            </div>
          ),
        }}
      />
    </Card>
  );
}

export function Audit() {
  const [kind, setKind] = useState<Kind>('platform');
  const [events, setEvents] = useState<Record<Kind, AuditEvent[]>>({ platform: [], cluster: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [form] = Form.useForm<FilterForm>();

  const load = useCallback(async (target: Kind, query: AuditQuery = {}) => {
    setLoading(true);
    setError(null);
    try {
      const list = await listAudit({ ...query, kind: target, limit: query.limit ?? 200 });
      setEvents((prev) => ({ ...prev, [target]: list }));
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载审计日志失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load('platform', { limit: 200 });
    void load('cluster', { limit: 200 });
  }, [load]);

  const search = async () => {
    const values = form.getFieldsValue();
    await load(kind, {
      user: values.user?.trim() || undefined,
      cluster: values.cluster?.trim() || undefined,
      limit: values.limit ?? 200,
    });
  };

  return (
    <div>
      <PageHeader
        title="审计日志"
        subtitle="平台事件走 stdio 并落库；集群操作走审计库，可经 syslog 导出到远程日志服务器。"
        extra={
          <Button
            icon={<ReloadOutlined />}
            onClick={() =>
              void load(kind, {
                user: form.getFieldValue('user')?.trim() || undefined,
                cluster: form.getFieldValue('cluster')?.trim() || undefined,
                limit: form.getFieldValue('limit') ?? 200,
              })
            }
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
          initialValues={{ limit: 200 }}
          onFinish={() => void search()}
          className="audit-filter"
        >
          <Form.Item name="user" label="用户">
            <Input placeholder="精确匹配 sub" allowClear className="mono" />
          </Form.Item>
          <Form.Item name="cluster" label="集群">
            <Input placeholder="精确匹配集群 ID" allowClear className="mono" />
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
        <PlatformTable events={events.platform} loading={loading} />
      ) : (
        <ClusterTable events={events.cluster} loading={loading} />
      )}
    </div>
  );
}
