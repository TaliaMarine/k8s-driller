<script setup lang="ts">
import { computed, watchEffect } from 'vue'
import { useTheme } from 'vuetify'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { useClusterStore } from '@/stores/cluster'

const authStore = useAuthStore()
const themeStore = useThemeStore()
const clusterStore = useClusterStore()
const router = useRouter()
const vuetifyTheme = useTheme()

watchEffect(() => {
  vuetifyTheme.global.name.value = themeStore.name
})

const themeIcon = computed(() =>
  themeStore.name === 'dark' ? 'mdi-weather-sunny' : 'mdi-weather-night',
)

const statusColor = computed(() => {
  switch (clusterStore.status) {
    case 'live':
      return 'healthy'
    case 'reconnecting':
      return 'warning'
    default:
      return 'critical'
  }
})

async function logout() {
  await authStore.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <v-app>
    <v-app-bar flat>
      <v-app-bar-title>
        <v-icon icon="mdi-drill" class="mr-2" />
        Kubernetes Driller
      </v-app-bar-title>
      <v-spacer />

      <v-chip
        v-if="authStore.isAuthenticated"
        :color="statusColor"
        size="small"
        class="mr-4"
        variant="flat"
      >
        {{ clusterStore.status }}
      </v-chip>

      <v-btn
        icon
        :title="`Switch to ${themeStore.name === 'dark' ? 'light' : 'dark'} theme`"
        @click="themeStore.toggle"
      >
        <v-icon :icon="themeIcon" />
      </v-btn>

      <v-btn v-if="authStore.isAdmin" to="/admin/users" variant="text">Users</v-btn>
      <v-btn v-if="authStore.isAdmin" to="/admin/alerts" variant="text">Alerts</v-btn>
      <v-btn v-if="authStore.isAuthenticated" variant="text" @click="logout">Logout</v-btn>
    </v-app-bar>

    <v-main>
      <router-view />
    </v-main>
  </v-app>
</template>
