<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import NodePickerModal from './components/NodePickerModal.vue'

function normalizeActiveTab(value) {
  return value === 'tree' || value === 'stats' || value === 'settings' ? value : 'tree'
}

function tabFromHash(hash) {
  const raw = (hash || '').replace(/^#\/?/, '')
  if (!raw) return null
  return raw === 'tree' || raw === 'stats' || raw === 'settings' ? raw : null
}

function loadActiveTab() {
  return tabFromHash(window.location.hash) || 'tree'
}

function syncHashFromTab(tab) {
  const nextHash = `#/${normalizeActiveTab(tab)}`
  if (window.location.hash === nextHash) return
  window.history.replaceState(null, '', nextHash)
}

const activeTab = ref(loadActiveTab())
const me = ref(null)
const isCheckingAuth = ref(true)
const loginForm = ref({ username: 'ts65213', password: '' })
const loginError = ref('')
const addNodeError = ref('')
const showAddNodeModal = ref(false)
const showManualRecordModal = ref(false)
const showUserMenu = ref(false)
const showNodeActionModal = ref(false)
const nodeActionError = ref('')
const longPressTimer = ref(null)
const longPressFired = ref(false)
const nodeActionDraft = ref({
  id: null,
  type: 'item',
  name: '',
  hidden: false,
  dailyTargetMinutes: 0,
})
const nodes = ref([])
const records = ref([])
const statsBaseRecords = ref([])
const recordFilters = ref({ fromDate: '', toDate: '', itemId: '', source: '' })
const statsScopeNodeId = ref(null)
const statsRangePreset = ref('today')
const statsRangeStartDate = ref('')
const statsRangeEndDate = ref('')
const settings = ref({ confirmBeforeSaveTimerRecord: true, showHiddenNodes: false, skipShortTimerRecord: false, statsIncludeHiddenNodes: false })
const loadingSettings = ref(false)
const timerState = ref({
  activeItemId: null,
  sessionStartAt: null,
  accumulatedPauseMs: 0,
  isPaused: false,
  pauseStartedAt: null,
  draftDescription: '',
})
const createNodeDraft = ref({ type: 'category', name: '', parentId: null, dailyTargetMinutes: 60 })
const manualRecordDraft = ref({
  id: null,
  itemId: null,
  startAt: '',
  endAt: '',
  pauseDurationSec: 0,
  description: '',
  applySplitByDate: true,
})
const showItemPickerModal = ref(false)
const itemPickerInitialSelectedId = ref(null)
const showStatsScopePickerModal = ref(false)
const statsScopePickerInitialId = ref(null)
const showMoveSourcePickerModal = ref(false)
const moveSourcePickerInitialId = ref(null)
const showMoveTargetPickerModal = ref(false)
const moveTargetPickerInitialId = ref(null)
const savingRecord = ref(false)
const selectedItemIdForTimer = ref(null)
const moveDraft = ref({ sourceId: null, targetId: null })
const moveTip = ref('')
const nodeMenuId = ref(null)
const nodeMenuPosition = ref({ x: 24, y: 140 })
const heatmapWrapRef = ref(null)
const statsTreeCollapsedMap = ref({})
const statsLongPressTimer = ref(null)
const statsLongPressFired = ref(false)
const recordMenuId = ref(null)
const recordMenuPosition = ref({ x: 24, y: 140 })
const recordLongPressTimer = ref(null)
const recordLongPressFired = ref(false)
const recordPressStartPoint = ref(null)
const recordPressStartScrollTop = ref(0)
const timelineVisibleCount = ref(20)
const statsTrendGranularity = ref('day')
const statsTrendWrapRef = ref(null)
const statsTrendWindowStart = ref(0)
const statsTrendWindowEnd = ref(-1)
const statsTrendEdge = ref('none')
const statsTrendScrollLeft = ref(0)
const statsModuleCollapsed = ref({
  ratio: false,
  timeline: false,
  trend: false,
  heatmap: false,
})
const timerFreezeMs = ref(null)
const timerMinDisplayMs = ref(0)
const timerLocalResumeAt = ref(null)
const timerLocalResumeBaseMs = ref(0)

function localDateKey(date) {
  const pad = (num) => String(num).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

function weekdayText(date) {
  const d = date instanceof Date ? date : new Date(date)
  const day = d.getDay()
  const map = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
  return map[day] || ''
}

function timeText(value) {
  const d = value instanceof Date ? value : new Date(value)
  const pad = (x) => String(x).padStart(2, '0')
  if (!Number.isFinite(d.getTime())) return '--:--'
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function recordEffectiveMs(record) {
  const start = new Date(record?.startAt).getTime()
  const end = new Date(record?.endAt).getTime()
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return 0
  const totalMs = end - start
  const pauseMs = Math.min(Math.max(0, Number(record?.pauseDurationMs || 0)), totalMs)
  return Math.max(0, totalMs - pauseMs)
}

function recordDateWeekText(record) {
  const d = new Date(record?.startAt)
  if (!Number.isFinite(d.getTime())) return ''
  return `${localDateKey(d)} ${weekdayText(d)}`
}

function recordTimeRangeText(record) {
  return `${timeText(record?.startAt)} → ${timeText(record?.endAt)}`
}

function itemCategoryPathText(itemId) {
  const out = []
  let cursor = nodeMap.value.get(itemId)
  while (cursor) {
    if (cursor.type === 'category') out.push(cursor.name)
    const parentId = cursor.parentId
    if (parentId === null || parentId === undefined) break
    cursor = nodeMap.value.get(parentId)
  }
  out.reverse()
  return out.join(' / ')
}

function openItemPicker() {
  itemPickerInitialSelectedId.value = manualRecordDraft.value.itemId ? Number(manualRecordDraft.value.itemId) : null
  showItemPickerModal.value = true
}

function onConfirmItemPicker(id) {
  manualRecordDraft.value.itemId = id
}

function openStatsScopePicker() {
  statsScopePickerInitialId.value = statsScopeNodeId.value ? Number(statsScopeNodeId.value) : null
  showStatsScopePickerModal.value = true
}

function onConfirmStatsScopePicker(id) {
  jumpStatsScope(id)
  activeTab.value = 'stats'
}

function setStatsRangeByPreset(preset) {
  const today = startOfDay(new Date())
  if (preset === 'all') {
    statsRangePreset.value = 'all'
    statsRangeStartDate.value = ''
    statsRangeEndDate.value = ''
    return
  }
  const dayMap = {
    today: 1,
    last3: 3,
    last7: 7,
    last30: 30,
    last90: 90,
  }
  const days = dayMap[preset] || 1
  const start = addDays(today, -(days - 1))
  statsRangePreset.value = preset
  statsRangeStartDate.value = localDateKey(start)
  statsRangeEndDate.value = localDateKey(today)
}

function isTextSelectionAllowed(target) {
  const el = target instanceof Element ? target : null
  if (!el) return false
  if (el.closest('input, textarea, select, [contenteditable="true"]')) return true
  return false
}

function preventLongPressMenuAndSelection() {
  const onContextMenu = (e) => {
    if (isTextSelectionAllowed(e.target)) return
    e.preventDefault()
  }
  const onSelectStart = (e) => {
    if (isTextSelectionAllowed(e.target)) return
    e.preventDefault()
  }
  window.addEventListener('contextmenu', onContextMenu, { capture: true })
  window.addEventListener('selectstart', onSelectStart, { capture: true })
}

function onHashChange() {
  const nextTab = tabFromHash(window.location.hash)
  if (!nextTab || nextTab === activeTab.value) return
  activeTab.value = nextTab
}

onMounted(() => {
  onHashChange()
  syncHashFromTab(activeTab.value)
  window.addEventListener('hashchange', onHashChange)
  preventLongPressMenuAndSelection()
})

onUnmounted(() => {
  window.removeEventListener('hashchange', onHashChange)
})

setStatsRangeByPreset('today')

const statsRangePickerValue = computed({
  get() {
    if (!statsRangeStartDate.value || !statsRangeEndDate.value) return []
    return [statsRangeStartDate.value, statsRangeEndDate.value]
  },
  set(v) {
    if (!Array.isArray(v) || v.length !== 2 || !v[0] || !v[1]) {
      setStatsRangeByPreset('all')
      return
    }
    statsRangeStartDate.value = v[0]
    statsRangeEndDate.value = v[1]
    statsRangePreset.value = 'custom'
  },
})

function onStatsRangePickerClear() {
  setStatsRangeByPreset('all')
}

function buildPresetShortcut(preset, text) {
  return {
    text,
    value: () => {
      setStatsRangeByPreset(preset)
      return [new Date(`${statsRangeStartDate.value}T00:00:00`), new Date(`${statsRangeEndDate.value}T00:00:00`)]
    },
  }
}

const statsRangePickerShortcuts = [
  buildPresetShortcut('today', '今日'),
  buildPresetShortcut('last3', '近3天'),
  buildPresetShortcut('last7', '7天'),
  buildPresetShortcut('last30', '30天'),
  buildPresetShortcut('last90', '90天'),
]

function buildStatsFromRecords(list) {
  const byItemMap = new Map()
  const byHourArray = Array.from({ length: 24 }, (_, hour) => ({ hour, durationMs: 0 }))
  const byDayMap = new Map()
  let totalDurationMs = 0
  let totalTimerCount = 0

  for (const r of list || []) {
    const start = new Date(r.startAt)
    const end = new Date(r.endAt)
    const totalMs = end.getTime() - start.getTime()
    if (!Number.isFinite(totalMs) || totalMs <= 0) continue
    const pauseMs = Math.min(Math.max(0, Number(r.pauseDurationMs || 0)), totalMs)
    const effectiveMs = Math.max(0, totalMs - pauseMs)
    totalDurationMs += effectiveMs
    byItemMap.set(r.itemId, (byItemMap.get(r.itemId) || 0) + effectiveMs)
    if (r.source === 'timer') totalTimerCount++

    let cur = new Date(start)
    while (cur < end) {
      const nextHour = new Date(cur)
      nextHour.setMinutes(0, 0, 0)
      nextHour.setHours(nextHour.getHours() + 1)
      const segEnd = nextHour < end ? nextHour : end
      const segMs = segEnd.getTime() - cur.getTime()
      if (segMs > 0) {
        const h = cur.getHours()
        byHourArray[h].durationMs += segMs
        const key = localDateKey(cur)
        byDayMap.set(key, (byDayMap.get(key) || 0) + segMs)
      }
      cur = segEnd
    }
  }

  const byItem = [...byItemMap.entries()]
    .map(([itemId, durationMs]) => ({ itemId, durationMs }))
    .sort((a, b) => b.durationMs - a.durationMs)
  const byDay = [...byDayMap.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([date, durationMs]) => ({ date, durationMs }))

  return { totalDurationMs, totalTimerCount, byItem, byHour: byHourArray, byDay }
}

const itemNodes = computed(() => nodes.value.filter((n) => n.type === 'item'))
const nodeMap = computed(() => {
  const map = new Map()
  nodes.value.forEach((n) => map.set(n.id, n))
  return map
})

const childrenMap = computed(() => {
  const map = new Map()
  for (const n of nodes.value) {
    const key = n.parentId ?? 0
    if (!map.has(key)) map.set(key, [])
    map.get(key).push(n)
  }
  for (const arr of map.values()) {
    arr.sort((a, b) => a.orderNo - b.orderNo || a.id - b.id)
  }
  return map
})

const statsScopeNodeIds = computed(() => {
  if (!statsScopeNodeId.value) return null
  const root = nodeMap.value.get(statsScopeNodeId.value)
  if (!root) return null
  const ids = new Set()
  const stack = [root.id]
  while (stack.length) {
    const id = stack.pop()
    if (ids.has(id)) continue
    ids.add(id)
    for (const child of childrenMap.value.get(id) || []) {
      stack.push(child.id)
    }
  }
  return ids
})

const hiddenSubtreeNodeIds = computed(() => {
  const ids = new Set()
  if (settings.value.statsIncludeHiddenNodes) return ids
  const walk = (node, hiddenByParent) => {
    if (!node) return
    const hiddenNow = hiddenByParent || !!node.hidden
    if (hiddenNow) ids.add(node.id)
    for (const child of childrenMap.value.get(node.id) || []) {
      walk(child, hiddenNow)
    }
  }
  for (const root of childrenMap.value.get(0) || []) {
    walk(root, false)
  }
  return ids
})

const statsScopeItemIdSet = computed(() => {
  if (!statsScopeNodeIds.value) return null
  const set = new Set()
  for (const n of nodes.value) {
    if (
      n.type === 'item' &&
      statsScopeNodeIds.value.has(n.id) &&
      !hiddenSubtreeNodeIds.value.has(n.id)
    ) {
      set.add(n.id)
    }
  }
  return set
})

const statsRecords = computed(() => {
  if (!statsScopeItemIdSet.value) {
    if (settings.value.statsIncludeHiddenNodes) return statsBaseRecords.value
    const hiddenItemIds = new Set(
      nodes.value
        .filter((n) => n.type === 'item' && hiddenSubtreeNodeIds.value.has(n.id))
        .map((n) => n.id),
    )
    if (hiddenItemIds.size === 0) return statsBaseRecords.value
    return statsBaseRecords.value.filter((r) => !hiddenItemIds.has(r.itemId))
  }
  return statsBaseRecords.value.filter((r) => statsScopeItemIdSet.value.has(r.itemId))
})

const statsRangeText = computed(() => {
  const map = {
    today: '今日',
    last3: '近3天',
    last7: '7天',
    last30: '30天',
    last90: '90天',
    all: '全部',
    custom: '自定义',
  }
  return map[statsRangePreset.value] || '自定义'
})

const statsRangeBoundary = computed(() => {
  if (statsRangePreset.value === 'all') {
    return { startInclusive: null, endExclusive: null }
  }
  const today = localDateKey(new Date())
  const startText = statsRangeStartDate.value || today
  const endText = statsRangeEndDate.value || today
  const normalizedStart = startText <= endText ? startText : endText
  const normalizedEnd = startText <= endText ? endText : startText
  const startInclusive = new Date(`${normalizedStart}T00:00:00`)
  const endExclusive = addDays(new Date(`${normalizedEnd}T00:00:00`), 1)
  return { startInclusive, endExclusive }
})

const statsRangeDateText = computed(() => {
  if (statsRangePreset.value === 'all') return '全部时间'
  const { startInclusive, endExclusive } = statsRangeBoundary.value
  if (!startInclusive || !endExclusive) return '全部时间'
  const endInclusive = addDays(endExclusive, -1)
  return `${localDateKey(startInclusive)} ~ ${localDateKey(endInclusive)}`
})

const statsRangeTotalDays = computed(() => {
  const { startInclusive, endExclusive } = statsRangeBoundary.value
  if (startInclusive && endExclusive) {
    const days = Math.floor((endExclusive.getTime() - startInclusive.getTime()) / (24 * 60 * 60 * 1000))
    return Math.max(1, days)
  }
  if ((statsView.value.byDay || []).length > 0) {
    const first = new Date(`${statsView.value.byDay[0].date}T00:00:00`)
    const last = new Date(`${statsView.value.byDay[statsView.value.byDay.length - 1].date}T00:00:00`)
    return Math.max(1, Math.floor((last.getTime() - first.getTime()) / (24 * 60 * 60 * 1000)) + 1)
  }
  return 0
})

const statsRangeRecords = computed(() => {
  const { startInclusive, endExclusive } = statsRangeBoundary.value
  if (!startInclusive || !endExclusive) return statsRecords.value
  return statsRecords.value.filter((r) => {
    const startMs = new Date(r.startAt).getTime()
    const endMs = new Date(r.endAt).getTime()
    if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs) return false
    return startMs < endExclusive.getTime() && endMs > startInclusive.getTime()
  })
})

const statsView = computed(() => {
  return buildStatsFromRecords(statsRangeRecords.value)
})
const statsScopePathNodes = computed(() => {
  if (!statsScopeNodeId.value) {
    return [{ id: null, name: '全部' }]
  }
  const path = []
  let cursor = nodeMap.value.get(statsScopeNodeId.value)
  while (cursor) {
    path.push({ id: cursor.id, name: cursor.name })
    const parentId = cursor.parentId
    if (parentId === null || parentId === undefined) break
    cursor = nodeMap.value.get(parentId)
  }
  path.reverse()
  return [{ id: null, name: '全部' }, ...path]
})
const statsScopeCurrentName = computed(() => {
  if (!statsScopeNodeId.value) return '全部'
  return nodeMap.value.get(statsScopeNodeId.value)?.name || '全部'
})
const statsAverageDailyMs = computed(() => {
  const days = Math.max(0, Number(statsRangeTotalDays.value || 0))
  if (days <= 0) return 0
  return Math.floor(Number(statsView.value.totalDurationMs || 0) / days)
})
const statsActiveDays = computed(() => {
  return (statsView.value.byDay || []).filter((row) => Number(row.durationMs || 0) > 0).length
})
const statsActiveAverageMs = computed(() => {
  const activeDays = Math.max(0, Number(statsActiveDays.value || 0))
  if (activeDays <= 0) return 0
  return Math.floor(Number(statsView.value.totalDurationMs || 0) / activeDays)
})

const timelineRecords = computed(() => records.value.slice(0, timelineVisibleCount.value))
const timelineHasMore = computed(() => records.value.length > timelineVisibleCount.value)
const statsTrendBuckets = computed(() => {
  const granularity = statsTrendGranularity.value
  const buckets = new Map()
  const ensureBucket = (bucketStart) => {
    const start = trendBucketStart(bucketStart, granularity)
    const key = trendBucketKey(start, granularity)
    if (!buckets.has(key)) {
      buckets.set(key, {
        key,
        start,
        label: trendBucketLabel(start, granularity),
        shortLabel: trendBucketShortLabel(start, granularity),
        durationMs: 0,
      })
    }
    return buckets.get(key)
  }

  let trendStart = null
  let trendEnd = null
  let minStart = null
  let maxEnd = null
  for (const r of statsRecords.value || []) {
    const start = new Date(r.startAt)
    const end = new Date(r.endAt)
    if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || end <= start) continue
    if (!minStart || start < minStart) minStart = start
    if (!maxEnd || end > maxEnd) maxEnd = end
  }
  if (minStart && maxEnd) {
    trendStart = trendBucketStart(minStart, granularity)
    trendEnd = trendBucketStart(maxEnd, granularity)
  } else {
    const now = new Date()
    trendStart = trendBucketStart(now, granularity)
    trendEnd = trendStart
  }

  let cursor = new Date(trendStart)
  while (cursor <= trendEnd) {
    ensureBucket(cursor)
    cursor = trendBucketNext(cursor, granularity)
  }

  for (const r of statsRecords.value || []) {
    const start = new Date(r.startAt)
    const end = new Date(r.endAt)
    const totalMs = end.getTime() - start.getTime()
    if (!Number.isFinite(totalMs) || totalMs <= 0) continue
    const pauseMs = Math.min(Math.max(0, Number(r.pauseDurationMs || 0)), totalMs)
    const effectiveMs = Math.max(0, totalMs - pauseMs)
    if (effectiveMs <= 0) continue
    let segCursor = trendBucketStart(start, granularity)
    while (segCursor < end) {
      const segNext = trendBucketNext(segCursor, granularity)
      const segStart = start > segCursor ? start : segCursor
      const segEnd = end < segNext ? end : segNext
      const segMs = segEnd.getTime() - segStart.getTime()
      if (segMs > 0) {
        const bucket = ensureBucket(segCursor)
        bucket.durationMs += Math.round((effectiveMs * segMs) / totalMs)
      }
      segCursor = segNext
    }
  }

  return [...buckets.values()].sort((a, b) => a.start - b.start)
})
const statsTrendDefaultWindow = computed(() => {
  const all = statsTrendBuckets.value
  if (all.length === 0) return { start: 0, end: -1 }
  const last = all.length - 1
  const minVisible = 10
  const { startInclusive, endExclusive } = statsRangeBoundary.value
  if (!startInclusive || !endExclusive) {
    const size = defaultStatsTrendWindowSize(statsTrendGranularity.value)
    const finalSize = Math.max(minVisible, size)
    return { start: Math.max(0, last - finalSize + 1), end: last }
  }
  const rangeStart = trendBucketStart(startInclusive, statsTrendGranularity.value)
  const rangeEnd = trendBucketStart(addDays(endExclusive, -1), statsTrendGranularity.value)
  let start = all.findIndex((row) => row.start >= rangeStart)
  if (start === -1) start = last
  let end = start
  for (let i = start; i < all.length; i++) {
    if (all[i].start <= rangeEnd) {
      end = i
    } else {
      break
    }
  }
  if (end < start) end = start
  let count = end - start + 1
  if (count < minVisible) {
    const need = minVisible - count
    const expandLeft = Math.min(start, need)
    start -= expandLeft
    count += expandLeft
    if (count < minVisible) {
      const remain = minVisible - count
      end = Math.min(last, end + remain)
    }
  }
  return { start, end }
})
const statsTrendVisibleBuckets = computed(() => {
  const all = statsTrendBuckets.value
  if (all.length === 0) return []
  const start = Math.max(0, Math.min(statsTrendWindowStart.value, all.length - 1))
  const end = Math.max(start, Math.min(statsTrendWindowEnd.value, all.length - 1))
  return all.slice(start, end + 1)
})
const statsTrendCanLoadLeft = computed(() => statsTrendWindowStart.value > 0)
const statsTrendCanLoadRight = computed(() => statsTrendWindowEnd.value >= 0 && statsTrendWindowEnd.value < statsTrendBuckets.value.length - 1)
const statsTrendHasMore = computed(() => {
  if (statsTrendEdge.value === 'left') return statsTrendCanLoadLeft.value
  if (statsTrendEdge.value === 'right') return statsTrendCanLoadRight.value
  return false
})
const statsTrendLoadMoreText = computed(() => (statsTrendEdge.value === 'left' ? '加载更早' : '加载更晚'))
const statsTrendMaxDurationMs = computed(() => {
  return statsTrendVisibleBuckets.value.reduce((max, row) => Math.max(max, Number(row.durationMs || 0)), 0)
})
const statsTrendLeftVisibleYear = computed(() => {
  const list = statsTrendVisibleBuckets.value
  if (list.length === 0) return '--'
  const unit = statsTrendUnitWidth()
  const index = Math.max(0, Math.min(list.length - 1, Math.floor(statsTrendScrollLeft.value / unit)))
  return String(list[index].start.getFullYear())
})
const statsTrendYAxisRows = computed(() => {
  const max = statsTrendMaxDurationMs.value
  const values = [max, Math.floor(max * 0.75), Math.floor(max * 0.5), Math.floor(max * 0.25), 0]
  return values.map((v) => ({ value: v, text: durationTextHourMin(v) }))
})

const treeRows = computed(() => {
  const out = []
  const walk = (parentId, depth, hiddenByParent) => {
    const list = childrenMap.value.get(parentId ?? 0) || []
    for (const n of list) {
      if (!settings.value.showHiddenNodes && n.hidden) continue
      const parentHidden = hiddenByParent || n.hidden
      out.push({ ...n, depth, hiddenByParent: parentHidden })
      if (n.type === 'category' && !n.collapsed) {
        walk(n.id, depth + 1, parentHidden)
      }
    }
  }
  walk(null, 0, false)
  return out
})

const statsTreeRows = computed(() => {
  const out = []
  const walk = (node, depth) => {
    if (!node) return
    if (hiddenSubtreeNodeIds.value.has(node.id)) return
    out.push({ ...node, depth })
    if (node.type !== 'category' || statsTreeCollapsedMap.value[node.id]) return
    const children = childrenMap.value.get(node.id) || []
    for (const child of children) {
      walk(child, depth + 1)
    }
  }
  if (statsScopeNodeId.value) {
    walk(nodeMap.value.get(statsScopeNodeId.value), 0)
    return out
  }
  for (const root of childrenMap.value.get(0) || []) {
    walk(root, 0)
  }
  return out
})

const nodeTodayTargetMap = computed(() => {
  const map = new Map()
  const visit = (node) => {
    let doneMin = 0
    let targetMin = 0
    if (node.type === 'item') {
      doneMin += Math.floor((todayItemDurationMap.value.get(node.id) || 0) / 60000)
      targetMin += Math.max(0, node.dailyTargetMinutes || 0)
    }
    const children = childrenMap.value.get(node.id) || []
    for (const child of children) {
      const c = visit(child)
      doneMin += c.doneMin
      targetMin += c.targetMin
    }
    map.set(node.id, { doneMin, targetMin })
    return { doneMin, targetMin }
  }
  for (const root of childrenMap.value.get(0) || []) {
    visit(root)
  }
  return map
})

const nodeTotalDurationMap = computed(() => {
  const map = new Map()
  const itemMap = new Map()
  for (const row of statsView.value.byItem || []) {
    itemMap.set(row.itemId, row.durationMs)
  }
  const visit = (node) => {
    let totalMs = 0
    if (node.type === 'item') {
      totalMs += itemMap.get(node.id) || 0
    }
    const children = childrenMap.value.get(node.id) || []
    for (const child of children) {
      totalMs += visit(child)
    }
    map.set(node.id, totalMs)
    return totalMs
  }
  for (const root of childrenMap.value.get(0) || []) {
    visit(root)
  }
  return map
})

const statsNodeOffsetMap = computed(() => {
  const map = new Map()
  const total = Number(statsView.value.totalDurationMs || 0)
  if (total <= 0) return map

  const visit = (node, parentOffsetMs) => {
    map.set(node.id, (parentOffsetMs / total) * 100)
    let currentOffsetMs = parentOffsetMs
    const children = childrenMap.value.get(node.id) || []
    for (const child of children) {
      visit(child, currentOffsetMs)
      currentOffsetMs += nodeTotalDurationMap.value.get(child.id) || 0
    }
  }

  if (statsScopeNodeId.value) {
    const scopeNode = nodeMap.value.get(statsScopeNodeId.value)
    if (scopeNode) {
      visit(scopeNode, 0)
    }
  } else {
    let rootOffsetMs = 0
    const roots = childrenMap.value.get(0) || []
    for (const root of roots) {
      visit(root, rootOffsetMs)
      rootOffsetMs += nodeTotalDurationMap.value.get(root.id) || 0
    }
  }
  return map
})

const heatmapMonthColumns = computed(() => {
  const byDayRows = buildStatsFromRecords(statsRecords.value).byDay || []
  const byDayMap = new Map(byDayRows.map((row) => [row.date, Number(row.durationMs || 0)]))
  const maxDurationMs = byDayRows.reduce((max, row) => Math.max(max, Number(row.durationMs || 0)), 0)
  const firstDateText = byDayRows[0]?.date
  const lastDateText = byDayRows[byDayRows.length - 1]?.date
  let startMonth = firstDateText ? startOfMonth(new Date(`${firstDateText}T00:00:00`)) : startOfMonth(new Date())
  const endMonth = lastDateText ? startOfMonth(new Date(`${lastDateText}T00:00:00`)) : startOfMonth(new Date())
  const minStartMonth = addMonths(endMonth, -23)
  if (startMonth > minStartMonth) {
    startMonth = minStartMonth
  }
  const months = []
  let cursor = new Date(startMonth)
  while (cursor <= endMonth) {
    const year = cursor.getFullYear()
    const month = cursor.getMonth() + 1
    const daysInMonth = new Date(year, month, 0).getDate()
    const days = Array.from({ length: daysInMonth }, (_, idx) => {
      const day = idx + 1
      const date = localDateKey(new Date(year, month - 1, day))
      const durationMs = byDayMap.get(date) || 0
      let level = 0
      if (durationMs > 0 && maxDurationMs > 0) {
        level = Math.max(1, Math.min(4, Math.ceil((durationMs / maxDurationMs) * 4)))
      }
      return { date, durationMs, level }
    })
    months.push({
      key: `${year}-${String(month).padStart(2, '0')}`,
      yearText: month === 1 ? `${String(year).slice(-2)}\n年` : '',
      monthText: `${month}\n月`,
      days,
    })
    cursor = addMonths(cursor, 1)
  }
  return months
})

function pieSlicePath(startAngle, endAngle) {
  const center = 56
  const radius = 52
  const startRad = ((startAngle - 90) * Math.PI) / 180
  const endRad = ((endAngle - 90) * Math.PI) / 180
  const x1 = center + radius * Math.cos(startRad)
  const y1 = center + radius * Math.sin(startRad)
  const x2 = center + radius * Math.cos(endRad)
  const y2 = center + radius * Math.sin(endRad)
  const largeArcFlag = endAngle - startAngle > 180 ? 1 : 0
  return `M ${center} ${center} L ${x1} ${y1} A ${radius} ${radius} 0 ${largeArcFlag} 1 ${x2} ${y2} Z`
}

const categoryPieData = computed(() => {
  const scopeId = statsScopeNodeId.value || 0
  const children = childrenMap.value.get(scopeId) || []
  const rows = children
    .map((node) => ({
      nodeId: node.id,
      name: node.name,
      durationMs: nodeTotalDurationMap.value.get(node.id) || 0,
    }))
    .filter((row) => row.durationMs > 0)
  const total = rows.reduce((sum, row) => sum + row.durationMs, 0)
  if (total <= 0) return []
  const palette = ['#4f46e5', '#06b6d4', '#22c55e', '#f59e0b', '#ef4444', '#8b5cf6', '#14b8a6', '#f97316']
  let currentAngle = 0
  return rows.map((row, idx) => {
    const percent = Math.floor((row.durationMs / total) * 100)
    const angle = (row.durationMs / total) * 360
    const startAngle = currentAngle
    const endAngle = currentAngle + angle
    currentAngle = endAngle
    return {
      ...row,
      percent,
      color: palette[idx % palette.length],
      path: pieSlicePath(startAngle, endAngle),
    }
  })
})

function hourBucketLabel(hour) {
  if (hour < 6) return '00:00-05:59'
  if (hour < 12) return '06:00-11:59'
  if (hour < 18) return '12:00-17:59'
  return '18:00-23:59'
}

const crossViewRows = computed(() => {
  const agg = new Map()
  for (const r of statsRangeRecords.value || []) {
    const start = new Date(r.startAt)
    const end = new Date(r.endAt)
    const totalMs = end.getTime() - start.getTime()
    if (!Number.isFinite(totalMs) || totalMs <= 0) continue
    const pauseMs = Math.min(Math.max(0, Number(r.pauseDurationMs || 0)), totalMs)
    const effectiveMs = Math.max(0, totalMs - pauseMs)
    let categoryName = '未分类'
    let cursor = nodeMap.value.get(r.itemId)
    while (cursor) {
      if (cursor.type === 'category') {
        categoryName = cursor.name
        break
      }
      cursor = nodeMap.value.get(cursor.parentId)
    }
    const date = localDateKey(start)
    const bucket = hourBucketLabel(start.getHours())
    const key = `${date}|${categoryName}|${bucket}`
    if (!agg.has(key)) {
      agg.set(key, { date, categoryName, bucket, durationMs: 0, timerCount: 0, recordCount: 0 })
    }
    const row = agg.get(key)
    row.durationMs += effectiveMs
    row.recordCount += 1
    if (r.source === 'timer') row.timerCount += 1
  }
  return [...agg.values()]
    .sort((a, b) => b.durationMs - a.durationMs || b.timerCount - a.timerCount || b.date.localeCompare(a.date))
    .slice(0, 15)
})

function startOfDay(date) {
  const d = new Date(date)
  d.setHours(0, 0, 0, 0)
  return d
}

function addDays(date, days) {
  const d = new Date(date)
  d.setDate(d.getDate() + days)
  return d
}

function startOfMonth(date) {
  const d = new Date(date)
  d.setDate(1)
  d.setHours(0, 0, 0, 0)
  return d
}

function addMonths(date, months) {
  const d = new Date(date)
  d.setMonth(d.getMonth() + months)
  return startOfMonth(d)
}

function startOfWeek(date) {
  const d = startOfDay(date)
  const offset = (d.getDay() + 6) % 7
  d.setDate(d.getDate() - offset)
  return d
}

function trendBucketStart(date, granularity) {
  if (granularity === 'month') return startOfMonth(date)
  if (granularity === 'week') return startOfWeek(date)
  return startOfDay(date)
}

function trendBucketNext(date, granularity) {
  if (granularity === 'month') return addMonths(date, 1)
  if (granularity === 'week') return addDays(date, 7)
  return addDays(date, 1)
}

function trendBucketKey(date, granularity) {
  if (granularity === 'month') {
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`
  }
  return localDateKey(date)
}

function trendBucketLabel(date, granularity) {
  if (granularity === 'month') {
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`
  }
  if (granularity === 'week') {
    const end = addDays(date, 6)
    return `${localDateKey(date)} ~ ${localDateKey(end)}`
  }
  return localDateKey(date)
}

function trendBucketShortLabel(date, granularity) {
  if (granularity === 'month') {
    return `${String(date.getFullYear()).slice(-2)}-${String(date.getMonth() + 1).padStart(2, '0')}`
  }
  if (granularity === 'week') {
    return `${String(date.getMonth() + 1).padStart(2, '0')}/${String(date.getDate()).padStart(2, '0')}`
  }
  return `${String(date.getMonth() + 1).padStart(2, '0')}/${String(date.getDate()).padStart(2, '0')}`
}

function defaultStatsTrendWindowSize(granularity) {
  if (granularity === 'month') return 12
  if (granularity === 'week') return 16
  return 21
}

function statsTrendLoadStep(granularity) {
  if (granularity === 'month') return 6
  if (granularity === 'week') return 8
  return 14
}

function statsTrendUnitWidth() {
  return 38
}

function resetStatsTrendWindow() {
  const { start, end } = statsTrendDefaultWindow.value
  statsTrendWindowStart.value = start
  statsTrendWindowEnd.value = end
  statsTrendScrollLeft.value = 0
  statsTrendEdge.value = 'none'
}

function statsTrendBarHeight(durationMs) {
  const value = Number(durationMs || 0)
  if (value <= 0) return 0
  const max = statsTrendMaxDurationMs.value
  if (max <= 0) return 0
  const ratio = value / max
  return Math.max(8, Math.round(ratio * 150))
}

function onStatsTrendScroll(event) {
  const el = event?.target
  if (!el) return
  statsTrendScrollLeft.value = el.scrollLeft || 0
  const maxScroll = Math.max(0, el.scrollWidth - el.clientWidth)
  if (maxScroll <= 0) {
    if (statsTrendCanLoadLeft.value) {
      statsTrendEdge.value = 'left'
    } else if (statsTrendCanLoadRight.value) {
      statsTrendEdge.value = 'right'
    } else {
      statsTrendEdge.value = 'none'
    }
    return
  }
  if (el.scrollLeft <= 2) {
    statsTrendEdge.value = 'left'
  } else if (el.scrollLeft >= maxScroll - 2) {
    statsTrendEdge.value = 'right'
  } else {
    statsTrendEdge.value = 'none'
  }
}

function showStatsTrendTip(bar) {
  window.alert(`${bar.label} ${durationTextHourMin(bar.durationMs)}`)
}

async function loadMoreStatsTrend() {
  if (!statsTrendHasMore.value) return
  const step = statsTrendLoadStep(statsTrendGranularity.value)
  if (statsTrendEdge.value === 'left' && statsTrendCanLoadLeft.value) {
    statsTrendWindowStart.value = Math.max(0, statsTrendWindowStart.value - step)
    await nextTick()
    const el = statsTrendWrapRef.value
    if (el) {
      el.scrollLeft = 0
      onStatsTrendScroll({ target: el })
    }
    return
  }
  if (statsTrendEdge.value === 'right' && statsTrendCanLoadRight.value) {
    statsTrendWindowEnd.value = Math.min(statsTrendBuckets.value.length - 1, statsTrendWindowEnd.value + step)
    await nextTick()
    const el = statsTrendWrapRef.value
    if (el) {
      el.scrollLeft = el.scrollWidth
      onStatsTrendScroll({ target: el })
    }
  }
}

function scrollHeatmapToRight() {
  const el = heatmapWrapRef.value
  if (!el) return
  el.scrollLeft = el.scrollWidth
}

watch(
  () => [activeTab.value, heatmapMonthColumns.value.length],
  async ([tab, count]) => {
    if (tab !== 'stats' || count <= 0) return
    await nextTick()
    scrollHeatmapToRight()
  },
)

watch(
  () => activeTab.value,
  (tab, prev) => {
    syncHashFromTab(tab)
    if (tab === 'stats' && prev !== 'stats') {
      timelineVisibleCount.value = 20
      resetStatsTrendWindow()
      nextTick(() => {
        const el = statsTrendWrapRef.value
        if (!el) return
        el.scrollLeft = el.scrollWidth
        onStatsTrendScroll({ target: el })
      })
    }
  },
)

watch(
  () => statsTrendGranularity.value,
  () => {
    resetStatsTrendWindow()
    nextTick(() => {
      const el = statsTrendWrapRef.value
      if (!el) return
      el.scrollLeft = el.scrollWidth
      onStatsTrendScroll({ target: el })
    })
  },
)

watch(
  () => [statsRangePreset.value, statsRangeStartDate.value, statsRangeEndDate.value, statsScopeNodeId.value],
  () => {
    resetStatsTrendWindow()
    nextTick(() => {
      const el = statsTrendWrapRef.value
      if (!el) return
      el.scrollLeft = el.scrollWidth
      onStatsTrendScroll({ target: el })
    })
  },
)

const todayItemDurationMap = computed(() => {
  const start = new Date()
  start.setHours(0, 0, 0, 0)
  const end = new Date(start)
  end.setDate(end.getDate() + 1)
  const map = new Map()
  for (const r of records.value) {
    const rs = new Date(r.startAt)
    const re = new Date(r.endAt)
    const overlapStart = rs > start ? rs : start
    const overlapEnd = re < end ? re : end
    const overlapMs = overlapEnd - overlapStart
    if (overlapMs <= 0) continue
    const totalMs = re - rs
    const pausePart = totalMs > 0 ? Math.floor((r.pauseDurationMs || 0) * overlapMs / totalMs) : 0
    const effective = Math.max(0, overlapMs - pausePart)
    map.set(r.itemId, (map.get(r.itemId) || 0) + effective)
  }
  return map
})

const now = ref(Date.now())

function calcTimerElapsedRawMs(state, currentNowMs) {
  if (!state?.sessionStartAt) return 0
  const start = new Date(state.sessionStartAt).getTime()
  const pausedMs = state.accumulatedPauseMs || 0
  if (state.isPaused && state.pauseStartedAt) {
    const pauseStart = new Date(state.pauseStartedAt).getTime()
    return Math.max(0, pauseStart - start - pausedMs)
  }
  return Math.max(0, currentNowMs - start - pausedMs)
}

function floorToSecondMs(ms) {
  return Math.max(0, Math.floor(Number(ms || 0) / 1000) * 1000)
}

const timerElapsedMs = computed(() => {
  if (!timerState.value.sessionStartAt) return 0
  if (timerFreezeMs.value != null) return timerFreezeMs.value
  if (timerLocalResumeAt.value != null && !timerState.value.isPaused) {
    const localExpected = timerLocalResumeBaseMs.value + floorToSecondMs(now.value - timerLocalResumeAt.value)
    return Math.max(localExpected, timerMinDisplayMs.value)
  }
  const raw = floorToSecondMs(calcTimerElapsedRawMs(timerState.value, now.value))
  return Math.max(raw, timerMinDisplayMs.value)
})

const timerItem = computed(() => {
  const id = timerState.value.activeItemId || selectedItemIdForTimer.value
  if (!id) return null
  return nodeMap.value.get(id) || null
})

const isLoggedIn = computed(() => !!me.value)
const moveSourceNode = computed(() => {
  if (!moveDraft.value.sourceId) return null
  return nodeMap.value.get(moveDraft.value.sourceId) || null
})
const moveTargetNode = computed(() => {
  if (!moveDraft.value.targetId) return null
  return nodeMap.value.get(moveDraft.value.targetId) || null
})
const isMovingNode = computed(() => !!moveSourceNode.value)
const moveSourceLabel = computed(() => moveSourceNode.value?.name || '请选择节点')
const moveTargetLabel = computed(() => moveTargetNode.value?.name || '请选择节点')
const canMoveBeforeAvailable = computed(() => {
  if (!moveSourceNode.value || !moveTargetNode.value) return false
  return canMoveBeforeAfter(moveSourceNode.value.id, moveTargetNode.value.id)
})
const canMoveAfterAvailable = computed(() => {
  if (!moveSourceNode.value || !moveTargetNode.value) return false
  return canMoveBeforeAfter(moveSourceNode.value.id, moveTargetNode.value.id)
})
const nodeMenuNode = computed(() => {
  if (!nodeMenuId.value) return null
  return nodeMap.value.get(nodeMenuId.value) || null
})
const recordMenuRecord = computed(() => {
  if (!recordMenuId.value) return null
  return records.value.find((r) => r.id === recordMenuId.value) || null
})
const canMoveDownAvailable = computed(() => {
  if (!moveSourceNode.value || !moveTargetNode.value) return false
  return canMoveInto(moveSourceNode.value.id, moveTargetNode.value.id)
})
const showMoveInsideButton = computed(() => {
  if (!moveTargetNode.value || moveTargetNode.value.type !== 'category') return false
  const children = childrenMap.value.get(moveTargetNode.value.id) || []
  return children.length === 0
})
const moveActionShortText = computed(() => (moveTargetNode.value?.type === 'category' ? '前/后/下' : '前/后'))
const moveActionQuotedText = computed(() => (moveTargetNode.value?.type === 'category' ? '“前”“后”或“下”' : '“前”“后”'))
const nodeMenuStyle = computed(() => {
  const vw = window.innerWidth || 360
  const vh = window.innerHeight || 640
  const menuWidth = 176
  const minGap = 12
  const left = Math.min(Math.max(minGap, nodeMenuPosition.value.x), vw - menuWidth - minGap)
  const top = Math.min(Math.max(minGap, nodeMenuPosition.value.y), vh - 260)
  return { left: `${left}px`, top: `${top}px` }
})
const recordMenuStyle = computed(() => {
  const vw = window.innerWidth || 360
  const vh = window.innerHeight || 640
  const menuWidth = 176
  const minGap = 12
  const left = Math.min(Math.max(minGap, recordMenuPosition.value.x), vw - menuWidth - minGap)
  const top = Math.min(Math.max(minGap, recordMenuPosition.value.y), vh - 160)
  return { left: `${left}px`, top: `${top}px` }
})

function durationText(ms) {
  const totalSec = Math.floor(ms / 1000)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  
  const pad = (num) => String(num).padStart(2, '0')
  return `${pad(h)}:${pad(m)}:${pad(s)}`
}

function durationTextHourMin(ms) {
  const totalMin = Math.floor(ms / 60000)
  const h = Math.floor(totalMin / 60)
  const m = totalMin % 60
  return `${h}h ${m}m`
}

function shortDateTime(valueMs) {
  const d = new Date(valueMs)
  const pad = (num) => String(num).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function toInputDateTime(value) {
  if (!value) return ''
  const d = new Date(value)
  const pad = (x) => String(x).padStart(2, '0')
  const y = d.getFullYear()
  const mo = pad(d.getMonth() + 1)
  const da = pad(d.getDate())
  const hh = pad(d.getHours())
  const mm = pad(d.getMinutes())
  return `${y}-${mo}-${da}T${hh}:${mm}`
}

function fromInputDateTime(v) {
  if (!v) return ''
  const d = new Date(v)
  return d.toISOString()
}

function getDatePart(v) {
  if (!v) return ''
  return String(v).split('T')[0] || ''
}

function getTimePart(v) {
  if (!v) return ''
  const t = String(v).split('T')[1] || ''
  return t.slice(0, 5)
}

function setDatePart(field, value) {
  const current = manualRecordDraft.value[field] || ''
  const time = getTimePart(current) || '00:00'
  manualRecordDraft.value[field] = value ? `${value}T${time}` : ''
}

function setTimePart(field, value) {
  const current = manualRecordDraft.value[field] || ''
  const date = getDatePart(current) || getDatePart(toInputDateTime(new Date().toISOString()))
  manualRecordDraft.value[field] = value ? `${date}T${value}` : ''
}

async function api(path, options = {}) {
  const res = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  })
  if (res.status === 401) {
    me.value = null
    throw new Error('未登录')
  }
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    throw new Error(data?.message || '请求失败')
  }
  return data
}

async function checkMe() {
  try {
    me.value = await api('/api/auth/me')
  } catch {
    me.value = null
  } finally {
    isCheckingAuth.value = false
  }
}

async function doLogin() {
  loginError.value = ''
  try {
    await api('/api/auth/login', { method: 'POST', body: JSON.stringify(loginForm.value) })
    await bootstrap()
  } catch (e) {
    loginError.value = e.message
  }
}

async function doLogout() {
  await api('/api/auth/logout', { method: 'POST' })
  me.value = null
}

async function loadNodes() {
  nodes.value = await api('/api/nodes')
}

async function loadRecords() {
  const params = new URLSearchParams()
  if (recordFilters.value.fromDate) {
    params.set('from', new Date(`${recordFilters.value.fromDate}T00:00:00`).toISOString())
  }
  if (recordFilters.value.toDate) {
    params.set('to', new Date(`${recordFilters.value.toDate}T23:59:59`).toISOString())
  }
  if (recordFilters.value.itemId) {
    params.set('itemId', String(recordFilters.value.itemId))
  }
  if (recordFilters.value.source) {
    params.set('source', recordFilters.value.source)
  }
  const query = params.toString()
  records.value = await api(`/api/records${query ? `?${query}` : ''}`)
}

async function loadStatsBaseRecords() {
  statsBaseRecords.value = await api('/api/records')
}

async function applyRecordFilters() {
  await loadRecords()
}

async function resetRecordFilters() {
  recordFilters.value.fromDate = ''
  recordFilters.value.toDate = ''
  recordFilters.value.itemId = ''
  recordFilters.value.source = ''
  await loadRecords()
}

async function loadTimerState() {
  timerState.value = await api('/api/timer/state')
  if (!timerState.value.sessionStartAt) {
    timerFreezeMs.value = null
    timerMinDisplayMs.value = 0
    timerLocalResumeAt.value = null
    timerLocalResumeBaseMs.value = 0
  } else if (timerState.value.isPaused && timerFreezeMs.value == null) {
    timerFreezeMs.value = floorToSecondMs(calcTimerElapsedRawMs(timerState.value, Date.now()))
    timerLocalResumeAt.value = null
  } else if (!timerState.value.isPaused && timerLocalResumeAt.value == null) {
    const base = floorToSecondMs(calcTimerElapsedRawMs(timerState.value, Date.now()))
    timerLocalResumeBaseMs.value = base
    timerLocalResumeAt.value = Date.now()
    timerMinDisplayMs.value = Math.max(timerMinDisplayMs.value, base)
  }
}

async function loadSettings() {
  loadingSettings.value = true
  try {
    const remoteSettings = await api('/api/settings')
    settings.value = {
      ...settings.value,
      ...remoteSettings,
    }
  } finally {
    loadingSettings.value = false
  }
}

async function bootstrap() {
  await checkMe()
  if (!me.value) return
  await Promise.all([loadNodes(), loadRecords(), loadStatsBaseRecords(), loadTimerState(), loadSettings()])
}

async function createNode() {
  addNodeError.value = ''
  const name = createNodeDraft.value.name.trim()
  if (!name) {
    addNodeError.value = '请输入节点名称'
    return
  }
  let parentId = null
  if (createNodeDraft.value.parentId !== null && createNodeDraft.value.parentId !== '' && createNodeDraft.value.parentId !== 'null') {
    parentId = Number(createNodeDraft.value.parentId)
    if (!Number.isFinite(parentId)) {
      parentId = null
    }
  }
  try {
    await api('/api/nodes', {
      method: 'POST',
      body: JSON.stringify({
        type: createNodeDraft.value.type,
        name,
        parentId,
        orderNo: Date.now(),
        hidden: false,
        collapsed: false,
        dailyTargetMinutes: createNodeDraft.value.type === 'item'
          ? Math.max(0, Number(createNodeDraft.value.dailyTargetMinutes || 0))
          : 0,
      }),
    })
    createNodeDraft.value.name = ''
    createNodeDraft.value.parentId = null
    showAddNodeModal.value = false
    await loadNodes()
  } catch (e) {
    addNodeError.value = e.message || '添加失败'
  }
}

function openAddNodeModal(parentId = null) {
  addNodeError.value = ''
  createNodeDraft.value.parentId = parentId
  showAddNodeModal.value = true
}

async function updateNode(id, patch) {
  await api(`/api/nodes/${id}`, { method: 'PUT', body: JSON.stringify(patch) })
  await loadNodes()
}

async function removeNode(id) {
  await api(`/api/nodes/${id}`, { method: 'DELETE' })
  await Promise.all([loadNodes(), loadRecords(), loadStatsBaseRecords()])
}

async function startTimer(itemId) {
  if (!itemId) return
  timerFreezeMs.value = null
  timerMinDisplayMs.value = 0
  timerLocalResumeAt.value = null
  timerLocalResumeBaseMs.value = 0
  await api('/api/timer/start', {
    method: 'POST',
    body: JSON.stringify({ itemId, startAt: new Date().toISOString(), draftDescription: '' }),
  })
  await loadTimerState()
  selectedItemIdForTimer.value = null
}

async function pauseTimer() {
  const frozen = floorToSecondMs(timerElapsedMs.value)
  timerFreezeMs.value = frozen
  timerMinDisplayMs.value = Math.max(timerMinDisplayMs.value, frozen)
  timerLocalResumeAt.value = null
  if (!timerState.value.isPaused) {
    timerState.value.isPaused = true
    timerState.value.pauseStartedAt = new Date().toISOString()
  }
  await api('/api/timer/pause', { method: 'POST', body: '{}' })
  await loadTimerState()
}

async function resumeTimer() {
  const base = floorToSecondMs(timerElapsedMs.value)
  timerMinDisplayMs.value = Math.max(timerMinDisplayMs.value, base)
  timerLocalResumeBaseMs.value = base
  timerLocalResumeAt.value = Date.now()
  if (timerState.value.isPaused && timerState.value.pauseStartedAt) {
    const addMs = Date.now() - new Date(timerState.value.pauseStartedAt).getTime()
    if (Number.isFinite(addMs) && addMs > 0) {
      timerState.value.accumulatedPauseMs = (timerState.value.accumulatedPauseMs || 0) + addMs
    }
    timerState.value.isPaused = false
    timerState.value.pauseStartedAt = null
  }
  timerFreezeMs.value = null
  await api('/api/timer/resume', { method: 'POST', body: '{}' })
  await loadTimerState()
}

async function stopTimer() {
  let desc = ''
  desc = window.prompt('请输入结束描述（可选）:') || ''
  await api('/api/timer/stop', {
    method: 'POST',
    body: JSON.stringify({
      endAt: new Date().toISOString(),
      description: desc,
      save: true,
    }),
  })
  timerLocalResumeAt.value = null
  timerLocalResumeBaseMs.value = 0
  await Promise.all([loadTimerState(), loadRecords(), loadStatsBaseRecords()])
}

function openManualRecordModal(preferredItemId = null) {
  const id = preferredItemId || timerState.value.activeItemId || selectedItemIdForTimer.value
  if (!id) return
  
  const end = new Date()
  const start = new Date(end.getTime() - 30 * 60000)
  
  manualRecordDraft.value = {
    id: null,
    itemId: id,
    startAt: toInputDateTime(start.toISOString()),
    endAt: toInputDateTime(end.toISOString()),
    pauseDurationSec: 0,
    description: '',
    applySplitByDate: true,
  }
  showManualRecordModal.value = true
}

function openManualRecordForItem(itemId) {
  if (!itemId) return
  openManualRecordModal(itemId)
}

async function addManualRecord() {
  if (!manualRecordDraft.value.itemId) return
  if (!manualRecordDraft.value.startAt || !manualRecordDraft.value.endAt) return
  savingRecord.value = true
  try {
    const body = {
      itemId: Number(manualRecordDraft.value.itemId),
      startAt: fromInputDateTime(manualRecordDraft.value.startAt),
      endAt: fromInputDateTime(manualRecordDraft.value.endAt),
      pauseDurationMs: Number(manualRecordDraft.value.pauseDurationSec || 0) * 60 * 1000,
      description: manualRecordDraft.value.description || '',
    }
    if (manualRecordDraft.value.id) {
      await api(`/api/records/${manualRecordDraft.value.id}`, {
        method: 'PUT',
        body: JSON.stringify(body),
      })
    } else {
      await api('/api/records', {
        method: 'POST',
        body: JSON.stringify({ ...body, source: 'manual', applySplitByDate: manualRecordDraft.value.applySplitByDate }),
      })
    }
    manualRecordDraft.value.id = null
    manualRecordDraft.value.description = ''
    showManualRecordModal.value = false
    await Promise.all([loadRecords(), loadStatsBaseRecords()])
  } finally {
    savingRecord.value = false
  }
}

function openEditRecordModal(record) {
  manualRecordDraft.value = {
    id: record.id,
    itemId: record.itemId,
    startAt: toInputDateTime(record.startAt),
    endAt: toInputDateTime(record.endAt),
    pauseDurationSec: Math.floor((record.pauseDurationMs || 0) / 60000),
    description: record.description || '',
    applySplitByDate: false,
  }
  showManualRecordModal.value = true
}

async function removeRecord(id) {
  await api(`/api/records/${id}`, { method: 'DELETE' })
  await Promise.all([loadRecords(), loadStatsBaseRecords()])
}

async function saveSettings(patch) {
  if (loadingSettings.value) return
  await api('/api/settings', {
    method: 'PUT',
    body: JSON.stringify(patch),
  })
}

async function onChangeShowHiddenNodes() {
  const next = !!settings.value.showHiddenNodes
  try {
    await saveSettings({ showHiddenNodes: next })
  } catch (e) {
    settings.value.showHiddenNodes = !next
    throw e
  }
}

async function onChangeSkipShortTimerRecord() {
  const next = !!settings.value.skipShortTimerRecord
  try {
    await saveSettings({ skipShortTimerRecord: next })
  } catch (e) {
    settings.value.skipShortTimerRecord = !next
    throw e
  }
}

async function onChangeStatsIncludeHiddenNodes() {
  const next = !!settings.value.statsIncludeHiddenNodes
  try {
    await saveSettings({ statsIncludeHiddenNodes: next })
  } catch (e) {
    settings.value.statsIncludeHiddenNodes = !next
    throw e
  }
}

function progress(node) {
  const today = nodeTodayTargetMap.value.get(node.id) || { doneMin: 0, targetMin: 0 }
  const doneMs = today.doneMin * 60000
  const targetMs = today.targetMin * 60000
  if (targetMs === 0) {
    return { doneMs, targetMs, percent: 0, barPercent: 0, done: false }
  }
  const percent = Math.floor((doneMs / targetMs) * 100)
  return {
    doneMs,
    targetMs,
    percent,
    barPercent: Math.min(100, Math.max(0, percent)),
    done: percent >= 100,
  }
}

function todayTargetText(node) {
  const v = nodeTodayTargetMap.value.get(node.id) || { doneMin: 0, targetMin: 0 }
  if (v.targetMin > 0) {
    return `${v.doneMin}/${v.targetMin}m`
  }
  return `${v.doneMin}m`
}

function startStatsNodePress(node) {
  clearStatsNodePress()
  statsLongPressFired.value = false
  statsLongPressTimer.value = setTimeout(() => {
    statsLongPressFired.value = true
    openStatsForNode(node.id)
  }, 500)
}

function clearStatsNodePress() {
  if (statsLongPressTimer.value) {
    clearTimeout(statsLongPressTimer.value)
    statsLongPressTimer.value = null
  }
}

function onStatsNodeClick(node) {
  if (statsLongPressFired.value) {
    statsLongPressFired.value = false
    return
  }
  if (node.type === 'category') {
    toggleStatsNodeCollapsed(node.id)
  }
}

function statsNodeDurationMs(node) {
  return nodeTotalDurationMap.value.get(node.id) || 0
}

function statsNodePercent(node) {
  const total = Number(statsView.value.totalDurationMs || 0)
  if (total <= 0) return 0
  const percent = (statsNodeDurationMs(node) / total) * 100
  return Math.min(100, Math.max(0, percent))
}

function statsNodePercentText(node) {
  return `${statsNodePercent(node).toFixed(1)}%`
}

function statsNodeColor(node) {
  if (node.type === 'item') return '#10b981'
  const colors = [
    '#4f46e5',
    '#f59e0b',
    '#ef4444',
    '#06b6d4',
    '#8b5cf6',
    '#ec4899',
  ]
  return colors[node.depth % colors.length]
}

function statsDurationText(ms) {
  const totalMin = Math.max(0, Math.floor(Number(ms || 0) / 60000))
  const days = Math.floor(totalMin / (24 * 60))
  const hours = Math.floor((totalMin % (24 * 60)) / 60)
  const mins = totalMin % 60
  if (days > 0) {
    return `${days}天${hours}时`
  }
  return `${hours}时${mins}分`
}

function showHeatmapCellTip(cell) {
  window.alert(`${cell.date} ${durationTextHourMin(cell.durationMs)}`)
}

function loadMoreTimeline() {
  timelineVisibleCount.value += 20
}

function getPressPoint(event) {
  if (event?.touches?.length) {
    return { x: event.touches[0].clientX, y: event.touches[0].clientY }
  }
  if (event?.changedTouches?.length) {
    return { x: event.changedTouches[0].clientX, y: event.changedTouches[0].clientY }
  }
  if (typeof event?.clientX === 'number' && typeof event?.clientY === 'number') {
    return { x: event.clientX, y: event.clientY }
  }
  const rect = event?.currentTarget?.getBoundingClientRect?.()
  if (rect) {
    return { x: rect.right - 8, y: rect.top + 12 }
  }
  return { x: 24, y: 140 }
}

function currentPageScrollTop() {
  return window.scrollY || document.documentElement.scrollTop || document.body.scrollTop || 0
}

function startNodePress(node, event) {
  if (isMovingNode.value) return
  const point = getPressPoint(event)
  clearNodePress()
  longPressFired.value = false
  longPressTimer.value = setTimeout(() => {
    longPressFired.value = true
    openNodeMenu(node, point)
  }, 500)
}

function clearNodePress() {
  if (longPressTimer.value) {
    clearTimeout(longPressTimer.value)
    longPressTimer.value = null
  }
}

function openNodeMenu(node, point) {
  nodeMenuPosition.value = { x: (point?.x || 24) + 6, y: (point?.y || 140) + 6 }
  nodeMenuId.value = node.id
}

function startRecordPress(record, event) {
  const point = getPressPoint(event)
  clearRecordPress()
  recordLongPressFired.value = false
  recordPressStartPoint.value = point
  recordPressStartScrollTop.value = currentPageScrollTop()
  recordLongPressTimer.value = setTimeout(() => {
    const scrolled = Math.abs(currentPageScrollTop() - recordPressStartScrollTop.value) > 2
    if (scrolled) return
    recordLongPressFired.value = true
    openRecordMenu(record, point)
  }, 500)
}

function onRecordPressMove(event) {
  if (!recordLongPressTimer.value || !recordPressStartPoint.value) return
  const point = getPressPoint(event)
  const dx = Math.abs(point.x - recordPressStartPoint.value.x)
  const dy = Math.abs(point.y - recordPressStartPoint.value.y)
  const moved = dx > 10 || dy > 10
  const scrolled = Math.abs(currentPageScrollTop() - recordPressStartScrollTop.value) > 2
  if (moved || scrolled) {
    clearRecordPress()
  }
}

function clearRecordPress() {
  if (recordLongPressTimer.value) {
    clearTimeout(recordLongPressTimer.value)
    recordLongPressTimer.value = null
  }
  recordPressStartPoint.value = null
  recordPressStartScrollTop.value = 0
}

function openRecordMenu(record, point) {
  recordMenuPosition.value = { x: (point?.x || 24) + 6, y: (point?.y || 140) + 6 }
  recordMenuId.value = record.id
}

function closeRecordMenu() {
  recordMenuId.value = null
}

function openNodeEditModal(node) {
  nodeActionError.value = ''
  moveDraft.value = { sourceId: null, targetId: null }
  moveTip.value = ''
  nodeMenuId.value = null
  nodeActionDraft.value = {
    id: node.id,
    type: node.type,
    name: node.name,
    hidden: !!node.hidden,
    dailyTargetMinutes: Number(node.dailyTargetMinutes || 0),
  }
  showNodeActionModal.value = true
}

function openStatsForNode(nodeId) {
  nodeMenuId.value = null
  statsScopeNodeId.value = nodeId
  activeTab.value = 'stats'
  showNodeActionModal.value = false
}

function clearStatsScope() {
  statsScopeNodeId.value = null
}

function jumpStatsScope(nodeId) {
  if (nodeId === null || nodeId === undefined) {
    clearStatsScope()
    return
  }
  statsScopeNodeId.value = nodeId
}

function toggleStatsModule(key) {
  statsModuleCollapsed.value = {
    ...statsModuleCollapsed.value,
    [key]: !statsModuleCollapsed.value[key],
  }
}

function isStatsModuleCollapsed(key) {
  return !!statsModuleCollapsed.value[key]
}

function isStatsNodeCollapsed(nodeId) {
  return !!statsTreeCollapsedMap.value[nodeId]
}

function toggleStatsNodeCollapsed(nodeId) {
  statsTreeCollapsedMap.value = {
    ...statsTreeCollapsedMap.value,
    [nodeId]: !statsTreeCollapsedMap.value[nodeId],
  }
}

async function saveNodeAction() {
  nodeActionError.value = ''
  const name = nodeActionDraft.value.name.trim()
  if (!name) {
    nodeActionError.value = '名称不能为空'
    return
  }
  try {
    const patch = {
      name,
      hidden: !!nodeActionDraft.value.hidden,
    }
    if (nodeActionDraft.value.type === 'item') {
      patch.dailyTargetMinutes = Math.max(0, Number(nodeActionDraft.value.dailyTargetMinutes || 0))
    }
    await updateNode(nodeActionDraft.value.id, patch)
    showNodeActionModal.value = false
  } catch (e) {
    nodeActionError.value = e.message || '保存失败'
  }
}

async function toggleNodeHidden(node) {
  await updateNode(node.id, { hidden: !node.hidden })
  nodeMenuId.value = null
}

async function deleteNodeFromMenu(node) {
  if (!window.confirm(`确认删除“${node.name}”吗？`)) return
  await removeNode(node.id)
  nodeMenuId.value = null
}

function isAncestorNode(ancestorId, nodeId) {
  let current = nodeMap.value.get(nodeId)
  while (current && current.parentId !== null && current.parentId !== undefined) {
    if (current.parentId === ancestorId) return true
    current = nodeMap.value.get(current.parentId)
  }
  return false
}

function canMoveBeforeAfter(sourceId, targetId) {
  const source = nodeMap.value.get(sourceId)
  const target = nodeMap.value.get(targetId)
  if (!source || !target) return false
  if (source.id === target.id) return false
  const nextParentId = target.parentId ?? null
  if (nextParentId === source.id) return false
  if (nextParentId !== null && isAncestorNode(source.id, nextParentId)) return false
  return true
}

function canMoveInto(sourceId, targetId) {
  const source = nodeMap.value.get(sourceId)
  const target = nodeMap.value.get(targetId)
  if (!source || !target) return false
  if (target.type !== 'category') return false
  if (source.id === target.id) return false
  if (isAncestorNode(source.id, target.id)) return false
  return true
}

function startMoveFromAction(sourceId = nodeActionDraft.value.id) {
  if (!sourceId) return
  clearNodePress()
  longPressFired.value = false
  moveDraft.value = { sourceId, targetId: null }
  moveTip.value = ''
  nodeMenuId.value = null
  showNodeActionModal.value = false
  activeTab.value = 'tree'
}

function cancelMoveMode() {
  moveDraft.value = { sourceId: null, targetId: null }
  moveTip.value = ''
}

function openMoveSourcePicker() {
  moveSourcePickerInitialId.value = moveDraft.value.sourceId ? Number(moveDraft.value.sourceId) : null
  showMoveSourcePickerModal.value = true
}

function onConfirmMoveSourcePicker(id) {
  moveDraft.value = { ...moveDraft.value, sourceId: id }
}

function openMoveTargetPicker() {
  moveTargetPickerInitialId.value = moveDraft.value.targetId ? Number(moveDraft.value.targetId) : null
  showMoveTargetPickerModal.value = true
}

function onConfirmMoveTargetPicker(id) {
  moveDraft.value = { ...moveDraft.value, targetId: id }
}

async function moveNodeRelative(sourceId, targetId, position) {
  const source = nodeMap.value.get(sourceId)
  const target = nodeMap.value.get(targetId)
  if (!source || !target) return
  const targetParentKey = position === 'inside' ? target.id : (target.parentId ?? 0)
  const targetParentId = position === 'inside' ? target.id : (target.parentId ?? null)
  const siblings = (childrenMap.value.get(targetParentKey) || []).filter((n) => n.id !== sourceId)
  let insertIdx = siblings.length
  if (position !== 'inside') {
    const targetIdx = siblings.findIndex((n) => n.id === targetId)
    if (targetIdx === -1) return
    insertIdx = position === 'before' ? targetIdx : targetIdx + 1
  }
  const prev = siblings[insertIdx - 1] || null
  const next = siblings[insertIdx] || null

  let prevOrder = prev ? Number(prev.orderNo) : null
  let nextOrder = next ? Number(next.orderNo) : null

  if (
    prevOrder !== null &&
    nextOrder !== null &&
    Number.isFinite(prevOrder) &&
    Number.isFinite(nextOrder) &&
    nextOrder-prevOrder <= 1
  ) {
    const updates = []
    for (let i = 0; i < siblings.length; i++) {
      const sib = siblings[i]
      const normalized = (i + 1) * 1000
      if (Number(sib.orderNo) !== normalized) {
        updates.push(api(`/api/nodes/${sib.id}`, {
          method: 'PUT',
          body: JSON.stringify({ orderNo: normalized }),
        }))
      }
    }
    if (updates.length) {
      await Promise.all(updates)
    }
    prevOrder = insertIdx > 0 ? insertIdx * 1000 : null
    nextOrder = insertIdx < siblings.length ? (insertIdx + 1) * 1000 : null
  }

  let orderNo = 1000
  if (prevOrder !== null && nextOrder !== null) {
    orderNo = Math.floor((prevOrder + nextOrder) / 2)
  } else if (prevOrder === null && nextOrder !== null) {
    orderNo = nextOrder - 1000
  } else if (prevOrder !== null && nextOrder === null) {
    orderNo = prevOrder + 1000
  }

  const patch = { orderNo }
  if ((source.parentId ?? null) !== targetParentId) {
    patch.parentId = targetParentId
  }
  await api(`/api/nodes/${sourceId}`, { method: 'PUT', body: JSON.stringify(patch) })
  await loadNodes()
}

async function confirmMoveNode(position) {
  if (!moveSourceNode.value || !moveTargetNode.value) return
  const canMove = position === 'inside'
    ? canMoveInto(moveSourceNode.value.id, moveTargetNode.value.id)
    : canMoveBeforeAfter(moveSourceNode.value.id, moveTargetNode.value.id)
  if (!canMove) {
    return
  }
  try {
    await moveNodeRelative(moveSourceNode.value.id, moveTargetNode.value.id, position)
    cancelMoveMode()
  } catch (e) {
    window.alert(e.message || '移动失败')
  }
}

async function onNodeClick(node) {
  if (isMovingNode.value) {
    if (canMoveBeforeAfter(moveSourceNode.value.id, node.id) || canMoveInto(moveSourceNode.value.id, node.id)) {
      moveDraft.value.targetId = node.id
      moveTip.value = `已选择“${node.name}”，请点击${moveActionQuotedText.value}`
    } else {
      moveTip.value = '该节点不能作为目标，请选择其它节点'
    }
    if (node.type === 'category') {
      try {
        await updateNode(node.id, { collapsed: !node.collapsed })
      } catch (e) {
        moveTip.value = e.message || '展开/折叠失败'
      }
    }
    return
  }
  if (longPressFired.value) {
    longPressFired.value = false
    return
  }
  if (node.type === 'category') {
    selectedItemIdForTimer.value = null
    await updateNode(node.id, { collapsed: !node.collapsed })
  } else if (node.type === 'item') {
    if (timerState.value.activeItemId === node.id) {
      return
    }
    if (timerState.value.activeItemId) {
      const confirmStop = window.confirm(`当前正在记录"${timerItem.value?.name || '其他事项'}"，是否结束并保存它？`)
      if (!confirmStop) return
      await stopTimer()
    }
    timerState.value.activeItemId = null
    selectedItemIdForTimer.value = node.id
  }
}

onMounted(async () => {
  await bootstrap()
  setInterval(() => {
    now.value = Date.now()
  }, 1000)

  window.addEventListener('scroll', () => {
    clearStatsNodePress()
    clearNodePress()
  }, { passive: true })
})
</script>

<template>
  <div class="app" :class="{ 'has-timer-panel': timerState.activeItemId || selectedItemIdForTimer }">
    <template v-if="isCheckingAuth">
      <div class="loading-container">
        <p>正在加载...</p>
      </div>
    </template>
    <template v-else-if="!isLoggedIn">
      <div class="card login-card">
        <h1>登录</h1>
        <label>用户名</label>
        <input v-model="loginForm.username" />
        <label>密码</label>
        <input v-model="loginForm.password" type="password" />
        <button class="primary" @click="doLogin">登录</button>
        <p class="error" v-if="loginError">{{ loginError }}</p>
      </div>
    </template>

    <template v-else>
      <nav class="tabs">
        <button :class="{ active: activeTab === 'tree' }" @click="activeTab = 'tree'">事项</button>
        <button :class="{ active: activeTab === 'stats' }" @click="activeTab = 'stats'">统计</button>
        <button :class="{ active: activeTab === 'settings' }" @click="activeTab = 'settings'">设置</button>
      </nav>

      <section v-if="activeTab === 'tree' && (timerState.activeItemId || selectedItemIdForTimer)" class="timer-floating-panel">
        <div class="timer-info">
          <p class="timer-name">
            <span v-if="itemCategoryPathText(timerItem?.id)" class="timer-path">{{ itemCategoryPathText(timerItem?.id) }} / </span>
            {{ timerItem?.name || '未知事项' }}
          </p>
          <p class="timer-clock" v-if="timerState.sessionStartAt">{{ durationText(timerElapsedMs) }}</p>
          <p class="timer-clock" v-else>00:00:00</p>
        </div>
        <div class="timer-controls">
          <button v-if="!timerState.sessionStartAt" class="timer-icon-btn primary" @click="startTimer(selectedItemIdForTimer)">▶</button>
          <template v-else>
            <button v-if="!timerState.isPaused" class="timer-icon-btn warning" @click="pauseTimer">⏸</button>
            <button v-else class="timer-icon-btn primary" @click="resumeTimer">▶</button>
            <button class="timer-icon-btn danger" @click="stopTimer()">■</button>
          </template>
        </div>
      </section>

      <section v-if="activeTab === 'tree'">
        <div class="tree-list">
          <div
            v-for="n in treeRows"
            :key="n.id"
            class="tree-row"
            :class="{ 'move-target-row': isMovingNode && moveDraft.targetId === n.id, 'move-source-row': isMovingNode && moveDraft.sourceId === n.id, 'category-row': n.type === 'category' }"
            :style="{ width: `calc(100% - ${2 * 32}px)`, marginLeft: `${n.depth * 30}px` }"
            @click="onNodeClick(n)"
            @mousedown="startNodePress(n, $event)"
            @mouseup="clearNodePress"
            @mouseleave="clearNodePress"
            @touchstart="startNodePress(n, $event)"
            @touchend="clearNodePress"
            @touchcancel="clearNodePress"
            @contextmenu.prevent
          >
            <div class="node-progress-bg" :class="n.type === 'category' ? 'category-progress-bg' : 'item-progress-bg'" :style="{ width: `${progress(n).barPercent}%` }"></div>
            <div class="node-line">
              <strong v-if="n.type === 'category'">{{ n.name }}</strong>
              <strong v-else :class="{ 'active-timer-text': (timerState.activeItemId === n.id || selectedItemIdForTimer === n.id) }">{{ n.name }}</strong>

              <div class="node-stats">
                <span>{{ todayTargetText(n) }}</span>
                <span class="percent" v-if="progress(n).targetMs > 0">{{ progress(n).percent }}%</span>
              </div>
            </div>
            <span v-if="n.type === 'category'" class="fold-icon fold-icon-edge" :class="{ open: !n.collapsed }">&gt;</span>
          </div>
          <button class="add-node-footer-btn" @click="openAddNodeModal(null)">添加节点</button>
        </div>
      </section>

      <section v-if="activeTab === 'stats'" class="stats-page-full">
        <div class="stats-module-card stats-overview-card">
          <div class="stats-filters">
            <div class="stats-filter-item">
              <span class="stats-filter-label">事项范围</span>
              <div class="stats-scope-selector">
                <button class="stats-scope-btn" @click="openStatsScopePicker">
                  <span class="stats-scope-name">{{ statsScopeCurrentName }}</span>
                  <span class="stats-scope-arrow">▼</span>
                </button>
                <button v-if="statsScopeNodeId" type="button" class="stats-scope-clear-btn" @click.stop="clearStatsScope">×</button>
              </div>
            </div>
            <div class="stats-filter-item">
              <span class="stats-filter-label">时间范围</span>
              <el-date-picker
                v-model="statsRangePickerValue"
                type="daterange"
                value-format="YYYY-MM-DD"
                format="YYYY-MM-DD"
                range-separator="~"
                start-placeholder="开始日期"
                end-placeholder="结束日期"
                class="stats-date-picker"
                popper-class="stats-range-popper"
                :editable="false"
                :clearable="true"
                :unlink-panels="true"
                :shortcuts="statsRangePickerShortcuts"
                @clear="onStatsRangePickerClear"
              />
            </div>
          </div>
          <div class="stats-summary-grid">
            <div class="stats-summary-card">
              <span class="stats-card-label">已选择</span>
              <strong class="stats-card-value">{{ statsRangeTotalDays }}<small>天</small></strong>
            </div>
            <div class="stats-summary-card">
              <span class="stats-card-label">活跃</span>
              <strong class="stats-card-value">{{ statsActiveDays }}<small>天</small></strong>
            </div>
            <div class="stats-summary-card">
              <span class="stats-card-label">总时长</span>
              <strong class="stats-card-value">{{ durationTextHourMin(statsView.totalDurationMs || 0) }}</strong>
            </div>
            <div class="stats-summary-card">
              <span class="stats-card-label">每日平均</span>
              <strong class="stats-card-value">{{ durationTextHourMin(statsAverageDailyMs) }}</strong>
            </div>
            <div class="stats-summary-card">
              <span class="stats-card-label">活跃日均</span>
              <strong class="stats-card-value">{{ durationTextHourMin(statsActiveAverageMs) }}</strong>
            </div>
          </div>
        </div>
        <div class="stats-module-card">
          <div class="stats-module-header">
            <h3 class="stats-section-title">时长占比</h3>
            <button class="stats-module-toggle" @click="toggleStatsModule('ratio')">{{ isStatsModuleCollapsed('ratio') ? '展开' : '折叠' }}</button>
          </div>
          <div v-if="!isStatsModuleCollapsed('ratio')" class="stats-tree-list">
            <div
              v-for="n in statsTreeRows"
              :key="`stats-tree-${n.id}`"
              class="stats-tree-row"
              :class="{ 'stats-category-row': n.type === 'category' }"
              @click="onStatsNodeClick(n)"
              @mousedown="startStatsNodePress(n)"
              @mouseup="clearStatsNodePress"
              @mouseleave="clearStatsNodePress"
              @touchstart="startStatsNodePress(n)"
              @touchend="clearStatsNodePress"
              @touchcancel="clearStatsNodePress"
              @contextmenu.prevent
            >
              <div
                class="stats-node-progress-bg"
                :style="{ 
                  width: `${statsNodePercent(n)}%`,
                  left: `${statsNodeOffsetMap.get(n.id) || 0}%`,
                  backgroundColor: statsNodeColor(n),
                  borderTopLeftRadius: (statsNodeOffsetMap.get(n.id) || 0) < 0.1 ? 'var(--radius)' : '0',
                  borderBottomLeftRadius: (statsNodeOffsetMap.get(n.id) || 0) < 0.1 ? 'var(--radius)' : '0',
                  borderTopRightRadius: (statsNodeOffsetMap.get(n.id) || 0) + statsNodePercent(n) > 99.9 ? 'var(--radius)' : '0',
                  borderBottomRightRadius: (statsNodeOffsetMap.get(n.id) || 0) + statsNodePercent(n) > 99.9 ? 'var(--radius)' : '0'
                }"
              />
              <div class="stats-node-line" :style="{ '--stats-indent': `${n.depth * 18}px` }">
                <strong>{{ n.name }}</strong>
                <div class="stats-node-stats">
                  <span>{{ statsDurationText(statsNodeDurationMs(n)) }}</span>
                  <span class="percent">{{ statsNodePercentText(n) }}</span>
                </div>
              </div>
              <span v-if="n.type === 'category'" class="fold-icon fold-icon-edge stats-fold-icon" :class="{ open: !isStatsNodeCollapsed(n.id) }">&gt;</span>
            </div>
          </div>
        </div>
        <div class="stats-module-card">
          <div class="stats-module-header">
            <h3 class="stats-section-title">时间轴</h3>
            <button class="stats-module-toggle" @click="toggleStatsModule('timeline')">{{ isStatsModuleCollapsed('timeline') ? '展开' : '折叠' }}</button>
          </div>
          <div v-if="!isStatsModuleCollapsed('timeline')" class="timeline-box">
            <div class="record-list">
              <div
                class="record-item timeline-record-item"
                v-for="r in timelineRecords"
                :key="r.id"
                :title="r.description || ''"
                @mousedown="startRecordPress(r, $event)"
                @mouseup="clearRecordPress"
                @mouseleave="clearRecordPress"
                @touchstart="startRecordPress(r, $event)"
                @touchmove="onRecordPressMove($event)"
                @touchend="clearRecordPress"
                @touchcancel="clearRecordPress"
                @contextmenu.prevent
              >
                <div class="timeline-record-line timeline-record-line-1">
                  <span v-if="itemCategoryPathText(r.itemId)" class="timeline-record-path">{{ itemCategoryPathText(r.itemId) }} / </span>
                  <strong class="timeline-record-item-name">{{ nodeMap.get(r.itemId)?.name || `事项#${r.itemId}` }}</strong>
                </div>
                <div class="timeline-record-line timeline-record-line-2">
                  <span>{{ recordDateWeekText(r) }}</span>
                </div>
                <div class="timeline-record-line timeline-record-line-3">
                  <span>{{ recordTimeRangeText(r) }}</span>
                  <span class="timeline-record-effective">有效 {{ durationTextHourMin(recordEffectiveMs(r)) }}</span>
                </div>
              </div>
            </div>
            <button v-if="timelineHasMore" class="ghost timeline-more-btn" @click="loadMoreTimeline">点击继续加载更多</button>
            <div v-else class="timeline-end-text">已显示全部记录</div>
          </div>
        </div>
        <div class="stats-module-card">
          <div class="stats-trend-header">
            <h3 class="stats-section-title">时长趋势</h3>
            <select v-model="statsTrendGranularity" class="stats-trend-select">
              <option value="day">按天</option>
              <option value="week">按周</option>
              <option value="month">按月</option>
            </select>
            <button class="stats-module-toggle" @click="toggleStatsModule('trend')">{{ isStatsModuleCollapsed('trend') ? '展开' : '折叠' }}</button>
          </div>
          <div v-if="!isStatsModuleCollapsed('trend')" class="stats-trend-panel">
            <div class="stats-trend-year-tag">{{ statsTrendLeftVisibleYear }}年</div>
            <div class="stats-trend-body">
              <div class="stats-trend-y-axis">
                <span v-for="row in statsTrendYAxisRows" :key="`axis-${row.value}`">{{ row.text }}</span>
              </div>
              <div ref="statsTrendWrapRef" class="stats-trend-wrap" @scroll.passive="onStatsTrendScroll">
                <div class="stats-trend-chart">
                  <div
                    v-for="bar in statsTrendVisibleBuckets"
                    :key="`trend-${statsTrendGranularity}-${bar.key}`"
                    class="stats-trend-col"
                  >
                    <div class="stats-trend-bar-box" :title="`${bar.label} ${durationTextHourMin(bar.durationMs)}`">
                      <div
                        class="stats-trend-bar"
                        :style="{ height: `${statsTrendBarHeight(bar.durationMs)}px` }"
                        @click="showStatsTrendTip(bar)"
                      />
                    </div>
                    <span class="stats-trend-label">{{ bar.shortLabel }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <button v-if="!isStatsModuleCollapsed('trend')" class="ghost stats-trend-more-btn" :class="{ 'is-hidden': !statsTrendHasMore }" @click="loadMoreStatsTrend">{{ statsTrendLoadMoreText }}</button>
        </div>
        <div class="stats-module-card">
          <div class="stats-module-header">
            <h3 class="stats-section-title">热力图</h3>
            <button class="stats-module-toggle" @click="toggleStatsModule('heatmap')">{{ isStatsModuleCollapsed('heatmap') ? '展开' : '折叠' }}</button>
          </div>
          <div v-if="!isStatsModuleCollapsed('heatmap')" ref="heatmapWrapRef" class="heatmap-wrap">
            <div class="heatmap-months">
              <div
                v-for="month in heatmapMonthColumns"
                :key="`heatmap-month-${month.key}`"
                class="heatmap-month-column"
              >
                <div class="heatmap-month-label">
                  <span class="heatmap-year-text">{{ month.yearText }}</span>
                  <span class="heatmap-month-text">{{ month.monthText }}</span>
                </div>
                <div class="heatmap-month-days">
                  <div
                    v-for="cell in month.days"
                    :key="`heatmap-${cell.date}`"
                    class="heatmap-cell"
                    :class="`level-${cell.level}`"
                    :title="`${cell.date} ${durationTextHourMin(cell.durationMs)}`"
                    @click="showHeatmapCellTip(cell)"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section v-if="activeTab === 'settings'" class="card">
        <div class="settings-header-row">
          <h2>设置</h2>
          <label class="settings-user-label">当前用户：{{ me.username }}</label>
        </div>
        <label class="checkbox-row">
          <input type="checkbox" v-model="settings.showHiddenNodes" @change="onChangeShowHiddenNodes" />
          <span>事项页显示隐藏的节点</span>
        </label>
        <label class="checkbox-row">
          <input type="checkbox" v-model="settings.statsIncludeHiddenNodes" @change="onChangeStatsIncludeHiddenNodes" />
          <span>统计页，显示隐藏的节点及其记录</span>
        </label>
        <label class="checkbox-row">
          <input type="checkbox" v-model="settings.skipShortTimerRecord" @change="onChangeSkipShortTimerRecord" />
          <span>不保存1分钟以下的记录</span>
        </label>
        <div class="row settings-actions">
          <button @click="doLogout">退出登录</button>
        </div>
      </section>

      <div v-if="isMovingNode" class="modal-mask move-node-mask" @click.self="cancelMoveMode">
        <div class="modal-card move-node-modal" @click.stop>
          <div class="move-node-row">
            <span class="move-node-prefix">把</span>
            <button class="move-node-select-btn" @click="openMoveSourcePicker">
              <span>{{ moveSourceLabel }}</span>
              <span>▼</span>
            </button>
          </div>
          <div class="move-node-row">
            <span class="move-node-prefix">移到</span>
            <button class="move-node-select-btn" @click="openMoveTargetPicker">
              <span>{{ moveTargetLabel }}</span>
              <span>▼</span>
            </button>
          </div>
          <div class="move-node-actions-row">
            <button class="primary" :disabled="!canMoveBeforeAvailable" @click="confirmMoveNode('before')">前面</button>
            <button class="primary" :disabled="!canMoveAfterAvailable" @click="confirmMoveNode('after')">后面</button>
            <button v-if="showMoveInsideButton" class="primary" :disabled="!canMoveDownAvailable" @click="confirmMoveNode('inside')">里面</button>
          </div>
        </div>
      </div>

      <div v-if="nodeMenuNode" class="node-menu-backdrop" @click="nodeMenuId = null"></div>
      <div v-if="nodeMenuNode" class="node-dropdown-menu" :style="nodeMenuStyle" @click.stop>
        <button v-if="nodeMenuNode.type === 'item'" class="node-dropdown-item" @click="openManualRecordForItem(nodeMenuNode.id); nodeMenuId = null">记录</button>
        <button class="node-dropdown-item" @click="openStatsForNode(nodeMenuNode.id)">统计</button>
        <button class="node-dropdown-item" @click="openNodeEditModal(nodeMenuNode)">编辑</button>
        <button class="node-dropdown-item" @click="startMoveFromAction(nodeMenuNode.id)">移动</button>
        <button class="node-dropdown-item" @click="toggleNodeHidden(nodeMenuNode)">{{ nodeMenuNode.hidden ? '取消隐藏' : '隐藏' }}</button>
        <button class="node-dropdown-item danger" @click="deleteNodeFromMenu(nodeMenuNode)">删除</button>
      </div>
      <div v-if="recordMenuRecord" class="node-menu-backdrop" @click="closeRecordMenu"></div>
      <div v-if="recordMenuRecord" class="node-dropdown-menu" :style="recordMenuStyle" @click.stop>
        <button class="node-dropdown-item" @click="openEditRecordModal(recordMenuRecord); closeRecordMenu()">编辑</button>
        <button class="node-dropdown-item danger" @click="removeRecord(recordMenuRecord.id); closeRecordMenu()">删除</button>
      </div>
      <NodePickerModal
        v-model="showItemPickerModal"
        title="选择事项"
        :nodeMap="nodeMap"
        :childrenMap="childrenMap"
        :showHiddenNodes="settings.showHiddenNodes"
        :initialSelectedId="itemPickerInitialSelectedId"
        @confirm="onConfirmItemPicker"
      />
      <NodePickerModal
        v-model="showStatsScopePickerModal"
        title="选择统计范围"
        :nodeMap="nodeMap"
        :childrenMap="childrenMap"
        :showHiddenNodes="settings.statsIncludeHiddenNodes"
        :initialSelectedId="statsScopePickerInitialId"
        :allowedTypes="['category', 'item']"
        @confirm="onConfirmStatsScopePicker"
      />
      <NodePickerModal
        v-model="showMoveSourcePickerModal"
        title="选择要移动的节点"
        :nodeMap="nodeMap"
        :childrenMap="childrenMap"
        :showHiddenNodes="settings.showHiddenNodes"
        :initialSelectedId="moveSourcePickerInitialId"
        :allowedTypes="['category', 'item']"
        @confirm="onConfirmMoveSourcePicker"
      />
      <NodePickerModal
        v-model="showMoveTargetPickerModal"
        title="选择目标节点"
        :nodeMap="nodeMap"
        :childrenMap="childrenMap"
        :showHiddenNodes="settings.showHiddenNodes"
        :initialSelectedId="moveTargetPickerInitialId"
        :allowedTypes="['category', 'item']"
        @confirm="onConfirmMoveTargetPicker"
      />

      <div v-if="showAddNodeModal" class="modal-mask" @click.self="showAddNodeModal = false">
        <div class="modal-card">
          <h3>添加节点</h3>
          <div class="row">
            <div class="add-node-type-switch">
              <button
                :class="{ 'is-active': createNodeDraft.type === 'category' }"
                @click="createNodeDraft.type = 'category'"
              >
                分类
              </button>
              <button
                :class="{ 'is-active': createNodeDraft.type === 'item' }"
                @click="createNodeDraft.type = 'item'"
              >
                事项
              </button>
            </div>
          </div>
          <div class="row">
            <input v-model="createNodeDraft.name" placeholder="节点名称" />
          </div>
          <div class="row modal-inline-row" v-if="createNodeDraft.type === 'item'">
            <span>目标</span>
            <input type="number" v-model="createNodeDraft.dailyTargetMinutes" placeholder="每日目标" />
            <span class="unit">分钟</span>
          </div>
          <p v-if="addNodeError" class="error">{{ addNodeError }}</p>
          <div class="row">
            <button class="primary" @click="createNode">添加</button>
            <button @click="showAddNodeModal = false">取消</button>
          </div>
        </div>
      </div>

      <div v-if="showNodeActionModal" class="modal-mask" @click.self="showNodeActionModal = false">
        <div class="modal-card">
          <h3>编辑节点</h3>
          <div class="row modal-inline-row">
            <span>名称</span>
            <input v-model="nodeActionDraft.name" placeholder="节点名称" />
          </div>
          <div class="row modal-inline-row" v-if="nodeActionDraft.type === 'item'">
            <span>目标</span>
            <input type="number" v-model="nodeActionDraft.dailyTargetMinutes" placeholder="每日目标" />
            <span class="unit">分钟</span>
          </div>
          <p v-if="nodeActionError" class="error">{{ nodeActionError }}</p>
          <div class="row node-action-save-row">
            <button @click="showNodeActionModal = false">取消</button>
            <button class="primary" @click="saveNodeAction">保存</button>
          </div>
        </div>
      </div>
      <div v-if="showManualRecordModal" class="modal-mask" @click.self="showManualRecordModal = false">
        <div class="modal-card manual-record-modal">
          <h3 class="manual-record-title">{{ manualRecordDraft.id ? '编辑记录' : '手动添加一条记录' }}</h3>
          <div class="row modal-inline-row">
            <span>事项</span>
            <button class="manual-item-select" type="button" @click="openItemPicker">
              {{ nodeMap.get(manualRecordDraft.itemId)?.name || '请选择事项' }}
            </button>
          </div>
          <div class="row modal-inline-row">
            <span>开始</span>
            <input class="dt-date" type="date" :value="getDatePart(manualRecordDraft.startAt)" @input="setDatePart('startAt', $event.target.value)" />
            <input class="dt-time" type="time" :value="getTimePart(manualRecordDraft.startAt)" @input="setTimePart('startAt', $event.target.value)" step="60" />
          </div>
          <div class="row modal-inline-row">
            <span>结束</span>
            <input class="dt-date" type="date" :value="getDatePart(manualRecordDraft.endAt)" @input="setDatePart('endAt', $event.target.value)" />
            <input class="dt-time" type="time" :value="getTimePart(manualRecordDraft.endAt)" @input="setTimePart('endAt', $event.target.value)" step="60" />
          </div>
          <div class="row modal-inline-row">
            <span>暂停</span>
            <input class="manual-pause-input" type="number" v-model="manualRecordDraft.pauseDurationSec" placeholder="0" />
            <span class="unit">分钟</span>
          </div>
          <div class="row">
            <textarea rows="3" v-model="manualRecordDraft.description" placeholder="描述（可选）"></textarea>
          </div>
          <div class="row manual-save-row">
            <button class="primary" :disabled="savingRecord" @click="addManualRecord">保存</button>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
