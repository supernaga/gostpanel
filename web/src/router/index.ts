import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '../stores/user'

const router = createRouter({
  history: createWebHistory(),
  // 滚动行为
  scrollBehavior(_to, _from, savedPosition) {
    if (savedPosition) {
      return savedPosition
    } else {
      return { top: 0 }
    }
  },
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/Login.vue'),
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('../views/Register.vue'),
    },
    {
      path: '/verify-email',
      name: 'verify-email',
      component: () => import('../views/VerifyEmail.vue'),
    },
    {
      path: '/forgot-password',
      name: 'forgot-password',
      component: () => import('../views/ForgotPassword.vue'),
    },
    {
      path: '/reset-password',
      name: 'reset-password',
      component: () => import('../views/ResetPassword.vue'),
    },
    {
      path: '/',
      component: () => import('../views/Layout.vue'),
      children: [
        {
          path: '',
          name: 'dashboard',
          component: () => import('../views/Dashboard.vue'),
        },
        {
          path: 'nodes',
          name: 'nodes',
          component: () => import('../views/Nodes.vue'),
        },
        {
          path: 'clients',
          name: 'clients',
          component: () => import('../views/Clients.vue'),
        },
        {
          path: 'users',
          name: 'users',
          component: () => import('../views/Users.vue'),
        },
        {
          path: 'notify',
          name: 'notify',
          component: () => import('../views/Notify.vue'),
        },
        {
          path: 'port-forwards',
          name: 'port-forwards',
          component: () => import('../views/PortForwards.vue'),
        },
        {
          path: 'node-groups',
          name: 'node-groups',
          component: () => import('../views/NodeGroups.vue'),
        },
        {
          path: 'proxy-chains',
          name: 'proxy-chains',
          component: () => import('../views/ProxyChains.vue'),
        },
        {
          path: 'tunnels',
          name: 'tunnels',
          component: () => import('../views/Tunnels.vue'),
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('../views/Settings.vue'),
        },
        {
          path: 'operation-logs',
          name: 'operation-logs',
          component: () => import('../views/OperationLogs.vue'),
        },
        {
          path: 'plans',
          name: 'plans',
          component: () => import('../views/Plans.vue'),
        },
        {
          path: 'rules',
          name: 'rules',
          component: () => import('../views/Rules.vue'),
        },
        {
          path: 'change-password',
          name: 'change-password',
          component: () => import('../views/ChangePassword.vue'),
        },
      ],
    },
  ],
})

// 公开页面（不需要登录）
const publicPages = ['login', 'register', 'verify-email', 'forgot-password', 'reset-password']

// 管理员专用页面
const adminOnlyPages = ['users', 'settings', 'notify', 'operation-logs', 'plans', 'rules']

// 路由守卫
router.beforeEach((to, _from, next) => {
  const token = localStorage.getItem('token')
  const isPublicPage = publicPages.includes(to.name as string)

  if (!isPublicPage && !token) {
    next({ name: 'login' })
  } else if (!isPublicPage && token) {
    // 检查是否访问管理员专用页面
    const isAdminPage = adminOnlyPages.includes(to.name as string)
    if (isAdminPage) {
      const userStore = useUserStore()
      // 如果 user 信息未加载，先放行（会在页面加载后由 API 返回 403）
      // 如果已加载且不是 admin，则重定向
      if (userStore.user && userStore.user.role !== 'admin') {
        next({ name: 'dashboard' })
        return
      }
    }
    next()
  } else {
    next()
  }
})

// 路由错误处理
router.onError((error) => {
  // 处理懒加载错误 - 可能是网络问题或资源更新
  if (error.message.includes('Failed to fetch dynamically imported module') ||
      error.message.includes('Loading chunk') ||
      error.message.includes('Loading CSS chunk')) {
    console.warn('Chunk loading error, reloading page...')
    window.location.reload()
  }
})

export default router
