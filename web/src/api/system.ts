import { apiGet } from './http';
import type { SystemInfo } from './types';

/** 平台进程运行时信息（管理面）。 */
export function fetchSystemInfo(): Promise<SystemInfo> {
  return apiGet<SystemInfo>('/api/v1/system');
}
