<template>
  <div class="login-page">
    <div class="login-card">
      <div class="app-name">Listune</div>
      <div class="app-tagline">Your music, everywhere</div>
      <button class="google-btn" @click="loginWithGoogle">
        <svg class="google-icon" viewBox="0 0 24 24" width="20" height="20">
          <path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/>
          <path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/>
          <path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/>
          <path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/>
        </svg>
        <span>Sign in with Google</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { backend } from 'src/services/backend';

function loginWithGoogle(): void {
  backend.loginGoogle();
}
</script>

<style scoped>
.login-page {
  position: fixed;
  inset: 0;
  background: #111318;
  display: flex;
  align-items: center;
  justify-content: center;
}

.login-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 48px 40px;
}

.app-name {
  font-size: 42px;
  font-weight: 800;
  color: #fff;
  letter-spacing: -0.02em;
}

.app-tagline {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.4);
  margin-bottom: 24px;
}

.google-btn {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 24px;
  background: #fff;
  border: none;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 500;
  color: #333;
  cursor: pointer;
  transition: box-shadow 0.15s ease, transform 0.1s ease;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
}

.google-btn:hover {
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
  transform: translateY(-1px);
}

.google-btn:active {
  transform: translateY(0);
}

.google-icon {
  flex-shrink: 0;
}
</style>
