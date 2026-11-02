// kamc 平台 API 类型（契约见 api/openapi.yaml）

/** 响应信封（模式 B：HTTP 恒 200，业务码在 status 字段） */
export interface Envelopeish<T = unknown> {
  status: number;
  message: string;
  data?: T;
}

export interface Cluster {
  id: string;
  name: string;
  desc?: string;
  status: 'pending' | 'online' | 'offline';
  created_at: string;
  updated_at: string;
}

export interface CreateClusterRequest {
  id: string;
  name: string;
  desc?: string;
}

export interface TokenDTO {
  cluster_id: string;
  kind: 'register' | 'longterm';
  status: 'active' | 'rotating';
  expires_at: string | null;
  created_at: string;
}

export interface IssuedToken {
  token: string;
  kind: 'register' | 'longterm';
  grace_ends_at?: string;
}

export interface IssuedCredential {
  token: string;
  expires_at: string;
  token_type: string;
}

export interface Claims {
  iss: string;
  sub: string;
  aud: string[];
  exp: number;
  iat: number;
  jti: string;
  email?: string;
  groups?: string[];
  am_ep?: number;
}

/** [sub, dom, obj, act]；act 为 K8s verb 正则片段（锚定匹配） */
export type PolicyRow = [string, string, string, string];

/** [user, role, dom] */
export type BindingRow = [string, string, string];

export interface AddPolicyRequest {
  sub: string;
  dom: string;
  obj: string;
  act: string;
}

export interface AddBindingRequest {
  user: string;
  role: string;
  dom: string;
}

export interface AuditEvent {
  id: number;
  timestamp: string;
  user: string;
  groups?: string;
  cluster: string;
  method: string;
  path: string;
  verb: string;
  allowed: boolean;
  reason: string;
  source_ip?: string;
  latency_ms?: number;
}

export interface AuthScript {
  name: string;
  content: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface ScriptExecuteResult {
  dry_run: boolean;
  actions?: { api: string; args: string[] }[];
}

export interface VersionInfo {
  version: string;
  commit: string;
  component: string;
}
