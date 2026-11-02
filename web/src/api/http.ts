// HTTP 客户端：信封解包（HTTP 恒 200、业务码在 status）+ 统一 401 处理
import type { Envelopeish } from './types';

export const TOKEN_KEY = 'kamc_token';

export class ApiError extends Error {
  /** HTTP 状态码（信封端点恒 200，中间件错误 401/403/429/5xx） */
  readonly httpStatus: number;
  /** 业务码（纯文本响应无信封时为 0） */
  readonly code: number;

  constructor(message: string, httpStatus: number, code = 0) {
    super(message);
    this.name = 'ApiError';
    this.httpStatus = httpStatus;
    this.code = code;
  }

  /** 管理员组不足（RequireAdmin 403） */
  get isForbidden(): boolean {
    return this.httpStatus === 403;
  }
}

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) || '';
}

export function setToken(token: string): void {
  if (token) {
    localStorage.setItem(TOKEN_KEY, token);
  } else {
    localStorage.removeItem(TOKEN_KEY);
  }
}

/** 未认证统一处理：清凭证回登录页（携带 redirect 供回跳） */
function onUnauthorized(): void {
  setToken('');
  if (!location.pathname.startsWith('/login')) {
    location.assign('/login?expired=1&redirect=' + encodeURIComponent(location.pathname + location.search));
  }
}

export interface ApiOptions {
  method?: string;
  /** 任意 JSON 可序列化请求体（自动序列化） */
  body?: unknown;
}

/** 发起管理面请求并解信封；业务码非 0 / 纯文本错误 / 401 均抛 ApiError */
export async function api<T>(path: string, opts: ApiOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (getToken()) headers.Authorization = 'Bearer ' + getToken();
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json';

  const resp = await fetch(path, {
    method: opts.method ?? (opts.body !== undefined ? 'POST' : 'GET'),
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });

  if (resp.status === 401) {
    onUnauthorized();
    throw new ApiError('未认证或登录已过期', 401);
  }

  const ct = resp.headers.get('content-type') || '';
  if (!ct.includes('application/json')) {
    const text = (await resp.text()).trim();
    throw new ApiError(text || resp.statusText || `HTTP ${resp.status}`, resp.status);
  }

  const env = (await resp.json()) as Envelopeish<T>;
  if (env.status !== 0) {
    throw new ApiError(env.message || `业务错误 ${env.status}`, resp.status, env.status);
  }
  return env.data as T;
}

/** 原始响应请求（kubeconfig YAML 下载等非信封端点），401 同样统一处理 */
export async function apiRaw(path: string): Promise<Response> {
  const headers: Record<string, string> = {};
  if (getToken()) headers.Authorization = 'Bearer ' + getToken();
  const resp = await fetch(path, { headers });
  if (resp.status === 401) {
    onUnauthorized();
    throw new ApiError('未认证或登录已过期', 401);
  }
  if (!resp.ok) {
    throw new ApiError((await resp.text()).trim() || `HTTP ${resp.status}`, resp.status);
  }
  return resp;
}
