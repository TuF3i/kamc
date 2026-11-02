import dayjs from 'dayjs';

export function fmtTime(iso?: string | null): string {
  if (!iso) return '—';
  const d = dayjs(iso);
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : '—';
}

export function fmtDate(iso?: string | null): string {
  if (!iso) return '—';
  const d = dayjs(iso);
  return d.isValid() ? d.format('YYYY-MM-DD') : '—';
}

/** 把 Unix 秒转换成“X 小时 Y 分”/“Y 分钟”/“已过期”。 */
export function fmtRemaining(expSeconds?: number | null): string {
  if (!expSeconds) return '—';
  const seconds = expSeconds - Math.floor(Date.now() / 1000);
  if (seconds <= 0) return '已过期';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours} 小时 ${minutes} 分`;
  if (minutes > 0) return `${minutes} 分钟`;
  return `${seconds} 秒`;
}

export function remainingSeconds(expSeconds?: number | null): number {
  if (!expSeconds) return 0;
  return Math.max(0, expSeconds - Math.floor(Date.now() / 1000));
}

/** 运行时长（秒）→ “X 天 Y 小时”/“Y 小时 Z 分”/“Z 分钟”。 */
export function fmtUptime(seconds?: number | null): string {
  if (!seconds || seconds <= 0) return '—';
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days} 天 ${hours} 小时`;
  if (hours > 0) return `${hours} 小时 ${minutes} 分`;
  return `${minutes} 分钟`;
}

/** 字节数 → MB/GB。 */
export function fmtBytes(bytes?: number | null): string {
  if (!bytes || bytes <= 0) return '0 MB';
  const mb = bytes / 1024 / 1024;
  if (mb >= 1024) return `${(mb / 1024).toFixed(2)} GB`;
  return `${mb.toFixed(1)} MB`;
}

/** 从 JWT payload 读取 exp（仅用于展示，不用于鉴权判断）。 */
export function decodeExp(token: string): number | null {
  try {
    const payload = token.split('.')[1];
    if (!payload) return null;
    const json = JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/'))) as { exp?: number };
    return typeof json.exp === 'number' ? json.exp : null;
  } catch {
    return null;
  }
}
