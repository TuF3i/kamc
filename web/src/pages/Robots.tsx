import {
  Show,
  createMemo,
  createSignal,
  onMount,
} from 'solid-js';
import {
  createRobot,
  deleteRobot,
  downloadRobotKubeconfig,
  listRobots,
} from '@/api/robots';
import { listClusters } from '@/api/clusters';
import type { Cluster, CreateRobotResponse, Robot } from '@/api/types';
import { toast } from '@/lib/storage/toast';
import {
  Alert,
  Button,
  Card,
  Field,
  Input,
  Modal,
  Select,
  Table,
  Tabs,
  Tag,
  confirm,
  type TableColumnsType,
  type TableRowSelection,
} from '@/lib/widgets';
import { createForm } from '@/lib/widgets/form';
import { PageHeader } from '@/components/PageHeader';
import { PoliciesTab } from '@/components/access/PoliciesTab';
import { PolicyEditor } from '@/components/PolicyEditor';
import { CopyButton } from '@/components/CopyButton';
import { fmtTime } from '@/utils/format';
import { downloadTextFile } from '@/utils/download';
import { summarizeBatch } from '@/utils/batch';
import {
  CloudDownloadOutlined,
  DeleteOutlined,
  PlusOutlined,
  RobotOutlined,
  SettingOutlined,
} from '@/lib/icons';

interface RobotForm extends Record<string, unknown> {
  name: string;
  cluster_id: string;
}

function RobotStatusTag(props: { status: Robot['status'] }) {
  return props.status === 'active' ? (
    <Tag level="success">有效</Tag>
  ) : (
    <Tag level="error">已过期</Tag>
  );
}

export default function Robots() {
  const [robots, setRobots] = createSignal<Robot[]>([]);
  const [clusters, setClusters] = createSignal<Cluster[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);
  const [selectedKeys, setSelectedKeys] = createSignal<(string | number)[]>([]);
  const [createOpen, setCreateOpen] = createSignal(false);
  const [creating, setCreating] = createSignal(false);
  const [robotTab, setRobotTab] = createSignal<'list' | 'policies'>('list');
  const [created, setCreated] = createSignal<CreateRobotResponse | null>(null);
  const [revoking, setRevoking] = createSignal<string | null>(null);
  const [editorOpen, setEditorOpen] = createSignal(false);
  const [editorRobot, setEditorRobot] = createSignal<string | undefined>(undefined);
  const [policiesKey, setPoliciesKey] = createSignal(0);

  const form = createForm<RobotForm>({ name: '', cluster_id: '' });
  const rules = {
    name: [{ required: true, message: '请输入机器人名称' }],
    cluster_id: [{ required: true, message: '请选择集群' }],
  };

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

  onMount(() => void load());

  const submit = async () => {
    if (!form.validate(rules)) return;
    setCreating(true);
    try {
      const result = await createRobot({
        name: String(form.values.name).trim(),
        cluster_id: String(form.values.cluster_id),
      });
      setCreateOpen(false);
      form.reset();
      setCreated(result);
      toast.success(`机器人 ${result.robot.name} 已创建`);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '创建机器人失败');
    } finally {
      setCreating(false);
    }
  };

  const download = async (robot: Robot) => {
    try {
      await downloadRobotKubeconfig(robot);
      toast.success(`机器人 ${robot.name} 的 kubeconfig 已下载`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '下载 kubeconfig 失败');
    }
  };

  const remove = (robot: Robot) => {
    confirm({
      title: `吊销机器人「${robot.name}」？`,
      content: '删除后该机器人的 Token 立即失效，其全部直接策略一并清理，操作不可撤销。',
      okText: '吊销',
      danger: true,
      cancelText: '取消',
      onOk: async () => {
        setRevoking(robot.id);
        try {
          await deleteRobot(robot.id);
          toast.success(`机器人 ${robot.name} 已吊销`);
          setSelectedKeys((keys) => keys.filter((key) => key !== robot.id));
          await load();
        } catch (err) {
          toast.error(err instanceof Error ? err.message : '吊销机器人失败');
        } finally {
          setRevoking(null);
        }
      },
    });
  };

  const selectedRobots = createMemo(() =>
    robots().filter((robot) => selectedKeys().includes(robot.id)),
  );

  const batchRevoke = () => {
    const targets = selectedRobots();
    if (targets.length === 0) return;
    confirm({
      title: `批量吊销 ${targets.length} 个机器人？`,
      content: '吊销后对应 Token 立即失效，其全部直接策略一并清理，操作不可撤销。',
      okText: '批量吊销',
      danger: true,
      cancelText: '取消',
      onOk: async () => {
        const outcome = summarizeBatch(
          await Promise.allSettled(targets.map((robot) => deleteRobot(robot.id))),
        );
        if (outcome.failed === 0) {
          toast.success(`已吊销 ${outcome.succeeded} 个机器人`);
        } else {
          toast.error(
            `已吊销 ${outcome.succeeded} 个机器人，${outcome.failed} 个失败：${outcome.firstError}`,
          );
        }
        setSelectedKeys([]);
        await load();
      },
    });
  };

  const rowSelection = (): TableRowSelection<Robot> => ({
    selectedRowKeys: selectedKeys(),
    onChange: (keys) => setSelectedKeys([...keys]),
    selections: [Table.SELECTION_ALL, Table.SELECTION_INVERT, Table.SELECTION_NONE],
    preserveSelectedRowKeys: true,
  });

  const configurePolicy = (robot: Robot) => {
    setRobotTab('policies');
    setEditorRobot(robot.id);
    setEditorOpen(true);
  };

  const columns: TableColumnsType<Robot> = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 160,
      sorter: (a, b) => a.name.localeCompare(b.name),
      render: (value) => (
        <span class="inline-flex items-center gap-2">
          <RobotOutlined class="h-4 w-4 text-primary" />
          <strong>{String(value)}</strong>
        </span>
      ),
    },
    {
      title: '集群',
      dataIndex: 'cluster_id',
      key: 'cluster_id',
      width: 140,
      render: (value) => <code class="mono">{String(value)}</code>,
    },
    {
      title: '主体',
      dataIndex: 'sub',
      key: 'sub',
      width: 220,
      render: (value) => <code class="mono">{String(value)}</code>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 90,
      render: (_value, record) => <RobotStatusTag status={record.status} />,
    },
    {
      title: '到期时间',
      dataIndex: 'expires_at',
      key: 'expires_at',
      width: 160,
      sorter: (a, b) => a.expires_at.localeCompare(b.expires_at),
      render: (value) => fmtTime(String(value)),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      sorter: (a, b) => a.created_at.localeCompare(b.created_at),
      render: (value) => fmtTime(String(value)),
    },
    {
      title: '操作',
      key: 'actions',
      width: 250,
      fixed: 'right',
      render: (_value, record) => (
        <div class="table-actions">
          <Button
            type="link"
            size="small"
            icon={<SettingOutlined class="h-3.5 w-3.5" />}
            onClick={() => configurePolicy(record)}
          >
            配置策略
          </Button>
          <Button
            type="link"
            size="small"
            icon={<CloudDownloadOutlined class="h-3.5 w-3.5" />}
            disabled={record.status === 'expired'}
            onClick={() => void download(record)}
          >
            下载
          </Button>
          <Button
            type="link"
            size="small"
            danger
            icon={<DeleteOutlined class="h-3.5 w-3.5" />}
            loading={revoking() === record.id}
            onClick={() => remove(record)}
          >
            吊销
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title="机器人"
        subtitle="面向 CICD 场景的机器人身份"
        extra={
          <Show when={robotTab() === 'list'}>
            <Button
              type="primary"
              icon={<PlusOutlined class="h-4 w-4" />}
              onClick={() => {
                form.reset();
                setCreateOpen(true);
              }}
            >
              签发机器人
            </Button>
          </Show>
        }
      />

      <Show when={error()}>
        <Alert
          type="error"
          message="机器人列表加载失败"
          description={error() ?? undefined}
          class="mb-4"
          action={
            <Button size="small" onClick={() => void load()}>
              重试
            </Button>
          }
        />
      </Show>

      <Tabs
        activeKey={robotTab()}
        onChange={(key) => setRobotTab(key as 'list' | 'policies')}
        items={[
          {
            key: 'list',
            label: '机器人',
            children: () => (
              <Card class="soft-card" contentClass="p-0">
                <Show when={selectedKeys().length > 0}>
                  <div class="batch-bar">
                    <div class="flex items-center gap-3">
                      <div>
                        已选择 <span class="batch-count">{selectedKeys().length}</span>{' '}
                        个机器人
                      </div>
                      <Button
                        type="link"
                        size="small"
                        onClick={() => setSelectedKeys([])}
                      >
                        清空
                      </Button>
                    </div>
                    <Button
                      danger
                      size="small"
                      icon={<DeleteOutlined class="h-3.5 w-3.5" />}
                      onClick={batchRevoke}
                    >
                      批量吊销
                    </Button>
                  </div>
                </Show>
                <Table<Robot>
                  rowKey="id"
                  columns={columns}
                  dataSource={robots()}
                  loading={loading()}
                  rowSelection={rowSelection()}
                  pagination={{ pageSize: 10, showTotal: (total) => `共 ${total} 个机器人` }}
                  scroll={{ x: 1220 }}
                  locale={{
                    emptyText: (
                      <div class="px-4 py-7 text-center">
                        <p class="mb-3 text-sm text-layer-content/50">
                          尚未签发机器人。创建后请在「权限」页签为机器人配置策略。
                        </p>
                        <Button
                          type="primary"
                          icon={<PlusOutlined class="h-4 w-4" />}
                          onClick={() => setCreateOpen(true)}
                        >
                          签发第一个机器人
                        </Button>
                      </div>
                    ),
                  }}
                />
              </Card>
            ),
          },
          {
            key: 'policies',
            label: '权限',
            children: () => (
              <PoliciesTab
                subjectScope="robot"
                refreshKey={policiesKey()}
                onCreate={() => {
                  setEditorRobot(undefined);
                  setEditorOpen(true);
                }}
              />
            ),
          },
        ]}
      />

      <Modal
        open={createOpen()}
        title="签发机器人"
        onClose={() => setCreateOpen(false)}
        footer={
          <>
            <Button type="ghost" size="small" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button
              type="primary"
              size="small"
              loading={creating()}
              onClick={() => void submit()}
            >
              签发
            </Button>
          </>
        }
      >
        <div class="form-stack mt-4">
          <Field label="机器人名称" required error={form.errors.name}>
            <Input
              value={String(form.values.name)}
              onValueChange={(value) => form.setValue('name', value)}
              placeholder="ci-deployer"
              autocomplete="off"
              status={form.errors.name ? 'error' : undefined}
            />
          </Field>
          <Field
            label="集群"
            required
            error={form.errors.cluster_id}
            extra="创建后请到「权限」页签为机器人配置资源策略。"
          >
            <Select
              showSearch
              placeholder="选择集群"
              value={form.values.cluster_id || undefined}
              options={clusters().map((cluster) => ({
                value: cluster.id,
                label: `${cluster.id} · ${cluster.name}`,
              }))}
              optionFilterProp="label"
              onChange={(value) => form.setValue('cluster_id', String(value ?? ''))}
            />
          </Field>
        </div>
      </Modal>

      <Modal
        open={created() !== null}
        title="机器人 kubeconfig 已签发"
        onClose={() => setCreated(null)}
        width={760}
        footer={
          <div class="flex items-center gap-2">
            <Button type="ghost" size="small" onClick={() => setCreated(null)}>
              关闭
            </Button>
            <Button
              type="primary"
              size="small"
              icon={<CloudDownloadOutlined class="h-4 w-4" />}
              onClick={() => {
                const current = created();
                if (!current) return;
                downloadTextFile(
                  current.kubeconfig,
                  `kubeconfig-robot-${current.robot.name}.yaml`,
                );
              }}
            >
              下载 kubeconfig
            </Button>
          </div>
        }
      >
        <Show when={created()}>
          {(current) => (
            <div class="robot-kubeconfig">
              <div class="robot-meta">
                <span>
                  名称：<strong>{current().robot.name}</strong>
                </span>
                <span>
                  集群：<code class="mono">{current().robot.cluster_id}</code>
                </span>
                <span>
                  主体：<code class="mono">{current().robot.sub}</code>
                </span>
                <span>到期：{fmtTime(current().robot.expires_at)}</span>
              </div>
              <Alert
                type="info"
                message="机器人目前还没有权限"
                description="机器人是独立主体，请点击机器人列表中的「配置策略」为其单独授予权限（不依赖用户或组）。"
              />
              <pre class="code-block">{current().kubeconfig}</pre>
              <div class="flex flex-wrap items-center gap-2">
                <CopyButton text={current().kubeconfig} label="复制 YAML" type="primary" />
                <span class="text-xs text-layer-content/50">
                  可随时在机器人列表重新下载同一份凭证。
                </span>
              </div>
            </div>
          )}
        </Show>
      </Modal>

      <PolicyEditor
        open={editorOpen()}
        clusters={clusters()}
        robots={robots()}
        robotOnly
        initialSubjectType="robot"
        initialSubject={editorRobot()}
        onClose={() => setEditorOpen(false)}
        onCreated={() => setPoliciesKey((key) => key + 1)}
      />
    </div>
  );
}
