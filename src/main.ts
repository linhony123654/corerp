import { createApp } from 'vue'
import App from './App.vue'
import PlayWorkspace from './components/PlayWorkspace.vue'
import './styles/fonts.css'
import './styles/base.css'

// Historical fixture/inspector remains an explicit demo entry, never the Play default.
createApp(location.pathname === '/demo' ? App : PlayWorkspace).mount('#app')
