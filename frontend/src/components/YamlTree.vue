<script setup lang="ts">
import YamlNode from './YamlNode.vue'

defineProps<{ data: Record<string, unknown> }>()
</script>

<template>
  <div class="yaml-tree">
    <YamlNode v-for="(v, k) in data" :key="k" :label="String(k)" :value="v" :depth="0" />
  </div>
</template>

<style scoped>
.yaml-tree {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12.5px;
  line-height: 1.5;
  overflow-x: auto;
}
</style>
