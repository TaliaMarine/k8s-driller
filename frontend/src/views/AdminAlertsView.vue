<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { DrillerAlertConfigSpec, Webhook } from '@/types/api'

const config = reactive<DrillerAlertConfigSpec>({
  webhooks: [],
  thresholds: {
    nodeMemLivePct: 90,
    nodeCpuLivePct: 90,
    overcommitEnabled: true,
    oomRiskEnabled: true,
    throttlingRiskEnabled: true,
  },
  debounceMinutes: 15,
})

const saving = ref(false)
const testing = ref(false)
const newWebhook = reactive<Webhook>({
  type: 'slack',
  secretRef: { name: '', key: 'url' },
  enabled: true,
})

async function load() {
  const res = await fetch('/api/v1/admin/alerts/config', { credentials: 'include' })
  if (res.ok) {
    const loaded = (await res.json()) as DrillerAlertConfigSpec
    Object.assign(config, loaded, { webhooks: loaded.webhooks ?? [] })
  }
}

async function save() {
  saving.value = true
  await fetch('/api/v1/admin/alerts/config', {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  })
  saving.value = false
}

function addWebhook() {
  if (!newWebhook.secretRef.name) return
  config.webhooks.push({ ...newWebhook, secretRef: { ...newWebhook.secretRef } })
  newWebhook.secretRef.name = ''
}

function removeWebhook(index: number) {
  config.webhooks.splice(index, 1)
}

async function sendTest() {
  testing.value = true
  await fetch('/api/v1/admin/alerts/test', { method: 'POST', credentials: 'include' })
  testing.value = false
}

onMounted(load)
</script>

<template>
  <v-container style="max-width: 640px">
    <h2 class="text-h6 mb-4">Alert settings</h2>

    <v-card class="mb-4" title="Thresholds">
      <v-card-text>
        <div class="mb-4">
          <div class="text-caption mb-1">
            Node memory live % ({{ config.thresholds.nodeMemLivePct }}%)
          </div>
          <v-slider
            v-model="config.thresholds.nodeMemLivePct"
            min="50"
            max="100"
            step="1"
            hide-details
          />
        </div>
        <div class="mb-4">
          <div class="text-caption mb-1">
            Node CPU live % ({{ config.thresholds.nodeCpuLivePct }}%)
          </div>
          <v-slider
            v-model="config.thresholds.nodeCpuLivePct"
            min="50"
            max="100"
            step="1"
            hide-details
          />
        </div>
        <v-switch
          v-model="config.thresholds.overcommitEnabled"
          label="Overcommit alerts"
          hide-details
        />
        <v-switch v-model="config.thresholds.oomRiskEnabled" label="OOM-Risk alerts" hide-details />
        <v-switch
          v-model="config.thresholds.throttlingRiskEnabled"
          label="Throttling-Risk alerts"
          hide-details
        />
        <v-text-field
          v-model.number="config.debounceMinutes"
          type="number"
          label="Debounce (minutes)"
          class="mt-4"
          density="compact"
        />
      </v-card-text>
    </v-card>

    <v-card class="mb-4" title="Webhooks">
      <v-card-text>
        <v-list>
          <v-list-item v-for="(wh, i) in config.webhooks" :key="i">
            <v-list-item-title
              >{{ wh.type }} — {{ wh.secretRef.name }}/{{ wh.secretRef.key }}</v-list-item-title
            >
            <template #append>
              <v-switch v-model="wh.enabled" hide-details density="compact" class="mr-2" />
              <v-btn icon="mdi-delete" size="small" variant="text" @click="removeWebhook(i)" />
            </template>
          </v-list-item>
        </v-list>

        <v-divider class="my-4" />
        <div class="text-caption mb-2">
          Add a webhook (the Secret must already exist in this namespace — the URL itself is never
          entered here, per SPECS.md §5.2).
        </div>
        <div class="d-flex ga-2 align-center flex-wrap">
          <v-select
            v-model="newWebhook.type"
            :items="['slack', 'generic']"
            label="Type"
            density="compact"
            style="width: 120px"
            hide-details
          />
          <v-text-field
            v-model="newWebhook.secretRef.name"
            label="Secret name"
            density="compact"
            style="width: 200px"
            hide-details
          />
          <v-text-field
            v-model="newWebhook.secretRef.key"
            label="Secret key"
            density="compact"
            style="width: 140px"
            hide-details
          />
          <v-btn @click="addWebhook">Add</v-btn>
        </div>
      </v-card-text>
    </v-card>

    <div class="d-flex ga-2">
      <v-btn color="primary" :loading="saving" @click="save">Save</v-btn>
      <v-btn variant="outlined" :loading="testing" @click="sendTest">Send test alert</v-btn>
    </div>
  </v-container>
</template>
