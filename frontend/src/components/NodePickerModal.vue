<template>
  <div v-if="modelValue" class="modal-mask node-picker-mask" @click.self="close">
    <div class="modal-card node-picker-modal" @click.stop>
      <h3 class="node-picker-title">{{ title }}</h3>
      <div class="node-picker-tree">
        <div
          v-for="row in treeRows"
          :key="`picker-${row.id}`"
          class="node-picker-row"
          :class="{ 'is-category': row.type === 'category', 'is-selected': row.id === selectedId }"
          :style="{ '--node-offset': `${pickerRowOffset(row.depth)}px`, '--node-row-width': `calc(100% - ${pickerMaxOffset}px)` }"
          @click="onRowClick(row)"
        >
          <span class="node-picker-name">{{ row.name }}</span>
          <span v-if="row.type === 'category'" class="fold-icon node-picker-fold" :class="{ open: !isCollapsed(row.id) }">&gt;</span>
        </div>
      </div>
      <div class="node-picker-selected">
        <span>当前：</span>
        <strong>{{ selectedNode?.name || '未选择' }}</strong>
      </div>
      <div class="row node-picker-actions">
        <button @click="close">取消</button>
        <button class="primary" :disabled="!canConfirm" @click="confirm">确定</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'

const props = defineProps({
  modelValue: { type: Boolean, required: true },
  title: { type: String, default: '选择事项' },
  nodeMap: { type: Object, required: true },
  childrenMap: { type: Object, required: true },
  showHiddenNodes: { type: Boolean, default: false },
  initialSelectedId: { type: Number, default: null },
  allowedTypes: { type: Array, default: () => ['item'] },
})

const emit = defineEmits(['update:modelValue', 'confirm'])

const selectedId = ref(null)
const collapsedOverride = ref({})
const pickerMaxOffset = 48
const pickerLevelCount = 3

function pickerRowOffset(depth) {
  const safeDepth = Number.isFinite(depth) ? Math.max(0, depth) : 0
  const normalizedDepth = Math.min(safeDepth, pickerLevelCount - 1)
  if (pickerLevelCount <= 1) return 0
  return Math.round((pickerMaxOffset * normalizedDepth) / (pickerLevelCount - 1))
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      selectedId.value = props.initialSelectedId ?? null
    }
  },
)

function close() {
  emit('update:modelValue', false)
}

function isCollapsed(id) {
  if (Object.prototype.hasOwnProperty.call(collapsedOverride.value, id)) {
    return !!collapsedOverride.value[id]
  }
  const node = props.nodeMap.get(id)
  return !!node?.collapsed
}

function toggleCollapsed(id) {
  collapsedOverride.value = { ...collapsedOverride.value, [id]: !isCollapsed(id) }
}

const treeRows = computed(() => {
  const out = []
  const walk = (parentKey, depth, hiddenByParent) => {
    const list = props.childrenMap.get(parentKey) || []
    for (const n of list) {
      if (!props.showHiddenNodes && n.hidden) continue
      const nextHidden = hiddenByParent || !!n.hidden
      out.push({ ...n, depth, hiddenByParent: nextHidden })
      if (n.type === 'category' && !isCollapsed(n.id)) {
        walk(n.id, depth + 1, nextHidden)
      }
    }
  }
  walk(0, 0, false)
  return out
})

const selectedNode = computed(() => {
  if (!selectedId.value) return null
  return props.nodeMap.get(selectedId.value) || null
})

const canConfirm = computed(() => {
  const nodeType = selectedNode.value?.type
  if (!nodeType) return false
  return props.allowedTypes.includes(nodeType)
})

function onRowClick(node) {
  selectedId.value = node.id
  if (node.type === 'category') {
    toggleCollapsed(node.id)
  }
}

function confirm() {
  if (!canConfirm.value) return
  emit('confirm', selectedId.value)
  close()
}
</script>
