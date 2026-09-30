<script setup lang="ts">
/**
 * Administration: users and the audit log. Admin only, enforced by the BFF.
 *
 * A non-admin is not shown a broken page: the plate explains the role it needs.
 * That is cosmetic, though; `/bff/users` refuses anyone without the admin role
 * whether or not this page ever renders.
 */
import { computed, onMounted, ref } from 'vue'
import { useAdminStore } from '@/stores/controls'
import { useSessionStore } from '@/stores/session'
import { EMPTY_STATES } from '@/lib/lexicon'
import { ago, clock } from '@/lib/format'
import Notice from '@/components/panel/Notice.vue'
import PanelPlate from '@/components/panel/PanelPlate.vue'

const admin = useAdminStore()
const session = useSessionStore()

const ROLES = ['customer', 'viewer', 'operator', 'admin'] as const

const username = ref('')
const displayName = ref('')
const password = ref('')
const roles = ref<string[]>(['customer'])

const allowed = computed(() => session.canAdminister)

onMounted(() => {
  if (!allowed.value) return
  void admin.loadUsers()
  void admin.loadAudit()
})

async function create(): Promise<void> {
  const result = await admin.create({
    username: username.value.trim(),
    password: password.value,
    displayName: displayName.value.trim(),
    roles: [...roles.value],
  })
  if (result.ok) {
    username.value = ''
    displayName.value = ''
    password.value = ''
    roles.value = ['customer']
  }
}

function toggleRole(user: { username: string; roles: string[] }, role: string): void {
  const next = user.roles.includes(role)
    ? user.roles.filter((r) => r !== role)
    : [...user.roles, role]
  void admin.setRoles(user.username, next)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <PanelPlate
      v-if="!allowed"
      legend="Administration"
      sublegend="Users and the audit log"
      :empty="true"
      :empty-note="
        session.isGuest
          ? 'Administration requires the admin role. Sign in to continue.'
          : `Administration requires the admin role; you hold ${session.roleLabel}. The console’s backend refuses these routes, not just this page.`
      "
    />

    <template v-else>
      <Notice tone="error" :message="admin.notice" />

      <div class="grid gap-4 xl:grid-cols-[1.2fr_0.8fr]">
        <PanelPlate
          legend="Users"
          sublegend="Roles take effect on the user’s next token, one hour at most"
          :source="admin.usersError === null ? null : { ok: false, detail: admin.usersError }"
          :empty="admin.users.length === 0"
          :empty-note="EMPTY_STATES.noUsers"
        >
          <table class="w-full border-collapse text-[0.8rem]">
            <thead>
              <tr class="border-b border-rule text-left">
                <th class="engraved py-1 pr-3">Username</th>
                <th class="engraved py-1 pr-3">Name</th>
                <th class="engraved py-1 pr-3">Roles</th>
                <th class="engraved py-1"></th>
              </tr>
            </thead>
            <tbody class="typed">
              <tr
                v-for="user in admin.users"
                :key="user.username"
                class="border-b border-rule/40 last:border-0"
              >
                <td class="py-1.5 pr-3 font-[family-name:var(--font-gauge)]">{{ user.username }}</td>
                <td class="py-1.5 pr-3">{{ user.displayName ?? '-' }}</td>
                <td class="py-1.5 pr-3">
                  <span class="flex flex-wrap gap-1">
                    <button
                      v-for="role in ROLES"
                      :key="role"
                      type="button"
                      class="rounded border px-1.5 py-0.5 text-[0.68rem] tracking-wide uppercase"
                      :class="
                        user.roles.includes(role)
                          ? 'border-brass bg-brass/20 text-ink'
                          : 'border-rule text-ink-faint'
                      "
                      :disabled="admin.busy"
                      @click="toggleRole(user, role)"
                    >
                      {{ role }}
                    </button>
                  </span>
                </td>
                <td class="py-1.5 text-right">
                  <button
                    type="button"
                    class="knob knob-danger"
                    :disabled="admin.busy || user.username === session.session.username"
                    :title="
                      user.username === session.session.username
                        ? 'You cannot delete the account you are signed in as.'
                        : undefined
                    "
                    @click="admin.remove(user.username)"
                  >
                    Delete
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </PanelPlate>

        <PanelPlate legend="New user" sublegend="Usernames are 3 to 64 of [a-z0-9._-]">
          <form class="flex flex-col gap-3" @submit.prevent="create">
            <label class="flex flex-col gap-1">
              <span class="engraved">Username</span>
              <input v-model="username" class="field" pattern="[a-z0-9._\-]{3,64}" required />
            </label>
            <label class="flex flex-col gap-1">
              <span class="engraved">Display name</span>
              <input v-model="displayName" class="field" />
            </label>
            <label class="flex flex-col gap-1">
              <span class="engraved">Password</span>
              <input v-model="password" type="password" class="field" required autocomplete="new-password" />
              <span class="typed text-[0.7rem] text-ink-faint">
                Hashed with Argon2id by the detractor. This console never stores it.
              </span>
            </label>
            <fieldset class="flex flex-col gap-1">
              <legend class="engraved">Roles</legend>
              <label v-for="role in ROLES" :key="role" class="typed flex items-center gap-2 text-[0.8rem]">
                <input v-model="roles" type="checkbox" :value="role" />
                {{ role }}
              </label>
            </fieldset>
            <button type="submit" class="knob self-start" :disabled="admin.busy">Create</button>
          </form>
        </PanelPlate>
      </div>

      <PanelPlate
        legend="Audit log"
        sublegend="Login successes and failures, and user changes, from the detractor"
        :source="admin.auditError === null ? null : { ok: false, detail: admin.auditError }"
        :empty="admin.audit.length === 0"
        :empty-note="EMPTY_STATES.noAudit"
      >
        <table class="w-full border-collapse text-[0.78rem]">
          <thead>
            <tr class="border-b border-rule text-left">
              <th class="engraved py-1 pr-3">Time</th>
              <th class="engraved py-1 pr-3">Action</th>
              <th class="engraved py-1 pr-3">Subject</th>
              <th class="engraved py-1 pr-3">Actor</th>
              <th class="engraved py-1 pr-3">Result</th>
              <th class="engraved py-1">Detail</th>
            </tr>
          </thead>
          <tbody class="typed">
            <tr
              v-for="(entry, index) in admin.audit"
              :key="`${entry.time ?? index}-${index}`"
              class="border-b border-rule/40 last:border-0"
            >
              <td class="py-1 pr-3 whitespace-nowrap font-[family-name:var(--font-gauge)]">
                {{ clock(entry.time ?? null) }}
                <span class="text-ink-faint">({{ ago(entry.time ?? null) }})</span>
              </td>
              <td class="py-1 pr-3">{{ entry.action ?? 'no data' }}</td>
              <td class="py-1 pr-3">{{ entry.subject ?? 'no data' }}</td>
              <td class="py-1 pr-3">{{ entry.actor ?? 'no data' }}</td>
              <td class="py-1 pr-3">{{ entry.result ?? 'no data' }}</td>
              <td class="py-1">{{ entry.detail ?? '' }}</td>
            </tr>
          </tbody>
        </table>
      </PanelPlate>
    </template>
  </div>
</template>
