import { api } from './http';
import type { AuditEvent } from './types';

// —— 审计查询（管理面） ——

export interface AuditFilter {
  user?: string;
  cluster?: string;
  limit?: number;
}

export function listAudit(f: AuditFilter = {}): Promise<AuditEvent[]> {
  const q = new URLSearchParams();
  if (f.user) q.set('user', f.user);
  if (f.cluster) q.set('cluster', f.cluster);
  q.set('limit', String(f.limit ?? 200));
  const qs = q.toString();
  return api(`/api/v1/audit${qs ? '?' + qs : ''}`);
}
