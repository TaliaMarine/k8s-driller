<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { load as loadYaml } from 'js-yaml'
import type { PodDTO, PodManifestDTO } from '@/types/api'
import YamlTree from './YamlTree.vue'
import PodInfoTable from './PodInfoTable.vue'

const props = defineProps<{ pod: PodDTO }>()

const manifest = ref<PodManifestDTO | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const manifestTab = ref<'pod' | 'controller'>('pod')

// Loaded once, the moment this tab is actually shown (v-window-item is
// lazy — this component doesn't mount until then) — unlike the Analysis
// tab's explicit button, a manifest fetch is a couple of cheap dynamic-client
// Gets, not a Prometheus range query, so there's no reason to make it opt-in.
onMounted(async () => {
  loading.value = true
  error.value = null
  try {
    const res = await fetch(
      `/api/v1/pods/${encodeURIComponent(props.pod.namespace)}/${encodeURIComponent(props.pod.name)}/manifest`,
      { credentials: 'include' },
    )
    if (!res.ok) {
      error.value =
        res.status === 404
          ? 'Pod manifest not found (it may have just been deleted).'
          : `Manifest request failed (${res.status}).`
      return
    }
    manifest.value = await res.json()
  } catch {
    error.value = 'Manifest request failed.'
  } finally {
    loading.value = false
  }
})

function parseYaml(text: string | undefined): Record<string, unknown> | null {
  if (!text) return null
  try {
    return (loadYaml(text) as Record<string, unknown>) ?? null
  } catch {
    return null
  }
}

const activeManifest = computed(() => {
  if (!manifest.value) return null
  return manifestTab.value === 'controller' ? manifest.value.controller : manifest.value.pod
})
const activeParsed = computed(() => parseYaml(activeManifest.value?.yaml))

// Always the pod's own data, regardless of manifestTab — the info table is
// about the pod's stability/restarts, which the parent controller's YAML
// doesn't carry.
const podParsed = computed(() => parseYaml(manifest.value?.pod.yaml))
</script>

<template>
  <div>
    <v-alert v-if="error" type="warning" variant="tonal" density="compact" class="mb-4">
      {{ error }}
    </v-alert>
    <div v-if="loading" class="d-flex align-center ga-2 mb-4">
      <v-progress-circular indeterminate size="20" color="watch" />
      <span class="text-caption text-medium-emphasis">Loading manifest…</span>
    </div>

    <template v-if="manifest">
      <v-tabs v-model="manifestTab" density="compact" color="watch" class="mb-3">
        <v-tab value="pod" prepend-icon="mdi-cube-outline">Pod</v-tab>
        <v-tab v-if="manifest.controller" value="controller" prepend-icon="mdi-source-branch">
          {{ manifest.controller.kind }}
        </v-tab>
      </v-tabs>

      <v-row>
        <v-col cols="12" md="7">
          <YamlTree v-if="activeParsed" :data="activeParsed" />
          <div v-else class="text-caption text-medium-emphasis">Unable to parse manifest YAML.</div>
        </v-col>
        <v-col cols="12" md="5" class="pod-info-col">
          <PodInfoTable :pod-data="podParsed" />
        </v-col>
      </v-row>
    </template>
  </div>
</template>

<style scoped>
.pod-info-col {
  border-left: 1px solid rgba(var(--v-theme-on-surface), 0.1);
}
@media (max-width: 960px) {
  .pod-info-col {
    border-left: none;
    border-top: 1px solid rgba(var(--v-theme-on-surface), 0.1);
    padding-top: 12px;
  }
}
</style>
