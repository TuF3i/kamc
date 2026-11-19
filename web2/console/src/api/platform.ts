import { apiGet } from './http';
import type { PlatformInfo } from './types';

/** 平台信息页（管理面）：配置快照 + 运行时状态。 */
export function getPlatformInfo(): Promise<PlatformInfo> {
  return apiGet<PlatformInfo>('/api/v1/platform/info');
}
