import {
  For,
  Show,
  createResource,
  createSignal,
  onMount,
} from 'solid-js';
import { useNavigate, useParams } from '@solidjs/router';
import {
  createAgentInstallPreview,
  downloadKubeconfig,
  getCluster,
  getClusterInfo,
  issueToken,
  listTokens,
  revokeToken,
} from '@/api/clusters';
import type {
  AgentInstallPreview,
  Cluster,
  ClusterInfo,
  ClusterNodeInfo,
  IssuedToken,
  TokenDTO,
  TokenKind,
} from '@/api/types';
import { toast } from '@/lib/storage/toast';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  FullScreenLoading,
  Modal,
  Result,
  Skeleton,
  Table,
  Tabs,
  Tag,
  TextArea,
  Tooltip,
  confirm,
  type DescriptionItem,
  type TableColumnsType,
} from '@/lib/widgets';
import { CopyButton } from '@/components/CopyButton';
import { PageHeader } from '@/components/PageHeader';
import { ClusterStatusTag, TokenKindTag, TokenStatusTag } from '@/components/StatusTag';
import { fmtTime } from '@/utils/format';
import {
  ArrowLeftOutlined,
  CloudDownloadOutlined,
  ClusterOutlined,
  CodeOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  DesktopOutlined,
  KeyOutlined,
  ReloadOutlined,
} from '@/lib/icons';

function NodeCard(props: { node: ClusterNodeInfo }) {
  const role = () => props.node.roles?.[0] ?? 'worker';
  const isControlPlane = () =>
    role() === 'control-plane' || role() === 'master';
  const status = () =>
    props.node.unschedulable
      ? 'SchedulingDisabled'
      : props.node.ready
        ? 'Ready'
        : 'NotReady';
  const statusClass = () =>
    props.node.unschedulable ? 'scheduling' : props.node.ready ? 'ready' : 'notready';

  return (
    <Tooltip
      title={
        <div class="leading-7">
          <div>
            <strong>{props.node.name}</strong>
          </div>
          <Show when={props.node.internal_ip}>
            <div>InternalIP: {props.node.internal_ip}</div>
          </Show>
          <Show when={props.node.runtime}>
            <div>运行时: {props.node.runtime}</div>
          </Show>
          <Show when={props.node.os_image}>
            <div>系统: {props.node.os_image}</div>
          </Show>
          <Show when={props.node.kubelet_version}>
            <div>kubelet: {props.node.kubelet_version}</div>
          </Show>
          <Show when={props.node.cpu || props.node.memory}>
            <div>
              容量: {props.node.cpu ?? '—'} CPU / {props.node.memory ?? '—'}
            </div>
          </Show>
        </div>
      }
    >
      <div class={`node-card${props.node.ready ? '' : ' node-card-unready'}`}>
        <div class="node-card-head">
          <span class={`node-card-icon ${isControlPlane() ? 'cp' : 'worker'}`}>
            <Show
              when={isControlPlane()}
              fallback={<ClusterOutlined class="h-[18px] w-[18px]" />}
            >
              <DeploymentUnitOutlined class="h-[18px] w-[18px]" />
            </Show>
          </span>
          <div class="node-card-name">
            <span class="node-card-title" title={props.node.name}>
              {props.node.name}
            </span>
            <span class="node-card-ip mono">{props.node.internal_ip || '—'}</span>
          </div>
          <span class={`node-card-status ${statusClass()}`}>{status()}</span>
        </div>
        <div class="node-card-tags">
          <For each={props.node.roles ?? []}>
            {(item) => (
              <Tag level={isControlPlane() ? 'gold' : 'primary'} class="node-role-tag">
                {item}
              </Tag>
            )}
          </For>
          <Show when={props.node.unschedulable}>
            <Tag level="warning">不可调度</Tag>
          </Show>
        </div>
        <div class="node-card-meta">
          <span title="kubelet 版本">
            <CodeOutlined class="h-3.5 w-3.5" /> {props.node.kubelet_version || '—'}
          </span>
          <span title="CPU / 内存容量">
            <DashboardOutlined class="h-3.5 w-3.5" /> {props.node.cpu ?? '—'} CPU ·{' '}
            {props.node.memory ?? '—'}
          </span>
          <span title="操作系统">
            <DesktopOutlined class="h-3.5 w-3.5" />{' '}
            {props.node.os_image || props.node.architecture || '—'}
          </span>
        </div>
      </div>
    </Tooltip>
  );
}

function OverviewTab(props: { cluster: Cluster }) {
  const [info, setInfo] = createSignal<ClusterInfo | null>(null);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);

  const loadInfo = async () => {
    setLoading(true);
    setError(null);
    try {
      setInfo(await getClusterInfo(props.cluster.id));
    } catch (err) {
      setInfo(null);
      setError(err instanceof Error ? err.message : '读取集群信息失败');
    } finally {
      setLoading(false);
    }
  };

  onMount(() => void loadInfo());

  const clusterItems = (): DescriptionItem[] => [
    {
      label: '集群 ID',
      value: <code class="mono strong">{props.cluster.id}</code>,
    },
    { label: '名称', value: props.cluster.name },
    {
      label: '接入状态',
      value: <ClusterStatusTag status={props.cluster.status} />,
    },
    { label: '描述', value: props.cluster.desc || '—' },
    { label: '注册时间', value: fmtTime(props.cluster.created_at) },
    { label: '更新时间', value: fmtTime(props.cluster.updated_at) },
  ];

  const infoItems = (): DescriptionItem[] => {
    const current = info();
    if (!current) return [];
    return [
      {
        label: 'Kubernetes 版本',
        span: 2,
        value: <code class="mono strong">{current.version?.git_version || '—'}</code>,
      },
      {
        label: '控制平面',
        span: 2,
        value: current.control_plane ? (
          <code class="mono">{current.control_plane}</code>
        ) : (
          <span class="muted">—</span>
        ),
      },
      {
        label: 'CoreDNS',
        span: 2,
        value: current.coredns ? (
          <code class="mono">{current.coredns}</code>
        ) : (
          <span class="muted">—</span>
        ),
      },
      {
        label: 'apiserver Go 版本',
        value: <code class="mono">{current.version?.go_version || '—'}</code>,
      },
      { label: '采集时间', value: fmtTime(current.fetched_at) },
    ];
  };

  return (
    <div class="tab-stack">
      <Card title="集群信息" class="soft-card">
        <Descriptions items={clusterItems()} column={{ xs: 1, md: 2 }} />
      </Card>

      <Card
        title="集群详细信息"
        class="soft-card"
        extra={
          <Button
            icon={<ReloadOutlined class="h-4 w-4" />}
            size="small"
            loading={loading()}
            onClick={() => void loadInfo()}
          >
            刷新
          </Button>
        }
      >
        <Show
          when={!loading()}
          fallback={<Skeleton paragraph={{ rows: 4 }} />}
        >
          <Show
            when={!error() && info()}
            fallback={
              <Alert
                type="warning"
                message="无法读取集群信息"
                description="集群未接入或 Agent 隧道离线；接入 Agent 后即可读取集群详细信息。"
              />
            }
          >
            <Descriptions items={infoItems()} column={{ xs: 1, md: 2 }} />
          </Show>
        </Show>
      </Card>

      <div class="section-divider">
        <span>集群节点</span>
        <Show when={(info()?.nodes?.length ?? 0) > 0}>
          <span class="text-[13px] text-layer-content/50">
            共 {info()!.nodes!.length} 个 · Ready{' '}
            {info()!.nodes!.filter((node) => node.ready).length}
          </span>
        </Show>
      </div>

      <Show when={info()}>
        <Show
          when={(info()?.nodes?.length ?? 0) > 0}
          fallback={
            <Card class="soft-card">
              <Empty description="未能读取节点列表（可能 Agent 版本较旧或权限不足）" />
            </Card>
          }
        >
          <div class="node-grid">
            <For each={info()!.nodes}>
              {(node) => <NodeCard node={node} />}
            </For>
          </div>
        </Show>
      </Show>
    </div>
  );
}

function TokensTab(props: { clusterId: string }) {
  const [tokens, setTokens] = createSignal<TokenDTO[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);
  const [issuing, setIssuing] = createSignal<TokenKind | null>(null);
  const [issued, setIssued] = createSignal<IssuedToken | null>(null);
  const [revoking, setRevoking] = createSignal<TokenKind | null>(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      setTokens(await listTokens(props.clusterId));
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载 Token 列表失败');
    } finally {
      setLoading(false);
    }
  };

  onMount(() => void load());

  const doIssue = async (kind: TokenKind) => {
    setIssuing(kind);
    try {
      const result = await issueToken(props.clusterId, kind);
      setIssued(result);
      toast.success(kind === 'register' ? '注册 Token 已签发' : '长期 Token 已签发');
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '签发 Token 失败');
    } finally {
      setIssuing(null);
    }
  };

  const confirmIssue = (kind: TokenKind) => {
    if (kind === 'register') {
      void doIssue(kind);
      return;
    }
    confirm({
      title: '签发 / 轮换长期 Token？',
      content:
        '若已存在长期 Token，本次签发会将其轮换：旧 Token 进入 24 小时宽限期，宽限期内仍可建立隧道。',
      okText: '签发',
      cancelText: '取消',
      onOk: () => doIssue(kind),
    });
  };

  const confirmRevoke = (kind: TokenKind) => {
    confirm({
      title: `吊销${kind === 'register' ? '注册' : '长期'} Token？`,
      content: '吊销后立即失效，无宽限期；Agent 将无法再建立隧道。',
      okText: '吊销',
      danger: true,
      cancelText: '取消',
      onOk: async () => {
        setRevoking(kind);
        try {
          await revokeToken(props.clusterId, kind);
          toast.success('Token 已吊销');
          await load();
        } catch (err) {
          toast.error(err instanceof Error ? err.message : '吊销 Token 失败');
        } finally {
          setRevoking(null);
        }
      },
    });
  };

  const columns: TableColumnsType<TokenDTO> = [
    {
      title: '类型',
      dataIndex: 'kind',
      key: 'kind',
      width: 140,
      render: (kind) => <TokenKindTag kind={kind as TokenKind} />,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 110,
      render: (_value, record) => <TokenStatusTag status={record.status} />,
    },
    {
      title: '过期时间',
      dataIndex: 'expires_at',
      key: 'expires_at',
      width: 190,
      render: (value) =>
        value ? fmtTime(String(value)) : <span class="muted">长期有效</span>,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 190,
      render: (value) => fmtTime(String(value)),
    },
    {
      title: '操作',
      key: 'actions',
      width: 110,
      render: (_value, record) => (
        <Button
          type="link"
          size="small"
          danger
          loading={revoking() === record.kind}
          onClick={() => confirmRevoke(record.kind)}
        >
          吊销
        </Button>
      ),
    },
  ];

  return (
    <div class="tab-stack">
      <Card
        class="soft-card"
        title="Agent Token"
        extra={
          <div class="flex flex-wrap items-center gap-2">
            <Button
              icon={<KeyOutlined class="h-4 w-4" />}
              loading={issuing() === 'register'}
              onClick={() => confirmIssue('register')}
            >
              签发注册 Token（1h 一次性）
            </Button>
            <Button
              type="primary"
              icon={<KeyOutlined class="h-4 w-4" />}
              loading={issuing() === 'longterm'}
              onClick={() => confirmIssue('longterm')}
            >
              签发 / 轮换长期 Token
            </Button>
          </div>
        }
        contentClass="p-0"
      >
        <Show when={error()}>
          <div class="p-4 pb-0">
            <Alert
              type="error"
              message="Token 列表加载失败"
              description={error() ?? undefined}
              action={
                <Button size="small" onClick={() => void load()}>
                  重试
                </Button>
              }
            />
          </div>
        </Show>
        <Table<TokenDTO>
          rowKey={(record) => record.kind}
          columns={columns}
          dataSource={tokens()}
          loading={loading()}
          pagination={false}
          locale={{ emptyText: <Empty description="尚未签发 Agent Token" class="border-0" /> }}
          scroll={{ x: 760 }}
        />
      </Card>

      <Modal
        open={issued() !== null}
        title="Token 已签发（仅此一次）"
        onClose={() => setIssued(null)}
        width={680}
        footer={
          <Button type="primary" size="small" onClick={() => setIssued(null)}>
            我已保存
          </Button>
        }
      >
        <Show when={issued()}>
          {(token) => (
            <div class="token-issued">
              <Alert
                type="warning"
                message="关闭此窗口后将无法再次查看明文 Token"
                description="请立即复制并妥善保存；平台只保存 SHA256 哈希。"
                class="mb-4"
              />
              <div class="token-block">
                <TextArea
                  value={token().token}
                  readOnly
                  rows={3}
                  class="mono"
                />
                <CopyButton text={token().token} label="复制 Token" type="primary" />
              </div>
              <Show when={token().kind === 'register'}>
                <p class="mt-4 text-[13px] leading-6 text-layer-content/55">
                  注册 Token 1 小时内有效且只能使用一次：Agent 调用{' '}
                  <code class="mono">POST /api/v1/agent/register</code> 换取长期 Token 后即被删除。
                </p>
              </Show>
              <Show when={token().grace_ends_at}>
                <Alert
                  type="info"
                  class="mt-4"
                  message={`旧长期 Token 宽限期至 ${fmtTime(token().grace_ends_at)}`}
                  description="宽限期内旧 Token 仍可用于建立隧道；到期后自动失效。"
                />
              </Show>
            </div>
          )}
        </Show>
      </Modal>
    </div>
  );
}

function KubeconfigTab(props: { clusterId: string }) {
  const [mode, setMode] = createSignal<'exec' | 'static'>('exec');
  const [downloading, setDownloading] = createSignal(false);

  const download = async () => {
    setDownloading(true);
    try {
      await downloadKubeconfig(props.clusterId, mode());
      toast.success(`集群 ${props.clusterId} 的 kubeconfig 已下载`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '下载 kubeconfig 失败');
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div class="tab-stack">
      <Card title="kubeconfig 下载" class="soft-card">
        <div class="flex flex-col items-start gap-4">
          <div class="inline-flex rounded-lg border border-layer-content/10 bg-layer-content/5 p-0.5">
            <For each={[
              { label: 'exec（推荐）', value: 'exec' as const },
              { label: 'static', value: 'static' as const },
            ]}>
              {(option) => (
                <button
                  type="button"
                  class={`min-h-8 cursor-pointer rounded-md border border-transparent px-3 text-[13px] font-bold ${
                    mode() === option.value
                      ? 'border-layer-content/15 bg-layer/80 text-primary'
                      : 'text-layer-content/55'
                  }`}
                  onClick={() => setMode(option.value)}
                >
                  {option.label}
                </button>
              )}
            </For>
          </div>
          <div class="text-[13px] text-layer-content/50">
            {mode() === 'exec'
              ? '写入 kamc 命令行，凭证自动续期；需要本机 PATH 中存在 kamc。'
              : '直接嵌入当前 JWT，1 小时后过期，需要重新下载。'}
          </div>
          <Button
            type="primary"
            icon={<CloudDownloadOutlined class="h-4 w-4" />}
            loading={downloading()}
            onClick={() => void download()}
          >
            下载 kubeconfig
          </Button>
        </div>
      </Card>
    </div>
  );
}

function AgentInstallTab(props: { clusterId: string }) {
  const [preview, setPreview] = createSignal<AgentInstallPreview | null>(null);
  const [generating, setGenerating] = createSignal(false);

  const generate = () => {
    confirm({
      title: '生成一键安装命令？',
      content:
        '将签发/轮换该集群的长期 Agent Token（旧 Token 进入 24 小时宽限期），并生成包含 Token 的安装清单。明文 Token 仅此一次展示。',
      okText: '生成',
      cancelText: '取消',
      onOk: async () => {
        setGenerating(true);
        try {
          const issued = await issueToken(props.clusterId, 'longterm');
          const result = await createAgentInstallPreview(props.clusterId, issued.token);
          setPreview(result);
          toast.success('安装命令已生成');
        } catch (err) {
          toast.error(err instanceof Error ? err.message : '生成安装命令失败');
        } finally {
          setGenerating(false);
        }
      },
    });
  };

  return (
    <div class="tab-stack">
      <Card title="一键安装 Agent" class="soft-card">
        <Show
          when={preview()}
          fallback={
            <Empty
              description="尚未生成本集群的安装命令"
              class="border-0"
            >
              <Button
                type="primary"
                icon={<CodeOutlined class="h-4 w-4" />}
                loading={generating()}
                onClick={generate}
              >
                生成安装命令
              </Button>
            </Empty>
          }
        >
          {(install) => (
            <div>
              <Alert
                type="warning"
                class="mb-4"
                message="安装命令内含 Agent Token，等同集群接入凭证，请勿外泄"
                description="kubectl 会从平台拉取安装清单；清单中的 Token 已写入目标集群 Secret，可在 Agent Token 页签吊销。"
              />
              <Descriptions
                column={1}
                class="mb-4"
                items={[
                  {
                    label: 'Agent 镜像',
                    value: <code class="mono strong">{install().image}</code>,
                  },
                  {
                    label: '隧道入口',
                    value: <code class="mono">{install().tunnel_url}</code>,
                  },
                ]}
              />
              <div class="mb-2 font-bold">安装命令（在目标集群执行）</div>
              <div class="token-block">
                <pre class="code-block mb-2">{install().apply_command}</pre>
                <CopyButton
                  text={install().apply_command}
                  label="复制安装命令"
                  type="primary"
                />
              </div>
              <div class="mb-2 mt-4 font-bold">安装清单预览</div>
              <div class="token-block">
                <pre class="code-block mb-2 max-h-[360px] overflow-auto">{install().yaml}</pre>
                <CopyButton text={install().yaml} label="复制 YAML" />
              </div>
              <Button class="mt-4" loading={generating()} onClick={generate}>
                重新生成（轮换 Token）
              </Button>
            </div>
          )}
        </Show>
      </Card>
    </div>
  );
}

function DetailView(props: { cluster: Cluster }) {
  const navigate = useNavigate();
  return (
    <div>
      <PageHeader
        title={props.cluster.name}
        subtitle={
          <>
            集群 ID：<code class="mono strong">{props.cluster.id}</code>
          </>
        }
        tags={<ClusterStatusTag status={props.cluster.status} />}
        extra={
          <div class="flex flex-wrap items-center gap-2">
            <Button
              icon={<ReloadOutlined class="h-4 w-4" />}
              onClick={() => window.location.reload()}
            >
              刷新
            </Button>
            <Button
              icon={<ArrowLeftOutlined class="h-4 w-4" />}
              onClick={() => navigate('/clusters')}
            >
              返回列表
            </Button>
          </div>
        }
      />

      <Tabs
        defaultActiveKey="overview"
        items={[
          {
            key: 'overview',
            label: '概况',
            children: () => <OverviewTab cluster={props.cluster} />,
          },
          {
            key: 'tokens',
            label: 'Agent Token',
            children: () => <TokensTab clusterId={props.cluster.id} />,
          },
          {
            key: 'agent-install',
            label: 'Agent 安装',
            children: () => <AgentInstallTab clusterId={props.cluster.id} />,
          },
          {
            key: 'kubeconfig',
            label: 'kubeconfig',
            children: () => <KubeconfigTab clusterId={props.cluster.id} />,
          },
        ]}
      />
    </div>
  );
}

export default function ClusterDetail() {
  const navigate = useNavigate();
  const params = useParams<{ id: string }>();
  const [cluster] = createResource(() => params.id, (id) => getCluster(id));

  return (
    <Show
      when={!cluster.loading}
      fallback={<FullScreenLoading tip="正在加载集群…" />}
    >
      <Show
        when={!cluster.error && cluster()}
        fallback={
          <Result
            status="404"
            title="集群不存在或加载失败"
            subTitle={
              cluster.error instanceof Error
                ? cluster.error.message
                : cluster.error
                  ? '加载集群失败'
                  : `未找到集群 ${params.id}`
            }
            extra={
              <Button type="primary" onClick={() => navigate('/clusters')}>
                返回集群列表
              </Button>
            }
          />
        }
      >
        <DetailView cluster={cluster()!} />
      </Show>
    </Show>
  );
}
