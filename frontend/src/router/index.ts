import { createRouter, createWebHashHistory } from 'vue-router'
import OverviewPage from '../pages/OverviewPage.vue'
import LogsPage from '../pages/LogsPage.vue'
import SessionsPage from '../pages/SessionsPage.vue'
import ContactPage from '../pages/ContactPage.vue'
import MonitoringPage from '../pages/MonitoringPage.vue'
import ModelsPage from '../pages/ModelsPage.vue'
import ProxyPage from '../pages/ProxyPage.vue'

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      redirect: '/overview',
    },
    {
      path: '/overview',
      name: 'overview',
      component: OverviewPage,
    },
    {
      path: '/models',
      name: 'models',
      component: ModelsPage,
    },
    {
      path: '/proxy',
      name: 'proxy',
      component: ProxyPage,
    },
    {
      path: '/logs',
      name: 'logs',
      component: LogsPage,
    },
    {
      path: '/sessions',
      name: 'sessions',
      component: SessionsPage,
    },
    {
      path: '/monitoring',
      name: 'monitoring',
      component: MonitoringPage,
    },
    {
      path: '/contact',
      name: 'contact',
      component: ContactPage,
    },
  ],
})
