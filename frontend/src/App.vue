<script setup lang="ts">
import { computed, ref, watchEffect } from 'vue'
import { useTheme } from 'vuetify'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { useClusterStore } from '@/stores/cluster'
import ProfileDialog from '@/components/ProfileDialog.vue'

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

const displayName = computed(() => authStore.user?.name || authStore.user?.email || 'Account')
const showProfile = ref(false)

async function logout() {
  await authStore.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <v-app>
    <v-app-bar flat>
      <router-link to="/" class="app-title-link">
        <v-app-bar-title>
          <v-icon icon="mdi-drill" class="mr-2" />
          Kubernetes Driller
        </v-app-bar-title>
      </router-link>
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

      <v-menu v-if="authStore.isAuthenticated">
        <template #activator="{ props }">
          <v-btn v-bind="props" variant="text" class="text-none">
            <v-icon start icon="mdi-account-circle" />
            {{ displayName }}
            <v-icon end icon="mdi-chevron-down" size="small" />
          </v-btn>
        </template>
        <v-list density="compact">
          <v-list-item
            prepend-icon="mdi-account-circle"
            title="Profile"
            @click="showProfile = true"
          />
          <v-list-item prepend-icon="mdi-logout" title="Logout" @click="logout" />
        </v-list>
      </v-menu>
    </v-app-bar>

    <ProfileDialog v-model="showProfile" />

    <v-main>
      <router-view />
    </v-main>
  </v-app>
</template>

<style scoped>
.app-title-link {
  text-decoration: none;
  color: inherit;
}
</style>
