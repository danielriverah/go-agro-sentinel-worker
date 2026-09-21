import './assets/main.css'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import App from './App.vue'
import router from './router'
import es from './locales/es.json'
import en from './locales/en.json'

const savedLocale = localStorage.getItem('agro_locale')
const browserLocale = navigator.language.startsWith('es') ? 'es' : 'en'

const i18n = createI18n({
  legacy: false,
  locale: savedLocale ?? browserLocale,
  fallbackLocale: 'es',
  messages: { es, en },
})

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(i18n)
app.mount('#app')
