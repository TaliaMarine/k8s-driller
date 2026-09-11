<script setup lang="ts">
import { computed, ref } from 'vue'

// One key/value (or array item) in the collapsible YAML tree (see
// YamlTree.vue, the entry point). Recurses into itself for nested
// objects/arrays — Vue SFCs can reference themselves by filename in their
// own template, no explicit registration needed.
const props = defineProps<{
  value: unknown
  label?: string | null
  depth: number
  isArrayItem?: boolean
}>()

// js-yaml's default schema resolves ISO-8601-looking plain scalars (e.g.
// metadata.creationTimestamp) into JS Date objects — treated as a scalar
// here, not as an "object" node, or it would render as a misleading empty
// {} (Date has no enumerable own properties).
const isDate = computed(() => props.value instanceof Date)
const isArray = computed(() => Array.isArray(props.value))
const isObject = computed(
  () => !isDate.value && !isArray.value && props.value !== null && typeof props.value === 'object',
)
const isCollapsible = computed(() => isObject.value || isArray.value)

const entries = computed(() =>
  isObject.value ? Object.entries(props.value as Record<string, unknown>) : [],
)
const items = computed(() => (isArray.value ? (props.value as unknown[]) : []))
const isEmpty = computed(() => entries.value.length === 0 && items.value.length === 0)

// Expanded on the first level (depth 0) by default; every nested level
// starts collapsed so a large manifest doesn't open as one huge dump.
const expanded = ref(props.depth < 1)

function toggle() {
  if (isCollapsible.value && !isEmpty.value) expanded.value = !expanded.value
}

function scalarClass(v: unknown): string {
  if (v === null || v === undefined) return 'yaml-null'
  if (v instanceof Date) return 'yaml-string'
  switch (typeof v) {
    case 'string':
      return 'yaml-string'
    case 'number':
      return 'yaml-number'
    case 'boolean':
      return 'yaml-boolean'
    default:
      return ''
  }
}

function scalarText(v: unknown): string {
  if (v === null || v === undefined) return 'null'
  if (v instanceof Date) return v.toISOString()
  return String(v)
}

const collapsedHint = computed(() => {
  if (isArray.value) {
    const n = items.value.length
    return `[${n} item${n === 1 ? '' : 's'}]`
  }
  const n = entries.value.length
  return `{${n} key${n === 1 ? '' : 's'}}`
})
</script>

<template>
  <div class="yaml-node">
    <div class="yaml-line" :class="{ 'yaml-clickable': isCollapsible && !isEmpty }" @click="toggle">
      <v-icon
        v-if="isCollapsible && !isEmpty"
        :icon="expanded ? 'mdi-chevron-down' : 'mdi-chevron-right'"
        size="14"
        class="yaml-toggle"
      />
      <span v-else class="yaml-toggle-spacer" />
      <span v-if="isArrayItem" class="yaml-dash">-</span>
      <span v-if="label != null" class="yaml-key">{{ label }}:</span>
      <span v-if="!isCollapsible" :class="scalarClass(value)">{{ scalarText(value) }}</span>
      <span v-else-if="isEmpty" class="yaml-empty">{{ isArray ? '[]' : '{}' }}</span>
      <span v-else-if="!expanded" class="yaml-collapsed-hint text-medium-emphasis">{{
        collapsedHint
      }}</span>
    </div>
    <div v-if="isCollapsible && !isEmpty && expanded" class="yaml-children">
      <YamlNode v-for="[k, v] in entries" :key="k" :label="k" :value="v" :depth="depth + 1" />
      <YamlNode
        v-for="(item, i) in items"
        :key="i"
        :value="item"
        :depth="depth + 1"
        is-array-item
      />
    </div>
  </div>
</template>

<style scoped>
.yaml-line {
  display: flex;
  align-items: baseline;
  gap: 4px;
  padding: 1px 0;
  white-space: pre-wrap;
  word-break: break-word;
}
.yaml-clickable {
  cursor: pointer;
}
.yaml-clickable:hover {
  background: rgba(var(--v-theme-on-surface), 0.05);
}
.yaml-toggle {
  flex-shrink: 0;
  opacity: 0.6;
}
.yaml-toggle-spacer {
  display: inline-block;
  width: 14px;
  flex-shrink: 0;
}
.yaml-dash {
  opacity: 0.6;
}
.yaml-key {
  color: rgb(var(--v-theme-watch));
  font-weight: 500;
}
.yaml-string {
  color: rgb(var(--v-theme-on-surface));
}
.yaml-number {
  color: #7e57c2;
}
.yaml-boolean {
  color: #ef6c00;
}
.yaml-null {
  opacity: 0.5;
  font-style: italic;
}
.yaml-empty,
.yaml-collapsed-hint {
  opacity: 0.55;
  font-style: italic;
}
.yaml-children {
  margin-left: 9px;
  padding-left: 8px;
  border-left: 1px dotted rgba(var(--v-theme-on-surface), 0.15);
}
</style>
