import { apiDelete, apiGet, apiPatch, apiPost, apiPut } from './http';
import type { GroupDTO, PlatformUserDTO, UserScript } from './types';

// ── 用户 ──

export function listUsers(): Promise<PlatformUserDTO[]> {
  return apiGet<PlatformUserDTO[]>('/api/v1/users');
}

export interface CreateUserInput {
  username: string;
  nickname?: string;
  email?: string;
  avatar_url?: string;
  groups?: string[];
}

/** 手动创建本地用户（SSO/LDAP 之外的来源）。 */
export function createUser(input: CreateUserInput): Promise<PlatformUserDTO> {
  return apiPost<PlatformUserDTO>('/api/v1/users', input);
}

export interface UpdateUserInput {
  nickname?: string;
  email?: string;
  avatar_url?: string;
  groups?: string[];
}

export function updateUser(username: string, input: UpdateUserInput): Promise<PlatformUserDTO> {
  return apiPatch<PlatformUserDTO>(`/api/v1/users/${encodeURIComponent(username)}`, input);
}

export function deleteUser(username: string): Promise<void> {
  return apiDelete<void>(`/api/v1/users/${encodeURIComponent(username)}`);
}

// ── 组 ──

export function listGroups(): Promise<GroupDTO[]> {
  return apiGet<GroupDTO[]>('/api/v1/groups');
}

export function createGroup(name: string, desc: string): Promise<GroupDTO> {
  return apiPost<GroupDTO>('/api/v1/groups', { name, desc });
}

export function deleteGroup(name: string): Promise<void> {
  return apiDelete<void>(`/api/v1/groups/${encodeURIComponent(name)}`);
}

// ── 映射脚本 ──

export function listScripts(all = true): Promise<UserScript[]> {
  return apiGet<UserScript[]>(`/api/v1/scripts?all=${all ? 1 : 0}`);
}

export function getScript(name: string): Promise<UserScript> {
  return apiGet<UserScript>(`/api/v1/scripts/${encodeURIComponent(name)}`);
}

export function putScript(
  name: string,
  content: string,
  enabled: boolean,
): Promise<UserScript> {
  return apiPut<UserScript>(`/api/v1/scripts/${encodeURIComponent(name)}`, { content, enabled });
}

export function deleteScript(name: string): Promise<void> {
  return apiDelete<void>(`/api/v1/scripts/${encodeURIComponent(name)}`);
}

export interface ScriptRunResult {
  dry_run: boolean;
  actions: { api: string; args: string[] }[];
}

export function runScript(
  name: string,
  dryRun: boolean,
  claims?: { sub?: string; email?: string; groups?: string[]; source?: string },
): Promise<ScriptRunResult> {
  return apiPost<ScriptRunResult>(
    `/api/v1/scripts/${encodeURIComponent(name)}/execute?dry_run=${dryRun ? 1 : 0}`,
    claims ?? {},
  );
}

// ── LDAP 登录 ──

export function ldapLogin(username: string, password: string) {
  return apiPost<{ token: string; expires_at: string; token_type: string }>(
    '/auth/ldap-login',
    { username, password },
  );
}
