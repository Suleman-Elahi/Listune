import type { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    component: () => import('pages/LoginPage.vue'),
    meta: { public: true },
  },
  {
    path: '/google-callback',
    component: () => import('pages/GoogleCallbackPage.vue'),
    meta: { public: true },
  },
  {
    path: '/',
    component: () => import('pages/NowPlayingView.vue'),
  },
  {
    path: '/library',
    component: () => import('pages/LibraryView.vue'),
  },
  {
    path: '/playlists',
    component: () => import('pages/PlaylistsView.vue'),
  },
  {
    path: '/search',
    component: () => import('pages/LibraryView.vue'),
  },
  {
    path: '/:catchAll(.*)*',
    component: () => import('pages/ErrorNotFound.vue'),
    meta: { public: true },
  },
];

export default routes;
