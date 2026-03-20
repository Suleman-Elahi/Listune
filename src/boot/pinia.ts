// Feature: xp-music-player
// Requirements: 1.2
// Explicitly create and install Pinia so stores are available app-wide.

import { defineBoot } from '#q-app/wrappers';
import { createPinia } from 'pinia';

export default defineBoot(({ app }) => {
  app.use(createPinia());
});
