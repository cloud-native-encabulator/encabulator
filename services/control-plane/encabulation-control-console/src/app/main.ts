/**
 * SPA entry point.
 */

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { createConsoleRouter } from './router'
import './styles/theme.css'

const app = createApp(App)
app.use(createPinia())
app.use(createConsoleRouter())
app.mount('#panel')
