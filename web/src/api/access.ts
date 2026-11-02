import { api } from './http';
import type { AddBindingRequest, AddPolicyRequest, BindingRow, PolicyRow } from './types';

// —— Casbin 策略与绑定（管理面） ——

export function listPolicies(): Promise<PolicyRow[]> {
  return api('/api/v1/policies');
}

export function addPolicy(req: AddPolicyRequest): Promise<void> {
  return api('/api/v1/policies', { body: req });
}

export function removePolicy(req: AddPolicyRequest): Promise<void> {
  return api('/api/v1/policies', { method: 'DELETE', body: req });
}

export function listBindings(): Promise<BindingRow[]> {
  return api('/api/v1/bindings');
}

export function addBinding(req: AddBindingRequest): Promise<void> {
  return api('/api/v1/bindings', { body: req });
}

export function removeBinding(req: AddBindingRequest): Promise<void> {
  return api('/api/v1/bindings', { method: 'DELETE', body: req });
}

/** 踢出用户：该时刻之前签发的该用户全部 Token 立即失效（epoch 递增） */
export function revokeUser(sub: string): Promise<void> {
  return api('/api/v1/auth/revoke', { body: { sub } });
}
