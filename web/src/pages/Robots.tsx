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
  Select,
  Space,
  Table,
  Tag,
  Typography,
  type TableColumnsType,
  type TableProps,
} from 'antd';
import {
  CloudDownloadOutlined,
  DeleteOutlined,
  PlusOutlined,
  RobotOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import { createRobot, deleteRobot, downloadRobotKubeconfig, listRobots } from '@/api/robots';
import { listClusters } from '@/api/clusters';
import type { Cluster, CreateRobotResponse, Robot } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { CopyButton } from '@/components/CopyButton';
import { fmtTime } from '@/utils/format';
import { downloadTextFile } from '@/utils/download';
import { summarizeBatch } from '@/utils/batch';

interface RobotForm {
  name: string;
  cluster_id: string;
}

function RobotStatusTag({ status }: { status: Robot['status'] }) {
  return status === 'active' ? <Tag color="success">有效</Tag> : <Tag color="error">已过期</Tag>;
}

export function Robots() {
  const { message, modal } = App.useApp();
  const navigate = useNavigate();
  const [robots, setRobots] = useState<Robot[]>([]);
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedKeys, setSelectedKeys] = useState<React.Key[]>([]);
  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<CreateRobotResponse | null>(null);
  const [revoking, setRevoking] = useState<string | null>(null);
  const [form] = Form.useForm<RobotForm>();

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const [robotList, clusterList] = await Promise.all([listRobots(), listClusters()]);
      setRobots(robotList);
      setClusters(clusterList);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载机器人列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const submit = async (values: RobotForm) => {
    setCreating(true);
    try {
      const result = await createRobot({
        name: values.name.trim(),
        cluster_id: values.cluster_id,
      });
      setCreateOpen(false);
      form.resetFields();
      setCreated(result);
      message.success(`机器人 ${result.robot.name} 已创建`);
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '创建机器人失败');
    } finally {
      setCreating(false);
    }
  };

  const download = async (robot: Robot) => {
    try {
      await downloadRobotKubeconfig(robot);
      message.success(`机器人 ${robot.name} 的 kubeconfig 已下载`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '下载 kubeconfig 失败');
    }
  };

  const remove = (robot: Robot) => {
    modal.confirm({
      title: `吊销机器人「${robot.name}」？`,
      content: '删除后该机器人的 Token 立即失效，其全部直接策略一并清理，操作不可撤销。',
      okText: '吊销',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        setRevoking(robot.id);
        try {
          await deleteRobot(robot.id);
          message.success(`机器人 ${robot.name} 已吊销`);
          setSelectedKeys((keys) => keys.filter((key) => key !== robot.id));
          await load();
        } catch (err) {
          message.error(err instanceof Error ? err.message : '吊销机器人失败');
        } finally {
          setRevoking(null);
        }
      },
    });
  };

  const selectedRobots = useMemo(
    () => robots.filter((r) => selectedKeys.includes(r.id)),
    [robots, selectedKeys],
  );

  const batchRevoke = () => {
    const targets = selectedRobots;
    if (targets.length === 0) return;
    modal.confirm({
      title: `批量吊销 ${targets.length} 个机器人？`,
      content: '吊销后对应 Token 立即失效，其全部直接策略一并清理，操作不可撤销。',
      okText: '批量吊销',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        const outcome = summarizeBatch(
          await Promise.allSettled(targets.map((r) => deleteRobot(r.id))),
        );
        if (outcome.failed === 0) {
          message.success(`已吊销 ${outcome.succeeded} 个机器人`);
        } else {
          message.error(
            `已吊销 ${outcome.succeeded} 个机器人，${outcome.failed} 个失败：${outcome.firstError}`,
          );
        }
        setSelectedKeys([]);
        await load();
      },
    });
  };

  const rowSelection: TableProps<Robot>['rowSelection'] = {
    selectedRowKeys: selectedKeys,
    onChange: (keys) => setSelectedKeys([...keys]),
    selections: [Table.SELECTION_ALL, Table.SELECTION_INVERT, Table.SELECTION_NONE],
    preserveSelectedRowKeys: true,
  };

  const configurePolicy = (robot: Robot) => {
    navigate(`/access?subjectType=user&subject=${encodeURIComponent(robot.sub)}`);
  };

  const columns: TableColumnsType<Robot> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 180,
      sorter: (a, b) => a.name.localeCompare(b.name),
      render: (name: string) => (
        <Space size={8}>
          <RobotOutlined style={{ color: '#7cb3ff' }} />
          <strong>{name}</strong>
        </Space>
      ),
    },
    {
      title: '集群',
      dataIndex: 'cluster_id',
      key: 'cluster_id',
      width: 180,
      render: (value: string) => <code className="mono">{value}</code>,
    },
    {
      title: '主体',
      dataIndex: 'sub',
      key: 'sub',
      width: 260,
      render: (value: string) => <code className="mono">{value}</code>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: Robot['status']) => <RobotStatusTag status={status} />,
    },
    {
      title: '到期时间',
      dataIndex: 'expires_at',
      key: 'expires_at',
      width: 180,
      sorter: (a, b) => a.expires_at.localeCompare(b.expires_at),
      render: (value: string) => fmtTime(value),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      sorter: (a, b) => a.created_at.localeCompare(b.created_at),
      render: (value: string) => fmtTime(value),
    },
    {
      title: '操作',
      key: 'actions',
      width: 300,
      render: (_, record) => (
        <Space size={0}>
          <Button
            type="link"
            size="small"
            icon={<SettingOutlined />}
            onClick={() => configurePolicy(record)}
          >
            配置策略
          </Button>
          <Button
            type="link"
            size="small"
            icon={<CloudDownloadOutlined />}
            disabled={record.status === 'expired'}
            onClick={() => void download(record)}
          >
            下载
          </Button>
          <Button
            type="link"
            size="small"
            danger
            icon={<DeleteOutlined />}
            loading={revoking === record.id}
            onClick={() => remove(record)}
          >
            吊销
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title="机器人"
        subtitle="为 CICD 机器人签发长期 kubeconfig；机器人以 user:<sub> 主体在权限页配置策略。"
        extra={
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              form.resetFields();
              setCreateOpen(true);
            }}
          >
            签发机器人
          </Button>
        }
      />

      {error ? (
        <Alert
          type="error"
          showIcon
          message="机器人列表加载失败"
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
                已选择 <span className="batch-count">{selectedKeys.length}</span> 个机器人
              </Typography.Text>
              <Button type="link" size="small" onClick={() => setSelectedKeys([])}>
                清空
              </Button>
            </Space>
            <Button danger size="small" icon={<DeleteOutlined />} onClick={batchRevoke}>
              批量吊销
            </Button>
          </div>
        ) : null}
        <Table<Robot>
          rowKey="id"
          columns={columns}
          dataSource={robots}
          loading={loading}
          rowSelection={rowSelection}
          pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 个机器人` }}
          scroll={{ x: 1300 }}
          locale={{
            emptyText: (
              <div style={{ padding: '28px 0' }}>
                <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
                  尚未签发机器人。创建后请在权限页为机器人主体配置策略。
                </Typography.Paragraph>
                <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
                  签发第一个机器人
                </Button>
              </div>
            ),
          }}
        />
      </Card>

      <Modal
        title="签发机器人"
        open={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={() => form.submit()}
        okText="签发"
        cancelText="取消"
        confirmLoading={creating}
        destroyOnHidden
      >
        <Form<RobotForm>
          form={form}
          layout="vertical"
          onFinish={submit}
          preserve={false}
          style={{ marginTop: 16 }}
        >
          <Form.Item
            name="name"
            label="机器人名称"
            rules={[{ required: true, message: '请输入机器人名称' }]}
          >
            <Input placeholder="ci-deployer" autoComplete="off" />
          </Form.Item>
          <Form.Item
            name="cluster_id"
            label="集群"
            rules={[{ required: true, message: '请选择集群' }]}
            extra="创建后请到权限页为 user:<sub> 配置资源策略。"
          >
            <Select
              showSearch
              placeholder="选择集群"
              options={clusters.map((c) => ({
                value: c.id,
                label: `${c.id} · ${c.name}`,
              }))}
              optionFilterProp="label"
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="机器人 kubeconfig 已签发"
        open={created !== null}
        onCancel={() => setCreated(null)}
        width={760}
        footer={
          <Space>
            <Button onClick={() => setCreated(null)}>关闭</Button>
            <Button
              type="primary"
              icon={<CloudDownloadOutlined />}
              onClick={() => {
                if (!created) return;
                downloadTextFile(
                  created.kubeconfig,
                  `kubeconfig-robot-${created.robot.name}.yaml`,
                );
              }}
            >
              下载 kubeconfig
            </Button>
          </Space>
        }
      >
        {created ? (
          <div className="robot-kubeconfig">
            <div className="robot-meta">
              <span>
                名称：<strong>{created.robot.name}</strong>
              </span>
              <span>
                集群：<code className="mono">{created.robot.cluster_id}</code>
              </span>
              <span>
                主体：<code className="mono">{created.robot.sub}</code>
              </span>
              <span>到期：{fmtTime(created.robot.expires_at)}</span>
            </div>
            <Alert
              type="info"
              showIcon
              message="机器人目前还没有权限"
              description={
                <span>
                  请到权限页为 <code className="mono">user:{created.robot.sub}</code>{' '}
                  配置策略，或点击机器人列表中的「配置策略」。
                </span>
              }
            />
            <pre className="code-block">{created.kubeconfig}</pre>
            <Space>
              <CopyButton text={created.kubeconfig} label="复制 YAML" type="primary" />
              <Typography.Text type="secondary">
                可随时在机器人列表重新下载同一份凭证。
              </Typography.Text>
            </Space>
          </div>
        ) : null}
      </Modal>
    </div>
  );
}
