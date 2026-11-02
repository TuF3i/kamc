import { api } from './http';
import type { AuthScript, ScriptExecuteResult } from './types';

// —— 登录期权限绑定脚本（Lua，管理面） ——

/** 脚本列表（按 name 排序 = 登录时执行顺序）；all=1 含禁用脚本 */
export function listScripts(all = true): Promise<AuthScript[]> {
  return api(`/api/v1/auth-scripts${all ? '?all=1' : ''}`);
}

export function getScript(name: string): Promise<AuthScript> {
  return api(`/api/v1/auth-scripts/${encodeURIComponent(name)}`);
}

/** 保存（upsert）：后端先做 Lua 语法编译校验，语法错误不落库（50000） */
export function saveScript(name: string, content: string, enabled: boolean): Promise<AuthScript> {
  return api(`/api/v1/auth-scripts/${encodeURIComponent(name)}`, { method: 'PUT', body: { content, enabled } });
}

export function deleteScript(name: string): Promise<void> {
  return api(`/api/v1/auth-scripts/${encodeURIComponent(name)}`, { method: 'DELETE' });
}

/**
 * 手动执行：claims 缺省用调用者身份；dry_run=true 时桥接只记录动作不落地。
 * 执行失败抛 ApiError（message 与登录 fail-closed 同源）。
 * groups 为数组（契约如此），界面上的逗号分隔串由组件层拆分。
 */
export function executeScript(
  name: string,
  opts: { dryRun: boolean; sub?: string; email?: string; groups?: string[] },
): Promise<ScriptExecuteResult> {
  const q = opts.dryRun ? '?dry_run=1' : '';
  const body: Record<string, unknown> = {};
  if (opts.sub) body.sub = opts.sub;
  if (opts.email) body.email = opts.email;
  if (opts.groups?.length) body.groups = opts.groups;
  return api(`/api/v1/auth-scripts/${encodeURIComponent(name)}/execute${q}`, {
    method: 'POST',
    body: Object.keys(body).length ? body : undefined,
  });
}
