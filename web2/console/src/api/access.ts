import { apiDelete, apiGet, apiPost } from './http';
import type { CreatePolicyInput, CreatePolicyResult, PolicyDTO } from './types';

export interface RemovePolicyInput {
  sub: string;
  dom: string;
  obj: string;
  act: string;
}

export function listPolicies(): Promise<PolicyDTO[]> {
  return apiGet<PolicyDTO[]>('/api/v1/policies');
}

export function createPolicies(input: CreatePolicyInput): Promise<CreatePolicyResult> {
  return apiPost<CreatePolicyResult>('/api/v1/policies', input);
}

/** 删除策略：使用后端原始四元组精确匹配。 */
export function deletePolicy(input: RemovePolicyInput): Promise<void> {
  return apiDelete<void>('/api/v1/policies', input);
}
