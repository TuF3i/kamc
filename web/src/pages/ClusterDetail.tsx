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
  KeyOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import {
  downloadKubeconfig,
  getCluster,
  issueToken,
  listTokens,
  revokeToken,
} from '@/api/clusters';
import type { Cluster, IssuedToken, TokenDTO, TokenKind } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { ClusterStatusTag, TokenKindTag, TokenStatusTag } from '@/components/StatusTag';
import { CopyButton } from '@/components/CopyButton';
import { ExecTerminal } from '@/components/ExecTerminal';
import { fmtTime } from '@/utils/format';

function OverviewTab({ cluster }: { cluster: Cluster }) {
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

      <Card title="接入说明" className="soft-card">
        <Typography.Paragraph>
          Agent 使用注册 Token 调用 <code className="mono">POST /api/v1/agent/register</code>{' '}
          一次性换取长期 Token，随后通过 <code className="mono">/tunnel</code> WebSocket 建立反向隧道上线。
        </Typography.Paragraph>
        <Typography.Paragraph>
          数据面请求由平台经隧道转发，并在转发前完成 Casbin 鉴权与身份注入；Agent 只负责字节转发，
          不做权限判断。
        </Typography.Paragraph>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          部署清单见仓库 <code className="mono">deploy/</code> 目录。
        </Typography.Paragraph>
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
        <Typography.Paragraph>
          kubeconfig 只承载当前登录用户身份；数据面权限仍由平台 Casbin 在每次请求时判定。
          server 指向 <code className="mono">/proxy/{clusterId}</code>。
        </Typography.Paragraph>
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

      <Card title="使用方式" className="soft-card">
        <pre className="code-block">{`kubectl --kubeconfig ./kubeconfig-${clusterId}.yaml get pods -n dev`}</pre>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          exec 模式的凭证缓存于 <code className="mono">~/.config/kamc/login-cache.json</code>；
          static 模式在 JWT 过期后需重新下载。
        </Typography.Paragraph>
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
            key: 'terminal',
            label: '终端',
            children: <ExecTerminal clusterId={cluster.id} />,
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
