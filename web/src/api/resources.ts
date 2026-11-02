import { apiGet } from './http';
import type { ClusterResourcesResponse } from './types';

export function listClusterResources(
  clusterId: string,
  refresh = false,
): Promise<ClusterResourcesResponse> {
  const suffix = refresh ? '?refresh=1' : '';
  return apiGet<ClusterResourcesResponse>(
    `/api/v1/clusters/${encodeURIComponent(clusterId)}/resources${suffix}`,
  );
}
