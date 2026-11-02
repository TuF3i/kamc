import { apiGet, apiPost } from './http';
import type { Claims, IssuedCredential } from './types';

export interface DevTokenInput {
  sub: string;
  email?: string;
  groups?: string[];
}

export interface PasswordLoginInput {
  username: string;
  password: string;
}

/** 内置账号密码登录（默认 sysadmin/sysadmin123）。 */
export function passwordLogin(input: PasswordLoginInput): Promise<IssuedCredential> {
  return apiPost<IssuedCredential>('/auth/password-login', input);
}

/** 开发直签（仅 AM_DEV_ISSUE_TOKEN=1 时可用）。 */
export function devToken(input: DevTokenInput): Promise<IssuedCredential> {
  return apiPost<IssuedCredential>('/auth/dev-token', input);
}

/** 当前 JWT claims。 */
export function fetchMe(): Promise<Claims> {
  return apiGet<Claims>('/auth/me');
}

/** 登出：当前 jti 进入黑名单。 */
export function logout(): Promise<{ logout: boolean }> {
  return apiPost<{ logout: boolean }>('/auth/logout');
}
