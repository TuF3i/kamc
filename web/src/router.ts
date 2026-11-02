import { createRouter, createWebHistory } from 'vue-router';
import { useAuthStore } from '@/stores/auth';

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/auth/login',
      name: 'login',
      component: () => import('@/pages/Login.vue'),
      meta: { public: true, title: '登录' },
    },
    {
      path: '/',
      name: 'home',
      component: () => import('@/pages/Home.vue'),
      meta: { title: '概览' },
    },
    {
      path: '/clusters',
      name: 'clusters',
      component: () => import('@/pages/Clusters.vue'),
      meta: { title: '集群', admin: true },
    },
    {
      path: '/clusters/:id',
      name: 'cluster-detail',
      component: () => import('@/pages/ClusterDetail.vue'),
      meta: { title: '集群详情', admin: true },
    },
    {
      path: '/access',
      name: 'access',
      component: () => import('@/pages/Access.vue'),
      meta: { title: '权限管理', admin: true },
    },
    {
      path: '/scripts',
      name: 'scripts',
      component: () => import('@/pages/Scripts.vue'),
      meta: { title: '绑定脚本', admin: true },
    },
    {
      path: '/scripts/:name',
      name: 'script-edit',
      component: () => import('@/pages/ScriptEdit.vue'),
      meta: { title: '编辑脚本', admin: true },
    },
    {
      path: '/audit',
      name: 'audit',
      component: () => import('@/pages/Audit.vue'),
      meta: { title: '审计', admin: true },
    },
    {
      path: '/account',
      name: 'account',
      component: () => import('@/pages/Account.vue'),
      meta: { title: '个人中心' },
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
});

router.beforeEach(async (to) => {
  const auth = useAuthStore();
  if (!auth.checked) {
    auth.adoptExistingToken();
    await auth.restore();
  }
  if (to.meta.public) {
    return auth.loggedIn ? { path: '/' } : true;
  }
  if (!auth.loggedIn) {
    return { path: '/auth/login', query: { redirect: to.fullPath } };
  }
  if (to.meta.admin && !auth.isAdmin) {
    return { path: '/' };
  }
  return true;
});

router.afterEach((to) => {
  const base = 'kamc · 访问管理中心';
  document.title = to.meta.title ? `${to.meta.title} · ${base}` : base;
});

export default router;
