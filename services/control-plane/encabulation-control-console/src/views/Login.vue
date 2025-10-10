<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function handleLogin() {
  error.value = ''
  loading.value = true
  
  try {
    await authStore.login(username.value, password.value)
    
    const redirect = route.query.redirect as string || '/'
    router.push(redirect)
  } catch (err) {
    error.value = 'Invalid credentials'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex items-center justify-center p-4">
    <div class="w-full max-w-md">
      <div class="card">
        <div class="text-center mb-8">
          <h1 class="text-3xl font-bold text-primary mb-2">
            🔧 Encabulation Control
          </h1>
          <p class="text-text-secondary">
            Sign in to access the admin portal
          </p>
        </div>
        
        <form @submit.prevent="handleLogin" class="space-y-4">
          <div>
            <label for="username" class="block text-sm font-medium mb-2">
              Username
            </label>
            <input
              id="username"
              v-model="username"
              type="text"
              required
              class="input w-full"
              placeholder="Enter username"
            />
          </div>
          
          <div>
            <label for="password" class="block text-sm font-medium mb-2">
              Password
            </label>
            <input
              id="password"
              v-model="password"
              type="password"
              required
              class="input w-full"
              placeholder="Enter password"
            />
          </div>
          
          <div v-if="error" class="text-danger text-sm">
            {{ error }}
          </div>
          
          <button
            type="submit"
            :disabled="loading"
            class="btn btn-primary w-full"
          >
            {{ loading ? 'Signing in...' : 'Sign In' }}
          </button>
        </form>
        
        <div class="mt-6 text-center space-y-2">
          <div class="text-text-muted text-sm">or</div>
          <router-link to="/" class="text-sm text-primary hover:underline block">
            Continue as Guest (read-only, 24h session)
          </router-link>
        </div>
      </div>
    </div>
  </div>
</template>
