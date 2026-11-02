// 平台 API 数据类型：字段与 internal/platform/modules 的 JSON tag 保持一致。

export interface Envelope<T> {
  status: number;
  message: string;
  data?: T;
}

export type ClusterStatus = 'pending' | 'online' | 'offline';

export interface Cluster {
  id: string;
  name: string;
  desc: string;
  status: ClusterStatus;
  created_at: string;
  updated_at: string;
}

export type TokenKind = 'register' | 'longterm';
export type TokenStatus = 'active' | 'rotating';

export interface TokenDTO {
  cluster_id: string;
  kind: TokenKind;
  status: TokenStatus;
  /** 注册 Token 有值；长期 Token 通常省略该字段 */
  expires_at?: string;
  created_at: string;
}

export interface IssuedToken {
  token: string;
  kind: TokenKind;
  /** 长期 Token 轮换时，旧 Token 的宽限期截止时间 */
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

export interface AuditEvent {
  id: number;
  timestamp: string;
  user: string;
  /** 逗号分隔的组字符串（不是 JSON 数组） */
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

export interface PolicyDTO {
  subject_type: 'org' | 'user' | 'legacy';
  subject: string;
  cluster: string;
  namespace: string;
  resource: string;
  subresource?: string;
  verbs: string[];
  sub: string;
  dom: string;
  obj: string;
  act: string;
}

export interface PolicyRuleInput {
  namespace: string;
  resource: string;
  subresource?: string;
  namespaced: boolean;
  verbs: string[];
}

export interface CreatePolicyInput {
  subject_type: 'org' | 'user';
  subject: string;
  cluster: string;
  rules: PolicyRuleInput[];
}

export interface CreatePolicyResult {
  created: number;
  policies: PolicyDTO[];
}

export interface DirectoryOption {
  value: string;
  label: string;
}

export interface ClusterResource {
  group: string;
  version: string;
  resource: string;
  subresource?: string;
  gvr: string;
  kind: string;
  namespaced: boolean;
  verbs: string[];
}

export interface ClusterResourcesResponse {
  cluster_id: string;
  namespaces: string[];
  resources: ClusterResource[];
  partial: boolean;
  failed_group_versions?: string[];
  cached: boolean;
  discovered_at: string;
}

export interface VersionInfo {
  version: string;
  commit: string;
  component: string;
}

export type RobotStatus = 'active' | 'expired';

export interface Robot {
  id: string;
  name: string;
  cluster_id: string;
  sub: string;
  status: RobotStatus;
  issued_at: string;
  expires_at: string;
  created_at: string;
}

export interface CreateRobotResponse {
  robot: Robot;
  kubeconfig: string;
}

export interface SystemInfo {
  go_version: string;
  goroutines: number;
  num_cpu: number;
  heap_alloc_bytes: number;
  heap_sys_bytes: number;
  sys_bytes: number;
  total_alloc_bytes: number;
  num_gc: number;
  uptime_seconds: number;
  db_open_connections: number;
  db_in_use: number;
  db_idle: number;
  db_wait_count: number;
  db_max_open_connections: number;
}

export interface K8sList<T> {
  items: T[];
}

export interface K8sObjectMeta {
  name: string;
  namespace?: string;
}

export interface K8sNamespace {
  metadata: K8sObjectMeta;
}

export interface K8sPod {
  metadata: K8sObjectMeta;
  spec?: {
    containers?: { name: string }[];
  };
  status?: {
    phase?: string;
  };
}
