// Mock 模式数据层：VITE_APP_MODE=mock 时前端完全脱离后端，
// 所有 api()/downloadFile() 调用被拦截到本模块（内存态，刷新即重置）。
// Release 构建不会进入本模块（http.ts 中按编译期常量分支）。
import { ApiError } from '@/api/http';
import type {
  AuditEvent,
  Cluster,
  ClusterInfo,
  ClusterResource,
  ClusterResourcesResponse,
  GroupDTO,
  PlatformInfo,
  PlatformUserDTO,
  PolicyDTO,
  Robot,
  SystemInfo,
  TokenDTO,
  UserScript,
} from '@/api/types';

const delay = (ms = 120) => new Promise((r) => setTimeout(r, ms));
const nowIso = () => new Date().toISOString();
const daysAgo = (d: number) => new Date(Date.now() - d * 864e5).toISOString();
let seq = 5000;
const nextID = () => ++seq;

// ── 种子数据 ──

const clusters: Cluster[] = [
  { id: 'prod-bj', name: '北京生产集群', desc: '核心业务生产环境', status: 'online', created_at: daysAgo(120), updated_at: daysAgo(0.1) },
  { id: 'prod-sh', name: '上海生产集群', desc: '交易系统生产环境', status: 'online', created_at: daysAgo(90), updated_at: daysAgo(0.2) },
  { id: 'staging', name: '预发集群', desc: '上线前验证', status: 'pending', created_at: daysAgo(30), updated_at: daysAgo(1) },
  { id: 'dev-local', name: '本地开发集群', desc: 'kind 本地测试', status: 'offline', created_at: daysAgo(8), updated_at: daysAgo(2) },
];

const users: PlatformUserDTO[] = [
  { uid: 'u-sysadmin', username: 'sysadmin', nickname: '系统管理员', avatar_url: '/icon-mark.png', groups: ['am:admin'], source: 'local', email: 'sysadmin@example.com', metadata: {}, last_login: nowIso(), created_at: daysAgo(120) },
  { uid: 'u-alice', username: 'alice', nickname: '爱丽丝', avatar_url: '', groups: ['dev-team'], source: 'sso', email: 'alice@example.com', metadata: { dept: '平台研发' }, last_login: daysAgo(0.5), created_at: daysAgo(60) },
  { uid: 'u-bob', username: 'bob', nickname: 'Bob', avatar_url: '', groups: ['qa-team'], source: 'ldap', email: 'bob@example.com', metadata: {}, last_login: daysAgo(2), created_at: daysAgo(45) },
];

const groups: GroupDTO[] = [
  { name: 'am:admin', desc: '平台管理员组', source: 'local', is_admin: true, members: ['sysadmin'], created_at: daysAgo(120) },
  { name: 'dev-team', desc: '研发团队', source: 'sso', is_admin: false, members: ['alice'], created_at: daysAgo(60) },
  { name: 'qa-team', desc: '测试团队', source: 'ldap', is_admin: false, members: ['bob'], created_at: daysAgo(45) },
];

const policies: PolicyDTO[] = [
  { subject_type: 'org', subject: 'dev-team', cluster: 'prod-bj', namespace: '*', resource: 'deployments', verbs: ['get', 'list'], sub: 'org:dev-team', dom: 'prod-bj', obj: '*/deployments', act: 'get|list' },
  { subject_type: 'org', subject: 'dev-team', cluster: 'prod-bj', namespace: '', resource: 'deployments', verbs: ['get', 'list'], sub: 'org:dev-team', dom: 'prod-bj', obj: 'cluster/deployments', act: 'get|list' },
  { subject_type: 'user', subject: 'alice', cluster: 'prod-sh', namespace: 'default', resource: 'pods', verbs: ['get', 'list', 'watch'], sub: 'user:alice', dom: 'prod-sh', obj: 'default/pods', act: 'get|list|watch' },
  { subject_type: 'robot', subject: 'robot-mock1', cluster: 'prod-bj', namespace: '*', resource: '*', verbs: ['*'], sub: 'robot:robot-mock1', dom: 'prod-bj', obj: '*', act: '.*' },
];

const robots: Robot[] = [
  { id: 'robot-mock1', name: 'ci-deployer', cluster_id: 'prod-bj', sub: 'robot:robot-mock1', status: 'active', issued_at: daysAgo(20), expires_at: daysAgo(-345), created_at: daysAgo(20) },
];

const scripts: UserScript[] = [
  {
    name: 'example-policy',
    enabled: false,
    content: "-- 示例：dev-team 成员自动获得 prod-bj 只读权限\nfor _, g in ipairs(claims.groups) do\n  if g == 'dev-team' then\n    kamc.add_policy('user:' .. claims.sub, 'prod-bj', 'default/*', 'get|list|watch')\n  end\nend\n",
    created_at: daysAgo(10),
    updated_at: daysAgo(3),
  },
];

const tokens: Record<string, TokenDTO[]> = {
  'prod-bj': [{ cluster_id: 'prod-bj', kind: 'longterm', status: 'active', created_at: daysAgo(20) }],
  staging: [{ cluster_id: 'staging', kind: 'register', status: 'active', expires_at: daysAgo(-0.04), created_at: daysAgo(0.01) }],
};

const auditPlatform: AuditEvent[] = [
  { id: nextID(), kind: 'platform', event: 'login-success', timestamp: daysAgo(0.02), user: 'sysadmin', groups: 'am:admin', cluster: '', method: 'POST', path: '/auth/password-login', verb: '', allowed: true, reason: 'ok', source_ip: '203.0.113.9', forwarded_for: '203.0.113.9', user_agent: 'Mozilla/5.0 (X11; Linux x86_64)', geo_country: 'CN', geo_region: 'Beijing', geo_city: 'Beijing', status_code: 200 },
  { id: nextID(), kind: 'platform', event: 'kubeconfig-download', timestamp: daysAgo(0.1), user: 'alice', groups: 'dev-team', cluster: 'prod-bj', method: 'GET', path: '/api/v1/clusters/prod-bj/kubeconfig', verb: '', allowed: true, reason: 'mode=exec', source_ip: '198.51.100.7', user_agent: 'curl/8.14.1', geo_country: 'CN', geo_region: 'Shanghai', geo_city: 'Shanghai', status_code: 200 },
  { id: nextID(), kind: 'platform', event: 'token-issue', timestamp: daysAgo(0.3), user: 'sysadmin', groups: 'am:admin', cluster: 'prod-bj', method: 'POST', path: '/api/v1/clusters/prod-bj/tokens', verb: '', allowed: true, reason: 'kind=longterm', source_ip: '127.0.0.1', user_agent: 'curl/8.14.1' },
  { id: nextID(), kind: 'platform', event: 'login-failed', timestamp: daysAgo(0.6), user: 'unknown', groups: '', cluster: '', method: 'POST', path: '/auth/password-login', verb: '', allowed: false, reason: 'bad-credentials', source_ip: '203.0.113.44', user_agent: 'python-requests/2.32', geo_country: 'US', geo_region: 'California', geo_city: 'Los Angeles' },
  { id: nextID(), kind: 'platform', event: 'policy-create', timestamp: daysAgo(1), user: 'sysadmin', groups: 'am:admin', cluster: 'prod-bj', method: 'POST', path: '/api/v1/policies', verb: '', allowed: true, reason: 'user/alice', source_ip: '127.0.0.1', user_agent: 'Mozilla/5.0' },
  { id: nextID(), kind: 'platform', event: 'robot-create', timestamp: daysAgo(20), user: 'sysadmin', groups: 'am:admin', cluster: 'prod-bj', method: 'POST', path: '/api/v1/robots', verb: '', allowed: true, reason: 'ci-deployer', source_ip: '127.0.0.1', user_agent: 'Mozilla/5.0' },
];

const clusterDump = `GET /api/v1/nodes HTTP/1.1\r\nAccept: application/json\r\nAuthorization: ***\r\nUser-Agent: kubectl/v1.31\r\n\r\n(无 body)`;
const clusterResp = `HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\n  "kind": "NodeList",\n  "apiVersion": "v1",\n  "items": [\n    {\n      "kind": "Node",\n      "metadata": {\n        "name": "node-1",\n        "labels": {\n          "node-role.kubernetes.io/worker": ""\n        }\n      },\n      "status": {\n        "conditions": [\n          {\n            "type": "Ready",\n            "status": "True"\n          }\n        ]\n      }\n    }\n  ]\n}`;
const auditCluster: AuditEvent[] = [
  { id: nextID(), kind: 'cluster', event: 'k8s-api', timestamp: daysAgo(0.01), user: 'alice', groups: 'dev-team', cluster: 'prod-bj', method: 'GET', path: '/proxy/prod-bj/api/v1/nodes', object: 'cluster/nodes', verb: 'list', allowed: true, reason: 'allowed', source_ip: '198.51.100.7', forwarded_for: '198.51.100.7', user_agent: 'kubectl/v1.31', geo_country: 'CN', geo_region: 'Shanghai', geo_city: 'Shanghai', status_code: 200, request_body: clusterDump, response_body: clusterResp, latency_ms: 36 },
  { id: nextID(), kind: 'cluster', event: 'k8s-api', timestamp: daysAgo(0.05), user: 'alice', groups: 'dev-team', cluster: 'prod-bj', method: 'DELETE', path: '/proxy/prod-bj/apis/apps/v1/namespaces/default/deployments/legacy-api', object: 'default/deployments', verb: 'delete', allowed: false, reason: 'denied-casbin', source_ip: '198.51.100.7', user_agent: 'kubectl/v1.31', geo_country: 'CN', geo_region: 'Shanghai', geo_city: 'Shanghai', status_code: 403 },
  { id: nextID(), kind: 'cluster', event: 'k8s-api', timestamp: daysAgo(0.4), user: 'sysadmin', groups: 'am:admin', cluster: 'prod-sh', method: 'POST', path: '/proxy/prod-sh/api/v1/namespaces/default/pods', object: 'default/pods', verb: 'create', allowed: true, reason: 'allowed', source_ip: '203.0.113.9', user_agent: 'kubectl/v1.31', geo_country: 'CN', geo_region: 'Beijing', geo_city: 'Beijing', status_code: 201, request_body: `POST /api/v1/namespaces/default/pods HTTP/1.1\r\nContent-Type: application/json\r\nAuthorization: ***\r\n\r\n{\n  "apiVersion": "v1",\n  "kind": "Pod",\n  "metadata": {\n    "name": "demo"\n  }\n}`, response_body: `HTTP/1.1 201 Created\r\nContent-Type: application/json\r\n\r\n{\n  "kind": "Pod",\n  "metadata": {\n    "name": "demo"\n  }\n}`, latency_ms: 52 },
];

const mockClaims = {
  iss: 'https://kamc.local',
  sub: 'sysadmin',
  aud: ['kamc'],
  exp: Math.floor(Date.now() / 1000) + 3600,
  iat: Math.floor(Date.now() / 1000),
  jti: 'mock-jti',
  email: 'sysadmin@example.com',
  groups: ['am:admin'],
};

const systemInfo: SystemInfo = {
  go_version: 'go1.27.2', goroutines: 42, num_cpu: 8,
  heap_alloc_bytes: 8 * 1024 * 1024, heap_sys_bytes: 24 * 1024 * 1024,
  sys_bytes: 48 * 1024 * 1024, total_alloc_bytes: 512 * 1024 * 1024,
  num_gc: 21, uptime_seconds: 3 * 86400 + 7200,
  db_open_connections: 4, db_in_use: 1, db_idle: 3, db_wait_count: 0, db_max_open_connections: 20,
};

const platformInfo: PlatformInfo = {
  version: { version: 'dev', commit: 'mock', component: 'platform' },
  settings: {
    listen_addr: ':8080', external_url: 'http://127.0.0.1:8080', admin_group: 'am:admin',
    password_login: true, password_user: 'sysadmin', robot_token_ttl: 31536000, agent_image: 'tuf3i/k8s-am-agent:dev',
  },
  sso: { enabled: true, issuer: 'https://auth.example.com/application/o/kamc/', client_id: 'kamc', redirect_url: 'http://127.0.0.1:8080/auth/callback' },
  database: { dsn: 'host=127.0.0.1 port=5432 user=postgres password=*** dbname=kamc sslmode=disable', open_connections: 4, in_use: 1, idle: 3, max_open_connections: 20, wait_count: 0 },
  cache: { enabled: true, addrs: '127.0.0.1:6379', db: 0 },
  runtime: { go_version: 'go1.27.2', goroutines: 42, num_cpu: 8, heap_alloc_bytes: 8 * 1024 * 1024, sys_bytes: 48 * 1024 * 1024, total_alloc_bytes: 512 * 1024 * 1024, num_gc: 21, uptime_seconds: 3 * 86400 + 7200 },
};

const namespaces = ['default', 'kube-system', 'dev', 'ops'];
const resources: ClusterResource[] = [
  { group: '', version: 'v1', resource: 'pods', gvr: 'v1/pods', kind: 'Pod', namespaced: true, verbs: ['create', 'delete', 'get', 'list', 'patch', 'update', 'watch'] },
  { group: '', version: 'v1', resource: 'services', gvr: 'v1/services', kind: 'Service', namespaced: true, verbs: ['create', 'delete', 'get', 'list', 'patch', 'update', 'watch'] },
  { group: '', version: 'v1', resource: 'namespaces', gvr: 'v1/namespaces', kind: 'Namespace', namespaced: false, verbs: ['create', 'delete', 'get', 'list', 'patch', 'update', 'watch'] },
  { group: 'apps', version: 'v1', resource: 'deployments', gvr: 'apps/v1/deployments', kind: 'Deployment', namespaced: true, verbs: ['create', 'delete', 'get', 'list', 'patch', 'update', 'watch'] },
  { group: 'apps', version: 'v1', resource: 'statefulsets', gvr: 'apps/v1/statefulsets', kind: 'StatefulSet', namespaced: true, verbs: ['create', 'delete', 'get', 'list', 'patch', 'update', 'watch'] },
];

// ── 内部工具 ──

function notFound(msg = 'not found'): never {
  throw new ApiError(msg, 404, 40400);
}

function findCluster(id: string): Cluster {
  const c = clusters.find((x) => x.id === id);
  if (!c) notFound('cluster not found');
  return c;
}

function findUser(username: string): PlatformUserDTO {
  const u = users.find((x) => x.username === username);
  if (!u) notFound('user not found');
  return u;
}

function findGroup(name: string): GroupDTO {
  const g = groups.find((x) => x.name === name);
  if (!g) notFound('group not found');
  return g;
}

/** 各集群资源目录（mock 统一返回同一份示例数据） */
function clusterResources(id: string): ClusterResourcesResponse {
  findCluster(id);
  return {
    cluster_id: id,
    namespaces,
    resources,
    partial: false,
    cached: false,
    discovered_at: nowIso(),
  };
}

function clusterInfo(id: string): ClusterInfo {
  findCluster(id);
  return {
    version: { major: '1', minor: '31', git_version: 'v1.31.4', git_commit: 'mock', build_date: '2024-10-15T10:00:00Z', go_version: 'go1.22.8', compiler: 'gc', platform: 'linux/amd64' },
    control_plane: 'https://10.96.0.1:6443',
    coredns: '10.96.0.10:53',
    nodes: [
      { name: 'node-1', roles: ['control-plane'], ready: true, internal_ip: '10.0.0.11', kubelet_version: 'v1.31.4', os_image: 'Ubuntu 22.04.5 LTS', architecture: 'amd64', cpu: '4', memory: '16 Gi' },
      { name: 'node-2', roles: ['worker'], ready: true, internal_ip: '10.0.0.12', kubelet_version: 'v1.31.4', os_image: 'Ubuntu 22.04.5 LTS', architecture: 'amd64', cpu: '8', memory: '32 Gi' },
    ],
    fetched_at: nowIso(),
  };
}

// ── 主分发 ──

export async function mockApi<T>(method: string, rawPath: string, body?: unknown): Promise<T> {
  await delay();
  const [path, qs] = rawPath.split('?');
  const q = new URLSearchParams(qs ?? '');
  const m = method.toUpperCase();
  const data = (v: unknown) => v as T;
  const payload = (body ?? {}) as Record<string, any>;

  // 认证
  if (path === '/auth/password-login' && m === 'POST') {
    if (!payload.username || !payload.password) throw new ApiError('用户名或密码不能为空', 400, 10001);
    return data({ token: 'mock-token', expires_at: new Date(Date.now() + 3600e3).toISOString(), token_type: 'Bearer' });
  }
  if (path === '/auth/me') return data(mockClaims);
  if (path === '/auth/logout') return data({ logout: true });
  if (path === '/version') return data({ version: 'dev', commit: 'mock', component: 'platform' });

  // 自服务资料（头像等展示字段）
  if (path === '/api/v1/profile') return data(findUser(mockClaims.sub));

  // 概览 / 平台
  if (path === '/api/v1/system') return data(systemInfo);
  if (path === '/api/v1/platform/info') return data(platformInfo);

  // 集群
  if (path === '/api/v1/clusters' && m === 'GET') return data(clusters);
  if (path === '/api/v1/clusters' && m === 'POST') {
    const id = String(payload.id ?? '').trim();
    if (!id) throw new ApiError('集群 ID 不能为空', 400, 10001);
    if (clusters.some((c) => c.id === id)) throw new ApiError('集群已存在', 400, 20002);
    const c: Cluster = { id, name: payload.name || id, desc: payload.desc || '', status: 'pending', created_at: nowIso(), updated_at: nowIso() };
    clusters.push(c);
    return data(c);
  }

  let mm = /^\/api\/v1\/clusters\/([^/]+)$/.exec(path);
  if (mm) {
    const id = decodeURIComponent(mm[1]);
    if (m === 'GET') return data(findCluster(id));
    if (m === 'DELETE') {
      const idx = clusters.findIndex((c) => c.id === id);
      if (idx < 0) notFound('cluster not found');
      clusters.splice(idx, 1);
      return data(undefined as T);
    }
  }
  mm = /^\/api\/v1\/clusters\/([^/]+)\/(info|resources|tokens|agent-install)$/.exec(path);
  if (mm) {
    const id = decodeURIComponent(mm[1]);
    const action = mm[2];
    if (action === 'info') return data(clusterInfo(id));
    if (action === 'resources') return data(clusterResources(id));
    if (action === 'tokens' && m === 'GET') return data(tokens[id] ?? []);
    if (action === 'tokens' && m === 'POST') {
      const kind = (payload.kind ?? 'register') as TokenDTO['kind'];
      const list = tokens[id] ?? [];
      list.push({ cluster_id: id, kind, status: 'active', expires_at: kind === 'register' ? new Date(Date.now() + 3600e3).toISOString() : undefined, created_at: nowIso() });
      tokens[id] = list;
      return data({ token: `am-${kind === 'register' ? 'ten' : 'clu'}-mock${nextID()}`, kind, grace_ends_at: undefined });
    }
    if (action === 'agent-install' && m === 'POST') {
      const image = 'tuf3i/k8s-am-agent:dev';
      const token = String(payload.token ?? 'am-clu-mock');
      const yaml = `apiVersion: v1\nkind: Namespace\nmetadata:\n  name: k8s-am-system\n---\n# mock agent install for ${id}\n# image: ${image}\n# token: ${token}\n`;
      return data({
        cluster_id: id, image, tunnel_url: 'ws://127.0.0.1:8080/tunnel',
        apply_command: `kubectl apply -f "http://127.0.0.1:8080/agent-install.yaml?token=${token}"`,
        yaml,
      });
    }
  }
  mm = /^\/api\/v1\/clusters\/([^/]+)\/tokens\/([^/]+)$/.exec(path);
  if (mm && m === 'DELETE') {
    const id = decodeURIComponent(mm[1]);
    const kind = decodeURIComponent(mm[2]);
    tokens[id] = (tokens[id] ?? []).filter((t) => t.kind !== kind);
    return data(undefined as T);
  }

  // 策略
  if (path === '/api/v1/policies' && m === 'GET') return data(policies);
  if (path === '/api/v1/policies' && m === 'POST') {
    const subjType =
      payload.subject_type === 'org' || payload.subject_type === 'robot'
        ? payload.subject_type
        : 'user';
    const sub = `${subjType}:${payload.subject}`;
    const created: PolicyDTO[] = [];
    for (const rule of (payload.rules ?? []) as any[]) {
      const resource = rule.resource ?? '*';
      const subresource = rule.subresource ? `/${rule.subresource}` : '';
      if (resource === '*') {
        created.push({ subject_type: subjType as PolicyDTO['subject_type'], subject: payload.subject, cluster: payload.cluster, namespace: '*', resource: '*', verbs: ['*'], sub, dom: payload.cluster, obj: '*', act: '.*' });
      } else {
        const ns = rule.namespaced ? (rule.namespace || '*') : '';
        const obj = rule.namespaced ? `${ns}/${resource}${subresource}` : `cluster/${resource}${subresource}`;
        const verbs = (rule.verbs ?? []) as string[];
        created.push({ subject_type: subjType as PolicyDTO['subject_type'], subject: payload.subject, cluster: payload.cluster, namespace: ns || '*', resource, subresource: rule.subresource, verbs, sub, dom: payload.cluster, obj, act: verbs.map((v) => (v === '*' ? '.*' : v)).join('|') });
      }
    }
    policies.push(...created);
    return data({ created: created.length, policies: created });
  }
  if (path === '/api/v1/policies' && m === 'DELETE') {
    const idx = policies.findIndex((p) => p.sub === payload.sub && p.dom === payload.dom && p.obj === payload.obj && p.act === payload.act);
    if (idx < 0) throw new ApiError('policy not found', 400, 40001);
    policies.splice(idx, 1);
    return data(undefined as T);
  }

  // 机器人
  if (path === '/api/v1/robots' && m === 'GET') return data(robots);
  if (path === '/api/v1/robots' && m === 'POST') {
    const id = `robot-mock${nextID()}`;
    const r: Robot = { id, name: payload.name, cluster_id: payload.cluster_id, sub: `robot:${id}`, status: 'active', issued_at: nowIso(), expires_at: new Date(Date.now() + 365 * 864e5).toISOString(), created_at: nowIso() };
    robots.push(r);
    return data({ robot: r, kubeconfig: robotKubeconfig(r) });
  }
  mm = /^\/api\/v1\/robots\/([^/]+)$/.exec(path);
  if (mm && m === 'DELETE') {
    const idx = robots.findIndex((r) => r.id === decodeURIComponent(mm![1]));
    if (idx < 0) notFound('robot not found');
    robots.splice(idx, 1);
    return data(undefined as T);
  }

  // 用户
  if (path === '/api/v1/users' && m === 'GET') return data(users);
  if (path === '/api/v1/users' && m === 'POST') {
    const username = String(payload.username ?? '').trim();
    if (!username) throw new ApiError('用户名不能为空', 400, 10001);
    if (users.some((u) => u.username === username)) throw new ApiError('用户已存在', 400, 40010);
    const u: PlatformUserDTO = {
      uid: `u-${nextID()}`, username, nickname: payload.nickname || '', avatar_url: payload.avatar_url || '',
      groups: [], source: 'local', email: payload.email || '', metadata: {}, last_login: nowIso(), created_at: nowIso(),
    };
    users.push(u);
    return data(u);
  }
  mm = /^\/api\/v1\/users\/([^/]+)$/.exec(path);
  if (mm) {
    const username = decodeURIComponent(mm[1]);
    if (m === 'PATCH') {
      const u = findUser(username);
      if (payload.nickname !== undefined) u.nickname = payload.nickname;
      if (payload.email !== undefined) u.email = payload.email;
      if (payload.avatar_url !== undefined) u.avatar_url = payload.avatar_url;
      if (payload.groups !== undefined) u.groups = payload.groups;
      return data(u);
    }
    if (m === 'DELETE') {
      const idx = users.findIndex((u) => u.username === username);
      if (idx < 0) notFound('user not found');
      users.splice(idx, 1);
      groups.forEach((g) => { g.members = g.members.filter((x) => x !== username); });
      return data(undefined as T);
    }
  }

  // 组
  if (path === '/api/v1/groups' && m === 'GET') return data(groups);
  if (path === '/api/v1/groups' && m === 'POST') {
    const name = String(payload.name ?? '').trim();
    if (!name) throw new ApiError('组名不能为空', 400, 10001);
    if (groups.some((g) => g.name === name)) throw new ApiError('组已存在', 400, 40007);
    const g: GroupDTO = { name, desc: payload.desc || '', source: 'local', is_admin: payload.is_admin === true, members: [], created_at: nowIso() };
    groups.push(g);
    return data(g);
  }
  mm = /^\/api\/v1\/groups\/([^/]+)$/.exec(path);
  if (mm) {
    const name = decodeURIComponent(mm[1]);
    if (m === 'PATCH') {
      const g = findGroup(name);
      const newName = String(payload.name ?? '').trim();
      if (newName && newName !== name) {
        if (groups.some((x) => x.name === newName)) throw new ApiError('组已存在', 400, 40007);
        users.forEach((u) => { u.groups = u.groups.map((x) => (x === name ? newName : x)); });
        g.name = newName;
        g.members = g.members.slice();
      }
      if (payload.desc !== undefined) g.desc = payload.desc;
      if (payload.is_admin !== undefined) g.is_admin = payload.is_admin === true;
      return data(g);
    }
    if (m === 'DELETE') {
      const idx = groups.findIndex((g) => g.name === name);
      if (idx < 0) notFound('group not found');
      groups.splice(idx, 1);
      return data(undefined as T);
    }
  }
  mm = /^\/api\/v1\/groups\/([^/]+)\/members(?:\/([^/]+))?$/.exec(path);
  if (mm) {
    const name = decodeURIComponent(mm[1]);
    const g = findGroup(name);
    if (m === 'POST') {
      const username = String(payload.username ?? '');
      findUser(username);
      if (!g.members.includes(username)) {
        g.members.push(username);
        const u = users.find((x) => x.username === username)!;
        if (!u.groups.includes(name)) u.groups.push(name);
      }
      return data(undefined as T);
    }
    if (m === 'DELETE' && mm[2]) {
      const username = decodeURIComponent(mm[2]);
      g.members = g.members.filter((x) => x !== username);
      const u = users.find((x) => x.username === username);
      if (u) u.groups = u.groups.filter((x) => x !== name);
      return data(undefined as T);
    }
  }

  // 映射脚本
  if (path === '/api/v1/scripts' && m === 'GET') {
    const all = q.get('all') === '1';
    return data(all ? scripts : scripts.filter((s) => s.enabled));
  }
  mm = /^\/api\/v1\/scripts\/([^/]+)$/.exec(path);
  if (mm) {
    const name = decodeURIComponent(mm[1]);
    if (m === 'GET') {
      const s = scripts.find((x) => x.name === name);
      if (!s) notFound('script not found');
      return data(s);
    }
    if (m === 'PUT') {
      const existing = scripts.find((x) => x.name === name);
      if (existing) {
        existing.content = payload.content;
        existing.enabled = !!payload.enabled;
        existing.updated_at = nowIso();
        return data(existing);
      }
      const s: UserScript = { name, content: payload.content ?? '', enabled: !!payload.enabled, created_at: nowIso(), updated_at: nowIso() };
      scripts.push(s);
      return data(s);
    }
    if (m === 'DELETE') {
      const idx = scripts.findIndex((x) => x.name === name);
      if (idx < 0) notFound('script not found');
      scripts.splice(idx, 1);
      return data(undefined as T);
    }
  }
  mm = /^\/api\/v1\/scripts\/([^/]+)\/execute$/.exec(path);
  if (mm && m === 'POST') {
    const name = decodeURIComponent(mm[1]);
    if (!scripts.some((s) => s.name === name)) notFound('script not found');
    const dry = q.get('dry_run') === '1';
    return data({
      dry_run: dry,
      actions: [
        { api: 'add_policy', args: ['user:alice', 'prod-bj', 'default/*', 'get|list|watch'] },
        { api: 'upsert_group', args: ['dev-team', '研发团队'] },
      ],
    });
  }

  // 审计
  if (path === '/api/v1/audit') {
    const kind = q.get('kind');
    const limit = Number(q.get('limit') ?? 200);
    const source =
      kind === 'cluster'
        ? auditCluster
        : kind === 'platform'
          ? auditPlatform
          : [...auditPlatform, ...auditCluster].sort((a, b) =>
              b.timestamp.localeCompare(a.timestamp),
            );
    return data(source.slice(0, limit));
  }

  throw new ApiError(`mock: 未实现的接口 ${m} ${path}`, 404, 40400);
}

/** mock 模式下下载文本（kubeconfig 等） */
export function mockDownload(rawPath: string): { text: string; filename: string } {
  const [path] = rawPath.split('?');
  let mm = /^\/api\/v1\/clusters\/([^/]+)\/kubeconfig$/.exec(path);
  if (mm) {
    const id = decodeURIComponent(mm[1]);
    return { filename: `kubeconfig-${id}.yaml`, text: kubeconfigYAML(id, 'mock-token') };
  }
  mm = /^\/api\/v1\/robots\/([^/]+)\/kubeconfig$/.exec(path);
  if (mm) {
    const r = robots.find((x) => x.id === decodeURIComponent(mm![1]));
    return { filename: `kubeconfig-robot-${r?.name ?? 'robot'}.yaml`, text: robotKubeconfig(r ?? robots[0]) };
  }
  if (path === '/agent-install.yaml') {
    return { filename: 'agent-install.yaml', text: 'apiVersion: v1\nkind: Namespace\nmetadata:\n  name: k8s-am-system\n' };
  }
  return { filename: 'download.txt', text: 'mock download' };
}

function kubeconfigYAML(cluster: string, token: string): string {
  return `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: http://127.0.0.1:8080/proxy/${cluster}
  name: ${cluster}
contexts:
- context:
    cluster: ${cluster}
    user: sysadmin
  name: ${cluster}
current-context: ${cluster}
users:
- name: sysadmin
  user:
    token: ${token}
`;
}

function robotKubeconfig(r: Robot): string {
  return kubeconfigYAML(r.cluster_id, `mock-robot-token-${r.id}`);
}

/** mock 模式头像：直接把 data URL 写进内存态（立即显示） */
export function mockSetAvatar(username: string, dataUrl: string): void {
  const u = users.find((x) => x.username === username);
  if (u) u.avatar_url = dataUrl;
}
