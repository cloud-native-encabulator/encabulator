<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'

interface UserData {
  username: string
  email: string
  full_name: string | null
  disabled: boolean
  roles: string[]
  created_at: string
  updated_at: string
  last_login: string | null
}

interface ActivityLog {
  id: string
  username: string
  action: string
  resource: string | null
  timestamp: string
  ip_address: string | null
  success: boolean
}

const authStore = useAuthStore()
const users = ref<UserData[]>([])
const selectedUser = ref<UserData | null>(null)
const userActivity = ref<ActivityLog[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

// Create user form
const showCreateModal = ref(false)
const newUser = ref({
  username: '',
  email: '',
  password: '',
  full_name: '',
  roles: ['viewer'] as string[]
})

// Edit user form
const showEditModal = ref(false)
const editUser = ref({
  email: '',
  full_name: '',
  roles: [] as string[],
  disabled: false
})

const API_BASE = '/api/auth'

async function fetchUsers() {
  loading.value = true
  error.value = null
  try {
    const response = await fetch(`${API_BASE}/users`, {
      headers: { 'Authorization': `Bearer ${authStore.token}` }
    })
    if (!response.ok) throw new Error('Failed to fetch users')
    users.value = await response.json()
  } catch (err: any) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function selectUser(user: UserData) {
  selectedUser.value = user
  await fetchUserActivity(user.username)
}

async function fetchUserActivity(username: string) {
  try {
    const response = await fetch(`${API_BASE}/users/${username}/activity?limit=50`, {
      headers: { 'Authorization': `Bearer ${authStore.token}` }
    })
    if (!response.ok) throw new Error('Failed to fetch activity')
    userActivity.value = await response.json()
  } catch (err) {
    console.error('Failed to fetch activity:', err)
    userActivity.value = []
  }
}

async function createUser() {
  loading.value = true
  error.value = null
  try {
    const response = await fetch(`${API_BASE}/users`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${authStore.token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(newUser.value)
    })
    if (!response.ok) {
      const data = await response.json()
      throw new Error(data.detail || 'Failed to create user')
    }
    showCreateModal.value = false
    newUser.value = { username: '', email: '', password: '', full_name: '', roles: ['viewer'] }
    await fetchUsers()
  } catch (err: any) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function openEditModal(user: UserData) {
  editUser.value = {
    email: user.email,
    full_name: user.full_name || '',
    roles: [...user.roles],
    disabled: user.disabled
  }
  selectedUser.value = user
  showEditModal.value = true
}

async function updateUser() {
  if (!selectedUser.value) return
  loading.value = true
  error.value = null
  try {
    const response = await fetch(`${API_BASE}/users/${selectedUser.value.username}`, {
      method: 'PUT',
      headers: {
        'Authorization': `Bearer ${authStore.token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(editUser.value)
    })
    if (!response.ok) {
      const data = await response.json()
      throw new Error(data.detail || 'Failed to update user')
    }
    showEditModal.value = false
    await fetchUsers()
    if (selectedUser.value) {
      const updated = users.value.find(u => u.username === selectedUser.value!.username)
      if (updated) selectedUser.value = updated
    }
  } catch (err: any) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function deleteUser(username: string) {
  if (!confirm(`Are you sure you want to delete user "${username}"?`)) return
  loading.value = true
  error.value = null
  try {
    const response = await fetch(`${API_BASE}/users/${username}`, {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${authStore.token}` }
    })
    if (!response.ok) {
      const data = await response.json()
      throw new Error(data.detail || 'Failed to delete user')
    }
    if (selectedUser.value?.username === username) {
      selectedUser.value = null
    }
    await fetchUsers()
  } catch (err: any) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

function formatDate(dateString: string | null): string {
  if (!dateString) return 'Never'
  return new Date(dateString).toLocaleString()
}

function getRoleBadgeClass(role: string): string {
  switch (role) {
    case 'admin': return 'bg-red-500/20 text-red-400'
    case 'operator': return 'bg-blue-500/20 text-blue-400'
    case 'viewer': return 'bg-green-500/20 text-green-400'
    default: return 'bg-gray-500/20 text-gray-400'
  }
}

onMounted(() => {
  fetchUsers()
})
</script>

<template>
  <div class="p-8">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold">User Management</h1>
      <button @click="showCreateModal = true" class="btn btn-primary">
        + Create User
      </button>
    </div>

    <!-- Error Message -->
    <div v-if="error" class="bg-danger/20 border border-danger text-danger px-4 py-3 rounded-lg mb-4">
      {{ error }}
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <!-- Users List -->
      <div class="card">
        <h2 class="text-lg font-semibold mb-4">Users</h2>
        <div v-if="loading && users.length === 0" class="text-center py-8 text-text-muted">
          Loading...
        </div>
        <div v-else-if="users.length === 0" class="text-center py-8 text-text-muted">
          No users found
        </div>
        <div v-else class="space-y-2">
          <div
            v-for="user in users"
            :key="user.username"
            @click="selectUser(user)"
            :class="[
              'p-4 rounded-lg border cursor-pointer transition-colors',
              selectedUser?.username === user.username
                ? 'border-primary bg-primary/10'
                : 'border-text-muted/20 hover:border-primary/50'
            ]"
          >
            <div class="flex items-start justify-between">
              <div class="flex-1">
                <div class="font-semibold">{{ user.username }}</div>
                <div class="text-sm text-text-secondary">{{ user.email }}</div>
                <div class="flex gap-2 mt-2">
                  <span
                    v-for="role in user.roles"
                    :key="role"
                    :class="['text-xs px-2 py-1 rounded', getRoleBadgeClass(role)]"
                  >
                    {{ role }}
                  </span>
                  <span v-if="user.disabled" class="text-xs px-2 py-1 rounded bg-gray-500/20 text-gray-400">
                    disabled
                  </span>
                </div>
              </div>
              <div class="flex gap-2">
                <button
                  @click.stop="openEditModal(user)"
                  class="text-primary hover:text-primary/80 text-sm"
                >
                  Edit
                </button>
                <button
                  @click.stop="deleteUser(user.username)"
                  class="text-danger hover:text-danger/80 text-sm"
                  :disabled="user.username === authStore.user?.username"
                >
                  Delete
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- User Details & Activity -->
      <div class="space-y-6">
        <div v-if="selectedUser" class="card">
          <h2 class="text-lg font-semibold mb-4">User Details</h2>
          <dl class="space-y-3">
            <div>
              <dt class="text-sm text-text-secondary">Username</dt>
              <dd class="font-medium">{{ selectedUser.username }}</dd>
            </div>
            <div>
              <dt class="text-sm text-text-secondary">Email</dt>
              <dd class="font-medium">{{ selectedUser.email }}</dd>
            </div>
            <div v-if="selectedUser.full_name">
              <dt class="text-sm text-text-secondary">Full Name</dt>
              <dd class="font-medium">{{ selectedUser.full_name }}</dd>
            </div>
            <div>
              <dt class="text-sm text-text-secondary">Created</dt>
              <dd class="font-medium">{{ formatDate(selectedUser.created_at) }}</dd>
            </div>
            <div>
              <dt class="text-sm text-text-secondary">Last Login</dt>
              <dd class="font-medium">{{ formatDate(selectedUser.last_login) }}</dd>
            </div>
          </dl>
        </div>

        <div v-if="selectedUser" class="card">
          <h2 class="text-lg font-semibold mb-4">Recent Activity</h2>
          <div v-if="userActivity.length === 0" class="text-center py-4 text-text-muted text-sm">
            No activity recorded
          </div>
          <div v-else class="space-y-2 max-h-96 overflow-y-auto">
            <div
              v-for="log in userActivity"
              :key="log.id"
              class="p-3 rounded-lg bg-background border border-text-muted/10"
            >
              <div class="flex items-start justify-between">
                <div class="flex-1">
                  <div class="text-sm font-medium">{{ log.action }}</div>
                  <div v-if="log.resource" class="text-xs text-text-secondary">{{ log.resource }}</div>
                </div>
                <div :class="['w-2 h-2 rounded-full mt-1', log.success ? 'bg-success' : 'bg-danger']"></div>
              </div>
              <div class="text-xs text-text-muted mt-1">
                {{ formatDate(log.timestamp) }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Create User Modal -->
    <div v-if="showCreateModal" class="fixed inset-0 bg-black/50 flex items-center justify-center z-50" @click.self="showCreateModal = false">
      <div class="bg-surface rounded-lg p-6 w-full max-w-md">
        <h2 class="text-xl font-bold mb-4">Create New User</h2>
        <form @submit.prevent="createUser" class="space-y-4">
          <div>
            <label class="block text-sm font-medium mb-1">Username</label>
            <input v-model="newUser.username" type="text" required class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg" />
          </div>
          <div>
            <label class="block text-sm font-medium mb-1">Email</label>
            <input v-model="newUser.email" type="email" required class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg" />
          </div>
          <div>
            <label class="block text-sm font-medium mb-1">Password</label>
            <input v-model="newUser.password" type="password" required minlength="8" class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg" />
          </div>
          <div>
            <label class="block text-sm font-medium mb-1">Full Name (optional)</label>
            <input v-model="newUser.full_name" type="text" class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg" />
          </div>
          <div>
            <label class="block text-sm font-medium mb-1">Roles</label>
            <div class="space-y-2">
              <label class="flex items-center gap-2">
                <input type="checkbox" value="admin" v-model="newUser.roles" />
                <span>Admin</span>
              </label>
              <label class="flex items-center gap-2">
                <input type="checkbox" value="operator" v-model="newUser.roles" />
                <span>Operator</span>
              </label>
              <label class="flex items-center gap-2">
                <input type="checkbox" value="viewer" v-model="newUser.roles" />
                <span>Viewer</span>
              </label>
            </div>
          </div>
          <div class="flex gap-2 justify-end">
            <button type="button" @click="showCreateModal = false" class="btn btn-secondary">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="loading">Create</button>
          </div>
        </form>
      </div>
    </div>

    <!-- Edit User Modal -->
    <div v-if="showEditModal" class="fixed inset-0 bg-black/50 flex items-center justify-center z-50" @click.self="showEditModal = false">
      <div class="bg-surface rounded-lg p-6 w-full max-w-md">
        <h2 class="text-xl font-bold mb-4">Edit User</h2>
        <form @submit.prevent="updateUser" class="space-y-4">
          <div>
            <label class="block text-sm font-medium mb-1">Email</label>
            <input v-model="editUser.email" type="email" required class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg" />
          </div>
          <div>
            <label class="block text-sm font-medium mb-1">Full Name</label>
            <input v-model="editUser.full_name" type="text" class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg" />
          </div>
          <div>
            <label class="block text-sm font-medium mb-1">Roles</label>
            <div class="space-y-2">
              <label class="flex items-center gap-2">
                <input type="checkbox" value="admin" v-model="editUser.roles" />
                <span>Admin</span>
              </label>
              <label class="flex items-center gap-2">
                <input type="checkbox" value="operator" v-model="editUser.roles" />
                <span>Operator</span>
              </label>
              <label class="flex items-center gap-2">
                <input type="checkbox" value="viewer" v-model="editUser.roles" />
                <span>Viewer</span>
              </label>
            </div>
          </div>
          <div>
            <label class="flex items-center gap-2">
              <input type="checkbox" v-model="editUser.disabled" />
              <span>Disabled</span>
            </label>
          </div>
          <div class="flex gap-2 justify-end">
            <button type="button" @click="showEditModal = false" class="btn btn-secondary">Cancel</button>
            <button type="submit" class="btn btn-primary" :disabled="loading">Update</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>
