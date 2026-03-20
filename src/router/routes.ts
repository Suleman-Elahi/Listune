import type { RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
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
    component: () => import('pages/LibraryView.vue'), // reuse library with search focused
  },

  // Always leave this as last one,
  // but you can also remove it
  {
    path: '/:catchAll(.*)*',
    component: () => import('pages/ErrorNotFound.vue'),
  },
];

export default routes;
