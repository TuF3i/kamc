import { useEffect, useMemo, useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Row,
  Skeleton,
  Space,
  Statistic,
  Table,
  Tag,
  Tooltip,
  type TableColumnsType,
} from 'antd';
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
} from '@ant-design/icons';
import {
  Area,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ComposedChart,
  Legend,
  Line,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip as ChartTooltip,
  XAxis,
  YAxis,
} from 'recharts';
import dayjs from 'dayjs';
import { useAuthStore } from '@/stores/auth';
import { listClusters } from '@/api/clusters';
import { listAudit } from '@/api/audit';
import { listPolicies } from '@/api/access';
import { listRobots } from '@/api/robots';
import { fetchSystemInfo } from '@/api/system';
import type { AuditEvent, Cluster, Robot, SystemInfo } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { AuditDetailDrawer } from '@/components/AuditDetailDrawer';
import { fmtUptime } from '@/utils/format';

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

const CHART_COLORS = {
  ops: '#4c9aff',
  blocked: '#f87171',
  logins: '#34d399',
  grid: '#1a2a3e',
  axis: '#8aa0b8',
  palette: ['#4c9aff', '#34d399', '#fbbf24', '#a78bfa', '#f87171', '#22d3ee', '#f472b6', '#94a3b8'],
};

const chartTooltipStyle = {
  background: '#0d1826',
  border: '1px solid #1e2e44',
  borderRadius: 8,
  color: '#dbe7f5',
  fontSize: 12,
};

const chartAxisTick = { fill: CHART_COLORS.axis, fontSize: 12 };

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
  const days = Array.from({ length: 7 }, (_, i) => dayjs().subtract(6 - i, 'day'));
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
  if (/k8s-am-login/i.test(ua)) return 'k8s-am-login';
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

function RankList({
  items,
  empty,
  unit = '次',
  compact = false,
}: {
  items: RankItem[];
  empty: string;
  unit?: string;
  compact?: boolean;
}) {
  if (items.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={empty} style={{ padding: '18px 0' }} />;
  }
  const max = Math.max(...items.map((item) => item.value), 1);
  return (
    <div className="mini-list">
      {items.map((item) => (
        <div className={`mini-row${compact ? ' compact' : ''}`} key={item.name}>
          <Tooltip title={item.raw || item.name}>
            <span className="mini-name">{item.name}</span>
          </Tooltip>
          {compact ? null : (
            <div className="mini-bar">
              <span style={{ width: `${Math.max(6, (item.value / max) * 100)}%` }} />
            </div>
          )}
          <span className="mini-count">
            {item.value}
            {unit}
            {item.denied ? <em className="mini-denied"> · {item.denied} 拦截</em> : null}
          </span>
        </div>
      ))}
    </div>
  );
}

export function Dashboard() {
  const navigate = useNavigate();
  const claims = useAuthStore((s) => s.claims);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [robots, setRobots] = useState<Robot[]>([]);
  const [policyCount, setPolicyCount] = useState(0);
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [system, setSystem] = useState<SystemInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [detail, setDetail] = useState<AuditEvent | null>(null);

  const load = async () => {
    if (!isAdmin) return;
    setLoading(true);
    setError(null);
    try {
      const [clusterList, systemInfo, robotList, policies, clusterAudit, platformAudit] =
        await Promise.all([
          listClusters(),
          fetchSystemInfo().catch(() => null),
          listRobots().catch(() => []),
          listPolicies().then((list) => list.length).catch(() => 0),
          listAudit({ kind: 'cluster', event: 'k8s-api', limit: 1000 }).catch(() => []),
          listAudit({ kind: 'platform', limit: 1000 }).catch(() => []),
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

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAdmin]);

  const windowEvents = useMemo(() => {
    const cutoff = dayjs().startOf('day').subtract(6, 'day');
    return audit.filter((event) => !dayjs(event.timestamp).isBefore(cutoff));
  }, [audit]);
  const platformEvents = useMemo(
    () => windowEvents.filter((event) => event.kind === 'platform' && event.event !== 'tunnel-attached'),
    [windowEvents],
  );
  // 只统计数据面代理请求（k8s-api）；历史隧道/后台事件不算访问画像
  const clusterEvents = useMemo(
    () => windowEvents.filter((event) => event.kind === 'cluster' && event.event === 'k8s-api'),
    [windowEvents],
  );
  const analyticsEvents = useMemo(
    () => [...clusterEvents, ...platformEvents],
    [clusterEvents, platformEvents],
  );

  const trend = useMemo(() => buildTrend(analyticsEvents), [analyticsEvents]);
  const verbSlices = useMemo(
    () => countBy(clusterEvents, (event) => event.verb, 6),
    [clusterEvents],
  );
  const resourceSlices = useMemo(() => resourceBreakdown(clusterEvents, 6), [clusterEvents]);
  const userSlices = useMemo(
    () => actorBreakdown(clusterEvents, (event) => (event.user && event.user !== 'unknown' ? event.user : ''), 5),
    [clusterEvents],
  );
  const groupSlices = useMemo(
    () =>
      actorBreakdown(
        clusterEvents,
        (event) =>
          event.groups
            ? event.groups
                .split(',')
                .map((group) => group.trim())
                .filter((group) => Boolean(group) && group !== 'robot')
            : '',
        5,
      ),
    [clusterEvents],
  );
  const countrySlices = useMemo(
    () =>
      countBy(
        analyticsEvents,
        (event) => (event.geo_country ? countryLabel(event.geo_country) : ''),
        6,
      ),
    [analyticsEvents],
  );
  const citySlices = useMemo(
    () =>
      countBy(
        analyticsEvents,
        (event) =>
          event.geo_city
            ? `${event.geo_country ? `${event.geo_country} · ` : ''}${event.geo_city}`
            : '',
        4,
      ),
    [analyticsEvents],
  );
  const uaSlices = useMemo(() => uaBreakdown(analyticsEvents, 5), [analyticsEvents]);

  const recent = useMemo(
    () =>
      [...clusterEvents]
        .sort((a, b) => b.timestamp.localeCompare(a.timestamp))
        .slice(0, 8),
    [clusterEvents],
  );

  const payload = useMemo(() => {
    let requestChars = 0;
    let responseChars = 0;
    let latencySum = 0;
    let latencyCount = 0;
    for (const event of clusterEvents) {
      requestChars += event.request_body?.length ?? 0;
      responseChars += event.response_body?.length ?? 0;
      if (event.latency_ms && event.latency_ms > 0) {
        latencySum += event.latency_ms;
        latencyCount += 1;
      }
    }
    const top = [...clusterEvents]
      .filter((event) => event.response_body)
      .sort((a, b) => (b.response_body?.length ?? 0) - (a.response_body?.length ?? 0))
      .slice(0, 3);
    return {
      requestChars,
      responseChars,
      avgLatency: latencyCount > 0 ? Math.round(latencySum / latencyCount) : 0,
      top,
    };
  }, [clusterEvents]);

  const online = clusters.filter((cluster) => cluster.status === 'online').length;
  const pending = clusters.filter((cluster) => cluster.status === 'pending').length;
  const offline = clusters.filter((cluster) => cluster.status === 'offline').length;
  const activeRobots = robots.filter((robot) => robot.status === 'active').length;
  const totalOps = trend.reduce((sum, point) => sum + point.ops, 0);
  const totalBlocked = trend.reduce((sum, point) => sum + point.blocked, 0);
  const totalRequests = totalOps + totalBlocked;
  const denyRate = totalRequests > 0 ? (totalBlocked / totalRequests) * 100 : 0;
  const hasTrendData = trend.some((point) => point.logins > 0 || point.ops > 0 || point.blocked > 0);

  const activeUsers = new Set(
    analyticsEvents
      .filter((event) => event.user && event.user !== 'unknown')
      .map((event) => event.user),
  ).size;
  const activeGroups = new Set(
    analyticsEvents.flatMap((event) =>
      event.groups
        ? event.groups
            .split(',')
            .map((group) => group.trim())
            .filter((group) => Boolean(group) && group !== 'robot')
        : [],
    ),
  ).size;
  const countryCount = new Set(analyticsEvents.map((event) => event.geo_country).filter(Boolean)).size;

  const recentColumns: TableColumnsType<AuditEvent> = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      width: 150,
      render: (value: string) => dayjs(value).format('MM-DD HH:mm:ss'),
    },
    {
      title: '用户',
      dataIndex: 'user',
      key: 'user',
      width: 130,
      render: (value: string) => <code className="mono">{value || '—'}</code>,
    },
    {
      title: '集群',
      dataIndex: 'cluster',
      key: 'cluster',
      width: 110,
      render: (value: string) => <code className="mono">{value || '—'}</code>,
    },
    {
      title: '动作',
      dataIndex: 'verb',
      key: 'verb',
      width: 90,
      render: (value: string) => (value ? <Tag color="blue">{value}</Tag> : '—'),
    },
    {
      title: '资源',
      key: 'resource',
      width: 170,
      ellipsis: true,
      render: (_, record) => <code className="mono">{resourceName(record)}</code>,
    },
    {
      title: '来源',
      key: 'source',
      width: 190,
      render: (_, record) => (
        <div className="table-source">
          <span>{[record.geo_country, record.geo_city].filter(Boolean).join(' · ') || '—'}</span>
          <code className="mono">{record.source_ip || '—'}</code>
        </div>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status_code',
      key: 'status',
      width: 80,
      render: (value?: number) => {
        if (!value) return '—';
        const color = value >= 500 ? 'error' : value >= 400 ? 'warning' : 'success';
        return <Tag color={color}>{value}</Tag>;
      },
    },
    {
      title: '耗时',
      dataIndex: 'latency_ms',
      key: 'latency',
      width: 84,
      align: 'right',
      render: (value?: number) => (value ? `${value} ms` : '—'),
    },
    {
      title: '请求/响应',
      key: 'payload',
      width: 120,
      render: (_, record) => (
        <span className="mono dashboard-size">
          {fmtChars(record.request_body?.length)} / {fmtChars(record.response_body?.length)}
        </span>
      ),
    },
    {
      title: '',
      key: 'detail',
      width: 82,
      fixed: 'right',
      render: (_, record) => (
        <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => setDetail(record)}>
          详情
        </Button>
      ),
    },
  ];

  if (!isAdmin) {
    return <Navigate to="/kubeconfig" replace />;
  }

  return (
    <div>
      <PageHeader
        title={`欢迎回来，${claims?.sub ?? '管理员'}`}
        subtitle="集群运行状态与近 7 天访问画像：地理、资源、动作、身份与客户端。"
        tags={<Tag color="blue">管理员</Tag>}
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={() => void load()} loading={loading}>
              刷新
            </Button>
            <Button icon={<FileSearchOutlined />} onClick={() => navigate('/audit')}>
              审计日志
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/clusters')}>
              管理集群
            </Button>
          </Space>
        }
      />

      {error ? (
        <Alert
          type="error"
          showIcon
          message="概览加载失败"
          description={error}
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" onClick={() => void load()}>
              重试
            </Button>
          }
        />
      ) : null}

      {loading ? (
        <Row gutter={[16, 16]}>
          {[0, 1, 2, 3, 4, 5].map((index) => (
            <Col xs={12} lg={8} xl={4} key={index}>
              <Card>
                <Skeleton active paragraph={{ rows: 1 }} />
              </Card>
            </Col>
          ))}
        </Row>
      ) : (
        <>
          <Row gutter={[16, 16]}>
            <Col xs={12} lg={8} xl={4}>
              <Card className="stat-card">
                <Statistic
                  title="集群总数"
                  value={clusters.length}
                  prefix={<ClusterOutlined className="stat-icon blue" />}
                />
                <div className="stat-foot">
                  {online} 在线 · {pending} 待接入 · {offline} 离线
                </div>
              </Card>
            </Col>
            <Col xs={12} lg={8} xl={4}>
              <Card className="stat-card">
                <Statistic
                  title="在线隧道"
                  value={online}
                  valueStyle={{ color: '#34d399' }}
                  prefix={<ApiOutlined className="stat-icon green" />}
                />
                <div className="stat-foot">Agent 反向隧道已建立</div>
              </Card>
            </Col>
            <Col xs={12} lg={8} xl={4}>
              <Card className="stat-card">
                <Statistic
                  title="机器人"
                  value={activeRobots}
                  prefix={<RobotOutlined className="stat-icon violet" />}
                />
                <div className="stat-foot">共 {robots.length} 个 CICD 机器人</div>
              </Card>
            </Col>
            <Col xs={12} lg={8} xl={4}>
              <Card className="stat-card">
                <Statistic
                  title="策略"
                  value={policyCount}
                  prefix={<SafetyCertificateOutlined className="stat-icon amber" />}
                />
                <div className="stat-foot">已配置访问策略</div>
              </Card>
            </Col>
            <Col xs={12} lg={8} xl={4}>
              <Card className="stat-card">
                <Statistic
                  title="近 7 天操作"
                  value={totalOps}
                  prefix={<ThunderboltOutlined className="stat-icon blue" />}
                />
                <div className="stat-foot">
                  拦截 {totalBlocked} · 拦截率 {denyRate.toFixed(1)}%
                </div>
              </Card>
            </Col>
            <Col xs={12} lg={8} xl={4}>
              <Card className="stat-card">
                <Statistic
                  title="活跃身份"
                  value={activeUsers}
                  prefix={<ApiOutlined className="stat-icon violet" />}
                />
                <div className="stat-foot">
                  {activeGroups} 个组 · {countryCount} 个国家/地区
                </div>
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]} className="dashboard-row">
            <Col xs={24} xl={16}>
              <Card
                title="近 7 天访问趋势"
                className="soft-card dashboard-chart-card"
                extra={
                  <span className="chart-summary">
                    操作 {totalOps} · 拦截 {totalBlocked} · 登录{' '}
                    {trend.reduce((sum, point) => sum + point.logins, 0)}
                  </span>
                }
              >
                {hasTrendData ? (
                  <div className="dashboard-chart">
                    <ResponsiveContainer width="100%" height="100%">
                      <ComposedChart data={trend} margin={{ top: 8, right: 12, bottom: 0, left: -12 }}>
                        <defs>
                          <linearGradient id="opsFill" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor={CHART_COLORS.ops} stopOpacity={0.35} />
                            <stop offset="95%" stopColor={CHART_COLORS.ops} stopOpacity={0} />
                          </linearGradient>
                          <linearGradient id="blockedFill" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor={CHART_COLORS.blocked} stopOpacity={0.3} />
                            <stop offset="95%" stopColor={CHART_COLORS.blocked} stopOpacity={0} />
                          </linearGradient>
                        </defs>
                        <CartesianGrid strokeDasharray="3 3" stroke={CHART_COLORS.grid} vertical={false} />
                        <XAxis
                          dataKey="date"
                          tick={chartAxisTick}
                          axisLine={{ stroke: CHART_COLORS.grid }}
                          tickLine={false}
                        />
                        <YAxis
                          yAxisId="left"
                          allowDecimals={false}
                          tick={chartAxisTick}
                          axisLine={false}
                          tickLine={false}
                        />
                        <YAxis
                          yAxisId="right"
                          orientation="right"
                          allowDecimals={false}
                          tick={chartAxisTick}
                          axisLine={false}
                          tickLine={false}
                        />
                        <ChartTooltip contentStyle={chartTooltipStyle} labelStyle={{ color: '#8aa0b8' }} />
                        <Legend wrapperStyle={{ color: '#8aa0b8', fontSize: 12 }} />
                        <Area
                          yAxisId="left"
                          type="monotone"
                          dataKey="ops"
                          name="K8s 操作"
                          stroke={CHART_COLORS.ops}
                          fill="url(#opsFill)"
                          strokeWidth={2}
                        />
                        <Area
                          yAxisId="left"
                          type="monotone"
                          dataKey="blocked"
                          name="操作拦截"
                          stroke={CHART_COLORS.blocked}
                          fill="url(#blockedFill)"
                          strokeWidth={2}
                        />
                        <Line
                          yAxisId="right"
                          type="monotone"
                          dataKey="logins"
                          name="登录次数"
                          stroke={CHART_COLORS.logins}
                          strokeWidth={2}
                          dot={false}
                          activeDot={{ r: 4 }}
                        />
                      </ComposedChart>
                    </ResponsiveContainer>
                  </div>
                ) : (
                  <Empty description="最近 7 天暂无审计数据" style={{ padding: '48px 0' }} />
                )}
              </Card>
            </Col>

            <Col xs={24} xl={8}>
              <Card
                title="K8s 动作分布"
                className="soft-card dashboard-chart-card"
                extra={<span className="chart-summary">共 {clusterEvents.length} 次</span>}
              >
                {verbSlices.length > 0 ? (
                  <>
                    <div className="dashboard-donut">
                      <ResponsiveContainer width="100%" height="100%">
                        <PieChart>
                          <Pie
                            data={verbSlices}
                            dataKey="value"
                            nameKey="name"
                            innerRadius={58}
                            outerRadius={88}
                            paddingAngle={2}
                            stroke="none"
                          >
                            {verbSlices.map((slice, index) => (
                              <Cell
                                key={slice.name}
                                fill={CHART_COLORS.palette[index % CHART_COLORS.palette.length]}
                              />
                            ))}
                          </Pie>
                          <ChartTooltip contentStyle={chartTooltipStyle} />
                        </PieChart>
                      </ResponsiveContainer>
                    </div>
                    <div className="donut-legend">
                      {verbSlices.map((slice, index) => (
                        <span key={slice.name}>
                          <i style={{ background: CHART_COLORS.palette[index % CHART_COLORS.palette.length] }} />
                          {slice.name}
                          <b>{slice.value}</b>
                        </span>
                      ))}
                    </div>
                  </>
                ) : (
                  <Empty description="暂无 K8s 动作数据" style={{ padding: '48px 0' }} />
                )}
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]} className="dashboard-row">
            <Col xs={24} lg={8}>
              <Card
                title="访问地理分布"
                className="soft-card dashboard-panel-card"
                extra={<span className="chart-summary">{countryCount} 个国家/地区</span>}
              >
                <RankList items={countrySlices} empty="暂无 GeoIP 数据" />
                {citySlices.length > 0 ? (
                  <>
                    <div className="mini-divider" />
                    <div className="mini-heading">热门城市</div>
                    <Space size={[6, 6]} wrap>
                      {citySlices.map((city) => (
                        <Tag key={city.name}>
                          {city.name} · {city.value}
                        </Tag>
                      ))}
                    </Space>
                  </>
                ) : null}
              </Card>
            </Col>

            <Col xs={24} lg={8}>
              <Card
                title="资源访问热点"
                className="soft-card dashboard-panel-card"
                extra={<span className="chart-summary">Top {resourceSlices.length}</span>}
              >
                {resourceSlices.length > 0 ? (
                  <div className="dashboard-bar">
                    <ResponsiveContainer width="100%" height="100%">
                      <BarChart
                        data={resourceSlices}
                        layout="vertical"
                        margin={{ top: 0, right: 12, bottom: 0, left: 8 }}
                        barCategoryGap={10}
                      >
                        <CartesianGrid strokeDasharray="3 3" stroke={CHART_COLORS.grid} horizontal={false} />
                        <XAxis type="number" allowDecimals={false} tick={chartAxisTick} axisLine={false} tickLine={false} />
                        <YAxis
                          type="category"
                          dataKey="name"
                          width={104}
                          tick={{ fill: CHART_COLORS.axis, fontSize: 11 }}
                          axisLine={false}
                          tickLine={false}
                        />
                        <ChartTooltip contentStyle={chartTooltipStyle} cursor={{ fill: 'rgba(76,154,255,0.06)' }} />
                        <Legend wrapperStyle={{ color: '#8aa0b8', fontSize: 12 }} />
                        <Bar dataKey="allowed" name="放行" stackId="resource" fill={CHART_COLORS.ops} radius={[0, 4, 4, 0]} />
                        <Bar dataKey="denied" name="拦截" stackId="resource" fill={CHART_COLORS.blocked} radius={[0, 4, 4, 0]} />
                      </BarChart>
                    </ResponsiveContainer>
                  </div>
                ) : (
                  <Empty description="暂无资源访问数据" style={{ padding: '48px 0' }} />
                )}
              </Card>
            </Col>

            <Col xs={24} lg={8}>
              <Card
                title="用户与组活跃度"
                className="soft-card dashboard-panel-card"
                extra={<span className="chart-summary">K8s 操作</span>}
              >
                <div className="dashboard-split">
                  <div>
                    <div className="mini-heading">操作用户 Top 5</div>
                    <RankList items={userSlices} empty="暂无操作用户" compact />
                  </div>
                  <div>
                    <div className="mini-heading">活跃组 Top 5</div>
                    <RankList items={groupSlices} empty="暂无组信息" compact />
                  </div>
                </div>
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]} className="dashboard-row">
            <Col xs={24} lg={8}>
              <Card
                title="客户端 / UA"
                className="soft-card dashboard-panel-card"
                extra={<span className="chart-summary">{analyticsEvents.length} 次请求</span>}
              >
                <RankList items={uaSlices} empty="暂无 UA 数据" />
              </Card>
            </Col>

            <Col xs={24} lg={8}>
              <Card title="请求 / 响应体量" className="soft-card dashboard-panel-card">
                <div className="payload-grid">
                  <div className="payload-metric">
                    <span>请求体量</span>
                    <strong>{payload.requestChars ? `${fmtChars(payload.requestChars)} 字符` : '—'}</strong>
                  </div>
                  <div className="payload-metric">
                    <span>响应体量</span>
                    <strong>{payload.responseChars ? `${fmtChars(payload.responseChars)} 字符` : '—'}</strong>
                  </div>
                  <div className="payload-metric">
                    <span>平均耗时</span>
                    <strong>{payload.avgLatency ? `${payload.avgLatency} ms` : '—'}</strong>
                  </div>
                  <div className="payload-metric">
                    <span>最大响应</span>
                    <strong>{fmtChars(payload.top[0]?.response_body?.length)}</strong>
                  </div>
                </div>
                <div className="mini-heading">响应体最大 Top 3</div>
                {payload.top.length > 0 ? (
                  <div className="payload-top">
                    {payload.top.map((event) => (
                      <div className="payload-row" key={event.id}>
                        <Tooltip title={event.path || resourceName(event)}>
                          <span className="mini-name">
                            {resourceName(event)} · {event.verb || event.method}
                          </span>
                        </Tooltip>
                        <span className="mini-count">{fmtChars(event.response_body?.length)}</span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无报文数据" style={{ padding: '10px 0' }} />
                )}
              </Card>
            </Col>

            <Col xs={24} lg={8}>
              <Card title="系统信息" className="soft-card dashboard-panel-card">
                <div className="runtime-grid">
                  <div className="runtime-metric">
                    <div className="runtime-icon">
                      <CodeOutlined />
                    </div>
                    <div className="runtime-label">Go 版本</div>
                    <div className="runtime-value mono">{system?.go_version ?? '—'}</div>
                  </div>
                  <div className="runtime-metric">
                    <div className="runtime-icon">
                      <ClockCircleOutlined />
                    </div>
                    <div className="runtime-label">运行时长</div>
                    <div className="runtime-value">{fmtUptime(system?.uptime_seconds)}</div>
                  </div>
                  <div className="runtime-metric">
                    <div className="runtime-icon">
                      <BranchesOutlined />
                    </div>
                    <div className="runtime-label">Goroutines</div>
                    <div className="runtime-value">{system?.goroutines ?? '—'}</div>
                  </div>
                </div>
                <div className="info-row">
                  <span className="info-label">堆内存</span>
                  <span className="info-value">{fmtBytes(system?.heap_alloc_bytes)}</span>
                </div>
                <div className="info-row">
                  <span className="info-label">DB 连接</span>
                  <span className="info-value">
                    {system ? `${system.db_in_use} / ${system.db_open_connections}` : '—'}
                  </span>
                </div>
                <div className="info-row">
                  <span className="info-label">审计样本</span>
                  <span className="info-value">{analyticsEvents.length} 条</span>
                </div>
                <div className="info-row">
                  <span className="info-label">拦截率</span>
                  <span className="info-value">{denyRate.toFixed(1)}%</span>
                </div>
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]} className="dashboard-row">
            <Col span={24}>
              <Card
                title="最近 K8s 请求"
                className="soft-card"
                extra={
                  <Button type="link" size="small" onClick={() => navigate('/audit')}>
                    查看全部
                  </Button>
                }
                styles={{ body: { padding: 0 } }}
              >
                {recent.length > 0 ? (
                  <Table<AuditEvent>
                    rowKey="id"
                    columns={recentColumns}
                    dataSource={recent}
                    pagination={false}
                    scroll={{ x: 1200 }}
                    size="middle"
                  />
                ) : (
                  <Empty description="最近 7 天暂无集群操作" style={{ padding: '32px 0' }} />
                )}
              </Card>
            </Col>
          </Row>
        </>
      )}

      <AuditDetailDrawer event={detail} onClose={() => setDetail(null)} />
    </div>
  );
}
