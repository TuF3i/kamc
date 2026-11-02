import { apiGet, queryString } from './http';
import type { AuditEvent } from './types';

export interface AuditQuery {
  user?: string;
  cluster?: string;
  limit?: number;
}

export function listAudit(query: AuditQuery = {}): Promise<AuditEvent[]> {
  return apiGet<AuditEvent[]>(
    `/api/v1/audit${queryString({
      user: query.user,
      cluster: query.cluster,
      limit: query.limit,
    })}`,
  );
}
