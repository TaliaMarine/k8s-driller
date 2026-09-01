<script setup lang="ts">
function login() {
  window.location.href = '/api/v1/auth/login'
}
</script>

<template>
  <v-container class="fill-height" fluid>
    <v-row justify="center" align="center">
      <v-col cols="12" sm="6" md="4">
        <v-card class="text-center pa-6">
          <v-icon icon="mdi-drill" size="48" class="mb-4" />
          <v-card-title>Kubernetes Driller</v-card-title>
          <v-card-text>Sign in with your organization's identity provider to continue.</v-card-text>
          <v-btn color="primary" block @click="login">Sign in with OIDC</v-btn>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>
