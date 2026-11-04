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

export type AuditKind = 'platform' | 'cluster';

export interface AuditEvent {
  id: number;
  kind: AuditKind;
  /** 事件名：login-success / kubeconfig-download / token-issue / k8s-api … */
  event: string;
  timestamp: string;
  user: string;
  /** 逗号分隔的组字符串（不是 JSON 数组） */
  groups?: string;
  cluster: string;
  method: string;
  /** 原始请求 URI（含 query） */
  path: string;
  /** Casbin 语义资源（<ns>/<resource> 等） */
  object?: string;
  verb: string;
  allowed: boolean;
  reason: string;
  source_ip?: string;
  forwarded_for?: string;
  user_agent?: string;
  geo_country?: string;
  geo_region?: string;
  geo_city?: string;
  status_code?: number;
  request_body?: string;
  response_body?: string;
  latency_ms?: number;
}

export interface PolicyDTO {
  subject_type: 'org' | 'user' | 'robot' | 'legacy';
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
  subject_type: 'org' | 'user' | 'robot';
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

/** 集群节点概要（kubectl get nodes -o wide 子集） */
export interface ClusterNodeInfo {
  name: string;
  roles?: string[];
  ready: boolean;
  unschedulable?: boolean;
  internal_ip?: string;
  kubelet_version?: string;
  os_image?: string;
  architecture?: string;
  runtime?: string;
  cpu?: string;
  memory?: string;
}

/** 目标集群概要（kubectl cluster-info 语义，经 Agent 隧道读取） */
export interface ClusterInfo {
  version: ClusterVersionInfo | null;
  /** 控制平面地址（https://ip:port，读不到为空） */
  control_plane?: string;
  /** CoreDNS 地址（clusterIP:port，读不到为空） */
  coredns?: string;
  /** 集群节点列表（读取失败为空） */
  nodes?: ClusterNodeInfo[];
  fetched_at: string;
}

export interface ClusterVersionInfo {
  major: string;
  minor: string;
  git_version: string;
  git_commit: string;
  build_date: string;
  go_version: string;
  compiler: string;
  platform: string;
}

/** Agent 一键安装预览（YAML 亦可经 /agent-install.yaml?token= 被 kubectl 拉取） */
export interface AgentInstallPreview {
  cluster_id: string;
  image: string;
  tunnel_url: string;
  apply_command: string;
  yaml: string;
}

/** 平台信息页（/api/v1/platform/info，敏感值已在后端脱敏） */
export interface PlatformInfo {
  version: { version: string; commit: string; component: string };
  settings: {
    listen_addr: string;
    external_url: string;
    admin_group: string;
    password_login: boolean;
    password_user: string;
    robot_token_ttl: number;
    agent_image: string;
  };
  sso: { enabled: boolean; issuer: string; client_id: string; redirect_url: string };
  database: {
    dsn: string;
    open_connections: number;
    in_use: number;
    idle: number;
    max_open_connections: number;
    wait_count: number;
  };
  cache: { enabled: boolean; addrs: string; db: number };
  runtime: {
    go_version: string;
    goroutines: number;
    num_cpu: number;
    heap_alloc_bytes: number;
    sys_bytes: number;
    total_alloc_bytes: number;
    num_gc: number;
    uptime_seconds: number;
  };
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

// ── 平台用户目录（SSO/LDAP 登录自动同步）──

export type UserSource = 'sso' | 'ldap' | 'local';

export interface PlatformUserDTO {
  uid: string;
  username: string;
  nickname: string;
  avatar_url: string;
  groups: string[];
  source: UserSource;
  email: string;
  metadata: Record<string, unknown>;
  last_login?: string | null;
  created_at: string;
}

export interface GroupDTO {
  name: string;
  desc: string;
  source: string;
  is_admin: boolean;
  members: string[];
  created_at: string;
}

export interface UserScript {
  name: string;
  content: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
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
