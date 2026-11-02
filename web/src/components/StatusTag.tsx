import { Tag } from 'antd';
import type { ClusterStatus, TokenStatus } from '@/api/types';

type TagColor = 'success' | 'warning' | 'error' | 'processing' | 'default' | 'magenta' | 'blue' | 'green' | 'red' | 'orange';

const CLUSTER_MAP: Record<ClusterStatus, { color: TagColor; label: string }> = {
  online: { color: 'success', label: '在线' },
  pending: { color: 'warning', label: '待接入' },
  offline: { color: 'error', label: '离线' },
};

const TOKEN_MAP: Record<TokenStatus, { color: TagColor; label: string }> = {
  active: { color: 'success', label: '有效' },
  rotating: { color: 'warning', label: '轮换中' },
};

const REASON_MAP: Record<string, TagColor> = {
  allowed: 'success',
  'denied-casbin': 'error',
  'denied-casbin-error': 'error',
  'tunnel-attached': 'blue',
  'user-revoked': 'orange',
  'login-success': 'success',
  'login-failed': 'error',
  'binding-script-failed': 'magenta',
  'script-failed': 'magenta',
};

export function ClusterStatusTag({ status }: { status: ClusterStatus }) {
  const meta = CLUSTER_MAP[status] ?? { color: 'default' as TagColor, label: status };
  return <Tag color={meta.color}>{meta.label}</Tag>;
}

export function TokenStatusTag({ status }: { status: TokenStatus }) {
  const meta = TOKEN_MAP[status] ?? { color: 'default' as TagColor, label: status };
  return <Tag color={meta.color}>{meta.label}</Tag>;
}

export function EnabledTag({ enabled }: { enabled: boolean }) {
  return enabled ? <Tag color="success">启用</Tag> : <Tag>禁用</Tag>;
}

export function AuditResultTag({ allowed }: { allowed: boolean }) {
  return allowed ? <Tag color="success">允许</Tag> : <Tag color="error">拒绝</Tag>;
}

export function ReasonTag({ reason }: { reason: string }) {
  return <Tag color={REASON_MAP[reason] ?? 'default'}>{reason}</Tag>;
}

export function TokenKindTag({ kind }: { kind: 'register' | 'longterm' }) {
  return kind === 'register' ? <Tag color="blue">注册 Token</Tag> : <Tag color="geekblue">长期 Token</Tag>;
}
