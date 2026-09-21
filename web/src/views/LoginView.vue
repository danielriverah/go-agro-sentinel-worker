<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore, apiErrorMessage } from '@/stores/auth'
import type { AxiosError } from 'axios'

const { t, locale } = useI18n()
const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    const redirect = (route.query.redirect as string) || '/'
    router.push(redirect)
  } catch (err) {
    const status = (err as AxiosError).response?.status
    if (status === 401) error.value = t('auth.invalid_credentials')
    else if (status === 403) error.value = t('auth.account_disabled')
    else error.value = apiErrorMessage(err)
  } finally {
    loading.value = false
  }
}

function toggleLocale() {
  locale.value = locale.value === 'es' ? 'en' : 'es'
  localStorage.setItem('agro_locale', locale.value)
}
</script>

<template>
  <div class="min-h-screen bg-gray-50 flex items-center justify-center p-4">
    <div class="w-full max-w-sm">

      <!-- Header -->
      <div class="text-center mb-8">
        <h1 class="text-2xl font-bold text-gray-900">🌱 {{ t('app.name') }}</h1>
        <p class="text-sm text-gray-500 mt-1">{{ t('app.tagline') }}</p>
      </div>

      <!-- Card -->
      <div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6 space-y-4">
        <h2 class="text-lg font-semibold text-gray-800">{{ t('auth.login') }}</h2>

        <form @submit.prevent="submit" class="space-y-4">
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              {{ t('auth.username') }}
            </label>
            <input
              v-model="username"
              type="text"
              autocomplete="username"
              required
              class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-green-500"
            />
          </div>

          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">
              {{ t('auth.password') }}
            </label>
            <input
              v-model="password"
              type="password"
              autocomplete="current-password"
              required
              class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-green-500"
            />
          </div>

          <!-- Error -->
          <p v-if="error" class="text-sm text-red-600 bg-red-50 rounded-lg px-3 py-2">
            {{ error }}
          </p>

          <button
            type="submit"
            :disabled="loading"
            class="w-full bg-green-600 hover:bg-green-700 disabled:opacity-60 text-white font-medium rounded-lg py-2 text-sm transition-colors"
          >
            {{ loading ? t('auth.signing_in') : t('auth.login') }}
          </button>
        </form>
      </div>

      <!-- Language toggle -->
      <div class="text-center mt-4">
        <button @click="toggleLocale" class="text-xs text-gray-400 hover:text-gray-600">
          {{ locale === 'es' ? '🇺🇸 English' : '🇲🇽 Español' }}
        </button>
      </div>

    </div>
  </div>
</template>
