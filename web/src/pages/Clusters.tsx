import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Alert,
  App,
  Button,
  Card,
  Form,
  Input,
  Modal,
  Space,
  Table,
  Tooltip,
  Typography,
  type TableColumnsType,
  type TableProps,
} from 'antd';
import { DeleteOutlined, PlusOutlined, SearchOutlined } from '@ant-design/icons';
import { createCluster, deleteCluster, listClusters } from '@/api/clusters';
import type { Cluster } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { ClusterStatusTag } from '@/components/StatusTag';
import { fmtTime } from '@/utils/format';
import { summarizeBatch } from '@/utils/batch';

interface CreateFormValues {
  id: string;
  name: string;
  desc?: string;
}

export function Clusters() {
  const { message, modal } = App.useApp();
  const navigate = useNavigate();
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [selectedKeys, setSelectedKeys] = useState<React.Key[]>([]);
  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Cluster | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState('');
  const [deleting, setDeleting] = useState(false);
  const [form] = Form.useForm<CreateFormValues>();

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      setClusters(await listClusters());
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载集群列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const filtered = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) return clusters;
    return clusters.filter((c) =>
      [c.id, c.name, c.desc].some((v) => v.toLowerCase().includes(keyword)),
    );
  }, [clusters, search]);

  const openCreate = () => {
    form.resetFields();
    setCreateOpen(true);
  };

  const submitCreate = async (values: CreateFormValues) => {
    setCreating(true);
    try {
      await createCluster({
        id: values.id.trim(),
        name: values.name.trim(),
        desc: values.desc?.trim() || undefined,
      });
      message.success(`集群 ${values.id.trim()} 已注册`);
      setCreateOpen(false);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '注册集群失败');
    } finally {
      setCreating(false);
    }
  };

  const openDelete = (cluster: Cluster) => {
    setDeleteTarget(cluster);
    setDeleteConfirm('');
  };

  const submitDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deleteCluster(deleteTarget.id);
      message.success(`集群 ${deleteTarget.id} 已注销`);
      setSelectedKeys((keys) => keys.filter((k) => k !== deleteTarget.id));
      setDeleteTarget(null);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '注销集群失败');
    } finally {
      setDeleting(false);
    }
  };

  const selectedClusters = useMemo(
    () => clusters.filter((c) => selectedKeys.includes(c.id)),
    [clusters, selectedKeys],
  );

  const batchDelete = () => {
    const targets = selectedClusters;
    if (targets.length === 0) return;
    modal.confirm({
      title: `批量注销 ${targets.length} 个集群？`,
      content: '仅删除集群记录，该操作不可撤销；已签发的 Agent Token、Casbin 策略不会自动清理，需要另行处理。',
      okText: '批量注销',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        const outcome = summarizeBatch(
          await Promise.allSettled(targets.map((c) => deleteCluster(c.id))),
        );
        if (outcome.failed === 0) {
          message.success(`已注销 ${outcome.succeeded} 个集群`);
        } else {
          message.error(
            `已注销 ${outcome.succeeded} 个集群，${outcome.failed} 个失败：${outcome.firstError}`,
          );
        }
        setSelectedKeys([]);
        await load();
      },
    });
  };

  const rowSelection: TableProps<Cluster>['rowSelection'] = {
    selectedRowKeys: selectedKeys,
    onChange: (keys) => setSelectedKeys([...keys]),
    selections: [Table.SELECTION_ALL, Table.SELECTION_INVERT, Table.SELECTION_NONE],
    preserveSelectedRowKeys: true,
  };

  const columns: TableColumnsType<Cluster> = [
    {
      title: '集群 ID',
      dataIndex: 'id',
      key: 'id',
      width: 150,
      sorter: (a, b) => a.id.localeCompare(b.id),
      render: (id: string) => (
        <Tooltip title={id}>
          <code className="mono strong">{id.length > 18 ? `${id.slice(0, 15)}…` : id}</code>
        </Tooltip>
      ),
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 200,
      sorter: (a, b) => a.name.localeCompare(b.name, 'zh-CN'),
    },
    {
      title: '描述',
      dataIndex: 'desc',
      key: 'desc',
      ellipsis: true,
      render: (desc: string) => desc || <Typography.Text type="secondary">—</Typography.Text>,
    },
    {
      title: '接入状态',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      filters: [
        { text: '在线', value: 'online' },
        { text: '待接入', value: 'pending' },
        { text: '离线', value: 'offline' },
      ],
      onFilter: (value, record) => record.status === value,
      render: (status: Cluster['status']) => <ClusterStatusTag status={status} />,
    },
    {
      title: '注册时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      sorter: (a, b) => a.created_at.localeCompare(b.created_at),
      render: (value: string) => fmtTime(value),
    },
    {
      title: '操作',
      key: 'actions',
      width: 150,
      render: (_, record) => (
        <Space size={0} onClick={(e) => e.stopPropagation()}>
          <Button type="link" size="small" onClick={() => navigate(`/clusters/${encodeURIComponent(record.id)}`)}>
            详情
          </Button>
          <Button
            type="link"
            size="small"
            danger
            icon={<DeleteOutlined />}
            onClick={() => openDelete(record)}
          >
            注销
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title="集群"
        subtitle="注册并管理目标 Kubernetes 集群；接入状态来自 Agent 反向隧道。"
        extra={
          <Space>
            <Input
              allowClear
              prefix={<SearchOutlined />}
              placeholder="搜索 ID / 名称 / 描述"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              style={{ width: 240 }}
            />
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
              注册集群
            </Button>
          </Space>
        }
      />

      {error ? (
        <Alert
          type="error"
          showIcon
          message="集群列表加载失败"
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
        {selectedKeys.length > 0 ? (
          <div className="batch-bar">
            <Space size={12}>
              <Typography.Text>
                已选择 <span className="batch-count">{selectedKeys.length}</span> 个集群
              </Typography.Text>
              <Button type="link" size="small" onClick={() => setSelectedKeys([])}>
                清空
              </Button>
            </Space>
            <Button danger size="small" icon={<DeleteOutlined />} onClick={batchDelete}>
              批量注销
            </Button>
          </div>
        ) : null}
        <Table<Cluster>
          rowKey="id"
          className="clickable-table"
          columns={columns}
          dataSource={filtered}
          loading={loading}
          rowSelection={rowSelection}
          onRow={(record) => ({
            onClick: (event) => {
              if ((event.target as HTMLElement).closest('.ant-table-selection-column')) return;
              navigate(`/clusters/${encodeURIComponent(record.id)}`);
            },
          })}
          pagination={{
            pageSize: 10,
            showSizeChanger: true,
            pageSizeOptions: [10, 20, 50],
            showTotal: (total) => `共 ${total} 个集群`,
          }}
          scroll={{ x: 900 }}
        />
      </Card>

      <Modal
        title="注册集群"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => form.submit()}
        okText="注册"
        cancelText="取消"
        confirmLoading={creating}
        destroyOnHidden
      >
        <Form<CreateFormValues>
          form={form}
          layout="vertical"
          onFinish={submitCreate}
          preserve={false}
          style={{ marginTop: 16 }}
        >
          <Form.Item
            name="id"
            label="集群 ID"
            rules={[
              { required: true, message: '请输入集群 ID' },
              { max: 64, message: '集群 ID 最长 64 个字符' },
            ]}
            extra="唯一标识，同时作为 Casbin dom（如 prod-bj）；注册后不可修改。"
          >
            <Input placeholder="prod-bj" className="mono" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="name"
            label="名称"
            rules={[{ required: true, message: '请输入集群名称' }]}
          >
            <Input placeholder="北京生产集群" autoComplete="off" />
          </Form.Item>
          <Form.Item name="desc" label="描述">
            <Input.TextArea placeholder="可选，说明集群用途" rows={3} maxLength={500} showCount />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="注销集群"
        open={deleteTarget !== null}
        onCancel={() => setDeleteTarget(null)}
        onOk={() => void submitDelete()}
        okText="确认注销"
        cancelText="取消"
        okButtonProps={{ danger: true, disabled: deleteConfirm !== deleteTarget?.id }}
        confirmLoading={deleting}
      >
        <Alert
          type="warning"
          showIcon
          message="该操作不可撤销"
          description="仅删除集群记录。已签发的 Agent Token、Casbin 策略与绑定不会自动清理，需要另行处理。"
          style={{ marginBottom: 16 }}
        />
        <Typography.Paragraph>
          请输入集群 ID <code className="mono strong">{deleteTarget?.id}</code> 以确认：
        </Typography.Paragraph>
        <Input
          value={deleteConfirm}
          onChange={(e) => setDeleteConfirm(e.target.value)}
          placeholder={deleteTarget?.id}
          className="mono"
          autoComplete="off"
        />
      </Modal>
    </div>
  );
}
