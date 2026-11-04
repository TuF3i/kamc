import { apiDelete, apiGet, apiPatch, apiPost, apiPut } from './http';
import type { GroupDTO, PlatformUserDTO, UserScript } from './types';

// ── 用户 ──

/** 当前登录用户的平台资料（自服务；头像等展示字段不在 JWT 里）。 */
export function getProfile(): Promise<PlatformUserDTO> {
  return apiGet<PlatformUserDTO>('/api/v1/profile');
}

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

async function readAsDataURL(file: File): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(new Error('读取头像文件失败'));
    reader.readAsDataURL(file);
  });
}

async function postAvatar(path: string, file: File): Promise<string> {
  const { getToken } = await import('./http');
  const res = await fetch(path, {
    method: 'POST',
    headers: {
      'Content-Type': file.type,
      Authorization: `Bearer ${getToken()}`,
    },
    body: file,
  });
  if (!res.ok) {
    throw new Error(res.status === 413 ? '头像文件过大（上限 2MB）' : '头像上传失败');
  }
  const body = (await res.json()) as { data?: { avatar_url?: string } };
  const url = body.data?.avatar_url;
  if (!url) throw new Error('头像上传响应缺少 URL');
  return url;
}

/** 管理面上传指定用户头像（raw image body）；返回带签名 token 的展示 URL。 */
export async function uploadAvatar(username: string, file: File): Promise<string> {
  if (import.meta.env.VITE_APP_MODE === 'mock') {
    const dataUrl = await readAsDataURL(file);
    const { mockSetAvatar } = await import('@/mocks');
    mockSetAvatar(username, dataUrl);
    return dataUrl;
  }
  return postAvatar(`/api/v1/users/${encodeURIComponent(username)}/avatar`, file);
}

/** 当前登录用户修改自己的头像（自服务，仅需 JWT）。 */
export async function uploadAvatarSelf(file: File): Promise<string> {
  if (import.meta.env.VITE_APP_MODE === 'mock') {
    const dataUrl = await readAsDataURL(file);
    const { mockSetAvatar } = await import('@/mocks');
    mockSetAvatar('sysadmin', dataUrl); // mock 固定登录用户
    return dataUrl;
  }
  return postAvatar('/api/v1/profile/avatar', file);
}

// ── 组 ──

export function listGroups(): Promise<GroupDTO[]> {
  return apiGet<GroupDTO[]>('/api/v1/groups');
}

export function createGroup(name: string, desc: string, isAdmin = false): Promise<GroupDTO> {
  return apiPost<GroupDTO>('/api/v1/groups', { name, desc, is_admin: isAdmin });
}

export function deleteGroup(name: string): Promise<void> {
  return apiDelete<void>(`/api/v1/groups/${encodeURIComponent(name)}`);
}

/** 编辑组（重命名会同步用户组字段；引用旧组名的策略需手动调整）。 */
export function updateGroup(
  name: string,
  input: { name?: string; desc?: string; is_admin?: boolean },
): Promise<GroupDTO> {
  return apiPatch<GroupDTO>(`/api/v1/groups/${encodeURIComponent(name)}`, input);
}

export function addGroupMember(group: string, username: string): Promise<void> {
  return apiPost<void>(`/api/v1/groups/${encodeURIComponent(group)}/members`, { username });
}

export function removeGroupMember(group: string, username: string): Promise<void> {
  return apiDelete<void>(
    `/api/v1/groups/${encodeURIComponent(group)}/members/${encodeURIComponent(username)}`,
  );
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
