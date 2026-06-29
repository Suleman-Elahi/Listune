<template>
  <div class="callback-page">
    <div class="loading-text">Signing you in...</div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';

const router = useRouter();
const route = useRoute();

onMounted(() => {
  // Server redirects here with query params: ?email=...&name=...&picture=...&token=...
  // Try multiple parsing strategies for robustness
  let params: URLSearchParams;

  // Strategy 1: vue-router already parsed query params
  if (route.query.email && route.query.token) {
    params = new URLSearchParams(route.query as Record<string, string>);
  } else {
    // Strategy 2: parse from hash (hash history mode)
    const hash = window.location.hash; // e.g. #/google-callback?email=...
    const qIdx = hash.indexOf('?');
    if (qIdx !== -1) {
      params = new URLSearchParams(hash.substring(qIdx + 1));
    } else {
      // Strategy 3: parse from full URL search (in case browser puts it there)
      params = new URLSearchParams(window.location.search);
    }
  }

  const email = params.get('email');
  const name = params.get('name');
  const picture = params.get('picture');
  const token = params.get('token');

  if (email && token) {
    localStorage.setItem('listune_user', JSON.stringify({ email, name, picture, token }));
    router.replace('/');
  } else {
    // Auth failed — go back to login
    router.replace('/login');
  }
});
</script>

<style scoped>
.callback-page {
  position: fixed;
  inset: 0;
  background: #111318;
  display: flex;
  align-items: center;
  justify-content: center;
}

.loading-text {
  color: rgba(255, 255, 255, 0.6);
  font-size: 16px;
}
</style>
