import {
  For,
  Show,
  createEffect,
  createMemo,
  createSignal,
} from 'solid-js';
import { Navigate, useNavigate, useSearchParams } from '@solidjs/router';
import dayjs from 'dayjs';
import { auth } from '@/stores/auth';
import { listClusters } from '@/api/clusters';
import { listAudit } from '@/api/audit';
import { listPolicies } from '@/api/access';
import { listRobots } from '@/api/robots';
import { fetchSystemInfo } from '@/api/system';
import type { AuditEvent, Cluster, Robot, SystemInfo } from '@/api/types';
import { fmtUptime } from '@/utils/format';
import { colorScheme } from '@/lib/storage/theme';
import {
  Alert,
  Button,
  Card,
  Empty,
  Skeleton,
  Table,
  Tabs,
  Tag,
  Tooltip,
  type TableColumnsType,
} from '@/lib/widgets';
import { PageHeader } from '@/components/PageHeader';
import { AuditDetailDrawer } from '@/components/AuditDetailDrawer';
import { EChart } from '@/components/EChart';
import {
  chartPalette,
  donutOption,
  resourceBarOption,
  trendOption,
} from '@/components/chart';
import {
  ApiOutlined,
  BranchesOutlined,
  ClockCircleOutlined,
  ClusterOutlined,
  CodeOutlined,
  EyeOutlined,
  FileSearchOutlined,
  PlusOutlined,
  ReloadOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
} from '@/lib/icons';

type DashTab = 'ops' | 'access' | 'identity';

interface TrendPoint {
  date: string;
  logins: number;
  ops: number;
  blocked: number;
}

interface DayBucket {
  logins: number;
  ops: number;
  blocked: number;
}

interface RankItem {
  name: string;
  value: number;
  denied?: number;
  raw?: string;
}

interface ResourceSlice {
  name: string;
  allowed: number;
  denied: number;
}

const COUNTRY_LABELS: Record<string, string> = {
  CN: '中国',
  US: '美国',
  JP: '日本',
  RU: '俄罗斯',
  SG: '新加坡',
  DE: '德国',
  GB: '英国',
  FR: '法国',
  KR: '韩国',
  IN: '印度',
  AU: '澳大利亚',
  CA: '加拿大',
  NL: '荷兰',
  BR: '巴西',
};

function countryLabel(code: string): string {
  const label = COUNTRY_LABELS[code];
  return label ? `${label} (${code})` : code;
}

function buildTrend(events: AuditEvent[]): TrendPoint[] {
  const days = Array.from({ length: 7 }, (_, index) => dayjs().subtract(6 - index, 'day'));
  const buckets = new Map<string, DayBucket>();
  for (const day of days) {
    buckets.set(day.format('YYYY-MM-DD'), { logins: 0, ops: 0, blocked: 0 });
  }
  for (const event of events) {
    const bucket = buckets.get(dayjs(event.timestamp).format('YYYY-MM-DD'));
    if (!bucket) continue;
    if (event.kind === 'platform' && event.event === 'login-success' && event.allowed) {
      bucket.logins += 1;
    }
    if (event.kind === 'cluster') {
      if (event.allowed) bucket.ops += 1;
      else bucket.blocked += 1;
    }
  }
  return days.map((day) => {
    const bucket = buckets.get(day.format('YYYY-MM-DD')) as DayBucket;
    return { date: day.format('MM-DD'), ...bucket };
  });
}

/** 资源语义名：default/pods → pods，cluster/nodes → nodes（集群） */
function resourceName(event: AuditEvent): string {
  const obj = event.object || '';
  if (!obj) {
    const path = event.path || '';
    if (path.startsWith('nonresource')) return path.slice('nonresource'.length) || '非资源路径';
    if (path.startsWith('cluster/')) return `${path.slice('cluster/'.length)}（集群）`;
    return path ? (path.length > 36 ? `${path.slice(0, 36)}…` : path) : '—';
  }
  if (obj === '*') return '全部资源';
  if (obj.startsWith('nonresource')) return obj.slice('nonresource'.length) || '非资源路径';
  const rest = obj.startsWith('cluster/') ? obj.slice('cluster/'.length) : obj;
  const parts = rest.split('/');
  const label = parts.length >= 2 ? `${parts[1]}${parts[2] ? `/${parts[2]}` : ''}` : rest;
  return obj.startsWith('cluster/') ? `${label}（集群）` : label;
}

/** UA 归类：把冗长的浏览器 UA 收敛成可读客户端名 */
function uaLabel(ua?: string): string {
  if (!ua) return '未知客户端';
  const kubectl = /kubectl\/([\w.\-]+)/i.exec(ua);
  if (kubectl) return `kubectl ${kubectl[1]}`;
  if (/kamc\/[\w.\-]+/i.test(ua)) return 'kamc';
  if (/ZCode\/[\d.]+/i.test(ua)) return 'ZCode 客户端';
  if (/Edg\//i.test(ua)) return 'Edge 浏览器';
  if (/Firefox\//i.test(ua)) return 'Firefox 浏览器';
  if (/Chrome\//i.test(ua)) return 'Chrome 浏览器';
  if (/Safari\//i.test(ua)) return 'Safari 浏览器';
  if (/Go-http-client/i.test(ua)) return 'Go HTTP 客户端';
  const curl = /curl\/([\w.\-]+)/i.exec(ua);
  if (curl) return `curl ${curl[1]}`;
  const py = /python-requests\/([\w.\-]+)/i.exec(ua);
  if (py) return `python-requests ${py[1]}`;
  return ua.length > 32 ? `${ua.slice(0, 32)}…` : ua;
}

type KeyFn = (event: AuditEvent) => string | string[] | undefined;

function countBy(events: AuditEvent[], getKeys: KeyFn, limit: number): RankItem[] {
  const counts = new Map<string, number>();
  for (const event of events) {
    const raw = getKeys(event);
    const keys = Array.isArray(raw) ? raw : raw ? [raw] : [];
    const seen = new Set<string>();
    for (const key of keys) {
      const name = key.trim();
      if (!name || seen.has(name)) continue;
      seen.add(name);
      counts.set(name, (counts.get(name) ?? 0) + 1);
    }
  }
  return [...counts.entries()]
    .map(([name, value]) => ({ name, value }))
    .sort((a, b) => b.value - a.value)
    .slice(0, limit);
}

/** 用户/组维度：同时统计拦截次数 */
function actorBreakdown(events: AuditEvent[], getKeys: KeyFn, limit: number): RankItem[] {
  const map = new Map<string, { value: number; denied: number }>();
  for (const event of events) {
    const raw = getKeys(event);
    const keys = Array.isArray(raw) ? raw : raw ? [raw] : [];
    const seen = new Set<string>();
    for (const key of keys) {
      const name = key.trim();
      if (!name || seen.has(name)) continue;
      seen.add(name);
      const entry = map.get(name) ?? { value: 0, denied: 0 };
      entry.value += 1;
      if (!event.allowed) entry.denied += 1;
      map.set(name, entry);
    }
  }
  return [...map.entries()]
    .map(([name, entry]) => ({ name, ...entry }))
    .sort((a, b) => b.value - a.value)
    .slice(0, limit);
}

function resourceBreakdown(events: AuditEvent[], limit: number): ResourceSlice[] {
  const map = new Map<string, { allowed: number; denied: number }>();
  for (const event of events) {
    const name = resourceName(event);
    if (name === '—') continue;
    const entry = map.get(name) ?? { allowed: 0, denied: 0 };
    if (event.allowed) entry.allowed += 1;
    else entry.denied += 1;
    map.set(name, entry);
  }
  return [...map.entries()]
    .map(([name, entry]) => ({ name, ...entry }))
    .sort((a, b) => b.allowed + b.denied - (a.allowed + a.denied))
    .slice(0, limit);
}

function uaBreakdown(events: AuditEvent[], limit: number): RankItem[] {
  const map = new Map<string, { value: number; raw: string }>();
  for (const event of events) {
    const raw = event.user_agent || '';
    const name = uaLabel(raw);
    const entry = map.get(name) ?? { value: 0, raw };
    entry.value += 1;
    if (raw.length > entry.raw.length) entry.raw = raw;
    map.set(name, entry);
  }
  return [...map.entries()]
    .map(([name, entry]) => ({ name, value: entry.value, raw: entry.raw }))
    .sort((a, b) => b.value - a.value)
    .slice(0, limit);
}

function requestSize(event: AuditEvent): number {
  return event.request_size ?? event.request_body?.length ?? 0;
}

function responseSize(event: AuditEvent): number {
  return event.response_size ?? event.response_body?.length ?? 0;
}

function fmtChars(n?: number): string {
  if (!n) return '—';
  if (n < 1000) return `${n}`;
  if (n < 1_000_000) return `${(n / 1000).toFixed(1)}K`;
  return `${(n / 1_000_000).toFixed(2)}M`;
}

function fmtBytes(n?: number): string {
  if (!n) return '—';
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = n;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function param(value: string | string[] | undefined): string {
  if (Array.isArray(value)) return value[0] ?? '';
  return value ?? '';
}

function RankList(props: {
  items: RankItem[];
  empty: string;
  unit?: string;
  compact?: boolean;
}) {
  const max = createMemo(() => Math.max(...props.items.map((item) => item.value), 1));
  return (
    <Show
      when={props.items.length > 0}
      fallback={<Empty description={props.empty} class="border-0 py-4" />}
    >
      <div class="mini-list">
        <For each={props.items}>
          {(item) => (
            <div class={`mini-row${props.compact ? ' compact' : ''}`}>
              <Tooltip title={item.raw || item.name}>
                <span class="mini-name">{item.name}</span>
              </Tooltip>
              <Show when={!props.compact}>
                <div class="mini-bar">
                  <span
                    style={{
                      width: `${Math.max(6, (item.value / max()) * 100)}%`,
                    }}
                  />
                </div>
              </Show>
              <span class="mini-count">
                {item.value}
                {props.unit ?? '次'}
                <Show when={item.denied}>
                  <em class="mini-denied"> · {item.denied} 拦截</em>
                </Show>
              </span>
            </div>
          )}
        </For>
      </div>
    </Show>
  );
}

export default function Dashboard() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const [clusters, setClusters] = createSignal<Cluster[]>([]);
  const [robots, setRobots] = createSignal<Robot[]>([]);
  const [policyCount, setPolicyCount] = createSignal(0);
  const [audit, setAudit] = createSignal<AuditEvent[]>([]);
  const [system, setSystem] = createSignal<SystemInfo | null>(null);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal<string | null>(null);
  const [detail, setDetail] = createSignal<AuditEvent | null>(null);

  const load = async () => {
    if (!auth.isAdmin()) return;
    setLoading(true);
    setError(null);
    try {
      const [clusterList, systemInfo, robotList, policies, clusterAudit, platformAudit] =
        await Promise.all([
          listClusters(),
          fetchSystemInfo().catch(() => null),
          listRobots().catch(() => []),
          listPolicies()
            .then((list) => list.length)
            .catch(() => 0),
          listAudit({ kind: 'cluster', event: 'k8s-api', limit: 1000, summary: true }).catch(
            () => [],
          ),
          listAudit({ kind: 'platform', limit: 1000, summary: true }).catch(() => []),
        ]);
      setClusters(clusterList);
      setSystem(systemInfo);
      setRobots(robotList);
      setPolicyCount(policies);
      setAudit([...clusterAudit, ...platformAudit]);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载概览失败');
    } finally {
      setLoading(false);
    }
  };

  createEffect(() => {
    if (auth.isAdmin()) void load();
  });

  const windowEvents = createMemo(() => {
    const cutoff = dayjs().startOf('day').subtract(6, 'day');
    return audit().filter((event) => !dayjs(event.timestamp).isBefore(cutoff));
  });
  const platformEvents = createMemo(() =>
    windowEvents().filter(
      (event) => event.kind === 'platform' && event.event !== 'tunnel-attached',
    ),
  );
  // 只统计数据面代理请求（k8s-api）；历史隧道/后台事件不算访问画像
  const clusterEvents = createMemo(() =>
    windowEvents().filter(
      (event) => event.kind === 'cluster' && event.event === 'k8s-api',
    ),
  );
  const analyticsEvents = createMemo(() => [...clusterEvents(), ...platformEvents()]);

  const trend = createMemo(() => buildTrend(analyticsEvents()));
  const verbSlices = createMemo(() => countBy(clusterEvents(), (event) => event.verb, 6));
  const resourceSlices = createMemo(() => resourceBreakdown(clusterEvents(), 6));
  const userSlices = createMemo(() =>
    actorBreakdown(
      clusterEvents(),
      (event) => (event.user && event.user !== 'unknown' ? event.user : ''),
      5,
    ),
  );
  const groupSlices = createMemo(() =>
    actorBreakdown(
      clusterEvents(),
      (event) =>
        event.groups
          ? event.groups
              .split(',')
              .map((group) => group.trim())
              .filter((group) => Boolean(group) && group !== 'robot')
          : '',
      5,
    ),
  );
  const countrySlices = createMemo(() =>
    countBy(
      analyticsEvents(),
      (event) => (event.geo_country ? countryLabel(event.geo_country) : ''),
      6,
    ),
  );
  const citySlices = createMemo(() =>
    countBy(
      analyticsEvents(),
      (event) =>
        event.geo_city
          ? `${event.geo_country ? `${event.geo_country} · ` : ''}${event.geo_city}`
          : '',
      4,
    ),
  );
  const uaSlices = createMemo(() => uaBreakdown(analyticsEvents(), 5));

  const recent = createMemo(() =>
    [...clusterEvents()]
      .sort((a, b) => b.timestamp.localeCompare(a.timestamp))
      .slice(0, 8),
  );

  const payload = createMemo(() => {
    let requestChars = 0;
    let responseChars = 0;
    let latencySum = 0;
    let latencyCount = 0;
    for (const event of clusterEvents()) {
      requestChars += requestSize(event);
      responseChars += responseSize(event);
      if (event.latency_ms && event.latency_ms > 0) {
        latencySum += event.latency_ms;
        latencyCount += 1;
      }
    }
    const top = [...clusterEvents()]
      .filter((event) => responseSize(event) > 0)
      .sort((a, b) => responseSize(b) - responseSize(a))
      .slice(0, 3);
    return {
      requestChars,
      responseChars,
      avgLatency: latencyCount > 0 ? Math.round(latencySum / latencyCount) : 0,
      top,
    };
  });

  const online = createMemo(
    () => clusters().filter((cluster) => cluster.status === 'online').length,
  );
  const pending = createMemo(
    () => clusters().filter((cluster) => cluster.status === 'pending').length,
  );
  const offline = createMemo(
    () => clusters().filter((cluster) => cluster.status === 'offline').length,
  );
  const activeRobots = createMemo(
    () => robots().filter((robot) => robot.status === 'active').length,
  );
  const totalOps = createMemo(() => trend().reduce((sum, point) => sum + point.ops, 0));
  const totalBlocked = createMemo(() =>
    trend().reduce((sum, point) => sum + point.blocked, 0),
  );
  const totalRequests = createMemo(() => totalOps() + totalBlocked());
  const denyRate = createMemo(() =>
    totalRequests() > 0 ? (totalBlocked() / totalRequests()) * 100 : 0,
  );
  const hasTrendData = createMemo(() =>
    trend().some((point) => point.logins > 0 || point.ops > 0 || point.blocked > 0),
  );

  const activeUsers = createMemo(
    () =>
      new Set(
        analyticsEvents()
          .filter((event) => event.user && event.user !== 'unknown')
          .map((event) => event.user),
      ).size,
  );
  const activeGroups = createMemo(
    () =>
      new Set(
        analyticsEvents().flatMap((event) =>
          event.groups
            ? event.groups
                .split(',')
                .map((group) => group.trim())
                .filter((group) => Boolean(group) && group !== 'robot')
            : [],
        ),
      ).size,
  );
  const countryCount = createMemo(
    () =>
      new Set(
        analyticsEvents()
          .map((event) => event.geo_country)
          .filter(Boolean),
      ).size,
  );

  const palette = createMemo(() => chartPalette(colorScheme() === 'dark'));
  const trendChart = createMemo(() => trendOption(trend(), palette()));
  const donutChart = createMemo(() =>
    donutOption(
      verbSlices().map((slice) => ({ name: slice.name, value: slice.value })),
      palette(),
    ),
  );
  const barChart = createMemo(() => resourceBarOption(resourceSlices(), palette()));

  const recentColumns: TableColumnsType<AuditEvent> = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 150,
      render: (value) => dayjs(String(value)).format('MM-DD HH:mm:ss'),
    },
    {
      title: '用户',
      dataIndex: 'user',
      key: 'user',
      width: 130,
      render: (value) =>
        value ? (
          <code class="mono audit-actor" title={String(value)}>
            {String(value)}
          </code>
        ) : (
          '—'
        ),
    },
    {
      title: '集群',
      dataIndex: 'cluster',
      key: 'cluster',
      width: 110,
      render: (value) => <code class="mono">{String(value ?? '') || '—'}</code>,
    },
    {
      title: '动作',
      dataIndex: 'verb',
      key: 'verb',
      width: 90,
      render: (value) => (value ? <Tag level="primary">{String(value)}</Tag> : '—'),
    },
    {
      title: '资源',
      key: 'resource',
      width: 170,
      ellipsis: true,
      render: (_value, record) => <code class="mono">{resourceName(record)}</code>,
    },
    {
      title: '来源',
      key: 'source',
      width: 190,
      render: (_value, record) => (
        <div class="table-source">
          <span>
            {[record.geo_country, record.geo_city].filter(Boolean).join(' · ') || '—'}
          </span>
          <code class="mono">{record.source_ip || '—'}</code>
        </div>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status_code',
      key: 'status',
      width: 80,
      render: (value) => {
        const code = Number(value);
        if (!code) return '—';
        return (
          <Tag level={code >= 500 ? 'error' : code >= 400 ? 'warning' : 'success'}>
            {code}
          </Tag>
        );
      },
    },
    {
      title: '耗时',
      dataIndex: 'latency_ms',
      key: 'latency',
      width: 84,
      align: 'right',
      render: (value) => (value ? `${value} ms` : '—'),
    },
    {
      title: '请求/响应',
      key: 'payload',
      width: 120,
      render: (_value, record) => (
        <span class="mono dashboard-size">
          {fmtChars(requestSize(record))} / {fmtChars(responseSize(record))}
        </span>
      ),
    },
    {
      title: '',
      key: 'detail',
      width: 82,
      fixed: 'right',
      render: (_value, record) => (
        <Button
          type="link"
          size="small"
          icon={<EyeOutlined class="h-3.5 w-3.5" />}
          onClick={() => setDetail(record)}
        >
          详情
        </Button>
      ),
    },
  ];

  const tab = createMemo<DashTab>(() => {
    const value = param(searchParams.tab);
    return value === 'access' || value === 'identity' ? value : 'ops';
  });
  const onTabChange = (key: string) => {
    setSearchParams({ tab: key }, { replace: true });
  };

  return (
    <Show when={auth.isAdmin()} fallback={<Navigate href="/kubeconfig" />}>
      <div>
        <PageHeader
          title={`欢迎回来，${auth.profile()?.nickname || auth.claims()?.sub || '管理员'}`}
          subtitle="平台运行态势"
          tags={<Tag level="primary">管理员</Tag>}
          extra={
            <div class="flex flex-wrap items-center gap-2">
              <Button
                icon={<ReloadOutlined class="h-4 w-4" />}
                onClick={() => void load()}
                loading={loading()}
              >
                刷新
              </Button>
              <Button
                icon={<FileSearchOutlined class="h-4 w-4" />}
                onClick={() => navigate('/audit')}
              >
                审计日志
              </Button>
              <Button
                type="primary"
                icon={<PlusOutlined class="h-4 w-4" />}
                onClick={() => navigate('/clusters')}
              >
                管理集群
              </Button>
            </div>
          }
        />

        <Show when={error()}>
          <Alert
            type="error"
            message="概览加载失败"
            description={error() ?? undefined}
            class="mb-4"
            action={
              <Button size="small" onClick={() => void load()}>
                重试
              </Button>
            }
          />
        </Show>

        <Show
          when={!loading()}
          fallback={
            <div class="stat-grid">
              <For each={[0, 1, 2, 3, 4, 5]}>
                {() => (
                  <Card>
                    <Skeleton paragraph={{ rows: 1 }} />
                  </Card>
                )}
              </For>
            </div>
          }
        >
          <div class="stat-grid">
            <Card class="stat-card">
              <div class="stat-head">
                <span class="stat-title">集群总数</span>
                <ClusterOutlined class="stat-icon blue" />
              </div>
              <div class="stat-value">{clusters().length}</div>
              <div class="stat-foot">
                {online()} 在线 · {pending()} 待接入 · {offline()} 离线
              </div>
            </Card>
            <Card class="stat-card">
              <div class="stat-head">
                <span class="stat-title">在线隧道</span>
                <ApiOutlined class="stat-icon green" />
              </div>
              <div class="stat-value" style={{ color: 'var(--private-color-success)' }}>
                {online()}
              </div>
              <div class="stat-foot">Agent 反向隧道已建立</div>
            </Card>
            <Card class="stat-card">
              <div class="stat-head">
                <span class="stat-title">机器人</span>
                <RobotOutlined class="stat-icon violet" />
              </div>
              <div class="stat-value">{activeRobots()}</div>
              <div class="stat-foot">共 {robots().length} 个 CICD 机器人</div>
            </Card>
            <Card class="stat-card">
              <div class="stat-head">
                <span class="stat-title">策略</span>
                <SafetyCertificateOutlined class="stat-icon amber" />
              </div>
              <div class="stat-value">{policyCount()}</div>
              <div class="stat-foot">已配置访问策略</div>
            </Card>
            <Card class="stat-card">
              <div class="stat-head">
                <span class="stat-title">近 7 天操作</span>
                <ThunderboltOutlined class="stat-icon blue" />
              </div>
              <div class="stat-value">{totalOps()}</div>
              <div class="stat-foot">
                拦截 {totalBlocked()} · 拦截率 {denyRate().toFixed(1)}%
              </div>
            </Card>
            <Card class="stat-card">
              <div class="stat-head">
                <span class="stat-title">活跃身份</span>
                <ApiOutlined class="stat-icon violet" />
              </div>
              <div class="stat-value">{activeUsers()}</div>
              <div class="stat-foot">
                {activeGroups()} 个组 · {countryCount()} 个国家/地区
              </div>
            </Card>
          </div>

          <Tabs
            class="dashboard-tabs"
            activeKey={tab()}
            onChange={onTabChange}
            items={[
              { key: 'ops', label: '运行态势' },
              { key: 'access', label: '访问分析' },
              { key: 'identity', label: '身份与客户端' },
            ]}
          />

          <Show when={tab() === 'ops'}>
            <>
              <div class="dashboard-row cols-main">
                <Card
                  title="近 7 天访问趋势"
                  class="soft-card dashboard-chart-card"
                  extra={
                    <span class="chart-summary">
                      操作 {totalOps()} · 拦截 {totalBlocked()} · 登录{' '}
                      {trend().reduce((sum, point) => sum + point.logins, 0)}
                    </span>
                  }
                >
                  <Show
                    when={hasTrendData()}
                    fallback={<Empty description="最近 7 天暂无审计数据" class="border-0 py-12" />}
                  >
                    <div class="dashboard-chart">
                      <EChart option={trendChart()} />
                    </div>
                  </Show>
                </Card>
                <Card title="系统信息" class="soft-card dashboard-panel-card">
                  <div class="runtime-grid">
                    <div class="runtime-metric">
                      <div class="runtime-icon">
                        <CodeOutlined class="h-4 w-4" />
                      </div>
                      <div class="runtime-label">Go 版本</div>
                      <div class="runtime-value mono">{system()?.go_version ?? '—'}</div>
                    </div>
                    <div class="runtime-metric">
                      <div class="runtime-icon">
                        <ClockCircleOutlined class="h-4 w-4" />
                      </div>
                      <div class="runtime-label">运行时长</div>
                      <div class="runtime-value">{fmtUptime(system()?.uptime_seconds)}</div>
                    </div>
                    <div class="runtime-metric">
                      <div class="runtime-icon">
                        <BranchesOutlined class="h-4 w-4" />
                      </div>
                      <div class="runtime-label">Goroutines</div>
                      <div class="runtime-value">{system()?.goroutines ?? '—'}</div>
                    </div>
                  </div>
                  <div class="info-row">
                    <span class="info-label">堆内存</span>
                    <span class="info-value">{fmtBytes(system()?.heap_alloc_bytes)}</span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">DB 连接</span>
                    <span class="info-value">
                      {system()
                        ? `${system()!.db_in_use} / ${system()!.db_open_connections}`
                        : '—'}
                    </span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">审计样本</span>
                    <span class="info-value">{analyticsEvents().length} 条</span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">拦截率</span>
                    <span class="info-value">{denyRate().toFixed(1)}%</span>
                  </div>
                </Card>
              </div>

              <div class="dashboard-row">
                <Card
                  title="最近 K8s 请求"
                  class="soft-card"
                  extra={
                    <Button type="link" size="small" onClick={() => navigate('/audit')}>
                      查看全部
                    </Button>
                  }
                  contentClass="p-0"
                >
                  <Show
                    when={recent().length > 0}
                    fallback={<Empty description="最近 7 天暂无集群操作" class="m-4" />}
                  >
                    <Table<AuditEvent>
                      rowKey="id"
                      columns={recentColumns}
                      dataSource={recent()}
                      pagination={false}
                      scroll={{ x: 1200 }}
                    />
                  </Show>
                </Card>
              </div>
            </>
          </Show>

          <Show when={tab() === 'access'}>
            <div class="dashboard-row cols-3">
              <Card
                title="K8s 动作分布"
                class="soft-card dashboard-chart-card"
                extra={<span class="chart-summary">共 {clusterEvents().length} 次</span>}
              >
                <Show
                  when={verbSlices().length > 0}
                  fallback={<Empty description="暂无 K8s 动作数据" class="border-0 py-12" />}
                >
                  <div class="dashboard-donut">
                    <EChart option={donutChart()} />
                  </div>
                  <div class="donut-legend">
                    <For each={verbSlices()}>
                      {(slice, index) => (
                        <span>
                          <i
                            style={{
                              background:
                                palette().palette[index() % palette().palette.length],
                            }}
                          />
                          {slice.name}
                          <b>{slice.value}</b>
                        </span>
                      )}
                    </For>
                  </div>
                </Show>
              </Card>
              <Card
                title="访问地理分布"
                class="soft-card dashboard-panel-card"
                extra={<span class="chart-summary">{countryCount()} 个国家/地区</span>}
              >
                <RankList items={countrySlices()} empty="暂无 GeoIP 数据" />
                <Show when={citySlices().length > 0}>
                  <div class="mini-divider" />
                  <div class="mini-heading">热门城市</div>
                  <div class="flex flex-wrap gap-1.5">
                    <For each={citySlices()}>
                      {(city) => (
                        <Tag>
                          {city.name} · {city.value}
                        </Tag>
                      )}
                    </For>
                  </div>
                </Show>
              </Card>
              <Card
                title="资源访问热点"
                class="soft-card dashboard-panel-card"
                extra={<span class="chart-summary">Top {resourceSlices().length}</span>}
              >
                <Show
                  when={resourceSlices().length > 0}
                  fallback={<Empty description="暂无资源访问数据" class="border-0 py-12" />}
                >
                  <div class="dashboard-bar">
                    <EChart option={barChart()} />
                  </div>
                </Show>
              </Card>
            </div>
          </Show>

          <Show when={tab() === 'identity'}>
            <div class="dashboard-row cols-3">
              <Card
                title="用户与组活跃度"
                class="soft-card dashboard-panel-card"
                extra={<span class="chart-summary">K8s 操作</span>}
              >
                <div class="dashboard-split">
                  <div>
                    <div class="mini-heading">操作用户 Top 5</div>
                    <RankList items={userSlices()} empty="暂无操作用户" compact />
                  </div>
                  <div>
                    <div class="mini-heading">活跃组 Top 5</div>
                    <RankList items={groupSlices()} empty="暂无组信息" compact />
                  </div>
                </div>
              </Card>
              <Card
                title="客户端 / UA"
                class="soft-card dashboard-panel-card"
                extra={
                  <span class="chart-summary">{analyticsEvents().length} 次请求</span>
                }
              >
                <RankList items={uaSlices()} empty="暂无 UA 数据" />
              </Card>
              <Card title="请求 / 响应体量" class="soft-card dashboard-panel-card">
                <div class="payload-grid">
                  <div class="payload-metric">
                    <span>请求体量</span>
                    <strong>
                      {payload().requestChars
                        ? `${fmtChars(payload().requestChars)} 字符`
                        : '—'}
                    </strong>
                  </div>
                  <div class="payload-metric">
                    <span>响应体量</span>
                    <strong>
                      {payload().responseChars
                        ? `${fmtChars(payload().responseChars)} 字符`
                        : '—'}
                    </strong>
                  </div>
                  <div class="payload-metric">
                    <span>平均耗时</span>
                    <strong>
                      {payload().avgLatency ? `${payload().avgLatency} ms` : '—'}
                    </strong>
                  </div>
                  <div class="payload-metric">
                    <span>最大响应</span>
                    <strong>
                      {fmtChars(
                        payload().top[0] ? responseSize(payload().top[0]) : undefined,
                      )}
                    </strong>
                  </div>
                </div>
                <div class="mini-heading">响应体最大 Top 3</div>
                <Show
                  when={payload().top.length > 0}
                  fallback={<Empty description="暂无报文数据" class="border-0 py-3" />}
                >
                  <div class="payload-top">
                    <For each={payload().top}>
                      {(event) => (
                        <div class="payload-row">
                          <Tooltip title={event.path || resourceName(event)}>
                            <span class="mini-name">
                              {resourceName(event)} · {event.verb || event.method}
                            </span>
                          </Tooltip>
                          <span class="mini-count">{fmtChars(responseSize(event))}</span>
                        </div>
                      )}
                    </For>
                  </div>
                </Show>
              </Card>
            </div>
          </Show>
        </Show>

        <AuditDetailDrawer event={detail()} onClose={() => setDetail(null)} />
      </div>
    </Show>
  );
}
