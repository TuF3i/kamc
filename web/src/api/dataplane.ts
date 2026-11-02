import { kubeGet } from './http';
import type { K8sList, K8sNamespace, K8sPod } from './types';

export function listNamespaces(clusterId: string): Promise<K8sList<K8sNamespace>> {
  return kubeGet<K8sList<K8sNamespace>>(clusterId, '/api/v1/namespaces?limit=500');
}

export function listPods(clusterId: string, namespace: string): Promise<K8sList<K8sPod>> {
  return kubeGet<K8sList<K8sPod>>(
    clusterId,
    `/api/v1/namespaces/${encodeURIComponent(namespace)}/pods?limit=500`,
  );
}

export function getPod(clusterId: string, namespace: string, pod: string): Promise<K8sPod> {
  return kubeGet<K8sPod>(
    clusterId,
    `/api/v1/namespaces/${encodeURIComponent(namespace)}/pods/${encodeURIComponent(pod)}`,
  );
}
