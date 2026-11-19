import { createSignal } from 'solid-js';
import { ApiError, getToken, setToken } from '@/api/http';
import {
  devToken,
  fetchMe,
  logout,
  passwordLogin,
  type DevTokenInput,
  type PasswordLoginInput,
} from '@/api/auth';
import { listClusters } from '@/api/clusters';
import { getProfile } from '@/api/users';
import type { Claims, PlatformUserDTO } from '@/api/types';

const [token, setTokenSignal] = createSignal(getToken());
const [claims, setClaims] = createSignal<Claims | null>(null);
const [profile, setProfile] = createSignal<PlatformUserDTO | null>(null);
const [isAdmin, setIsAdmin] = createSignal(false);
const [checked, setChecked] = createSignal(false);
const [restoring, setRestoring] = createSignal(false);
const [error, setError] = createSignal<string | null>(null);

function resetIdentity(): void {
  setClaims(null);
  setProfile(null);
  setIsAdmin(false);
  setChecked(false);
  setError(null);
}

async function restore(): Promise<void> {
  const current = getToken();
  if (!current) {
    setTokenSignal('');
    setClaims(null);
    setProfile(null);
    setIsAdmin(false);
    setChecked(true);
    setRestoring(false);
    setError(null);
    return;
  }

  setTokenSignal(current);
  setRestoring(true);
  setError(null);
  try {
    const nextClaims = await fetchMe();
    let admin = false;
    try {
      await listClusters();
      admin = true;
    } catch (err) {
      // 403 是普通用户的正常结果；其他错误不阻断登录，只是无法确认管理员身份
      if (!(err instanceof ApiError && err.isForbidden)) {
        console.warn('[auth] 管理员身份探测失败', err);
      }
    }
    let nextProfile: PlatformUserDTO | null = null;
    try {
      nextProfile = await getProfile();
    } catch {
      // 资料读取失败不阻断登录，头像退化为用户名首字母
    }
    setClaims(nextClaims);
    setProfile(nextProfile);
    setIsAdmin(admin);
    setChecked(true);
    setRestoring(false);
    setError(null);
  } catch (err) {
    if (err instanceof ApiError && err.isUnauthorized) {
      setTokenSignal('');
      setClaims(null);
      setProfile(null);
      setIsAdmin(false);
      setChecked(true);
      setRestoring(false);
      setError(null);
      return;
    }
    setClaims(null);
    setProfile(null);
    setChecked(true);
    setRestoring(false);
    setError(err instanceof Error ? err.message : '登录状态校验失败');
  }
}

async function refreshProfile(): Promise<void> {
  if (!token()) return;
  try {
    setProfile(await getProfile());
  } catch {
    // 保留旧资料；下一次登录/刷新会再次尝试
  }
}

async function signInWithDevToken(input: DevTokenInput): Promise<void> {
  const credential = await devToken(input);
  setToken(credential.token);
  setTokenSignal(credential.token);
  resetIdentity();
  await restore();
  if (!claims()) {
    throw new Error(error() ?? '登录状态校验失败');
  }
}

async function signInWithPassword(input: PasswordLoginInput): Promise<void> {
  const credential = await passwordLogin(input);
  setToken(credential.token);
  setTokenSignal(credential.token);
  resetIdentity();
  await restore();
  if (!claims()) {
    throw new Error(error() ?? '登录状态校验失败');
  }
}

async function probeAdmin(): Promise<void> {
  await restore();
  if (!claims()) {
    throw new Error(error() ?? '登录状态校验失败');
  }
}

async function signOut(): Promise<void> {
  try {
    await logout();
  } catch {
    // 即使服务端吊销失败（网络/已过期），本地也必须清理
  }
  setToken('');
  setTokenSignal('');
  resetIdentity();
  try {
    sessionStorage.setItem('kamc:logged_out', '1');
  } catch {
    // 隐私模式忽略
  }
  window.location.assign('/#/login?logged_out=1');
}

function clearLocal(): void {
  setToken('');
  setTokenSignal('');
  setClaims(null);
  setProfile(null);
  setIsAdmin(false);
  setChecked(false);
  setError(null);
}

export const auth = {
  token,
  claims,
  profile,
  isAdmin,
  checked,
  restoring,
  error,
  restore,
  refreshProfile,
  signInWithDevToken,
  signInWithPassword,
  probeAdmin,
  signOut,
  clearLocal,
};
