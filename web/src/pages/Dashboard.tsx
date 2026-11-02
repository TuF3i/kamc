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
  Tag,
} from 'antd';
import {
  ApiOutlined,
  BranchesOutlined,
  ClockCircleOutlined,
  ClusterOutlined,
  CodeOutlined,
  FileSearchOutlined,
  PlusOutlined,
  ReloadOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip as ChartTooltip,
  XAxis,
  YAxis,
} from 'recharts';
import dayjs from 'dayjs';
import { useAuthStore } from '@/stores/auth';
import { listClusters } from '@/api/clusters';
import { listAudit } from '@/api/audit';
import { listRobots } from '@/api/robots';
import { listRoles } from '@/api/roles';
import { fetchSystemInfo } from '@/api/system';
import type { AuditEvent, Cluster, Robot, RoleDTO, SystemInfo } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { fmtUptime } from '@/utils/format';

interface TrendPoint {
  date: string;
  logins: number;
  ops: number;
  blocked: number;
}

interface DayBucket {
  users: Set<string>;
  ops: number;
  blocked: number;
}

function buildTrend(events: AuditEvent[]): TrendPoint[] {
  const days = Array.from({ length: 7 }, (_, i) => dayjs().subtract(6 - i, 'day'));
  const buckets = new Map<string, DayBucket>();
  for (const day of days) {
    buckets.set(day.format('YYYY-MM-DD'), { users: new Set(), ops: 0, blocked: 0 });
  }
  for (const event of events) {
    const key = dayjs(event.timestamp).format('YYYY-MM-DD');
    const bucket = buckets.get(key);
    if (!bucket) continue;
    if (event.reason === 'login-success' && event.user) {
      bucket.users.add(event.user);
    }
    if (event.cluster && event.verb && event.allowed) {
      bucket.ops += 1;
    }
    if (!event.allowed && event.cluster) {
      bucket.blocked += 1;
    }
  }
  return days.map((day) => {
    const bucket = buckets.get(day.format('YYYY-MM-DD')) as DayBucket;
    return {
      date: day.format('MM-DD'),
      logins: bucket.users.size,
      ops: bucket.ops,
      blocked: bucket.blocked,
    };
  });
}

const CHART_COLORS = {
  ops: '#4c9aff',
  blocked: '#f87171',
  logins: '#34d399',
  grid: '#1a2a3e',
  axis: '#8aa0b8',
};

export function Dashboard() {
  const navigate = useNavigate();
  const claims = useAuthStore((s) => s.claims);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const [clusters, setClusters] = useState<Cluster[]>([]);
  const [robots, setRobots] = useState<Robot[]>([]);
  const [roles, setRoles] = useState<RoleDTO[]>([]);
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [system, setSystem] = useState<SystemInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    if (!isAdmin) return;
    setLoading(true);
    setError(null);
    try {
      const [clusterList, systemInfo, robotList, roleList, auditList] = await Promise.all([
        listClusters(),
        fetchSystemInfo().catch(() => null),
        listRobots().catch(() => []),
        listRoles().catch(() => []),
        listAudit({ limit: 1000 }).catch(() => []),
      ]);
      setClusters(clusterList);
      setSystem(systemInfo);
      setRobots(robotList);
      setRoles(roleList);
      setAudit(auditList);
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

  const trend = useMemo(() => buildTrend(audit), [audit]);

  const online = clusters.filter((c) => c.status === 'online').length;
  const pending = clusters.filter((c) => c.status === 'pending').length;
  const offline = clusters.filter((c) => c.status === 'offline').length;
  const activeRobots = robots.filter((r) => r.status === 'active').length;
  const totalOps = trend.reduce((sum, p) => sum + p.ops, 0);
  const totalBlocked = trend.reduce((sum, p) => sum + p.blocked, 0);
  const denyRate = totalOps + totalBlocked > 0 ? (totalBlocked / (totalOps + totalBlocked)) * 100 : 0;
  const hasTrendData = trend.some((p) => p.logins > 0 || p.ops > 0 || p.blocked > 0);

  if (!isAdmin) {
    return <Navigate to="/kubeconfig" replace />;
  }

  return (
    <div>
      <PageHeader
        title={`欢迎回来，${claims?.sub ?? '管理员'}`}
        subtitle="平台运行状态与最近 7 天访问趋势。"
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
          {[0, 1, 2, 3].map((i) => (
            <Col xs={12} lg={6} key={i}>
              <Card>
                <Skeleton active paragraph={{ rows: 1 }} />
              </Card>
            </Col>
          ))}
        </Row>
      ) : (
        <Row gutter={[16, 16]}>
          <Col xs={12} lg={6}>
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
          <Col xs={12} lg={6}>
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
          <Col xs={12} lg={6}>
            <Card className="stat-card">
              <Statistic
                title="机器人"
                value={activeRobots}
                prefix={<RobotOutlined className="stat-icon violet" />}
              />
              <div className="stat-foot">共 {robots.length} 个 CICD 机器人</div>
            </Card>
          </Col>
          <Col xs={12} lg={6}>
            <Card className="stat-card">
              <Statistic
                title="角色"
                value={roles.length}
                prefix={<SafetyCertificateOutlined className="stat-icon amber" />}
              />
              <div className="stat-foot">已定义角色数量</div>
            </Card>
          </Col>
        </Row>
      )}

      <Row gutter={[16, 16]} className="dashboard-main-row">
        <Col xs={24} xl={16}>
          <Card
            title="最近 7 天趋势"
            className="soft-card dashboard-chart-card"
            extra={
              <span className="chart-summary">
                操作 {totalOps} · 拦截 {totalBlocked}
              </span>
            }
          >
            {loading ? (
              <Skeleton active paragraph={{ rows: 8 }} />
            ) : hasTrendData ? (
              <div className="dashboard-chart">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={trend} margin={{ top: 8, right: 12, bottom: 0, left: -12 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke={CHART_COLORS.grid} vertical={false} />
                    <XAxis
                      dataKey="date"
                      tick={{ fill: CHART_COLORS.axis, fontSize: 12 }}
                      axisLine={{ stroke: CHART_COLORS.grid }}
                      tickLine={false}
                    />
                    <YAxis
                      yAxisId="left"
                      allowDecimals={false}
                      tick={{ fill: CHART_COLORS.axis, fontSize: 12 }}
                      axisLine={false}
                      tickLine={false}
                    />
                    <YAxis
                      yAxisId="right"
                      orientation="right"
                      allowDecimals={false}
                      tick={{ fill: CHART_COLORS.axis, fontSize: 12 }}
                      axisLine={false}
                      tickLine={false}
                    />
                    <ChartTooltip
                      contentStyle={{
                        background: '#0d1826',
                        border: '1px solid #1e2e44',
                        borderRadius: 8,
                        color: '#dbe7f5',
                      }}
                      labelStyle={{ color: '#8aa0b8' }}
                    />
                    <Legend wrapperStyle={{ color: '#8aa0b8', fontSize: 12 }} />
                    <Line
                      yAxisId="left"
                      type="monotone"
                      dataKey="ops"
                      name="K8s 操作"
                      stroke={CHART_COLORS.ops}
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                    <Line
                      yAxisId="left"
                      type="monotone"
                      dataKey="blocked"
                      name="操作拦截"
                      stroke={CHART_COLORS.blocked}
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                    <Line
                      yAxisId="right"
                      type="monotone"
                      dataKey="logins"
                      name="登录用户"
                      stroke={CHART_COLORS.logins}
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <Empty
                description="最近 7 天暂无审计数据"
                style={{ padding: '48px 0' }}
              />
            )}
          </Card>
        </Col>

        <Col xs={24} xl={8}>
          <Card title="系统信息" className="soft-card dashboard-info-card">
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
              <span className="info-label">近 7 天操作</span>
              <span className="info-value">{totalOps}</span>
            </div>
            <div className="info-row">
              <span className="info-label">近 7 天拦截</span>
              <span className="info-value danger">{totalBlocked}</span>
            </div>
            <div className="info-row">
              <span className="info-label">拦截率</span>
              <span className="info-value">{denyRate.toFixed(1)}%</span>
            </div>
            <div className="info-row">
              <span className="info-label">审计样本</span>
              <span className="info-value">{audit.length} 条</span>
            </div>
            <div className="info-row">
              <span className="info-label">集群状态</span>
              <span className="info-value">
                <Tag color="success">{online} 在线</Tag>
                <Tag color="warning">{pending} 待接入</Tag>
                <Tag color="error">{offline} 离线</Tag>
              </span>
            </div>
          </Card>
        </Col>
      </Row>
    </div>
  );
}
