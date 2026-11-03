import { useEffect, useState } from 'react';
import {
  Alert,
  App,
  Button,
  Card,
  Descriptions,
  Form,
  Input,
  Select,
  Table,
  Tag,
  type TableColumnsType,
} from 'antd';
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import { listAudit, type AuditQuery } from '@/api/audit';
import type { AuditEvent } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { AuditResultTag, ReasonTag } from '@/components/StatusTag';
import { fmtTime } from '@/utils/format';

interface FilterForm {
  user?: string;
  cluster?: string;
  limit: number;
}

export function Audit() {
  const { message } = App.useApp();
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [form] = Form.useForm<FilterForm>();

  const load = async (query: AuditQuery = {}) => {
    setLoading(true);
    setError(null);
    try {
      setEvents(await listAudit(query));
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载审计日志失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load({ limit: 200 });
  }, []);

  const search = async (values: FilterForm) => {
    await load({
      user: values.user?.trim() || undefined,
      cluster: values.cluster?.trim() || undefined,
      limit: values.limit,
    });
    message.success('审计查询完成');
  };

  const columns: TableColumnsType<AuditEvent> = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 180,
      sorter: (a, b) => a.timestamp.localeCompare(b.timestamp),
      defaultSortOrder: 'descend',
      render: (value: string) => fmtTime(value),
    },
    {
      title: '用户',
      dataIndex: 'user',
      key: 'user',
      width: 150,
      render: (value: string) => <code className="mono strong">{value}</code>,
    },
    {
      title: '集群',
      dataIndex: 'cluster',
      key: 'cluster',
      width: 140,
      render: (value: string) => (value ? <code className="mono">{value}</code> : '—'),
    },
    {
      title: '方法',
      dataIndex: 'method',
      key: 'method',
      width: 90,
      render: (value: string) => <Tag>{value}</Tag>,
    },
    {
      title: '动词',
      dataIndex: 'verb',
      key: 'verb',
      width: 110,
      render: (value: string) => <code className="mono">{value}</code>,
    },
    {
      title: '资源',
      dataIndex: 'path',
      key: 'path',
      ellipsis: true,
      render: (value: string) => <code className="mono">{value}</code>,
    },
    {
      title: '结果',
      dataIndex: 'allowed',
      key: 'allowed',
      width: 90,
      render: (allowed: boolean) => <AuditResultTag allowed={allowed} />,
    },
    {
      title: '原因',
      dataIndex: 'reason',
      key: 'reason',
      width: 190,
      render: (reason: string) => <ReasonTag reason={reason} />,
    },
    {
      title: '耗时',
      dataIndex: 'latency_ms',
      key: 'latency_ms',
      width: 100,
      align: 'right',
      sorter: (a, b) => (a.latency_ms ?? 0) - (b.latency_ms ?? 0),
      render: (value?: number) => (value ? `${value} ms` : '—'),
    },
  ];

  return (
    <div>
      <PageHeader
        title="审计日志"
        subtitle="数据面访问与登录事件（时间倒序，含拒绝原因；异步批量落库，查询可能有约 0.5s 延迟）。"
        extra={
          <Button
            icon={<ReloadOutlined />}
            onClick={() =>
              void load({
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

      <Card className="soft-card filter-card">
        <Form<FilterForm>
          form={form}
          layout="inline"
          initialValues={{ limit: 200 }}
          onFinish={search}
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

      {error ? (
        <Alert
          type="error"
          showIcon
          message="审计日志加载失败"
          description={error}
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" onClick={() => void load({ limit: 200 })}>
              重试
            </Button>
          }
        />
      ) : null}

      <Card className="soft-card" styles={{ body: { padding: 0 } }}>
        <Table<AuditEvent>
          rowKey="id"
          columns={columns}
          dataSource={events}
          loading={loading}
          pagination={{
            pageSize: 20,
            showSizeChanger: true,
            pageSizeOptions: [20, 50, 100],
            showTotal: (total) => `共 ${total} 条事件`,
          }}
          scroll={{ x: 1200 }}
          expandable={{
            expandedRowRender: (record) => (
              <Descriptions size="small" column={{ xs: 1, md: 2 }} bordered>
                <Descriptions.Item label="事件 ID">{record.id}</Descriptions.Item>
                <Descriptions.Item label="来源 IP">
                  <code className="mono">{record.source_ip || '—'}</code>
                </Descriptions.Item>
                <Descriptions.Item label="方法">{record.method}</Descriptions.Item>
                <Descriptions.Item label="耗时">
                  {record.latency_ms ? `${record.latency_ms} ms` : '—'}
                </Descriptions.Item>
                <Descriptions.Item label="组" span={2}>
                  {record.groups
                    ? record.groups.split(',').map((g) => <Tag key={g}>{g}</Tag>)
                    : '—'}
                </Descriptions.Item>
                <Descriptions.Item label="资源路径" span={2}>
                  <code className="mono">{record.path}</code>
                </Descriptions.Item>
              </Descriptions>
            ),
          }}
        />
      </Card>
    </div>
  );
}
