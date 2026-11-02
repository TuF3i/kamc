import { api } from './http';
import type { Claims, IssuedCredential } from './types';

/** 开发直签平台 JWT（仅后端 AM_DEV_ISSUE_TOKEN=1 时可用，否则 404） */
export function devToken(sub: string, email: string, groups: string[]): Promise<IssuedCredential> {
  return api('/auth/dev-token', { body: { sub, email, groups } });
}

/** 当前身份 claims */
export function me(): Promise<Claims> {
  return api('/auth/me');
}

/** 登出（当前 jti 进入黑名单） */
export function logout(): Promise<void> {
  return api('/auth/logout', { method: 'POST' });
}

/** 构建版本信息 */
export function version(): Promise<{ version: string; commit: string; component: string }> {
  return api('/version');
}
