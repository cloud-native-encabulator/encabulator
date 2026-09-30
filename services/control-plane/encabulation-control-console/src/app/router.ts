/**
 * Routing.
 *
 * `createWebHistory`, not hash history, because the design asks for deep links
 * that survive a refresh; the BFF's history fallback is what makes that work
 * (see `registerSpa` in `src/server/app.ts`). Views are lazy chunks so the
 * Factory Floor paints without waiting for the Administration page.
 */

import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

export const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'factory-floor',
    component: () => import('@/views/FactoryFloor.vue'),
    meta: { title: 'Factory Floor' },
  },
  {
    path: '/grammeters',
    name: 'grammeters',
    component: () => import('@/views/Grammeters.vue'),
    meta: { title: 'Grammeters' },
  },
  {
    path: '/malaise',
    name: 'malaise',
    component: () => import('@/views/Malaise.vue'),
    meta: { title: 'Malaise' },
  },
  {
    path: '/scenarios',
    name: 'scenarios',
    component: () => import('@/views/Scenarios.vue'),
    meta: { title: 'Scenarios' },
  },
  {
    path: '/girdlespring',
    name: 'girdlespring',
    component: () => import('@/views/Girdlespring.vue'),
    meta: { title: 'Girdlespring' },
  },
  {
    path: '/works',
    name: 'works',
    component: () => import('@/views/Works.vue'),
    meta: { title: 'Works' },
  },
  {
    path: '/administration',
    name: 'administration',
    component: () => import('@/views/Administration.vue'),
    meta: { title: 'Administration' },
  },
  {
    path: '/sign-in',
    name: 'sign-in',
    component: () => import('@/views/SignIn.vue'),
    meta: { title: 'Sign in' },
  },
  // Anything else is not a page. The console says so rather than redirecting
  // silently, which would hide a broken link in a talk.
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/NotFound.vue'),
    meta: { title: 'No such plate' },
  },
]

export function createConsoleRouter() {
  const router = createRouter({ history: createWebHistory(), routes })
  router.afterEach((to) => {
    const title = typeof to.meta.title === 'string' ? to.meta.title : 'Console'
    document.title = `${title} — Encabulation Control Console`
  })
  return router
}
