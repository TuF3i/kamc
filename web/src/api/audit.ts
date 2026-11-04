import { apiGet, queryString } from './http';
import type { AuditEvent } from './types';

export interface AuditQuery {
  kind?: 'platform' | 'cluster';
  user?: string;
  cluster?: string;
  /** 事件名精确匹配（如 k8s-api） */
  event?: string;
  /** RFC3339 时间范围 */
  from?: string;
  to?: string;
  allowed?: boolean;
  /** 不返回报文正文，仅返回长度（列表/概览用） */
  summary?: boolean;
  limit?: number;
}

/** 单条审计详情（含完整请求/响应报文）。 */
export function getAudit(id: number): Promise<AuditEvent> {
  return apiGet<AuditEvent>(`/api/v1/audit/${id}`);
}

export function listAudit(query: AuditQuery = {}): Promise<AuditEvent[]> {
  return apiGet<AuditEvent[]>(
    `/api/v1/audit${queryString({
      kind: query.kind,
      user: query.user,
      cluster: query.cluster,
      event: query.event,
      from: query.from,
      to: query.to,
      allowed: query.allowed === undefined ? undefined : query.allowed ? 1 : 0,
      summary: query.summary ? 1 : undefined,
      limit: query.limit,
    })}`,
  );
}
