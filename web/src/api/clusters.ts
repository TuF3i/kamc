import { apiDelete, apiGet, apiPost, downloadFile } from './http';
import type {
  AgentInstallPreview,
  Cluster,
  ClusterInfo,
  IssuedToken,
  TokenDTO,
  TokenKind,
  VersionInfo,
} from './types';

export function listClusters(): Promise<Cluster[]> {
  return apiGet<Cluster[]>('/api/v1/clusters');
}

export function getCluster(id: string): Promise<Cluster> {
  return apiGet<Cluster>(`/api/v1/clusters/${encodeURIComponent(id)}`);
}

export interface CreateClusterInput {
  id: string;
  name: string;
  desc?: string;
}

export function createCluster(input: CreateClusterInput): Promise<Cluster> {
  return apiPost<Cluster>('/api/v1/clusters', input);
}

export function deleteCluster(id: string): Promise<void> {
  return apiDelete<void>(`/api/v1/clusters/${encodeURIComponent(id)}`);
}

/** 集群概要信息（kubectl cluster-info 语义；集群离线时返回 20003）。 */
export function getClusterInfo(id: string): Promise<ClusterInfo> {
  return apiGet<ClusterInfo>(`/api/v1/clusters/${encodeURIComponent(id)}/info`);
}

/**
 * Agent 一键安装预览：token 必须是刚签发的长期 Token 明文
 * （平台只存哈希，签发窗口之外无法再次生成）。
 */
export function createAgentInstallPreview(
  clusterId: string,
  token: string,
): Promise<AgentInstallPreview> {
  return apiPost<AgentInstallPreview>(
    `/api/v1/clusters/${encodeURIComponent(clusterId)}/agent-install`,
    { token },
  );
}

export function listTokens(clusterId: string): Promise<TokenDTO[]> {
  return apiGet<TokenDTO[]>(`/api/v1/clusters/${encodeURIComponent(clusterId)}/tokens`);
}

export function issueToken(clusterId: string, kind: TokenKind): Promise<IssuedToken> {
  return apiPost<IssuedToken>(`/api/v1/clusters/${encodeURIComponent(clusterId)}/tokens`, { kind });
}

export function revokeToken(clusterId: string, kind: TokenKind): Promise<void> {
  return apiDelete<void>(
    `/api/v1/clusters/${encodeURIComponent(clusterId)}/tokens/${encodeURIComponent(kind)}`,
  );
}

/** kubeconfig 必须带 Authorization 头 fetch 后以 Blob 下载。 */
export function downloadKubeconfig(clusterId: string, mode: 'exec' | 'static'): Promise<void> {
  return downloadFile(
    `/api/v1/clusters/${encodeURIComponent(clusterId)}/kubeconfig?mode=${mode}`,
    `kubeconfig-${clusterId}.yaml`,
  );
}

export function fetchVersion(): Promise<VersionInfo> {
  return apiGet<VersionInfo>('/version');
}
