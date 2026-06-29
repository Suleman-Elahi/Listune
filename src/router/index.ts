import {
  createRouter,
  createWebHashHistory,
} from 'vue-router';
import routes from './routes';

const Router = createRouter({
  scrollBehavior: () => ({ left: 0, top: 0 }),
  routes,
  history: createWebHashHistory(),
});

// Auth guard: redirect to /login if not authenticated
Router.beforeEach((to) => {
  if (to.meta.public) return;
  try {
    const stored = localStorage.getItem('listune_user');
    if (stored) {
      const user = JSON.parse(stored);
      if (user && user.token) return;
    }
  } catch {
    // corrupted data — clear it
    localStorage.removeItem('listune_user');
  }
  return '/login';
});

export default Router;
