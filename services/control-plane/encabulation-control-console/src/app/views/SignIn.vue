<script setup lang="ts">
/**
 * Sign in.
 *
 * The password goes to the BFF, which forwards it to the identity service and
 * keeps the resulting token in an httpOnly cookie. No token ever reaches this
 * page, which is why there is nothing here to store.
 */
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useSessionStore } from '@/stores/session'
import Notice from '@/components/panel/Notice.vue'
import PanelPlate from '@/components/panel/PanelPlate.vue'

const session = useSessionStore()
const router = useRouter()

const username = ref('')
const password = ref('')
const busy = ref(false)

const detractor = computed(() => session.upstreamFor('detractor'))

async function submit(): Promise<void> {
  busy.value = true
  try {
    await session.signIn(username.value.trim(), password.value)
    password.value = ''
    await router.push('/')
  } catch {
    // session.error carries the message; stay on the page.
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="mx-auto max-w-md">
    <PanelPlate
      legend="Sign in"
      sublegend="Inverse reactive credentials, supplied by the unilateral phase detractor"
    >
      <Notice tone="error" :message="session.error" />

      <p
        v-if="detractor !== null && !detractor.configured"
        class="typed mb-3 text-[0.78rem] text-ink-soft"
      >
        The identity service has no address: set <code>{{ detractor.env }}</code>. Until then the
        console can only be browsed as a guest.
      </p>

      <form class="mt-2 flex flex-col gap-3" @submit.prevent="submit">
        <label class="flex flex-col gap-1">
          <span class="engraved">Username</span>
          <input v-model="username" class="field" autocomplete="username" required />
        </label>
        <label class="flex flex-col gap-1">
          <span class="engraved">Password</span>
          <input
            v-model="password"
            type="password"
            class="field"
            autocomplete="current-password"
            required
          />
        </label>
        <button type="submit" class="knob self-start" :disabled="busy">
          {{ busy ? 'Signing in…' : 'Sign in' }}
        </button>
      </form>

      <template #footer>
        <p class="typed text-[0.72rem] text-ink-faint">
          A guest is a viewer: every gauge on this panel is live without signing in. An operator can
          drive traffic, induce malaise, run scenarios and set flags; an admin can also manage users.
          Those checks run on the server, not in this browser.
        </p>
      </template>
    </PanelPlate>
  </div>
</template>
