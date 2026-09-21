<script setup lang="ts">
// Editor del polígono de monitoreo. Solo reescribe s3_monitoring_producciones:
// el tile de descarga no se mueve (los multiband ya generados siguen alineados)
// y el polígono original permanece intacto en las tablas del ERP.
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { produccionesExtra, apiErrorMessage } from '@/api/client'
import { useProductionsStore } from '@/stores/productions'

type Pt = [number, number] // [lon, lat]

const route = useRoute()
const router = useRouter()
const prodStore = useProductionsStore()

const produccionId = Number(route.params.id)
const production = computed(() => prodStore.details[produccionId] ?? null)

const mapEl = ref<HTMLElement | null>(null)
let map: L.Map | null = null
let osmLayer: L.TileLayer | null = null
let satLayer: L.TileLayer | null = null
let polyLayer: L.Polygon | null = null
let tileRect: L.Rectangle | null = null
let crossLayer: L.CircleMarker | null = null
const vertexMarkers: L.Marker[] = []
const midMarkers: L.Marker[] = []

const isSatellite = ref(true)
const showTile = ref(true)

const points = ref<Pt[]>([])
const original = ref<Pt[]>([])
const undoStack = ref<Pt[][]>([])

const saving = ref(false)
const saveMsg = ref('')
const error = ref('')
const loading = ref(true)

// ── Validación geométrica (espejo de domain.Ring en el servidor) ─────────────
const M_PER_DEG_LAT = 110574

function distMeters(a: Pt, b: Pt): number {
  const meanLat = (a[1] + b[1]) / 2
  const dx = (b[0] - a[0]) * M_PER_DEG_LAT * Math.cos((meanLat * Math.PI) / 180)
  const dy = (b[1] - a[1]) * M_PER_DEG_LAT
  return Math.hypot(dx, dy)
}

function segInter(p1: Pt, p2: Pt, q1: Pt, q2: Pt): Pt | null {
  const rx = p2[0] - p1[0], ry = p2[1] - p1[1]
  const sx = q2[0] - q1[0], sy = q2[1] - q1[1]
  const denom = rx * sy - ry * sx
  const qpx = q1[0] - p1[0], qpy = q1[1] - p1[1]
  if (denom === 0) return null
  const t = (qpx * sy - qpy * sx) / denom
  const u = (qpx * ry - qpy * rx) / denom
  if (t < 0 || t > 1 || u < 0 || u > 1) return null
  return [p1[0] + t * rx, p1[1] + t * ry]
}

interface Issue { kind: 'cross' | 'near'; msg: string; at?: Pt; sides?: [number, number] }

const issues = computed<Issue[]>(() => {
  const pts = points.value
  const out: Issue[] = []
  const n = pts.length
  if (n < 3) {
    out.push({ kind: 'near', msg: 'El polígono necesita al menos 3 vértices' })
    return out
  }

  for (let i = 0; i < n; i++) {
    const j = (i + 1) % n
    const d = distMeters(pts[i]!, pts[j]!)
    if (d < 5) {
      out.push({ kind: 'near', msg: `Vértices ${i + 1} y ${j + 1} a ${d.toFixed(1)} m — mínimo 5 m` })
    }
  }

  for (let i = 0; i < n; i++) {
    for (let j = i + 1; j < n; j++) {
      if (j === i + 1 || (i === 0 && j === n - 1)) continue
      const at = segInter(pts[i]!, pts[(i + 1) % n]!, pts[j]!, pts[(j + 1) % n]!)
      if (at) {
        out.push({
          kind: 'cross',
          msg: `El lado ${i + 1} a ${i + 2 > n ? 1 : i + 2} cruza el lado ${j + 1} a ${j + 2 > n ? 1 : j + 2}`,
          at, sides: [i, j],
        })
      }
    }
  }
  return out
})

const hasCross = computed(() => issues.value.some(i => i.kind === 'cross'))
const isValid = computed(() => issues.value.length === 0)

const signedArea = computed(() => {
  const pts = points.value
  let sum = 0
  for (let i = 0; i < pts.length; i++) {
    const j = (i + 1) % pts.length
    sum += pts[i]![0] * pts[j]![1] - pts[j]![0] * pts[i]![1]
  }
  return sum
})
const isCCW = computed(() => signedArea.value > 0)

const areaHa = computed(() => {
  const pts = points.value
  if (pts.length < 3) return 0
  const meanLat = pts.reduce((s, p) => s + p[1], 0) / pts.length
  const mx = M_PER_DEG_LAT * Math.cos((meanLat * Math.PI) / 180)
  let sum = 0
  for (let i = 0; i < pts.length; i++) {
    const j = (i + 1) % pts.length
    sum += (pts[i]![0] * mx) * (pts[j]![1] * M_PER_DEG_LAT) - (pts[j]![0] * mx) * (pts[i]![1] * M_PER_DEG_LAT)
  }
  return Math.abs(sum) / 2 / 10000
})

const tileBBox = computed(() => {
  const t = production.value?.TileBBoxJSON
  if (!t) return null
  return { minLon: t.min_lon, minLat: t.min_lat, maxLon: t.max_lon, maxLat: t.max_lat }
})

const outsideTile = computed(() => {
  const t = tileBBox.value
  if (!t) return false
  return points.value.some(p => p[0] < t.minLon || p[0] > t.maxLon || p[1] < t.minLat || p[1] > t.maxLat)
})

const dirty = computed(() => JSON.stringify(points.value) !== JSON.stringify(original.value))
const canSave = computed(() => isValid.value && !outsideTile.value && dirty.value && !saving.value)

// ── Carga ────────────────────────────────────────────────────────────────────
onMounted(async () => {
  try {
    await prodStore.ensureDetail(produccionId)
    const raw = production.value?.PoligonoJSON
    if (raw) {
      const parsed = (typeof raw === 'string' ? JSON.parse(raw) : raw) as unknown
      if (Array.isArray(parsed)) {
        const pts = (parsed as number[][])
          .filter(c => Array.isArray(c) && c.length >= 2 && c[0] != null && c[1] != null)
          .map(c => [c[0]!, c[1]!] as Pt)
        // Quitar el punto de cierre si viene repetido.
        if (pts.length > 1) {
          const f = pts[0]!, l = pts[pts.length - 1]!
          if (f[0] === l[0] && f[1] === l[1]) pts.pop()
        }
        points.value = pts
        original.value = pts.map(p => [...p] as Pt)
      }
    }
    if (points.value.length === 0) error.value = 'Esta producción no tiene polígono cargado'
  } catch (err) {
    error.value = apiErrorMessage(err)
  } finally {
    loading.value = false
  }

  await nextTick()
  initMap()
})

onUnmounted(() => { if (map) { map.remove(); map = null } })

function initMap() {
  if (!mapEl.value || points.value.length === 0) return

  map = L.map(mapEl.value, { zoomControl: true })

  osmLayer = L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    attribution: '© OpenStreetMap', maxZoom: 19,
  })
  satLayer = L.tileLayer(
    'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
    { attribution: '© Esri World Imagery', maxZoom: 19 },
  )
  satLayer.addTo(map)

  redraw()
  fitToPolygon()
}

function fitToPolygon() {
  if (!map || points.value.length === 0) return
  const latlngs = points.value.map(p => L.latLng(p[1], p[0]))
  map.fitBounds(L.latLngBounds(latlngs).pad(0.25), { maxZoom: 18 })
}

// ── Dibujo ───────────────────────────────────────────────────────────────────
function clearMarkers() {
  for (const m of vertexMarkers) m.remove()
  for (const m of midMarkers) m.remove()
  vertexMarkers.length = 0
  midMarkers.length = 0
}

function vertexIcon(n: number, bad: boolean) {
  return L.divIcon({
    className: '',
    html: `<div style="width:22px;height:22px;border-radius:50%;display:flex;align-items:center;justify-content:center;
      font:500 11px system-ui;color:#fff;cursor:move;border:2px solid #fff;
      background:${bad ? '#E24B4A' : '#16a34a'};box-shadow:0 1px 3px rgba(0,0,0,.4)">${n}</div>`,
    iconSize: [22, 22],
    iconAnchor: [11, 11],
  })
}

const midIcon = L.divIcon({
  className: '',
  html: `<div style="width:14px;height:14px;border-radius:50%;background:rgba(255,255,255,.85);
    border:2px solid #16a34a;cursor:copy"></div>`,
  iconSize: [14, 14],
  iconAnchor: [7, 7],
})

function redraw() {
  if (!map) return

  const latlngs = points.value.map(p => L.latLng(p[1], p[0]))

  if (polyLayer) polyLayer.remove()
  polyLayer = L.polygon(latlngs, {
    color: hasCross.value ? '#E24B4A' : '#16a34a',
    weight: 2,
    fillOpacity: 0.12,
  }).addTo(map)

  if (tileRect) { tileRect.remove(); tileRect = null }
  const t = tileBBox.value
  if (t && showTile.value) {
    tileRect = L.rectangle(
      [[t.minLat, t.minLon], [t.maxLat, t.maxLon]],
      { color: '#378ADD', weight: 1.5, dashArray: '5 4', fill: false, interactive: false },
    ).addTo(map)
  }

  if (crossLayer) { crossLayer.remove(); crossLayer = null }
  const cross = issues.value.find(i => i.kind === 'cross' && i.at)
  if (cross?.at) {
    crossLayer = L.circleMarker(L.latLng(cross.at[1], cross.at[0]), {
      radius: 7, color: '#E24B4A', weight: 2, fillColor: '#E24B4A', fillOpacity: 0.9, interactive: false,
    }).addTo(map)
  }

  clearMarkers()
  const badIdx = new Set<number>()
  for (const i of issues.value) {
    if (i.sides) { badIdx.add(i.sides[0]); badIdx.add(i.sides[1]) }
  }

  points.value.forEach((p, i) => {
    const m = L.marker(L.latLng(p[1], p[0]), {
      icon: vertexIcon(i + 1, badIdx.has(i)),
      draggable: true,
    })
    m.on('dragstart', pushUndo)
    m.on('drag', (e) => {
      const ll = (e.target as L.Marker).getLatLng()
      const next = points.value.map(q => [...q] as Pt)
      next[i] = [ll.lng, ll.lat]
      points.value = next
      redrawShapeOnly()
    })
    m.on('dragend', redraw)
    m.on('contextmenu', (e) => {
      L.DomEvent.preventDefault(e as unknown as Event)
      removePoint(i)
    })
    m.addTo(map!)
    vertexMarkers.push(m)
  })

  points.value.forEach((p, i) => {
    const q = points.value[(i + 1) % points.value.length]!
    const mid = L.marker(L.latLng((p[1] + q[1]) / 2, (p[0] + q[0]) / 2), {
      icon: midIcon, draggable: true,
    })
    mid.on('dragstart', () => {
      pushUndo()
      const next = points.value.map(x => [...x] as Pt)
      next.splice(i + 1, 0, [(p[0] + q[0]) / 2, (p[1] + q[1]) / 2])
      points.value = next
    })
    mid.on('drag', (e) => {
      const ll = (e.target as L.Marker).getLatLng()
      const next = points.value.map(x => [...x] as Pt)
      next[i + 1] = [ll.lng, ll.lat]
      points.value = next
      redrawShapeOnly()
    })
    mid.on('dragend', redraw)
    mid.addTo(map!)
    midMarkers.push(mid)
  })
}

// Durante el arrastre solo se reposiciona el contorno — redibujar los markers
// en cada mousemove cancelaría el propio arrastre.
function redrawShapeOnly() {
  if (!polyLayer) return
  polyLayer.setLatLngs(points.value.map(p => L.latLng(p[1], p[0])))
  polyLayer.setStyle({ color: hasCross.value ? '#E24B4A' : '#16a34a' })
}

// ── Edición ──────────────────────────────────────────────────────────────────
function pushUndo() {
  undoStack.value = [...undoStack.value.slice(-24), points.value.map(p => [...p] as Pt)]
}

function undo() {
  const prev = undoStack.value[undoStack.value.length - 1]
  if (!prev) return
  undoStack.value = undoStack.value.slice(0, -1)
  points.value = prev
  redraw()
}

function removePoint(i: number) {
  if (points.value.length <= 3) return
  pushUndo()
  points.value = points.value.filter((_, k) => k !== i)
  redraw()
}

function reverseOrder() {
  pushUndo()
  points.value = [...points.value].reverse()
  redraw()
}

function restoreOriginal() {
  pushUndo()
  points.value = original.value.map(p => [...p] as Pt)
  redraw()
  fitToPolygon()
}

function toggleSatellite() {
  if (!map || !osmLayer || !satLayer) return
  if (isSatellite.value) { map.removeLayer(satLayer); osmLayer.addTo(map) }
  else { map.removeLayer(osmLayer); satLayer.addTo(map) }
  isSatellite.value = !isSatellite.value
}

function toggleTile() {
  showTile.value = !showTile.value
  redraw()
}

async function save() {
  if (!canSave.value) return
  saving.value = true
  saveMsg.value = ''
  error.value = ''
  try {
    const res = await produccionesExtra.putPoligono(produccionId, points.value)
    await prodStore.refreshDetail(produccionId)
    original.value = points.value.map(p => [...p] as Pt)
    undoStack.value = []
    saveMsg.value = `Guardado — ${res.vertices} vértices, ${res.area_hectareas.toFixed(2)} ha`
    setTimeout(() => { saveMsg.value = '' }, 6000)
  } catch (err) {
    error.value = apiErrorMessage(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="flex flex-col h-[calc(100vh-56px)]">

    <!-- Barra superior -->
    <div class="bg-white border-b border-gray-200 px-4 py-3 flex items-center gap-4 shrink-0 flex-wrap">
      <button
        @click="router.push({ name: 'produccion', params: { id: produccionId } })"
        class="text-sm text-gray-500 hover:text-gray-800 transition-colors shrink-0"
      >← Producción</button>

      <div class="min-w-0" v-if="production">
        <p class="font-bold text-gray-900 truncate text-sm">{{ production.Folio || '—' }}</p>
        <p class="text-xs text-gray-400 truncate">{{ production.Rancho }} · {{ production.Cosecha }}</p>
      </div>

      <div class="ml-auto flex items-center gap-2 shrink-0 flex-wrap">
        <span v-if="saveMsg" class="text-xs text-green-600 font-medium">{{ saveMsg }}</span>
        <button
          @click="undo"
          :disabled="undoStack.length === 0"
          class="text-xs px-2.5 py-1.5 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 disabled:opacity-40 transition-colors"
        >↶ Deshacer</button>
        <button
          @click="restoreOriginal"
          :disabled="!dirty"
          class="text-xs px-2.5 py-1.5 rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 disabled:opacity-40 transition-colors"
        >Restaurar</button>
        <button
          @click="save"
          :disabled="!canSave"
          class="text-xs px-3 py-1.5 rounded-lg bg-green-600 text-white hover:bg-green-700 disabled:opacity-50 transition-colors"
        >{{ saving ? 'Guardando...' : 'Guardar' }}</button>
      </div>
    </div>

    <div v-if="loading" class="flex-1 flex items-center justify-center text-sm text-gray-400">Cargando...</div>

    <div v-else-if="points.length === 0" class="flex-1 flex flex-col items-center justify-center gap-2 text-gray-400">
      <p class="text-4xl">🗺</p>
      <p class="text-sm">{{ error || 'Sin polígono para editar' }}</p>
    </div>

    <div v-else class="flex-1 flex overflow-hidden">

      <!-- Mapa -->
      <div class="flex-1 relative">
        <div ref="mapEl" class="absolute inset-0 z-0" />

        <div class="absolute top-3 right-3 z-[1000] flex flex-col gap-1.5">
          <button
            @click="toggleSatellite"
            class="text-xs px-2.5 py-1.5 rounded-lg shadow bg-white border border-gray-200 hover:bg-gray-50 transition-colors"
            :class="isSatellite ? 'text-blue-600 border-blue-300' : 'text-gray-600'"
          >🛰 {{ isSatellite ? 'OSM' : 'Satélite' }}</button>
          <button
            @click="toggleTile"
            class="text-xs px-2.5 py-1.5 rounded-lg shadow bg-white border border-gray-200 hover:bg-gray-50 transition-colors"
            :class="showTile ? 'text-blue-600 border-blue-300' : 'text-gray-600'"
          >▢ Tile</button>
          <button
            @click="fitToPolygon"
            class="text-xs px-2.5 py-1.5 rounded-lg shadow bg-white border border-gray-200 hover:bg-gray-50 text-gray-600 transition-colors"
          >⤢ Centrar</button>
        </div>
      </div>

      <!-- Panel lateral -->
      <div class="w-72 bg-white border-l border-gray-200 flex flex-col shrink-0 overflow-y-auto">

        <!-- Estado -->
        <div class="px-4 py-3 border-b border-gray-100 space-y-2">
          <div
            class="text-xs rounded-lg px-3 py-2 leading-snug"
            :class="hasCross || outsideTile ? 'bg-red-50 text-red-700'
              : issues.length > 0 ? 'bg-amber-50 text-amber-800'
              : 'bg-green-50 text-green-700'"
          >
            <template v-if="outsideTile">
              ⛔ El polígono sale del tile de descarga. Los vértices fuera del recuadro azul no tendrían pixeles.
            </template>
            <template v-else-if="issues.length === 0">✓ Polígono válido</template>
            <template v-else>
              <p v-for="(i, k) in issues" :key="k">{{ i.kind === 'cross' ? '⛔' : '⚠' }} {{ i.msg }}</p>
            </template>
          </div>

          <div class="grid grid-cols-2 gap-2 text-xs">
            <div>
              <p class="text-gray-400">Vértices</p>
              <p class="text-gray-800 font-medium tabular-nums">{{ points.length }}</p>
            </div>
            <div>
              <p class="text-gray-400">Área</p>
              <p class="text-gray-800 font-medium tabular-nums">{{ areaHa.toFixed(2) }} ha</p>
            </div>
          </div>

          <div class="flex items-center gap-2 text-xs">
            <span class="text-gray-400">Orden</span>
            <span :class="isCCW ? 'text-green-600' : 'text-amber-600'">
              {{ isCCW ? 'antihorario' : 'horario' }}
            </span>
            <button
              @click="reverseOrder"
              class="ml-auto text-xs px-2 py-0.5 rounded border border-gray-200 text-gray-600 hover:bg-gray-50 transition-colors"
            >Invertir</button>
          </div>
        </div>

        <!-- Instrucciones -->
        <div class="px-4 py-3 border-b border-gray-100 text-xs text-gray-500 space-y-1 leading-snug">
          <p>Arrastra un vértice numerado para moverlo.</p>
          <p>Arrastra un punto blanco del borde para insertar uno nuevo.</p>
          <p>Clic derecho sobre un vértice para quitarlo.</p>
        </div>

        <!-- Lista de vértices -->
        <div class="px-4 py-3 space-y-1">
          <p class="text-xs text-gray-400 font-medium uppercase tracking-wide mb-2">Coordenadas</p>
          <div
            v-for="(p, i) in points"
            :key="i"
            class="flex items-center gap-2 text-xs font-mono py-1 border-b border-gray-50 last:border-0"
          >
            <span class="w-5 h-5 rounded-full bg-green-600 text-white flex items-center justify-center shrink-0 text-xs font-sans">
              {{ i + 1 }}
            </span>
            <span class="text-gray-600 tabular-nums">{{ p[1].toFixed(6) }}</span>
            <span class="text-gray-600 tabular-nums">{{ p[0].toFixed(6) }}</span>
            <button
              v-if="points.length > 3"
              @click="removePoint(i)"
              class="ml-auto text-gray-300 hover:text-red-500 transition-colors shrink-0"
              title="Quitar vértice"
            >×</button>
          </div>
        </div>

        <div v-if="error" class="px-4 py-3 text-xs text-red-600 border-t border-gray-100">{{ error }}</div>
      </div>
    </div>
  </div>
</template>
