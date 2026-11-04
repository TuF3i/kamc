import { apiGet, queryString } from './http';
import type { AuditEvent } from './types';

export interface AuditQuery {
  kind?: 'platform' | 'cluster';
  user?: string;
  cluster?: string;
  /** 事件名精确匹配（如 k8s-api） */
  event?: string;
  limit?: number;
}

export function listAudit(query: AuditQuery = {}): Promise<AuditEvent[]> {
  return apiGet<AuditEvent[]>(
    `/api/v1/audit${queryString({
      kind: query.kind,
      user: query.user,
      cluster: query.cluster,
      event: query.event,
      limit: query.limit,
    })}`,
  );
}
