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
  const isLoggedIn = !!localStorage.getItem('listune_user');
  if (!to.meta.public && !isLoggedIn) {
    return '/login';
  }
});

export default Router;
