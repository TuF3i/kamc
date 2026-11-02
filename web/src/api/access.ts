import { apiDelete, apiGet, apiPost } from './http';
import type { BindingRow, PolicyRow } from './types';

export interface AddPolicyInput {
  sub: string;
  dom: string;
  obj: string;
  act: string;
}

export interface AddBindingInput {
  user: string;
  role: string;
  dom: string;
}

export function listPolicies(): Promise<PolicyRow[]> {
  return apiGet<PolicyRow[]>('/api/v1/policies');
}

export function addPolicy(input: AddPolicyInput): Promise<void> {
  return apiPost<void>('/api/v1/policies', input);
}

/** 删除策略：字段放在 DELETE body 中。 */
export function deletePolicy(input: AddPolicyInput): Promise<void> {
  return apiDelete<void>('/api/v1/policies', input);
}

export function listBindings(): Promise<BindingRow[]> {
  return apiGet<BindingRow[]>('/api/v1/bindings');
}

export function addBinding(input: AddBindingInput): Promise<void> {
  return apiPost<void>('/api/v1/bindings', input);
}

/** 删除绑定：字段放在 DELETE body 中。 */
export function deleteBinding(input: AddBindingInput): Promise<void> {
  return apiDelete<void>('/api/v1/bindings', input);
}

/** 踢出用户：epoch 递增，使其此前签发的全部 Token 失效。 */
export function revokeUser(sub: string): Promise<{ sub: string; revoked: string }> {
  return apiPost<{ sub: string; revoked: string }>('/api/v1/auth/revoke', { sub });
}
