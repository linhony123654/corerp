import { createApp } from 'vue'
import './styles/fonts.css'
import './styles/base.css'

// Route-level code splitting: the Play client never downloads Studio/demo bundles.
const path = location.pathname
const loader = path === '/demo'
  ? () => import('./App.vue')
  : path === '/studio'
    ? () => import('./components/StudioWorkspace.vue')
    : path === '/studio/create'
      ? () => import('./components/StudioCreate.vue')
      : () => import('./components/PlayWorkspace.vue')

// Historical fixture/inspector remains an explicit demo entry, never the Play default.
void loader().then(({ default: Root }) => createApp(Root).mount('#app'))
