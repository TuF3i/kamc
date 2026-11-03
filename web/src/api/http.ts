import type { Envelope } from './types';

export const TOKEN_KEY = 'kamc_token';

export class ApiError extends Error {
  readonly httpStatus: number;
  readonly code: number;

  constructor(message: string, httpStatus: number, code = 0) {
    super(message);
    this.name = 'ApiError';
    this.httpStatus = httpStatus;
    this.code = code;
  }

  get isUnauthorized(): boolean {
    return this.httpStatus === 401;
  }

  get isForbidden(): boolean {
    return this.httpStatus === 403;
  }
}

export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) ?? '';
  } catch {
    return '';
  }
}

export function setToken(token: string): void {
  try {
    if (token) {
      localStorage.setItem(TOKEN_KEY, token);
    } else {
      localStorage.removeItem(TOKEN_KEY);
    }
  } catch {
    // 隐私模式下 localStorage 可能不可用；内存态仍由 store 维护
  }
}

function redirectToLogin(): void {
  if (window.location.pathname !== '/login') {
    window.location.assign('/login?expired=1');
  }
}

function buildHeaders(init?: HeadersInit): Headers {
  const headers = new Headers(init);
  headers.set('Accept', 'application/json');
  const token = getToken();
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  return headers;
}

async function parseError(resp: Response): Promise<ApiError> {
  let text = '';
  try {
    text = (await resp.text()).trim();
  } catch {
    // ignore
  }
  if (resp.status === 401) {
    setToken('');
    redirectToLogin();
    return new ApiError('登录已过期，请重新登录', 401);
  }

  // 业务错误通常是 HTTP 200 的信封；系统错误可能是 HTTP 500 + JSON 信封
  const contentType = resp.headers.get('Content-Type') ?? '';
  if (contentType.includes('application/json') && text) {
    try {
      const body = JSON.parse(text) as {
        status?: number;
        message?: string;
        data?: { error?: string };
      };
      const detail = body.data?.error;
      return new ApiError(
        detail || body.message || `请求失败（HTTP ${resp.status}）`,
        resp.status,
        body.status ?? 0,
      );
    } catch {
      // 落到纯文本分支
    }
  }
  return new ApiError(text || `请求失败（HTTP ${resp.status}）`, resp.status);
}

/** 调用平台业务 API（统一 status/message/data 信封）。 */
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = buildHeaders(init.headers);
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  let resp: Response;
  try {
    resp = await fetch(path, { ...init, headers });
  } catch {
    throw new ApiError('网络连接失败，请确认平台服务可用', 0);
  }

  if (!resp.ok) {
    throw await parseError(resp);
  }

  const contentType = resp.headers.get('Content-Type') ?? '';
  if (!contentType.includes('application/json')) {
    const text = (await resp.text()).trim();
    throw new ApiError(text || '响应格式异常', resp.status);
  }

  const body = (await resp.json()) as Envelope<T>;
  if (body.status !== 0) {
    throw new ApiError(body.message || '请求失败', resp.status, body.status);
  }
  return body.data as T;
}

export function apiGet<T>(path: string): Promise<T> {
  return api<T>(path);
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return api<T>(path, {
    method: 'POST',
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function apiPut<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, {
    method: 'PUT',
    body: JSON.stringify(body),
  });
}

export function apiPatch<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, {
    method: 'PATCH',
    body: JSON.stringify(body),
  });
}

export function apiDelete<T>(path: string, body?: unknown): Promise<T> {
  return api<T>(path, {
    method: 'DELETE',
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

/** 下载二进制/文本文件（kubeconfig 等），处理 Content-Disposition 文件名。 */
export async function downloadFile(path: string, fallbackName: string): Promise<void> {
  const headers = new Headers();
  const token = getToken();
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  let resp: Response;
  try {
    resp = await fetch(path, { headers });
  } catch {
    throw new ApiError('网络连接失败，请确认平台服务可用', 0);
  }

  if (!resp.ok) {
    throw await parseError(resp);
  }

  const blob = await resp.blob();
  const disposition = resp.headers.get('Content-Disposition') ?? '';
  const match = /filename="?([^";]+)"?/i.exec(disposition);
  const filename = match?.[1] ?? fallbackName;

  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/**
 * 数据面请求：/proxy/{clusterID}/... 返回原生 Kubernetes JSON，不是平台信封。
 * 普通 HTTP 请求的 JWT 只认 Authorization 头。
 */
export async function kubeGet<T>(clusterId: string, path: string): Promise<T> {
  const headers = new Headers();
  headers.set('Accept', 'application/json');
  const token = getToken();
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  let resp: Response;
  try {
    resp = await fetch(`/proxy/${encodeURIComponent(clusterId)}${path}`, { headers });
  } catch {
    throw new ApiError('无法连接数据面，请确认集群隧道在线', 0);
  }

  if (!resp.ok) {
    throw await parseError(resp);
  }
  return (await resp.json()) as T;
}

/** 组装 query string，自动跳过空值。 */
export function queryString(params: Record<string, string | number | undefined | null>): string {
  const q = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== '') {
      q.set(key, String(value));
    }
  }
  const text = q.toString();
  return text ? `?${text}` : '';
}
