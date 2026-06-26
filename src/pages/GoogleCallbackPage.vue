<template>
  <div class="callback-page">
    <div class="loading-text">Signing you in...</div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';

const router = useRouter();

onMounted(() => {
  // Server redirects here with query params: ?email=...&name=...&picture=...&token=...
  const params = new URLSearchParams(window.location.hash.split('?')[1] || '');
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
