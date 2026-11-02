import { api, apiRaw } from './http';
import type { Cluster, CreateClusterRequest, IssuedToken, TokenDTO } from './types';

// —— 集群（管理面） ——

export function listClusters(): Promise<Cluster[]> {
  return api('/api/v1/clusters');
}

export function getCluster(id: string): Promise<Cluster> {
  return api(`/api/v1/clusters/${encodeURIComponent(id)}`);
}

export function createCluster(req: CreateClusterRequest): Promise<Cluster> {
  return api('/api/v1/clusters', { body: req });
}

export function deleteCluster(id: string): Promise<void> {
  return api(`/api/v1/clusters/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

// —— Agent Token（管理面） ——

export function listTokens(clusterId: string): Promise<TokenDTO[]> {
  return api(`/api/v1/clusters/${encodeURIComponent(clusterId)}/tokens`);
}

/** 签发 Token：kind=longterm 轮换（旧 Token 24h 宽限）/ kind=register（1h 一次性） */
export function issueToken(clusterId: string, kind: 'register' | 'longterm'): Promise<IssuedToken> {
  return api(`/api/v1/clusters/${encodeURIComponent(clusterId)}/tokens`, { body: { kind } });
}

export function revokeToken(clusterId: string, kind: 'register' | 'longterm'): Promise<void> {
  return api(`/api/v1/clusters/${encodeURIComponent(clusterId)}/tokens/${kind}`, { method: 'DELETE' });
}

// —— kubeconfig（自服务） ——

/** 下载 kubeconfig YAML（mode=exec 推荐：写 k8s-am-login 命令行，不嵌过期凭证） */
export async function downloadKubeconfig(clusterId: string, mode: 'static' | 'exec'): Promise<void> {
  const resp = await apiRaw(`/api/v1/clusters/${encodeURIComponent(clusterId)}/kubeconfig?mode=${mode}`);
  const blob = await resp.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = resp.headers.get('content-disposition')?.match(/filename="?([^";]+)"?/)?.[1] ?? `${clusterId}-kubeconfig.yaml`;
  a.click();
  URL.revokeObjectURL(url);
}
