<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()

function handleLogout() {
  authStore.logout()
  router.push('/login')
}
</script>

<template>
  <div class="min-h-screen flex">
    <!-- Sidebar -->
    <aside class="w-64 bg-surface border-r border-text-muted/20 flex flex-col">
      <div class="p-6 border-b border-text-muted/20">
        <h2 class="text-xl font-bold text-primary">Admin Portal</h2>
        <p class="text-sm text-text-secondary mt-1">
          {{ authStore.user?.username }}
        </p>
      </div>
      
      <nav class="flex-1 p-4 space-y-2">
        <router-link
          to="/admin/users"
          class="block px-4 py-2 rounded-lg hover:bg-background transition-colors"
          active-class="bg-primary text-background"
        >
          Users
        </router-link>
        
        <router-link
          to="/admin/monitoring"
          class="block px-4 py-2 rounded-lg hover:bg-background transition-colors"
          active-class="bg-primary text-background"
        >
          Monitoring
        </router-link>
        
        <router-link
          to="/admin/config"
          class="block px-4 py-2 rounded-lg hover:bg-background transition-colors"
          active-class="bg-primary text-background"
        >
          Configuration
        </router-link>
        
        <router-link
          to="/admin/audit"
          class="block px-4 py-2 rounded-lg hover:bg-background transition-colors"
          active-class="bg-primary text-background"
        >
          Audit Log
        </router-link>
      </nav>
      
      <div class="p-4 border-t border-text-muted/20 space-y-2">
        <router-link to="/" class="block btn btn-secondary w-full text-sm">
          Back to Console
        </router-link>
        <button @click="handleLogout" class="btn btn-danger w-full text-sm">
          Logout
        </button>
      </div>
    </aside>
    
    <!-- Main Content -->
    <main class="flex-1 overflow-auto">
      <router-view />
    </main>
  </div>
</template>
