import { Tag, type TagLevel } from '@/lib/widgets/tag';
import type { ClusterStatus, TokenStatus } from '@/api/types';

const CLUSTER_MAP: Record<ClusterStatus, { level: TagLevel; label: string }> = {
  online: { level: 'success', label: '在线' },
  pending: { level: 'warning', label: '待接入' },
  offline: { level: 'error', label: '离线' },
};

const TOKEN_MAP: Record<TokenStatus, { level: TagLevel; label: string }> = {
  active: { level: 'success', label: '有效' },
  rotating: { level: 'warning', label: '轮换中' },
};

const REASON_MAP: Record<string, TagLevel> = {
  allowed: 'success',
  'denied-casbin': 'error',
  'denied-casbin-error': 'error',
  'tunnel-attached': 'info',
  'user-revoked': 'gold',
  'login-success': 'success',
  'login-failed': 'error',
  'binding-script-failed': 'gold',
  'script-failed': 'gold',
};

export function ClusterStatusTag(props: { status: ClusterStatus }) {
  const meta = CLUSTER_MAP[props.status] ?? { level: 'neutral' as TagLevel, label: props.status };
  return <Tag level={meta.level}>{meta.label}</Tag>;
}

export function TokenStatusTag(props: { status: TokenStatus }) {
  const meta = TOKEN_MAP[props.status] ?? { level: 'neutral' as TagLevel, label: props.status };
  return <Tag level={meta.level}>{meta.label}</Tag>;
}

export function EnabledTag(props: { enabled: boolean }) {
  return props.enabled ? <Tag level="success">启用</Tag> : <Tag>禁用</Tag>;
}

export function AuditResultTag(props: { allowed: boolean }) {
  return props.allowed ? <Tag level="success">允许</Tag> : <Tag level="error">拒绝</Tag>;
}

export function ReasonTag(props: { reason: string }) {
  return <Tag level={REASON_MAP[props.reason] ?? 'neutral'}>{props.reason}</Tag>;
}

export function TokenKindTag(props: { kind: 'register' | 'longterm' }) {
  return props.kind === 'register' ? (
    <Tag level="primary">注册 Token</Tag>
  ) : (
    <Tag level="info">长期 Token</Tag>
  );
}
