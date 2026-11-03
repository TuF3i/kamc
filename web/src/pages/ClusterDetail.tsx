import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import {
  Alert,
  App,
  Button,
  Card,
  Descriptions,
  Empty,
  Input,
  Modal,
  Radio,
  Result,
  Skeleton,
  Space,
  Spin,
  Table,
  Tabs,
  Typography,
  type TableColumnsType,
} from 'antd';
import {
  ArrowLeftOutlined,
  CloudDownloadOutlined,
  CodeOutlined,
  KeyOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
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
  IssuedToken,
  TokenDTO,
  TokenKind,
} from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { ClusterStatusTag, TokenKindTag, TokenStatusTag } from '@/components/StatusTag';
import { CopyButton } from '@/components/CopyButton';
import { fmtTime } from '@/utils/format';

function OverviewTab({ cluster }: { cluster: Cluster }) {
  const [info, setInfo] = useState<ClusterInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadInfo = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setInfo(await getClusterInfo(cluster.id));
    } catch (err) {
      setInfo(null);
      setError(err instanceof Error ? err.message : '读取集群信息失败');
    } finally {
      setLoading(false);
    }
  }, [cluster.id]);

  useEffect(() => {
    void loadInfo();
  }, [loadInfo]);

  return (
    <div className="tab-stack">
      <Card title="集群信息" className="soft-card">
        <Descriptions column={{ xs: 1, md: 2 }} size="middle">
          <Descriptions.Item label="集群 ID">
            <code className="mono strong">{cluster.id}</code>
          </Descriptions.Item>
          <Descriptions.Item label="名称">{cluster.name}</Descriptions.Item>
          <Descriptions.Item label="接入状态">
            <ClusterStatusTag status={cluster.status} />
          </Descriptions.Item>
          <Descriptions.Item label="描述">{cluster.desc || '—'}</Descriptions.Item>
          <Descriptions.Item label="注册时间">{fmtTime(cluster.created_at)}</Descriptions.Item>
          <Descriptions.Item label="更新时间">{fmtTime(cluster.updated_at)}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card
        title="集群详细信息"
        className="soft-card"
        extra={
          <Button icon={<ReloadOutlined />} size="small" loading={loading} onClick={() => void loadInfo()}>
            刷新
          </Button>
        }
      >
        {loading ? (
          <Skeleton active paragraph={{ rows: 4 }} />
        ) : error || !info ? (
          <Alert
            type="warning"
            showIcon
            message="无法读取集群信息"
            description="集群未接入或 Agent 隧道离线；接入 Agent 后即可读取集群详细信息。"
          />
        ) : (
          <Descriptions column={{ xs: 1, md: 2 }} size="middle">
            <Descriptions.Item label="Kubernetes 版本" span={2}>
              <code className="mono strong">{info.version?.git_version || '—'}</code>
            </Descriptions.Item>
            <Descriptions.Item label="控制平面" span={2}>
              {info.control_plane ? (
                <code className="mono">{info.control_plane}</code>
              ) : (
                <Typography.Text type="secondary">—</Typography.Text>
              )}
            </Descriptions.Item>
            <Descriptions.Item label="CoreDNS" span={2}>
              {info.coredns ? (
                <code className="mono">{info.coredns}</code>
              ) : (
                <Typography.Text type="secondary">—</Typography.Text>
              )}
            </Descriptions.Item>
            <Descriptions.Item label="apiserver Go 版本">
              <code className="mono">{info.version?.go_version || '—'}</code>
            </Descriptions.Item>
            <Descriptions.Item label="采集时间">{fmtTime(info.fetched_at)}</Descriptions.Item>
          </Descriptions>
        )}
      </Card>
    </div>
  );
}

function TokensTab({ clusterId }: { clusterId: string }) {
  const { message } = App.useApp();
  const [tokens, setTokens] = useState<TokenDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [issuing, setIssuing] = useState<TokenKind | null>(null);
  const [issued, setIssued] = useState<IssuedToken | null>(null);
  const [revoking, setRevoking] = useState<TokenKind | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setTokens(await listTokens(clusterId));
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载 Token 列表失败');
    } finally {
      setLoading(false);
    }
  }, [clusterId]);

  useEffect(() => {
    void load();
  }, [load]);

  const doIssue = async (kind: TokenKind) => {
    setIssuing(kind);
    try {
      const result = await issueToken(clusterId, kind);
      setIssued(result);
      message.success(kind === 'register' ? '注册 Token 已签发' : '长期 Token 已签发');
      await load();
    } catch (err) {
      message.error(err instanceof Error ? err.message : '签发 Token 失败');
    } finally {
      setIssuing(null);
    }
  };

  const confirmIssue = (kind: TokenKind) => {
    if (kind === 'register') {
      void doIssue(kind);
      return;
    }
    Modal.confirm({
      title: '签发 / 轮换长期 Token？',
      content:
        '若已存在长期 Token，本次签发会将其轮换：旧 Token 进入 24 小时宽限期，宽限期内仍可建立隧道。',
      okText: '签发',
      cancelText: '取消',
      onOk: () => doIssue(kind),
    });
  };

  const confirmRevoke = (kind: TokenKind) => {
    Modal.confirm({
      title: `吊销${kind === 'register' ? '注册' : '长期'} Token？`,
      content: '吊销后立即失效，无宽限期；Agent 将无法再建立隧道。',
      okText: '吊销',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        setRevoking(kind);
        try {
          await revokeToken(clusterId, kind);
          message.success('Token 已吊销');
          await load();
        } catch (err) {
          message.error(err instanceof Error ? err.message : '吊销 Token 失败');
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
      render: (kind: TokenKind) => <TokenKindTag kind={kind} />,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 110,
      render: (status: TokenDTO['status']) => <TokenStatusTag status={status} />,
    },
    {
      title: '过期时间',
      dataIndex: 'expires_at',
      key: 'expires_at',
      width: 190,
      render: (value?: string) =>
        value ? fmtTime(value) : <Typography.Text type="secondary">长期有效</Typography.Text>,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 190,
      render: (value: string) => fmtTime(value),
    },
    {
      title: '操作',
      key: 'actions',
      width: 110,
      render: (_, record) => (
        <Button
          type="link"
          size="small"
          danger
          loading={revoking === record.kind}
          onClick={() => confirmRevoke(record.kind)}
        >
          吊销
        </Button>
      ),
    },
  ];

  return (
    <div className="tab-stack">
      <Card
        className="soft-card"
        title="Agent Token"
        extra={
          <Space>
            <Button
              icon={<KeyOutlined />}
              loading={issuing === 'register'}
              onClick={() => confirmIssue('register')}
            >
              签发注册 Token（1h 一次性）
            </Button>
            <Button
              type="primary"
              icon={<KeyOutlined />}
              loading={issuing === 'longterm'}
              onClick={() => confirmIssue('longterm')}
            >
              签发 / 轮换长期 Token
            </Button>
          </Space>
        }
      >
        {error ? (
          <Alert
            type="error"
            showIcon
            message="Token 列表加载失败"
            description={error}
            action={
              <Button size="small" onClick={() => void load()}>
                重试
              </Button>
            }
            style={{ marginBottom: 16 }}
          />
        ) : null}
        <Table<TokenDTO>
          rowKey={(record) => record.kind}
          columns={columns}
          dataSource={tokens}
          loading={loading}
          pagination={false}
          locale={{ emptyText: <Empty description="尚未签发 Agent Token" /> }}
          scroll={{ x: 760 }}
        />
      </Card>

      <Modal
        title="Token 已签发（仅此一次）"
        open={issued !== null}
        onCancel={() => setIssued(null)}
        footer={
          <Button type="primary" onClick={() => setIssued(null)}>
            我已保存
          </Button>
        }
        width={680}
      >
        {issued ? (
          <div className="token-issued">
            <Alert
              type="warning"
              showIcon
              message="关闭此窗口后将无法再次查看明文 Token"
              description="请立即复制并妥善保存；平台只保存 SHA256 哈希。"
              style={{ marginBottom: 16 }}
            />
            <div className="token-block">
              <Input.TextArea
                value={issued.token}
                readOnly
                autoSize={{ minRows: 2, maxRows: 4 }}
                className="mono"
              />
              <CopyButton text={issued.token} label="复制 Token" type="primary" />
            </div>
            {issued.kind === 'register' ? (
              <Typography.Paragraph type="secondary" style={{ marginTop: 16, marginBottom: 0 }}>
                注册 Token 1 小时内有效且只能使用一次：Agent 调用{' '}
                <code className="mono">POST /api/v1/agent/register</code> 换取长期 Token 后即被删除。
              </Typography.Paragraph>
            ) : null}
            {issued.grace_ends_at ? (
              <Alert
                type="info"
                showIcon
                style={{ marginTop: 16 }}
                message={`旧长期 Token 宽限期至 ${fmtTime(issued.grace_ends_at)}`}
                description="宽限期内旧 Token 仍可用于建立隧道；到期后自动失效。"
              />
            ) : null}
          </div>
        ) : null}
      </Modal>
    </div>
  );
}

function KubeconfigTab({ clusterId }: { clusterId: string }) {
  const { message } = App.useApp();
  const [mode, setMode] = useState<'exec' | 'static'>('exec');
  const [downloading, setDownloading] = useState(false);

  const download = async () => {
    setDownloading(true);
    try {
      await downloadKubeconfig(clusterId, mode);
      message.success(`集群 ${clusterId} 的 kubeconfig 已下载`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '下载 kubeconfig 失败');
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div className="tab-stack">
      <Card title="kubeconfig 下载" className="soft-card">
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Radio.Group value={mode} onChange={(e) => setMode(e.target.value as 'exec' | 'static')}>
            <Radio.Button value="exec">exec（推荐）</Radio.Button>
            <Radio.Button value="static">static</Radio.Button>
          </Radio.Group>
          <Typography.Text type="secondary">
            {mode === 'exec'
              ? '写入 k8s-am-login 命令行，凭证自动续期；需要本机 PATH 中存在 k8s-am-login。'
              : '直接嵌入当前 JWT，1 小时后过期，需要重新下载。'}
          </Typography.Text>
          <Button
            type="primary"
            icon={<CloudDownloadOutlined />}
            loading={downloading}
            onClick={() => void download()}
          >
            下载 kubeconfig
          </Button>
        </Space>
      </Card>
    </div>
  );
}

function AgentInstallTab({ clusterId }: { clusterId: string }) {
  const { message, modal } = App.useApp();
  const [preview, setPreview] = useState<AgentInstallPreview | null>(null);
  const [generating, setGenerating] = useState(false);

  const generate = () => {
    modal.confirm({
      title: '生成一键安装命令？',
      content:
        '将签发/轮换该集群的长期 Agent Token（旧 Token 进入 24 小时宽限期），并生成包含 Token 的安装清单。明文 Token 仅此一次展示。',
      okText: '生成',
      cancelText: '取消',
      onOk: async () => {
        setGenerating(true);
        try {
          const issued = await issueToken(clusterId, 'longterm');
          const result = await createAgentInstallPreview(clusterId, issued.token);
          setPreview(result);
          message.success('安装命令已生成');
        } catch (err) {
          message.error(err instanceof Error ? err.message : '生成安装命令失败');
        } finally {
          setGenerating(false);
        }
      },
    });
  };

  return (
    <div className="tab-stack">
      <Card title="一键安装 Agent" className="soft-card">
        <Typography.Paragraph>
          在目标集群执行下面的命令即可完成 Agent 部署（Namespace + RBAC + 双副本 Deployment，
          Token 经 Secret 挂载）。Agent 通过反向隧道上线后，集群接入状态变为「在线」。
        </Typography.Paragraph>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 16 }}>
          镜像仓库由平台启动配置 <code className="mono">AM_AGENT_IMAGE_REPO</code>{' '}
          决定（未设置默认走 Docker Hub），tag 与平台版本一致。
        </Typography.Paragraph>

        {preview ? (
          <div className="agent-install">
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 16 }}
              message="安装命令内含 Agent Token，等同集群接入凭证，请勿外泄"
              description="kubectl 会从平台拉取安装清单；清单中的 Token 已写入目标集群 Secret，可在 Agent Token 页签吊销。"
            />
            <Descriptions column={1} size="middle" style={{ marginBottom: 16 }}>
              <Descriptions.Item label="Agent 镜像">
                <code className="mono strong">{preview.image}</code>
              </Descriptions.Item>
              <Descriptions.Item label="隧道入口">
                <code className="mono">{preview.tunnel_url}</code>
              </Descriptions.Item>
            </Descriptions>
            <Typography.Paragraph strong style={{ marginBottom: 8 }}>
              安装命令（在目标集群执行）
            </Typography.Paragraph>
            <div className="token-block">
              <pre className="code-block" style={{ marginBottom: 8 }}>{preview.apply_command}</pre>
              <CopyButton text={preview.apply_command} label="复制安装命令" type="primary" />
            </div>
            <Typography.Paragraph strong style={{ margin: '16px 0 8px' }}>
              安装清单预览
            </Typography.Paragraph>
            <div className="token-block">
              <pre className="code-block" style={{ maxHeight: 360, overflow: 'auto', marginBottom: 8 }}>{preview.yaml}</pre>
              <CopyButton text={preview.yaml} label="复制 YAML" />
            </div>
            <Button
              style={{ marginTop: 16 }}
              loading={generating}
              onClick={generate}
            >
              重新生成（轮换 Token）
            </Button>
          </div>
        ) : (
          <Empty
            description="尚未生成本集群的安装命令"
            style={{ padding: '16px 0' }}
          >
            <Button type="primary" icon={<CodeOutlined />} loading={generating} onClick={generate}>
              生成安装命令
            </Button>
          </Empty>
        )}
      </Card>
    </div>
  );
}

export function ClusterDetail() {
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [cluster, setCluster] = useState<Cluster | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    getCluster(id)
      .then((data) => {
        if (!cancelled) setCluster(data);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : '加载集群失败');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [id]);

  if (loading) {
    return (
      <div className="full-center">
        <Spin size="large" tip="正在加载集群…">
          <div style={{ padding: 48 }} />
        </Spin>
      </div>
    );
  }

  if (error || !cluster) {
    return (
      <Result
        status="404"
        title="集群不存在或加载失败"
        subTitle={error ?? `未找到集群 ${id}`}
        extra={
          <Button type="primary" onClick={() => navigate('/clusters')}>
            返回集群列表
          </Button>
        }
      />
    );
  }

  return (
    <div>
      <PageHeader
        title={cluster.name}
        subtitle={
          <>
            集群 ID：<code className="mono strong">{cluster.id}</code>
          </>
        }
        tags={<ClusterStatusTag status={cluster.status} />}
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={() => window.location.reload()}>
              刷新
            </Button>
            <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/clusters')}>
              返回列表
            </Button>
          </Space>
        }
      />

      <Tabs
        defaultActiveKey="overview"
        items={[
          {
            key: 'overview',
            label: '概况',
            children: <OverviewTab cluster={cluster} />,
          },
          {
            key: 'tokens',
            label: 'Agent Token',
            children: <TokensTab clusterId={cluster.id} />,
          },
          {
            key: 'agent-install',
            label: 'Agent 安装',
            children: <AgentInstallTab clusterId={cluster.id} />,
          },
          {
            key: 'kubeconfig',
            label: 'kubeconfig',
            children: <KubeconfigTab clusterId={cluster.id} />,
          },
        ]}
      />
    </div>
  );
}
