import { For, Show, createSignal, onMount } from 'solid-js';
import { downloadKubeconfig, listMyClusters } from '@/api/clusters';
import type { Cluster } from '@/api/types';
import { auth } from '@/stores/auth';
import { toast } from '@/lib/storage/toast';
import {
  Alert,
  Button,
  Card,
  Drawer,
  Empty,
  Skeleton,
  Tag,
} from '@/lib/widgets';
import { PageHeader } from '@/components/PageHeader';
import { ClusterStatusTag } from '@/components/StatusTag';
import {
  ArrowRightOutlined,
  CloudDownloadOutlined,
  KeyOutlined,
  KubernetesOutlined,
  ThunderboltOutlined,
} from '@/lib/icons';

export default function Kubeconfig() {
  const [clusters, setClusters] = createSignal<Cluster[]>([]);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);
  const [selected, setSelected] = createSignal<Cluster | null>(null);
  const [mode, setMode] = createSignal<'exec' | 'static'>('exec');
  const [downloading, setDownloading] = createSignal(false);

  onMount(() => {
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
  });

  const openCluster = (cluster: Cluster) => {
    setSelected(cluster);
    setMode('exec');
  };

  const download = async () => {
    const cluster = selected();
    if (!cluster) return;
    setDownloading(true);
    try {
      await downloadKubeconfig(cluster.id, mode());
      toast.success(`集群 ${cluster.id} 的 kubeconfig 已下载`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '下载 kubeconfig 失败');
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div>
      <PageHeader
        title="Kubeconfig"
        subtitle="下载访问凭证"
        tags={
          auth.isAdmin() ? (
            <Tag level="primary">管理员</Tag>
          ) : (
            <Tag>普通用户</Tag>
          )
        }
      />

      <Show when={error()}>
        <Alert
          type="error"
          message="集群列表加载失败"
          description={error() ?? undefined}
          class="mb-4"
          action={
            <Button size="small" onClick={() => window.location.reload()}>
              重试
            </Button>
          }
        />
      </Show>

      <Show
        when={!loading()}
        fallback={
          <div class="kubeconfig-grid">
            <For each={[0, 1, 2]}>
              {() => (
                <Card class="kubeconfig-cluster-card">
                  <Skeleton avatar paragraph={{ rows: 2 }} />
                </Card>
              )}
            </For>
          </div>
        }
      >
        <Show
          when={clusters().length > 0}
          fallback={
            <Card class="soft-card">
              <Empty
                description={
                  auth.isAdmin()
                    ? '暂无集群；请先在「集群」页添加集群并分配访问策略。'
                    : '暂无可访问的集群，请联系管理员分配访问策略。'
                }
                class="border-0"
              />
            </Card>
          }
        >
          <div class="kubeconfig-grid">
            <For each={clusters()}>
              {(cluster) => (
                <button
                  type="button"
                  class="kubeconfig-cluster-card"
                  onClick={() => openCluster(cluster)}
                >
                  <div class="kubeconfig-cluster-head">
                    <span class="kubeconfig-cluster-icon">
                      <KubernetesOutlined class="h-[23px] w-[23px]" />
                    </span>
                    <ClusterStatusTag status={cluster.status} />
                  </div>
                  <div class="kubeconfig-cluster-name">{cluster.name || cluster.id}</div>
                  <code class="mono kubeconfig-cluster-id">{cluster.id}</code>
                  <div class="kubeconfig-cluster-desc">{cluster.desc || '暂无描述'}</div>
                  <span class="kubeconfig-cluster-action">
                    下载凭证 <ArrowRightOutlined class="h-4 w-4" />
                  </span>
                </button>
              )}
            </For>
          </div>
        </Show>
      </Show>

      <Drawer
        open={selected() !== null}
        title={selected() ? `下载凭证 · ${selected()!.name || selected()!.id}` : '下载凭证'}
        onClose={() => setSelected(null)}
        width={460}
      >
        <Show when={selected()}>
          {(cluster) => (
            <>
              <div class="kubeconfig-drawer-head">
                <span class="kubeconfig-cluster-icon">
                  <KubernetesOutlined class="h-[23px] w-[23px]" />
                </span>
                <div class="kubeconfig-drawer-title">
                  <div class="kubeconfig-drawer-name-row">
                    <span class="kubeconfig-cluster-name">
                      {cluster().name || cluster().id}
                    </span>
                    <code class="mono kubeconfig-drawer-id">{cluster().id}</code>
                  </div>
                  <div class="kubeconfig-drawer-desc">{cluster().desc || '暂无描述'}</div>
                </div>
                <ClusterStatusTag status={cluster().status} />
              </div>

              <div class="mini-heading">凭证模式</div>
              <div class="mode-cards">
                <button
                  type="button"
                  class={`mode-card${mode() === 'exec' ? ' selected' : ''}`}
                  onClick={() => setMode('exec')}
                >
                  <div class="mode-card-title">
                    <ThunderboltOutlined class="h-4 w-4" /> exec
                    <span class="mode-badge">推荐</span>
                  </div>
                  <div class="mode-card-desc">调用 kamc 自动续期，适合日常使用。</div>
                </button>
                <button
                  type="button"
                  class={`mode-card${mode() === 'static' ? ' selected' : ''}`}
                  onClick={() => setMode('static')}
                >
                  <div class="mode-card-title">
                    <KeyOutlined class="h-4 w-4" /> static
                  </div>
                  <div class="mode-card-desc">嵌入当前 JWT，1 小时后过期，需要重新下载。</div>
                </button>
              </div>

              <Button
                type="primary"
                size="large"
                block
                icon={<CloudDownloadOutlined class="h-4 w-4" />}
                loading={downloading()}
                onClick={() => void download()}
                class="kubeconfig-submit"
              >
                下载 kubeconfig
              </Button>
            </>
          )}
        </Show>
      </Drawer>
    </div>
  );
}
