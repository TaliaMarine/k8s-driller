<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { DrillerUserRole, Role } from '@/types/api'

const users = ref<DrillerUserRole[]>([])
const loading = ref(true)
const savingSubject = ref<string | null>(null)

async function load() {
  loading.value = true
  const res = await fetch('/api/v1/admin/users', { credentials: 'include' })
  users.value = res.ok ? ((await res.json()) as DrillerUserRole[]) : []
  loading.value = false
}

async function setRole(user: DrillerUserRole, role: Role) {
  savingSubject.value = user.spec.subject
  await fetch(`/api/v1/admin/users/${encodeURIComponent(user.spec.subject)}/role`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ role, email: user.spec.email }),
  })
  savingSubject.value = null
  await load()
}

onMounted(load)
</script>

<template>
  <v-container>
    <h2 class="text-h6 mb-4">User roles</h2>
    <v-table>
      <thead>
        <tr>
          <th>Subject</th>
          <th>Email</th>
          <th>Role</th>
          <th>Updated</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="user in users" :key="user.metadata.name">
          <td>{{ user.spec.subject }}</td>
          <td>{{ user.spec.email }}</td>
          <td>
            <v-select
              :model-value="user.spec.role"
              :items="['admin', 'viewer']"
              density="compact"
              hide-details
              :loading="savingSubject === user.spec.subject"
              style="max-width: 140px"
              @update:model-value="(role: Role) => setRole(user, role)"
            />
          </td>
          <td>{{ user.spec.updatedAt }}</td>
        </tr>
        <tr v-if="!loading && users.length === 0">
          <td colspan="4" class="text-center text-medium-emphasis">
            No roles assigned yet — every OIDC login defaults to viewer.
          </td>
        </tr>
      </tbody>
    </v-table>
  </v-container>
</template>
