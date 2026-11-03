import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  App,
  Button,
  Card,
  Input,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
  type TableColumnsType,
  type TableProps,
} from 'antd';
import { DeleteOutlined, SearchOutlined } from '@ant-design/icons';
import { deletePolicy, listPolicies } from '@/api/access';
import type { PolicyDTO } from '@/api/types';
import { summarizeBatch } from '@/utils/batch';

function subjectLabel(record: PolicyDTO): string {
  if (record.subject_type === 'org') return '组织';
  if (record.subject_type === 'user') return '用户';
  return '遗留';
}

function resourceLabel(record: PolicyDTO): string {
  if (record.resource === '*') return '全部资源';
  return record.subresource ? `${record.resource}/${record.subresource}` : record.resource;
}

interface PoliciesTabProps {
  /** 仅显示主体匹配该前缀的策略（如机器人页传 robot:<id>） */
  subjectPrefix?: string;
  /** 仅显示该集群的策略 */
  clusterFilter?: string;
}

/** 策略列表（权限 Tab 与机器人权限 Tab 共用）。 */
export function PoliciesTab({ subjectPrefix, clusterFilter }: PoliciesTabProps) {
  const { message, modal } = App.useApp();
  const [policies, setPolicies] = useState<PolicyDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [selectedKeys, setSelectedKeys] = useState<React.Key[]>([]);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      setPolicies(await listPolicies());
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载策略失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const filtered = useMemo(() => {
    let list = policies;
    if (subjectPrefix) {
      list = list.filter((p) => p.sub === `user:${subjectPrefix}`);
    }
    if (clusterFilter) {
      list = list.filter((p) => p.dom === clusterFilter);
    }
    const keyword = search.trim().toLowerCase();
    if (!keyword) return list;
    return list.filter((record) =>
      [
        record.subject,
        record.cluster,
        record.namespace,
        record.resource,
        record.subresource ?? '',
        record.verbs.join(' '),
      ].some((value) => value.toLowerCase().includes(keyword)),
    );
  }, [policies, search, subjectPrefix, clusterFilter]);

  const policyKey = (record: PolicyDTO) => `${record.sub}|${record.dom}|${record.obj}|${record.act}`;

  const policyByKey = useMemo(() => {
    const map = new Map<string, PolicyDTO>();
    for (const record of filtered) map.set(policyKey(record), record);
    return map;
  }, [filtered]);

  const selectedPolicies = useMemo(
    () =>
      selectedKeys
        .map((key) => policyByKey.get(String(key)))
        .filter((record): record is PolicyDTO => Boolean(record)),
    [selectedKeys, policyByKey],
  );

  const remove = async (record: PolicyDTO) => {
    try {
      await deletePolicy({ sub: record.sub, dom: record.dom, obj: record.obj, act: record.act });
      message.success('策略已删除');
      setSelectedKeys((keys) => keys.filter((key) => key !== policyKey(record)));
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除策略失败');
    }
  };

  const batchRemove = () => {
    const targets = selectedPolicies;
    if (targets.length === 0) return;
    modal.confirm({
      title: `批量删除 ${targets.length} 条策略？`,
      content: '删除立即生效，对应主体马上失去这些资源的访问权限。',
      okText: '批量删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        const outcome = summarizeBatch(
          await Promise.allSettled(
            targets.map((record) =>
              deletePolicy({ sub: record.sub, dom: record.dom, obj: record.obj, act: record.act }),
            ),
          ),
        );
        if (outcome.failed === 0) {
          message.success(`已删除 ${outcome.succeeded} 条策略`);
        } else {
          message.error(
            `已删除 ${outcome.succeeded} 条策略，${outcome.failed} 条失败：${outcome.firstError}`,
          );
        }
        setSelectedKeys([]);
        await load();
      },
    });
  };

  const rowSelection: TableProps<PolicyDTO>['rowSelection'] = {
    selectedRowKeys: selectedKeys,
    onChange: (keys) => setSelectedKeys([...keys]),
    selections: [Table.SELECTION_ALL, Table.SELECTION_INVERT, Table.SELECTION_NONE],
    preserveSelectedRowKeys: true,
  };

  const columns: TableColumnsType<PolicyDTO> = [
    {
      title: '主体',
      key: 'subject',
      width: 260,
      render: (_, record) => (
        <Space size={6}>
          <Tag color={record.subject_type === 'org' ? 'purple' : 'blue'}>
            {subjectLabel(record)}
          </Tag>
          <code className="mono strong">{record.subject}</code>
        </Space>
      ),
    },
    {
      title: '集群',
      dataIndex: 'cluster',
      key: 'cluster',
      width: 180,
      render: (value: string) => <code className="mono">{value}</code>,
    },
    {
      title: '命名空间',
      dataIndex: 'namespace',
      key: 'namespace',
      width: 140,
      render: (value: string) =>
        value === '*' ? (
          <Tag>全部</Tag>
        ) : value ? (
          <code className="mono">{value}</code>
        ) : (
          <Typography.Text type="secondary">—</Typography.Text>
        ),
    },
    {
      title: '资源',
      key: 'resource',
      width: 220,
      render: (_, record) =>
        record.resource === '*' ? (
          <Tag color="gold">全部资源</Tag>
        ) : (
          <code className="mono">{resourceLabel(record)}</code>
        ),
    },
    {
      title: '动作',
      key: 'verbs',
      render: (_, record) =>
        record.verbs.includes('*') ? (
          <Tag color="gold">全部动作</Tag>
        ) : (
          <Space size={[4, 4]} wrap>
            {record.verbs.map((verb) => (
              <Tag key={verb}>{verb}</Tag>
            ))}
          </Space>
        ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 100,
      render: (_, record) => (
        <Popconfirm
          title="删除这条策略？"
          description={`${record.sub} · ${record.dom} · ${record.obj} · ${record.act}`}
          okText="删除"
          cancelText="取消"
          okButtonProps={{ danger: true }}
          onConfirm={() => void remove(record)}
        >
          <Button type="link" size="small" danger icon={<DeleteOutlined />}>
            删除
          </Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <div>
      {error ? (
        <Alert
          type="error"
          showIcon
          message="策略加载失败"
          description={error}
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" onClick={() => void load()}>
              重试
            </Button>
          }
        />
      ) : null}
      <Space style={{ marginBottom: 12 }}>
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="搜索主体 / 集群 / 资源 / 动作"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          style={{ width: 260 }}
        />
      </Space>
      <Card className="soft-card" styles={{ body: { padding: 0 } }}>
        {selectedKeys.length > 0 ? (
          <div className="batch-bar">
            <Space size={12}>
              <Typography.Text>
                已选择 <span className="batch-count">{selectedKeys.length}</span> 条策略
              </Typography.Text>
              <Button type="link" size="small" onClick={() => setSelectedKeys([])}>
                清空
              </Button>
            </Space>
            <Button danger size="small" icon={<DeleteOutlined />} onClick={batchRemove}>
              批量删除
            </Button>
          </div>
        ) : null}
        <Table<PolicyDTO>
          rowKey={policyKey}
          columns={columns}
          dataSource={filtered}
          loading={loading}
          rowSelection={rowSelection}
          size="middle"
          pagination={{ pageSize: 10, showTotal: (total) => `共 ${total} 条策略` }}
          scroll={{ x: 1100 }}
          locale={{
            emptyText: (
              <div style={{ padding: '32px 0' }}>
                <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
                  尚未配置策略。创建策略后，组织或用户才能访问目标集群资源。
                </Typography.Paragraph>
              </div>
            ),
          }}
        />
      </Card>
    </div>
  );
}
