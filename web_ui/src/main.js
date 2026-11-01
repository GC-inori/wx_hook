import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import { initTheme } from './utils/theme'

initTheme()

createApp(App).mount('#app')
