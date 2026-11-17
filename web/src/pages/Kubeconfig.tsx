import { useEffect, useState } from 'react';
import { Alert, App, Button, Card, Drawer, Empty, Skeleton, Tag } from 'antd';
import {
  ArrowRightOutlined,
  CloudDownloadOutlined,
  KeyOutlined,
  KubernetesOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useAuthStore } from '@/stores/auth';
import { downloadKubeconfig, listMyClusters } from '@/api/clusters';
import type { Cluster } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { ClusterStatusTag } from '@/components/StatusTag';

export function Kubeconfig() {
  const { message } = App.useApp();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const claims = useAuthStore((s) => s.claims);
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Cluster | null>(null);
  const [mode, setMode] = useState<'exec' | 'static'>('exec');
  const [downloading, setDownloading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    listMyClusters()
      .then((list) => {
        if (!cancelled) setClusters(list);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : '加载可访问集群失败');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const openCluster = (cluster: Cluster) => {
    setSelected(cluster);
    setMode('exec');
  };

  const download = async () => {
    if (!selected) return;
    setDownloading(true);
    try {
      await downloadKubeconfig(selected.id, mode);
      message.success(`集群 ${selected.id} 的 kubeconfig 已下载`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : '下载 kubeconfig 失败');
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div>
      <PageHeader
        title="Kubeconfig"
        subtitle="下载访问凭证"
        tags={isAdmin ? <Tag color="blue">管理员</Tag> : <Tag>普通用户</Tag>}
      />

      {error ? (
        <Alert
          type="error"
          showIcon
          message="集群列表加载失败"
          description={error}
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" onClick={() => window.location.reload()}>
              重试
            </Button>
          }
        />
      ) : null}

      {loading ? (
        <div className="kubeconfig-grid">
          {[0, 1, 2].map((index) => (
            <Card key={index} className="kubeconfig-cluster-card">
              <Skeleton active avatar paragraph={{ rows: 2 }} />
            </Card>
          ))}
        </div>
      ) : clusters.length === 0 ? (
        <Card className="soft-card">
          <Empty
            description={
              isAdmin
                ? '暂无集群；请先在「集群」页添加集群并分配访问策略。'
                : '暂无可访问的集群，请联系管理员分配访问策略。'
            }
            style={{ padding: '40px 0' }}
          />
        </Card>
      ) : (
        <div className="kubeconfig-grid">
          {clusters.map((cluster) => (
            <button
              key={cluster.id}
              type="button"
              className="kubeconfig-cluster-card"
              onClick={() => openCluster(cluster)}
            >
              <div className="kubeconfig-cluster-head">
                <span className="kubeconfig-cluster-icon">
                  <KubernetesOutlined />
                </span>
                <ClusterStatusTag status={cluster.status} />
              </div>
              <div className="kubeconfig-cluster-name">{cluster.name || cluster.id}</div>
              <code className="mono kubeconfig-cluster-id">{cluster.id}</code>
              <div className="kubeconfig-cluster-desc">{cluster.desc || '暂无描述'}</div>
              <span className="kubeconfig-cluster-action">
                下载凭证 <ArrowRightOutlined />
              </span>
            </button>
          ))}
        </div>
      )}

      <Drawer
        title={selected ? `下载凭证 · ${selected.name || selected.id}` : '下载凭证'}
        open={selected !== null}
        onClose={() => setSelected(null)}
        width={460}
        destroyOnHidden
      >
        {selected ? (
          <>
            <div className="kubeconfig-drawer-head">
              <span className="kubeconfig-cluster-icon">
                <KubernetesOutlined />
              </span>
              <div className="kubeconfig-drawer-title">
                <div className="kubeconfig-cluster-name">{selected.name || selected.id}</div>
                <code className="mono">{selected.id}</code>
              </div>
              <ClusterStatusTag status={selected.status} />
            </div>
            <div className="kubeconfig-cluster-desc kubeconfig-drawer-desc">
              {selected.desc || '暂无描述'}
            </div>

            <div className="mini-heading">凭证模式</div>
            <div className="mode-cards">
              <button
                type="button"
                className={`mode-card${mode === 'exec' ? ' selected' : ''}`}
                onClick={() => setMode('exec')}
              >
                <div className="mode-card-title">
                  <ThunderboltOutlined /> exec
                  <span className="mode-badge">推荐</span>
                </div>
                <div className="mode-card-desc">调用 k8s-am-login 自动续期，适合日常使用。</div>
              </button>
              <button
                type="button"
                className={`mode-card${mode === 'static' ? ' selected' : ''}`}
                onClick={() => setMode('static')}
              >
                <div className="mode-card-title">
                  <KeyOutlined /> static
                </div>
                <div className="mode-card-desc">嵌入当前 JWT，1 小时后过期，需要重新下载。</div>
              </button>
            </div>

            <Button
              type="primary"
              size="large"
              block
              icon={<CloudDownloadOutlined />}
              loading={downloading}
              onClick={() => void download()}
              className="kubeconfig-submit"
            >
              下载 kubeconfig
            </Button>

            <div className="kubeconfig-panel-foot">
              <span>凭证只承载你的身份，数据面授权由平台在请求时判定。</span>
              <code className="mono">{claims?.sub}</code>
            </div>
          </>
        ) : null}
      </Drawer>
    </div>
  );
}
