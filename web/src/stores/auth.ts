import { defineStore } from 'pinia';
import * as authApi from '@/api/auth';
import { listClusters } from '@/api/clusters';
import { ApiError, getToken, setToken } from '@/api/http';
import type { Claims } from '@/api/types';

/**
 * 登录态：token 存 localStorage（OIDC 浏览器流由后端桥接页写入，dev-token 由本 store 写入）。
 * 管理员判定：平台无法从 claims 直接推断 AM_ADMIN_GROUP 的取值——登录后以
 * GET /api/v1/clusters 探测（403=非管理员），后续管理面路由按此放行。
 */
export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: getToken(),
    claims: null as Claims | null,
    /** 已完成 restore 探测（避免每个路由守卫重复请求） */
    checked: false,
    isAdmin: false,
  }),

  getters: {
    loggedIn: (s) => !!s.token && !!s.claims,
    /** exp 剩余秒数（本地时钟） */
    expiresIn: (s) => (s.claims ? Math.max(0, s.claims.exp - Math.floor(Date.now() / 1000)) : 0),
  },

  actions: {
    /** 应用启动/首次路由时恢复会话：校验 token 并探测管理员身份 */
    async restore(): Promise<boolean> {
      if (!this.token) {
        this.checked = true;
        return false;
      }
      try {
        this.claims = await authApi.me();
        await this.probeAdmin();
        this.checked = true;
        return true;
      } catch (e) {
        // me 探测失败（过期/吊销/网络）一律按未登录处理；401 已在 http 层清理 token
        if (e instanceof ApiError && e.httpStatus !== 401) {
          // 非认证类错误（后端不可达等）：保留 token 但视为未就绪，避免误清登录态
          this.checked = true;
          throw e;
        }
        this.clearLocal();
        this.checked = true;
        return false;
      }
    },

    /** 管理员探测：管理面首端点 200=管理员 / 403=普通用户 */
    async probeAdmin(): Promise<void> {
      try {
        await listClusters();
        this.isAdmin = true;
      } catch (e) {
        if (e instanceof ApiError && e.isForbidden) {
          this.isAdmin = false;
          return;
        }
        throw e;
      }
    },

    /** dev-token 直签登录（开发环境） */
    async signinWithDev(sub: string, email: string, groups: string[]): Promise<void> {
      const cred = await authApi.devToken(sub, email, groups);
      setToken(cred.token);
      this.token = cred.token;
      this.claims = await authApi.me();
      await this.probeAdmin();
      this.checked = true;
    },

    /** OIDC 桥接页已把 token 写入 localStorage；SPA 启动时从这里接手 */
    adoptExistingToken(): boolean {
      this.token = getToken();
      return !!this.token;
    },

    /** 登出：吊销当前 jti → 清本地 → 整页跳回登录页（硬导航，SPA 状态全部重建） */
    async signout(): Promise<void> {
      try {
        await authApi.logout();
      } catch {
        // 吊销失败也照常清理本地（token 可能已过期）
      }
      this.clearLocal();
      location.assign('/login?logged_out=1');
    },

    clearLocal(): void {
      setToken('');
      this.token = '';
      this.claims = null;
      this.isAdmin = false;
      this.checked = false;
    },
  },
});
