import { create } from 'zustand';
import { ApiError, getToken, setToken } from '@/api/http';
import { devToken, fetchMe, logout, passwordLogin, type DevTokenInput, type PasswordLoginInput } from '@/api/auth';
import { listClusters } from '@/api/clusters';
import type { Claims } from '@/api/types';

interface AuthState {
  token: string;
  claims: Claims | null;
  isAdmin: boolean;
  checked: boolean;
  restoring: boolean;
  error: string | null;
  restore: () => Promise<void>;
  signInWithDevToken: (input: DevTokenInput) => Promise<void>;
  signInWithPassword: (input: PasswordLoginInput) => Promise<void>;
  signOut: () => Promise<void>;
  clearLocal: () => void;
}

export const useAuthStore = create<AuthState>((set, get) => ({
  token: getToken(),
  claims: null,
  isAdmin: false,
  checked: false,
  restoring: false,
  error: null,

  restore: async () => {
    const token = getToken();
    if (!token) {
      set({ token: '', claims: null, isAdmin: false, checked: true, restoring: false, error: null });
      return;
    }

    set({ token, restoring: true, error: null });
    try {
      const claims = await fetchMe();
      let isAdmin = false;
      try {
        await listClusters();
        isAdmin = true;
      } catch (err) {
        // 403 是普通用户的正常结果；其他错误不阻断登录，只是无法确认管理员身份
        if (!(err instanceof ApiError && err.isForbidden)) {
          // eslint-disable-next-line no-console
          console.warn('[auth] 管理员身份探测失败', err);
        }
      }
      set({ claims, isAdmin, checked: true, restoring: false, error: null });
    } catch (err) {
      if (err instanceof ApiError && err.isUnauthorized) {
        set({
          token: '',
          claims: null,
          isAdmin: false,
          checked: true,
          restoring: false,
          error: null,
        });
        return;
      }
      set({
        claims: null,
        checked: true,
        restoring: false,
        error: err instanceof Error ? err.message : '登录状态校验失败',
      });
    }
  },

  signInWithDevToken: async (input) => {
    const credential = await devToken(input);
    setToken(credential.token);
    set({ token: credential.token, claims: null, isAdmin: false, checked: false, error: null });
    await get().restore();
    if (!get().claims) {
      throw new Error(get().error ?? '登录状态校验失败');
    }
  },

  signInWithPassword: async (input) => {
    const credential = await passwordLogin(input);
    setToken(credential.token);
    set({ token: credential.token, claims: null, isAdmin: false, checked: false, error: null });
    await get().restore();
    if (!get().claims) {
      throw new Error(get().error ?? '登录状态校验失败');
    }
  },

  signOut: async () => {
    try {
      await logout();
    } catch {
      // 即使服务端吊销失败（网络/已过期），本地也必须清理
    }
    setToken('');
    set({ token: '', claims: null, isAdmin: false, checked: false, error: null });
    window.location.assign('/login?logged_out=1');
  },

  clearLocal: () => {
    setToken('');
    set({ token: '', claims: null, isAdmin: false, checked: false, error: null });
  },
}));
