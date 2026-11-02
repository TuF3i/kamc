import { useEffect, useState } from 'react';
import { App, Button, Form, Input, Select, Tag } from 'antd';
import { CloudDownloadOutlined, KeyOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { useAuthStore } from '@/stores/auth';
import { downloadKubeconfig, listClusters } from '@/api/clusters';
import type { Cluster } from '@/api/types';

interface DownloadForm {
  cluster: string;
}

export function Kubeconfig() {
  const { message } = App.useApp();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const claims = useAuthStore((s) => s.claims);
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [mode, setMode] = useState<'exec' | 'static'>('exec');
  const [downloading, setDownloading] = useState(false);
  const [form] = Form.useForm<DownloadForm>();

  useEffect(() => {
    if (!isAdmin) return;
    listClusters()
      .then((list) => {
        setClusters(list);
        if (list.length > 0) form.setFieldValue('cluster', list[0].id);
      })
      .catch(() => {
        // 管理员列表加载失败时退化为手动输入
      });
  }, [isAdmin, form]);

  const download = async (values: DownloadForm) => {
    const clusterId = values.cluster?.trim();
    if (!clusterId) {
      message.warning('请选择或输入集群 ID');
      return;
    }
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
    <div className="kubeconfig-page">
      <div className="kubeconfig-hero">
        <div className="kubeconfig-kicker">KUBECONFIG</div>
        <h1>获取集群访问凭证</h1>
        <p>选择目标集群与凭证模式，下载标准 kubeconfig。</p>
        {isAdmin ? <Tag color="blue">管理员</Tag> : <Tag>普通用户</Tag>}
      </div>

      <div className="kubeconfig-panel">
        <Form<DownloadForm> form={form} layout="vertical" onFinish={download}>
          <Form.Item
            name="cluster"
            label="目标集群"
            rules={[{ required: true, message: '请选择或输入集群 ID' }]}
            extra={isAdmin ? undefined : '集群 ID 由管理员提供'}
          >
            {isAdmin ? (
              <Select
                showSearch
                placeholder="选择集群"
                options={clusters.map((c) => ({
                  value: c.id,
                  label: `${c.id} · ${c.name}`,
                }))}
                optionFilterProp="label"
              />
            ) : (
              <Input placeholder="输入管理员提供的集群 ID" className="mono" allowClear />
            )}
          </Form.Item>

          <Form.Item label="凭证模式">
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
                <div className="mode-card-desc">
                  调用 k8s-am-login 自动续期，适合日常使用。
                </div>
              </button>
              <button
                type="button"
                className={`mode-card${mode === 'static' ? ' selected' : ''}`}
                onClick={() => setMode('static')}
              >
                <div className="mode-card-title">
                  <KeyOutlined /> static
                </div>
                <div className="mode-card-desc">
                  嵌入当前 JWT，1 小时后过期，需要重新下载。
                </div>
              </button>
            </div>
          </Form.Item>

          <Button
            type="primary"
            size="large"
            block
            icon={<CloudDownloadOutlined />}
            htmlType="submit"
            loading={downloading}
            className="kubeconfig-submit"
          >
            下载 kubeconfig
          </Button>
        </Form>

        <div className="kubeconfig-panel-foot">
          <span>凭证只承载你的身份，数据面授权由平台在请求时判定。</span>
          <code className="mono">{claims?.sub}</code>
        </div>
      </div>
    </div>
  );
}
