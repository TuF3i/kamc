import { apiDelete, apiGet, apiPost } from './http';
import type { RoleDTO } from './types';

export interface CreateRoleInput {
  name: string;
  desc?: string;
}

export function listRoles(): Promise<RoleDTO[]> {
  return apiGet<RoleDTO[]>('/api/v1/roles');
}

export function createRole(input: CreateRoleInput): Promise<RoleDTO> {
  return apiPost<RoleDTO>('/api/v1/roles', input);
}

export function deleteRole(name: string): Promise<void> {
  // chi 对路径中的 %3A 解码不一致；角色名里的冒号保持字面量
  const segment = encodeURIComponent(name).replace(/%3A/gi, ':');
  return apiDelete<void>(`/api/v1/roles/${segment}`);
}
