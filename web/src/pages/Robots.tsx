import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  App,
  AutoComplete,
  Button,
  Card,
  Form,
  Input,
  Modal,
  Space,
  Table,
  Tag,
  Typography,
  type TableColumnsType,
} from 'antd';
import {
  CloudDownloadOutlined,
  DeleteOutlined,
  PlusOutlined,
  RobotOutlined,
} from '@ant-design/icons';
import { createRobot, deleteRobot, downloadRobotKubeconfig, listRobots } from '@/api/robots';
import { listRoles } from '@/api/roles';
import { listClusters } from '@/api/clusters';
import type { Cluster, CreateRobotResponse, Robot, RoleDTO } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { CopyButton } from '@/components/CopyButton';
import { fmtTime } from '@/utils/format';
import { downloadTextFile } from '@/utils/download';

interface RobotForm {
  name: string;
  cluster_id: string;
  role: string;
}

function RobotStatusTag({ status }: { status: Robot['status'] }) {
  return status === 'active' ? <Tag color="success">有效</Tag> : <Tag color="error">已过期</Tag>;
}

export function Robots() {
  const { message } = App.useApp();
  const [robots, setRobots] = useState<Robot[]>([]);
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [roles, setRoles] = useState<RoleDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<CreateRobotResponse | null>(null);
  const [revoking, setRevoking] = useState<string | null>(null);
  const [form] = Form.useForm<RobotForm>();

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const [robotList, clusterList, roleList] = await Promise.all([
        listRobots(),
        listClusters(),
        listRoles(),
      ]);
      setRobots(robotList);
      setClusters(clusterList);
      setRoles(roleList);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载机器人列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const roleOptions = useMemo(
    () => roles.map((r) => ({ value: r.name, label: `${r.name}${r.desc ? ` · ${r.desc}` : ''}` })),
    [roles],
  );

  const submit = async (values: RobotForm) => {
    setCreating(true);
    try {
      const result = await createRobot({
        name: values.name.trim(),
        cluster_id: values.cluster_id,
        role: values.role.trim(),
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
    Modal.confirm({
      title: `吊销机器人「${robot.name}」？`,
      content: '删除后该机器人的 Token 立即失效，角色绑定一并移除，操作不可撤销。',
      okText: '吊销',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        setRevoking(robot.id);
        try {
          await deleteRobot(robot.id);
          message.success(`机器人 ${robot.name} 已吊销`);
          await load();
        } catch (err) {
          message.error(err instanceof Error ? err.message : '吊销机器人失败');
        } finally {
          setRevoking(null);
        }
      },
    });
  };

  const columns: TableColumnsType<Robot> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 200,
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
      width: 200,
      render: (value: string) => <code className="mono">{value}</code>,
    },
    {
      title: '角色',
      dataIndex: 'role',
      key: 'role',
      width: 200,
      render: (value: string) => <Tag color="blue">{value}</Tag>,
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
      width: 220,
      render: (_, record) => (
        <Space size={0}>
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
        subtitle="为 CICD 机器人签发长期 kubeconfig；机器人以独立身份访问指定集群。"
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
        <Table<Robot>
          rowKey="id"
          columns={columns}
          dataSource={robots}
          loading={loading}
          pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 个机器人` }}
          scroll={{ x: 1200 }}
          locale={{
            emptyText: (
              <div style={{ padding: '28px 0' }}>
                <Typography.Paragraph type="secondary" style={{ marginBottom: 12 }}>
                  尚未签发机器人。先创建角色并配置策略，再为机器人选择该角色。
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
          >
            <AutoComplete
              options={clusters.map((c) => ({
                value: c.id,
                label: `${c.id} · ${c.name}`,
              }))}
              placeholder="选择集群"
              filterOption={(input, option) =>
                String(option?.value ?? '').toLowerCase().includes(input.toLowerCase())
              }
            />
          </Form.Item>
          <Form.Item
            name="role"
            label="角色"
            rules={[{ required: true, message: '请选择角色' }]}
            extra="机器人会自动绑定到该角色；权限由角色策略决定。"
          >
            <AutoComplete
              options={roleOptions}
              placeholder="选择角色"
              filterOption={(input, option) =>
                String(option?.value ?? '').toLowerCase().includes(input.toLowerCase())
              }
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
                角色：<Tag color="blue">{created.robot.role}</Tag>
              </span>
              <span>到期：{fmtTime(created.robot.expires_at)}</span>
            </div>
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
