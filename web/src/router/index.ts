import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useNavContext } from '@/composables/useNavContext'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      name: 'dashboard',
      component: () => import('@/views/DashboardView.vue'),
    },
    {
      path: '/producciones',
      name: 'producciones',
      component: () => import('@/views/ProduccionesView.vue'),
    },
    {
      path: '/produccion/:id',
      name: 'produccion',
      component: () => import('@/views/ProduccionView.vue'),
    },
    {
      path: '/produccion/:id/poligono',
      name: 'poligono',
      component: () => import('@/views/PoligonoView.vue'),
    },
    {
      path: '/produccion/:produccionId/escena/:escenaId',
      name: 'escena',
      component: () => import('@/views/EscenaView.vue'),
    },
    {
      path: '/alertas',
      name: 'alertas',
      component: () => import('@/views/AlertasView.vue'),
    },
    {
      path: '/configuracion',
      name: 'configuracion',
      component: () => import('@/views/ConfiguracionView.vue'),
    },
  ],
})

// Guard global: rutas no públicas requieren sesión activa
router.beforeEach((to) => {
  const auth = useAuthStore()
  if (!to.meta.public && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
    return { name: 'dashboard' }
  }
})

// Registra de dónde sale cada navegación para que los listados puedan
// posicionarse en el último elemento visitado. Va en un guard y no en los
// botones para que funcione con cualquier salida, incluido el botón atrás
// del navegador.
router.afterEach((to, from) => {
  const { cameFromProduccion, cameFromEscena } = useNavContext()

  if (from.name === 'produccion') {
    cameFromProduccion.value = Number(from.params.id)
    cameFromEscena.value = null
  } else if (from.name === 'escena') {
    cameFromProduccion.value = Number(from.params.produccionId)
    cameFromEscena.value = Number(from.params.escenaId)
  } else if (from.name === 'poligono') {
    cameFromProduccion.value = Number(from.params.id)
  }

  // Al entrar a una escena distinta, el origen dentro del listado pasa a ser
  // esa escena; así el detalle de producción marca la última que se abrió.
  if (to.name === 'escena') {
    cameFromEscena.value = Number(to.params.escenaId)
    cameFromProduccion.value = Number(to.params.produccionId)
  }
})

export default router
