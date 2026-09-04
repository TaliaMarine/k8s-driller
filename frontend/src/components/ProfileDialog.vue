<script setup lang="ts">
import { ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'

const model = defineModel<boolean>({ default: false })
const authStore = useAuthStore()

const subjectRevealed = ref(false)
const copied = ref(false)

// Reset the reveal state every time the dialog reopens, rather than leaving
// the subject exposed if it's reopened later in the same session.
watch(model, (open) => {
  if (open) {
    subjectRevealed.value = false
    copied.value = false
  }
})

async function copySubject() {
  if (!authStore.user) return
  await navigator.clipboard.writeText(authStore.user.subject)
  copied.value = true
  setTimeout(() => (copied.value = false), 1500)
}
</script>

<template>
  <v-dialog v-model="model" max-width="480">
    <v-card v-if="authStore.user" title="Profile">
      <v-card-text>
        <v-list density="compact">
          <v-list-item v-if="authStore.user.name">
            <template #prepend><v-icon icon="mdi-account" /></template>
            <v-list-item-title>{{ authStore.user.name }}</v-list-item-title>
            <v-list-item-subtitle>Name</v-list-item-subtitle>
          </v-list-item>
          <v-list-item>
            <template #prepend><v-icon icon="mdi-email" /></template>
            <v-list-item-title>{{ authStore.user.email }}</v-list-item-title>
            <v-list-item-subtitle>Email</v-list-item-subtitle>
          </v-list-item>
          <v-list-item>
            <template #prepend><v-icon icon="mdi-shield-account" /></template>
            <v-list-item-title>
              <v-chip :color="authStore.user.role === 'admin' ? 'watch' : undefined" size="small">
                {{ authStore.user.role }}
              </v-chip>
            </v-list-item-title>
            <v-list-item-subtitle>Role</v-list-item-subtitle>
          </v-list-item>
          <v-list-item>
            <template #prepend><v-icon icon="mdi-clock-outline" /></template>
            <v-list-item-title>{{
              new Date(authStore.user.expires).toLocaleString()
            }}</v-list-item-title>
            <v-list-item-subtitle>Session expires</v-list-item-subtitle>
          </v-list-item>
          <v-list-item>
            <template #prepend><v-icon icon="mdi-key-outline" /></template>
            <v-list-item-title class="d-flex align-center ga-2">
              <span class="subject-value">{{
                subjectRevealed ? authStore.user.subject : '••••••••••••••••••••'
              }}</span>
              <v-btn
                :icon="subjectRevealed ? 'mdi-eye-off' : 'mdi-eye'"
                size="x-small"
                variant="text"
                :title="subjectRevealed ? 'Hide subject' : 'Show subject'"
                @click="subjectRevealed = !subjectRevealed"
              />
              <v-btn
                icon="mdi-content-copy"
                size="x-small"
                variant="text"
                title="Copy subject"
                @click="copySubject"
              />
              <span v-if="copied" class="text-caption text-healthy">Copied!</span>
            </v-list-item-title>
            <v-list-item-subtitle>
              OIDC subject — needed to promote your first admin (see README)
            </v-list-item-subtitle>
          </v-list-item>
        </v-list>
      </v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn variant="text" @click="model = false">Close</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<style scoped>
.subject-value {
  font-family: monospace;
  font-size: 0.85rem;
  word-break: break-all;
}
</style>
