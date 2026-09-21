<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { escenas as escenasApi, apiErrorMessage } from '@/api/client'
import { useNavContext } from '@/composables/useNavContext'
import { useProductionsStore } from '@/stores/productions'
import type { Scene, ArchivoItem, IAResult, IAResultDetail, FileType, Production } from '@/api/types'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let parseGeoraster: any = null
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let GeoRasterLayer: any = null

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()

const escenaId = Number(route.params.escenaId)
const produccionId = Number(route.params.produccionId)

// ── Navegación prev/next entre escenas ───────────────────────────────────────
const { sceneIds, productionIds } = useNavContext()

const sceneIdx = computed(() => sceneIds.value.indexOf(escenaId))
const prevSceneId = computed(() => sceneIdx.value > 0 ? sceneIds.value[sceneIdx.value - 1] : null)
const nextSceneId = computed(() => sceneIdx.value !== -1 && sceneIdx.value < sceneIds.value.length - 1 ? sceneIds.value[sceneIdx.value + 1] : null)

function goToEscena(id: number) {
  router.push({ name: 'escena', params: { produccionId, escenaId: id } })
}

// ── State ────────────────────────────────────────────────────────────────────
const scene      = ref<Scene | null>(null)
const production = ref<Production | null>(null)
// Producción donde se generó el multiband original (distinta cuando MultibandRefEscenaID != null)
const multibandProduction = ref<Production | null>(null)
const archivos   = ref<ArchivoItem[]>([])
const iaResult   = ref<IAResult | null>(null)
const loadingScene = ref(true)
const loadingMap   = ref(false)
const errorScene   = ref('')
const errorMap     = ref('')
const analyzing    = ref(false)
const analyzeMsg   = ref('')
const analyzeError = ref('')

// Modal IA
const showIaModal    = ref(false)
const iaDetail       = ref<IAResultDetail | null>(null)

// Capas de mapa — satélite vs OSM
const mapEl       = ref<HTMLElement | null>(null)
let map: L.Map | null = null
let currentLayer: L.Layer | null = null
let osmLayer:  L.TileLayer | null = null
let satLayer:  L.TileLayer | null = null
const isSatellite = ref(false)

// ── Band selector ─────────────────────────────────────────────────────────────
// Fuente de verdad: query DB real
//   tipo="image"     → natural.png(rgb), false_color, red_edge, swir, ndvi, evi, savi, ndre, gndvi, nbr, ndmi
//   tipo="truth_tif" → multiband.tif  (único GeoTIFF real, georaster)
//   tipo="params"/"ia_req"/"ia" → JSON
const IMAGE_TYPES: FileType[] = [
  'rgb', 'false_color', 'red_edge', 'swir',
  'ndvi', 'evi', 'savi', 'ndre', 'gndvi', 'nbr', 'ndmi',
  'multiband',
]
const JSON_TYPES: FileType[] = ['params', 'ia_req', 'ia_result']

const selectedBand  = ref<FileType>('rgb')
const jsonContent   = ref<Record<string, unknown> | null>(null)
const jsonError     = ref('')

// ── Multiband controls ────────────────────────────────────────────────────────
// Band order in multiband.tif (from domain.AllSpectralBands()):
// idx0=B02(Blue), idx1=B03(Green), idx2=B04(Red), idx3=B05(RedEdge),
// idx4=B06(RE2), idx5=B07(RE3), idx6=B08(NIR), idx7=B8A(NIR-narrow),
// idx8=B11(SWIR1), idx9=B12(SWIR2)
const MULTIBAND_BANDS = [
  { idx: 0, label: 'B02 Azul' },
  { idx: 1, label: 'B03 Verde' },
  { idx: 2, label: 'B04 Rojo' },
  { idx: 3, label: 'B05 RedEdge' },
  { idx: 4, label: 'B06 RE2' },
  { idx: 5, label: 'B07 RE3' },
  { idx: 6, label: 'B08 NIR' },
  { idx: 7, label: 'B8A NIR-n' },
  { idx: 8, label: 'B11 SWIR1' },
  { idx: 9, label: 'B12 SWIR2' },
]
const MB_PRESETS = [
  { label: 'Natural', r: 2, g: 1, b: 0 },
  { label: 'FalseColor', r: 6, g: 2, b: 1 },
  { label: 'RedEdge', r: 3, g: 2, b: 1 },
  { label: 'SWIR', r: 8, g: 6, b: 2 },
]
const mbR = ref(2)    // B04 Red
const mbG = ref(1)    // B03 Green
const mbB = ref(0)    // B02 Blue
const mbMin = ref(0)
const mbMax = ref(3000)

function clamp(v: number, lo: number, hi: number) { return v < lo ? lo : v > hi ? hi : v }

function mbPixelFn(values: number[]) {
  const range = mbMax.value - mbMin.value || 1
  const band = (idx: number) =>
    clamp(Math.round(((values[idx] ?? 0) - mbMin.value) / range * 255), 0, 255)
  return `rgba(${band(mbR.value)},${band(mbG.value)},${band(mbB.value)},1)`
}

function applyMbPreset(preset: { r: number; g: number; b: number }) {
  mbR.value = preset.r
  mbG.value = preset.g
  mbB.value = preset.b
  if (selectedBand.value === 'multiband') reloadMultiband()
}

async function reloadMultiband() {
  if (!map || !parseGeoraster || !GeoRasterLayer) return
  if (currentLayer) { map.removeLayer(currentLayer); currentLayer = null }
  errorMap.value = ''
  loadingMap.value = true
  try {
    const resp = await fetchWithAuth(streamUrl('multiband'))
    const buf  = await resp.arrayBuffer()
    const gr   = await parseGeoraster(buf)
    const layer = new GeoRasterLayer({
      georaster: gr,
      opacity: 0.9,
      resolution: 256,
      pixelValuesToColorFn: mbPixelFn,
    })
    layer.addTo(map!)
    const bounds = layer.getBounds()
    if (bounds.isValid()) map!.fitBounds(bounds, { padding: [10, 10] })
    currentLayer = layer
  } catch (err) {
    errorMap.value = apiErrorMessage(err)
  } finally {
    loadingMap.value = false
  }
}

// ── Computed ──────────────────────────────────────────────────────────────────
const availableTypes = computed(() => new Set(archivos.value.map(a => a.tipo)))
function hasFile(tipo: FileType) { return availableTypes.value.has(tipo) }
const isImageBand = computed(() => IMAGE_TYPES.includes(selectedBand.value))
const isJsonBand  = computed(() => JSON_TYPES.includes(selectedBand.value))

// La escena cumple para generar análisis IA cuando:
// - tiene ia_req.json generado (el procesador lo produce al terminar)
// - está COMPLETED y es Usable
const canAnalyze = computed(() =>
  hasFile('ia_req') &&
  scene.value?.Status === 'COMPLETED' &&
  (scene.value?.Usable ?? false)
)

// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(async () => {
  try {
    const [gr, grl] = await Promise.all([
      import('georaster'),
      import('georaster-layer-for-leaflet'),
    ])
    parseGeoraster = gr.default ?? gr
    GeoRasterLayer = grl.default ?? grl
  } catch { /* georaster no disponible */ }

  await Promise.all([loadScene(), loadArchivos()])
  // loadMultibandProduction depende de scene.MultibandRefEscenaID.
  await Promise.all([loadIaResult(), loadMultibandProduction()])

  const firstImage = IMAGE_TYPES.find(tipo => hasFile(tipo))
  if (firstImage) selectedBand.value = firstImage

  await nextTick()
  initMap()
  if (hasFile(selectedBand.value)) {
    await loadBandOnMap(selectedBand.value)
  }
})

onUnmounted(() => {
  if (map) { map.remove(); map = null }
  if (iaPollingTimer) { clearTimeout(iaPollingTimer); iaPollingTimer = null }
})

// ── Data loaders ──────────────────────────────────────────────────────────────
async function loadScene() {
  // Intentar obtener la escena del store (ya cargada al ver el detalle de producción)
  const cached = prodStore.details[produccionId]?.escenas?.find(s => s.ID === escenaId)
  if (cached) { scene.value = cached; return }
  // Fallback: fetch directo (entrada por URL directa)
  loadingScene.value = true
  try { scene.value = await escenasApi.get(escenaId) }
  catch (err) { errorScene.value = apiErrorMessage(err) }
  finally { loadingScene.value = false }
}

// La producción viene del store — ya fue cargada al entrar al detalle
const prodStore = useProductionsStore()
production.value = prodStore.details[produccionId] ?? null
if (!production.value) {
  // Fallback si se entró directo por URL
  prodStore.ensureDetail(produccionId).then(d => { if (d) production.value = d })
}

// Carga la producción donde se generó el multiband original, cuyo tile define
// el extent de las imágenes. Solo aplica si scene.MultibandRefEscenaID != null.
async function loadMultibandProduction() {
  const refId = scene.value?.MultibandRefEscenaID
  if (!refId) return
  try {
    const refScene = await escenasApi.get(refId)
    multibandProduction.value = await prodStore.ensureDetail(refScene.MonitoringProduccionID)
  } catch { /* silencioso — fallback a la producción actual */ }
}

async function loadArchivos() {
  try { archivos.value = await escenasApi.listArchivos(escenaId) }
  catch { /* silencioso */ }
}

async function loadIaResult() {
  try { iaResult.value = await escenasApi.getAnalisis(escenaId) }
  catch { /* sin resultado aún */ }
}

// ── Map ───────────────────────────────────────────────────────────────────────

// Parsea el formato real de pbox: {"pbox":[min_lon,min_lat,max_lon,max_lat], min_lat, max_lat, ...}
// Retorna L.LatLngBounds o null si el valor es inválido.
function parsePBox(raw: unknown): L.LatLngBounds | null {
  if (!raw) return null
  try {
    const obj = typeof raw === 'string' ? JSON.parse(raw) : raw as any
    // Formato con claves min_lat/max_lat/min_lon/max_lon (siempre presentes según DB)
    if (obj.min_lat != null && obj.max_lat != null && obj.min_lon != null && obj.max_lon != null) {
      return L.latLngBounds(
        [obj.min_lat, obj.min_lon],
        [obj.max_lat, obj.max_lon],
      )
    }
    // Formato alternativo: array pbox [min_lon, min_lat, max_lon, max_lat]
    if (Array.isArray(obj.pbox) && obj.pbox.length === 4) {
      const [minLon, minLat, maxLon, maxLat] = obj.pbox
      return L.latLngBounds([minLat, minLon], [maxLat, maxLon])
    }
  } catch { /* ignorar */ }
  return null
}

// ── Contexto de la producción ────────────────────────────────────────────────
// Día del ciclo a la fecha de esta escena. Es el dato que da sentido a los
// índices: un NDVI de 0.24 significa algo muy distinto en el día 20 que en el 90.
const diasCultivo = computed<number | null>(() => {
  const plant = production.value?.FechaPlantacion
  const fecha = scene.value?.Fecha
  if (!plant || !fecha) return null
  const ms = new Date(fecha).getTime() - new Date(plant).getTime()
  return Math.floor(ms / 86_400_000)
})

// Días restantes de monitoreo a la fecha de la escena; negativo = ya terminó.
const diasParaFin = computed<number | null>(() => {
  const fin = production.value?.FechaFin
  const fecha = scene.value?.Fecha
  if (!fin || !fecha) return null
  const ms = new Date(fin).getTime() - new Date(fecha).getTime()
  return Math.floor(ms / 86_400_000)
})

// Área del polígono en hectáreas, proyectando grados a metros con el coseno
// de la latitud media.
const areaHectareas = computed<number | null>(() => {
  const raw = production.value?.PoligonoJSON
  if (!raw) return null
  let pts: number[][]
  try {
    const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw
    if (!Array.isArray(parsed)) return null
    pts = parsed as number[][]
  } catch { return null }

  const ring = pts.filter(p => Array.isArray(p) && p[0] != null && p[1] != null)
  if (ring.length < 3) return null

  const M_PER_DEG = 110574
  const meanLat = ring.reduce((s, p) => s + p[1]!, 0) / ring.length
  const mx = M_PER_DEG * Math.cos((meanLat * Math.PI) / 180)
  let sum = 0
  for (let i = 0; i < ring.length; i++) {
    const a = ring[i]!, b = ring[(i + 1) % ring.length]!
    sum += (a[0]! * mx) * (b[1]! * M_PER_DEG) - (b[0]! * mx) * (a[1]! * M_PER_DEG)
  }
  return Math.abs(sum) / 2 / 10000
})

function formatFecha(iso: string | null | undefined): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString(locale.value === 'es' ? 'es-MX' : 'en-US', {
    year: 'numeric', month: 'short', day: 'numeric',
  })
}

// Producción cuyo tile define el extent real de las imágenes: la de origen
// cuando el multiband se reusó de otra escena, porque el raster descargado
// cubre ese tile y los PNG se derivan de él sin recortarlo.
function imageProd(): Production | null {
  return multibandProduction.value ?? production.value
}

// Bounds de los PNGs. El worker los reproyecta a EPSG:4326 recortados a este
// mismo rectángulo, así que el overlay coincide por construcción.
function getTileBounds(): L.LatLngBounds | null {
  return parsePBox(imageProd()?.TileBBoxJSON)
}

function initMap() {
  if (!mapEl.value || map) return

  const lat = imageProd()?.TileCenterLat ?? 24.5
  const lon = imageProd()?.TileCenterLon ?? -107

  map = L.map(mapEl.value, { zoomControl: true }).setView([lat, lon], 15)

  osmLayer = L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    attribution: '© OpenStreetMap',
    maxZoom: 19,
  })

  satLayer = L.tileLayer(
    'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
    { attribution: '© Esri World Imagery', maxZoom: 19 },
  )

  osmLayer.addTo(map)

  // Extent de las imágenes: el tile del multiband (el de la producción de
  // origen cuando es compartido). Verde punteado, y encuadre inicial.
  const tileBounds = getTileBounds()
  if (tileBounds) {
    L.rectangle(tileBounds, { color: '#16a34a', weight: 2, fillOpacity: 0.04, dashArray: '6 3' }).addTo(map)
    map.fitBounds(tileBounds, { padding: [40, 40] })
  }

  // Polígono del cultivo (azul, referencia del lote exacto)
  const poligono = production.value?.PoligonoJSON
  if (poligono) {
    try {
      const coords = (typeof poligono === 'string' ? JSON.parse(poligono) : poligono) as number[][]
      // El polígono viene como array de coordenadas [lon, lat], no GeoJSON
      const latlngs = Array.isArray(coords[0])
        ? coords.flatMap((c: number[]) =>
            c[0] != null && c[1] != null ? [L.latLng(c[1], c[0])] : [])
        : []
      if (latlngs.length) {
        L.polygon(latlngs, { color: '#2563eb', weight: 2, fillOpacity: 0.08 }).addTo(map)
      }
    } catch { /* ignorar */ }
  }
}

function toggleSatellite() {
  if (!map || !osmLayer || !satLayer) return
  if (isSatellite.value) {
    map.removeLayer(satLayer)
    osmLayer.addTo(map)
  } else {
    map.removeLayer(osmLayer)
    satLayer.addTo(map)
  }
  isSatellite.value = !isSatellite.value
}

// streamUrl devuelve la URL del proxy para un tipo de archivo.
// Todos los archivos se sirven a través del API (puerto 8088) para evitar
// que el browser intente contactar LocalStack directamente.
function streamUrl(tipo: string): string {
  return `/api/v1/escenas/${escenaId}/archivos/${tipo}/stream`
}

// fetchWithAuth hace un fetch autenticado con el token JWT del storage.
async function fetchWithAuth(url: string): Promise<Response> {
  const token = localStorage.getItem('agro_token') ?? ''
  const resp = await fetch(url, { headers: { Authorization: `Bearer ${token}` } })
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  return resp
}

async function loadBandOnMap(tipo: FileType) {
  if (!map) return
  errorMap.value = ''
  loadingMap.value = true

  if (currentLayer) { map.removeLayer(currentLayer); currentLayer = null }

  try {
    if (tipo === 'multiband') {
      if (!parseGeoraster || !GeoRasterLayer) {
        errorMap.value = 'georaster no disponible'
        return
      }
      const resp = await fetchWithAuth(streamUrl(tipo))
      const buf  = await resp.arrayBuffer()
      const gr   = await parseGeoraster(buf)
      const layer = new GeoRasterLayer({ georaster: gr, opacity: 0.9, resolution: 256, pixelValuesToColorFn: mbPixelFn })
      layer.addTo(map!)
      const bounds = layer.getBounds()
      if (bounds.isValid()) map!.fitBounds(bounds, { padding: [10, 10] })
      currentLayer = layer
    } else {
      // PNGs: fetchWithAuth → blob → objectURL → imageOverlay
      // Así el browser nunca accede a LocalStack directamente.
      const tileBounds = getTileBounds()
      if (!tileBounds) { errorMap.value = 'tile_bbox no disponible para posicionar imagen'; return }
      const resp = await fetchWithAuth(streamUrl(tipo))
      const blob = await resp.blob()
      const objectUrl = URL.createObjectURL(blob)
      const layer = L.imageOverlay(objectUrl, tileBounds, { opacity: 0.92 })
      layer.addTo(map!)
      map!.fitBounds(tileBounds, { padding: [20, 20] })
      currentLayer = layer
    }
  } catch (err) {
    errorMap.value = apiErrorMessage(err)
  } finally {
    loadingMap.value = false
  }
}

// ── Band selector ─────────────────────────────────────────────────────────────
async function selectBand(tipo: FileType) {
  selectedBand.value = tipo
  jsonContent.value = null
  jsonError.value = ''
  if (JSON_TYPES.includes(tipo)) {
    await loadJson(tipo)
  } else {
    await loadBandOnMap(tipo)
  }
}

async function loadJson(tipo: FileType) {
  jsonError.value = ''
  jsonContent.value = null
  loadingMap.value = true
  try {
    const resp = await fetchWithAuth(streamUrl(tipo))
    jsonContent.value = await resp.json()
  } catch (err) {
    jsonError.value = apiErrorMessage(err)
  } finally {
    loadingMap.value = false
  }
}

// ── IA Modal ──────────────────────────────────────────────────────────────────
function openIaModal() {
  if (!iaResult.value?.json_original) return
  try {
    iaDetail.value = JSON.parse(iaResult.value.json_original) as IAResultDetail
    showIaModal.value = true
  } catch { /* json inválido */ }
}

// ── IA Analysis ───────────────────────────────────────────────────────────────
let iaPollingTimer: ReturnType<typeof setTimeout> | null = null

async function runAnalysis() {
  analyzing.value = true
  analyzeMsg.value = ''
  analyzeError.value = ''
  if (iaPollingTimer) { clearTimeout(iaPollingTimer); iaPollingTimer = null }
  try {
    await escenasApi.analizar(escenaId)
    analyzeMsg.value = 'Analizando — actualizando cuando termine...'
    pollIaResult()
  } catch (err) {
    const status = (err as any)?.response?.status
    if (status === 422) analyzeError.value = t('scene.no_ia_req')
    else analyzeError.value = apiErrorMessage(err)
    analyzing.value = false
  }
}

// Consulta cada 3 s hasta obtener un resultado nuevo (fecha_analisis cambia).
async function pollIaResult(attempts = 0) {
  if (attempts > 40) { // máx ~2 min
    analyzing.value = false
    analyzeMsg.value = ''
    analyzeError.value = 'El análisis tardó demasiado — recarga la página'
    return
  }
  const prevFecha = iaResult.value?.fecha_analisis ?? null
  await loadIaResult()
  const newFecha = iaResult.value?.fecha_analisis ?? null
  if (newFecha && newFecha !== prevFecha) {
    analyzing.value = false
    analyzeMsg.value = 'Análisis actualizado'
    setTimeout(() => { analyzeMsg.value = '' }, 4000)
    return
  }
  iaPollingTimer = setTimeout(() => pollIaResult(attempts + 1), 3000)
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function formatDate(iso: string | null | undefined): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleString(locale.value === 'es' ? 'es-MX' : 'en-US', {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

const statusColors: Record<string, string> = {
  PENDING:    'bg-gray-100 text-gray-600',
  PROCESSING: 'bg-blue-100 text-blue-700',
  COMPLETED:  'bg-green-100 text-green-700',
  FAILED:     'bg-red-100 text-red-700',
  SKIPPED:    'bg-yellow-100 text-yellow-700',
}

const estadoColors: Record<string, string> = {
  optimo:    'text-green-700 bg-green-50 border-green-200',
  bueno:     'text-blue-700 bg-blue-50 border-blue-200',
  normal:    'text-blue-700 bg-blue-50 border-blue-200',
  alerta:    'text-yellow-700 bg-yellow-50 border-yellow-200',
  anomalia:  'text-orange-700 bg-orange-50 border-orange-200',
  critico:   'text-red-700 bg-red-50 border-red-200',
  sin_datos: 'text-gray-600 bg-gray-50 border-gray-200',
}

const riesgoColors: Record<string, string> = {
  bajo:  'text-green-600',
  medio: 'text-yellow-600',
  alto:  'text-red-600',
}

const severidadColors: Record<string, string> = {
  baja:  'bg-green-100 text-green-700',
  media: 'bg-yellow-100 text-yellow-700',
  alta:  'bg-red-100 text-red-700',
}
</script>

<template>
  <div class="flex flex-col h-[calc(100vh-56px)]">

    <!-- Top bar -->
    <div class="bg-white border-b border-gray-200 px-4 py-3 flex items-center gap-4 shrink-0 flex-wrap">
      <button
        @click="router.push({ name: 'produccion', params: { id: produccionId } })"
        class="text-sm text-gray-500 hover:text-gray-800 transition-colors shrink-0"
      >
        ← {{ t('production.title') }}
      </button>

      <!-- Navegación prev/next escena -->
      <div v-if="sceneIds.length > 1" class="flex items-center gap-1 shrink-0">
        <button
          @click="goToEscena(prevSceneId!)"
          :disabled="prevSceneId === null"
          class="px-2 py-0.5 text-sm rounded border border-gray-200 text-gray-500 hover:bg-gray-50 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          title="Escena anterior"
        >‹</button>
        <span class="text-xs text-gray-400 px-1 tabular-nums">{{ sceneIdx + 1 }} / {{ sceneIds.length }}</span>
        <button
          @click="goToEscena(nextSceneId!)"
          :disabled="nextSceneId === null"
          class="px-2 py-0.5 text-sm rounded border border-gray-200 text-gray-500 hover:bg-gray-50 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          title="Escena siguiente"
        >›</button>
      </div>

      <div class="flex-1 min-w-0" v-if="scene">
        <p class="font-mono text-xs text-gray-600 truncate">{{ scene.SceneName }}</p>
        <div class="flex items-center gap-3 text-xs text-gray-400 mt-0.5 flex-wrap">
          <span>{{ formatDate(scene.Fecha) }}</span>
          <!-- Cloud cover: tile completo (referencia) y producción/polígono (relevante) -->
          <span class="text-gray-400" title="Nubosidad del tile completo">
            ☁ tile {{ scene.CloudCover?.toFixed(1) ?? '-' }}%
          </span>
          <span
            v-if="scene.ProductionCloud != null"
            :class="scene.ProductionCloud > 40 ? 'text-red-500' : scene.ProductionCloud > 20 ? 'text-yellow-600' : 'text-green-600'"
            :title="`Nubosidad dentro del polígono de la producción: ${scene.ProductionCloud.toFixed(1)}%`"
          >
            ☁ prod {{ scene.ProductionCloud.toFixed(1) }}%
          </span>
          <span class="px-2 py-0.5 rounded-full text-xs" :class="statusColors[scene.Status]">
            {{ t(`scene.status_${scene.Status}`) }}
          </span>
          <span v-if="scene.MultibandRefEscenaID" class="text-gray-300 text-xs">↗ multiband compartido</span>
        </div>
      </div>
      <div v-else-if="loadingScene" class="text-xs text-gray-400">{{ t('common.loading') }}</div>
      <div v-if="errorScene" class="text-xs text-red-500">{{ errorScene }}</div>
    </div>

    <!-- Band selector: solo imágenes para el mapa -->
    <div class="bg-white border-b border-gray-200 px-4 py-2 shrink-0 overflow-x-auto">
      <div class="flex items-center gap-2 min-w-max">
        <span class="text-xs text-gray-400 shrink-0">Mapa:</span>
        <template v-for="tipo in IMAGE_TYPES" :key="tipo">
          <button
            @click="hasFile(tipo as FileType) && selectBand(tipo as FileType)"
            class="text-xs px-3 py-1.5 rounded-lg transition-colors whitespace-nowrap"
            :class="[
              selectedBand === tipo
                ? 'bg-green-600 text-white'
                : hasFile(tipo as FileType)
                  ? 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                  : 'bg-gray-50 text-gray-300 cursor-not-allowed'
            ]"
          >
            {{ t(`band.${tipo}`, tipo) }}
          </button>
        </template>
      </div>
    </div>

    <!-- Multiband controls (only when multiband selected) -->
    <div v-if="selectedBand === 'multiband'" class="bg-gray-50 border-b border-gray-200 px-4 py-2 shrink-0">
      <div class="flex items-center gap-4 flex-wrap">
        <!-- Presets -->
        <div class="flex items-center gap-1">
          <span class="text-xs text-gray-400 shrink-0 mr-1">Preset:</span>
          <button
            v-for="p in MB_PRESETS" :key="p.label"
            @click="applyMbPreset(p)"
            class="text-xs px-2 py-1 rounded-md border transition-colors"
            :class="mbR === p.r && mbG === p.g && mbB === p.b
              ? 'bg-green-600 text-white border-green-600'
              : 'bg-white text-gray-600 border-gray-200 hover:bg-gray-100'"
          >{{ p.label }}</button>
        </div>
        <!-- RGB selects -->
        <div class="flex items-center gap-2 text-xs">
          <label class="flex items-center gap-1">
            <span class="w-2 h-2 rounded-full bg-red-500 shrink-0" />
            <select v-model.number="mbR" class="text-xs border border-gray-200 rounded px-1 py-0.5 bg-white">
              <option v-for="b in MULTIBAND_BANDS" :key="b.idx" :value="b.idx">{{ b.label }}</option>
            </select>
          </label>
          <label class="flex items-center gap-1">
            <span class="w-2 h-2 rounded-full bg-green-500 shrink-0" />
            <select v-model.number="mbG" class="text-xs border border-gray-200 rounded px-1 py-0.5 bg-white">
              <option v-for="b in MULTIBAND_BANDS" :key="b.idx" :value="b.idx">{{ b.label }}</option>
            </select>
          </label>
          <label class="flex items-center gap-1">
            <span class="w-2 h-2 rounded-full bg-blue-500 shrink-0" />
            <select v-model.number="mbB" class="text-xs border border-gray-200 rounded px-1 py-0.5 bg-white">
              <option v-for="b in MULTIBAND_BANDS" :key="b.idx" :value="b.idx">{{ b.label }}</option>
            </select>
          </label>
        </div>
        <!-- Min/Max -->
        <div class="flex items-center gap-2 text-xs">
          <span class="text-gray-400">DN:</span>
          <label class="flex items-center gap-1">
            min <input v-model.number="mbMin" type="number" min="0" max="9999" step="100"
              class="w-16 text-xs border border-gray-200 rounded px-1 py-0.5 bg-white" />
          </label>
          <label class="flex items-center gap-1">
            max <input v-model.number="mbMax" type="number" min="1" max="10000" step="100"
              class="w-16 text-xs border border-gray-200 rounded px-1 py-0.5 bg-white" />
          </label>
          <button
            @click="reloadMultiband"
            class="text-xs px-2 py-1 rounded-md bg-green-600 text-white hover:bg-green-700 transition-colors"
          >Aplicar</button>
        </div>
      </div>
    </div>

    <!-- Main content -->
    <div class="flex-1 flex overflow-hidden">

      <!-- Map / JSON viewer -->
      <div class="flex-1 relative overflow-hidden">

        <!-- Controles de mapa -->
        <div v-if="isImageBand" class="absolute top-3 right-3 z-[1000] flex flex-col gap-1.5">
          <button
            @click="toggleSatellite"
            class="flex items-center gap-1.5 text-xs px-2.5 py-1.5 rounded-lg shadow bg-white border border-gray-200 hover:bg-gray-50 transition-colors"
            :class="isSatellite ? 'text-blue-600 border-blue-300' : 'text-gray-600'"
          >
            🛰 {{ isSatellite ? 'OSM' : 'Satélite' }}
          </button>
        </div>

        <!-- Loading overlay -->
        <div
          v-if="loadingMap"
          class="absolute inset-0 z-10 flex items-center justify-center bg-white/70 backdrop-blur-sm"
        >
          <div class="text-sm text-gray-600 flex items-center gap-2">
            <svg class="animate-spin h-4 w-4 text-green-600" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.4 0 0 5.4 0 12h4z"/>
            </svg>
            {{ t('common.loading') }}
          </div>
        </div>

        <!-- Map error -->
        <div
          v-if="errorMap && !loadingMap"
          class="absolute bottom-4 left-4 z-10 bg-white border border-red-200 text-red-600 text-xs px-3 py-2 rounded-lg shadow"
        >
          {{ errorMap }}
        </div>

        <!-- Leaflet map — siempre visible -->
        <div ref="mapEl" class="w-full h-full" />
      </div>

      <!-- Panel lateral: contexto de la producción + resultado IA -->
      <div class="w-72 bg-white border-l border-gray-200 flex flex-col shrink-0 overflow-y-auto">

        <!-- Producción -->
        <div v-if="production" class="px-4 py-3 border-b border-gray-100 space-y-2.5">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <p class="text-xs text-gray-400 font-medium uppercase tracking-wide">{{ t('production.folio') }}</p>
              <button
                @click="router.push({ name: 'produccion', params: { id: produccionId } })"
                class="font-bold text-gray-900 hover:text-green-700 transition-colors truncate text-left"
              >{{ production.Folio || '—' }}</button>
            </div>
            <div class="flex flex-wrap gap-1 justify-end shrink-0">
              <span v-if="production.PosibleCosecha" class="text-xs px-1.5 py-0.5 rounded-full bg-yellow-100 text-yellow-700" title="Posible cosecha">🌾</span>
              <span v-if="production.Bloqueado" class="text-xs px-1.5 py-0.5 rounded-full bg-red-100 text-red-700">{{ t('production.blocked') }}</span>
              <span v-if="production.IAuto" class="text-xs px-1.5 py-0.5 rounded-full bg-blue-100 text-blue-600">IA auto</span>
            </div>
          </div>

          <div class="space-y-1 text-xs">
            <div class="flex items-center gap-1.5 text-gray-700">
              <span class="text-gray-400 shrink-0">🏡</span>
              <span class="truncate">{{ production.Rancho || '—' }}</span>
            </div>
            <div class="flex items-center gap-1.5 text-gray-700">
              <span class="text-gray-400 shrink-0">🌱</span>
              <span class="truncate">{{ production.Cosecha || '—' }}</span>
            </div>
          </div>

          <!-- Día del ciclo: el contexto que da sentido a los índices -->
          <div v-if="diasCultivo !== null" class="rounded-lg bg-green-50 px-3 py-2">
            <p class="text-xs text-green-700">
              Día <span class="font-bold tabular-nums text-base">{{ diasCultivo }}</span> del cultivo
            </p>
            <p class="text-xs text-green-600/80 mt-0.5">
              a la fecha de esta escena
            </p>
          </div>

          <div class="grid grid-cols-2 gap-2 text-xs">
            <div>
              <p class="text-gray-400">Plantación</p>
              <p class="text-gray-800 font-medium">{{ formatFecha(production.FechaPlantacion) }}</p>
            </div>
            <div>
              <p class="text-gray-400">Fin monitoreo</p>
              <p class="font-medium" :class="diasParaFin !== null && diasParaFin < 0 ? 'text-gray-400' : 'text-gray-800'">
                {{ formatFecha(production.FechaFin) }}
              </p>
            </div>
            <div v-if="areaHectareas !== null">
              <p class="text-gray-400">Área</p>
              <p class="text-gray-800 font-medium tabular-nums">{{ areaHectareas.toFixed(2) }} ha</p>
            </div>
            <div v-if="production.stats">
              <p class="text-gray-400">Escenas</p>
              <p class="text-gray-800 font-medium tabular-nums">
                {{ production.stats.scenes_completadas }}/{{ production.stats.scenes_total }}
              </p>
            </div>
          </div>

          <!-- Origen del multiband cuando es compartido -->
          <p v-if="multibandProduction && multibandProduction.ID !== production.ID" class="text-xs text-gray-400 leading-snug">
            ↗ Imágenes derivadas del multiband de
            <span class="text-gray-600">{{ multibandProduction.Folio || `#${multibandProduction.ID}` }}</span>,
            por eso el recuadro verde no está centrado en este lote.
          </p>
        </div>

        <!-- IA -->
        <div class="px-4 py-3 border-b border-gray-100">
          <h3 class="text-sm font-semibold text-gray-800">{{ t('scene.ia_result') }}</h3>
        </div>

        <div v-if="iaResult" class="p-4 space-y-3">
          <!-- Estado badge — clic abre modal -->
          <button
            @click="openIaModal"
            :disabled="!iaResult.json_original"
            class="w-full rounded-xl border px-4 py-3 text-center transition-all hover:shadow-md active:scale-95"
            :class="[estadoColors[iaResult.estado_clave] ?? 'text-gray-700 bg-gray-50 border-gray-200', iaResult.json_original ? 'cursor-pointer' : 'cursor-default']"
            :title="iaResult.json_original ? 'Ver análisis completo' : ''"
          >
            <p class="text-lg font-bold">
              {{ t(`ia.estado_${iaResult.estado_clave}`, iaResult.estado_clave) }}
            </p>
            <p class="text-xs mt-0.5 opacity-75 line-clamp-2">{{ iaResult.estado_general }}</p>
            <p v-if="iaResult.json_original" class="text-xs mt-1 opacity-50">↗ ver detalle completo</p>
          </button>

          <!-- Riesgo -->
          <div class="flex items-center justify-between text-sm">
            <span class="text-gray-500">Riesgo</span>
            <span class="font-semibold" :class="riesgoColors[iaResult.riesgo_nivel] ?? 'text-gray-600'">
              {{ t(`ia.riesgo_${iaResult.riesgo_nivel}`, iaResult.riesgo_nivel) }}
            </span>
          </div>

          <!-- Motivo -->
          <div v-if="iaResult.riesgo_motivo" class="text-xs text-gray-600 bg-gray-50 rounded-lg p-3">
            {{ iaResult.riesgo_motivo }}
          </div>

          <!-- Fecha -->
          <div class="text-xs text-gray-400">{{ formatDate(iaResult.fecha_analisis) }}</div>
        </div>

        <div v-else class="p-4 text-xs text-gray-400 text-center">{{ t('ia.no_result') }}</div>

        <!-- Botones IA -->
        <div class="mt-auto px-4 py-4 border-t border-gray-100 space-y-2">
          <!-- Generar análisis: solo si la escena está lista (COMPLETED + Usable + ia_req) -->
          <button
            @click="runAnalysis"
            :disabled="analyzing || !canAnalyze"
            class="w-full text-sm py-2 rounded-lg bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {{ analyzing ? t('scene.analyzing') : 'Generar análisis IA' }}
          </button>

          <!-- Re-analizar: solo si ya existe un resultado previo -->
          <button
            v-if="iaResult"
            @click="runAnalysis"
            :disabled="analyzing || !canAnalyze"
            class="w-full text-sm py-2 rounded-lg border border-blue-300 text-blue-700 hover:bg-blue-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            Repetir análisis
          </button>

          <!-- Motivo de bloqueo -->
          <p v-if="!hasFile('ia_req')" class="text-xs text-gray-400 text-center">
            Sin ia_req.json — procesa la escena primero
          </p>
          <p v-else-if="scene && !scene.Usable" class="text-xs text-gray-400 text-center">
            Escena no usable
          </p>
          <p v-else-if="scene && scene.Status !== 'COMPLETED'" class="text-xs text-gray-400 text-center">
            Escena no completada ({{ scene.Status }})
          </p>

          <p v-if="analyzeMsg" class="text-xs text-green-600 text-center">{{ analyzeMsg }}</p>
          <p v-if="analyzeError" class="text-xs text-red-600 text-center">{{ analyzeError }}</p>
        </div>

        <!-- Archivos JSON -->
        <div class="px-4 pb-2 border-t border-gray-100 pt-3">
          <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Datos JSON</p>
          <div class="flex flex-wrap gap-1">
            <template v-for="tipo in JSON_TYPES" :key="tipo">
              <button
                @click="hasFile(tipo as FileType) && selectBand(tipo as FileType)"
                class="text-xs px-2.5 py-1 rounded-lg transition-colors whitespace-nowrap border"
                :class="[
                  selectedBand === tipo
                    ? 'bg-blue-600 text-white border-blue-600'
                    : hasFile(tipo as FileType)
                      ? 'bg-blue-50 text-blue-700 border-blue-200 hover:bg-blue-100'
                      : 'bg-gray-50 text-gray-300 border-gray-100 cursor-not-allowed'
                ]"
              >
                {{ t(`band.${tipo}`, tipo) }}
              </button>
            </template>
          </div>
          <!-- JSON viewer inline -->
          <div v-if="isJsonBand && jsonContent" class="mt-3 rounded-lg bg-gray-900 p-3 max-h-64 overflow-auto">
            <pre class="text-xs text-green-300 font-mono whitespace-pre-wrap break-words">{{ JSON.stringify(jsonContent, null, 2) }}</pre>
          </div>
          <p v-else-if="isJsonBand && jsonError" class="mt-2 text-xs text-red-400 font-mono">{{ jsonError }}</p>
          <p v-else-if="isJsonBand && loadingMap" class="mt-2 text-xs text-gray-400">{{ t('common.loading') }}</p>
        </div>

        <!-- Inventario de archivos -->
        <div v-if="archivos.length" class="px-4 pb-4 border-t border-gray-100 pt-3">
          <p class="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Archivos</p>
          <div class="space-y-1">
            <div
              v-for="a in archivos" :key="a.tipo"
              class="flex items-center justify-between text-xs text-gray-600"
            >
              <span class="font-mono truncate">{{ a.tipo }}</span>
              <span class="text-gray-400 shrink-0 ml-2">{{ (a.size_bytes / 1024).toFixed(0) }} KB</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- ── Modal Análisis IA ─────────────────────────────────────────────────── -->
  <Teleport to="body">
    <div
      v-if="showIaModal && iaDetail"
      class="fixed inset-0 z-[2000] flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
      @click.self="showIaModal = false"
    >
      <div class="bg-white rounded-2xl shadow-2xl w-full max-w-2xl max-h-[90vh] flex flex-col overflow-hidden">

        <!-- Modal header -->
        <div class="px-6 py-4 border-b border-gray-100 flex items-center justify-between shrink-0">
          <div class="flex items-center gap-3">
            <span
              class="px-3 py-1 rounded-full text-sm font-semibold border"
              :class="estadoColors[iaDetail.estado_clave] ?? 'bg-gray-50 text-gray-600 border-gray-200'"
            >
              {{ t(`ia.estado_${iaDetail.estado_clave}`, iaDetail.estado_clave) }}
            </span>
            <span class="text-sm font-medium" :class="riesgoColors[iaDetail.riesgo?.nivel] ?? 'text-gray-500'">
              Riesgo {{ t(`ia.riesgo_${iaDetail.riesgo?.nivel}`, iaDetail.riesgo?.nivel) }}
            </span>
            <span v-if="iaDetail.posible_cosecha" class="text-xs px-2 py-0.5 rounded-full bg-amber-100 text-amber-700 font-medium">
              🌾 Posible cosecha
            </span>
          </div>
          <button @click="showIaModal = false" class="text-gray-400 hover:text-gray-700 text-xl leading-none">✕</button>
        </div>

        <!-- Modal body -->
        <div class="overflow-y-auto flex-1 p-6 space-y-5">

          <!-- Estado general -->
          <div>
            <p class="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-1">Estado general</p>
            <p class="text-sm text-gray-700 leading-relaxed">{{ iaDetail.estado_general }}</p>
          </div>

          <!-- Resumen -->
          <div v-if="iaDetail.resumen">
            <p class="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-1">Resumen</p>
            <p class="text-sm text-gray-700 leading-relaxed">{{ iaDetail.resumen }}</p>
          </div>

          <!-- Hallazgos -->
          <div v-if="iaDetail.hallazgos?.length">
            <p class="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-2">Hallazgos</p>
            <div class="space-y-2">
              <div
                v-for="(h, i) in iaDetail.hallazgos" :key="i"
                class="border border-gray-100 rounded-xl p-3"
              >
                <div class="flex items-center gap-2 mb-1">
                  <span class="text-xs px-2 py-0.5 rounded-full font-medium" :class="severidadColors[h.severidad] ?? 'bg-gray-100 text-gray-600'">
                    {{ h.severidad }}
                  </span>
                  <span class="text-xs font-semibold text-gray-700 capitalize">{{ h.tipo.replace(/_/g, ' ') }}</span>
                  <span class="text-xs text-gray-400 ml-auto">{{ h.zona }}</span>
                </div>
                <p class="text-xs text-gray-600 leading-relaxed">{{ h.descripcion }}</p>
              </div>
            </div>
          </div>

          <!-- Riesgo motivo -->
          <div v-if="iaDetail.riesgo?.motivo">
            <p class="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-1">Motivo de riesgo</p>
            <p class="text-sm text-gray-700 leading-relaxed">{{ iaDetail.riesgo.motivo }}</p>
          </div>

          <!-- Recomendaciones -->
          <div v-if="iaDetail.recomendaciones?.length">
            <p class="text-xs font-semibold text-gray-400 uppercase tracking-wide mb-2">Recomendaciones</p>
            <ol class="space-y-2 list-none">
              <li
                v-for="(r, i) in iaDetail.recomendaciones" :key="i"
                class="flex gap-3 text-sm text-gray-700"
              >
                <span class="shrink-0 w-5 h-5 rounded-full bg-green-100 text-green-700 flex items-center justify-center text-xs font-bold">{{ i + 1 }}</span>
                <span class="leading-relaxed">{{ r }}</span>
              </li>
            </ol>
          </div>

          <!-- Fecha -->
          <p class="text-xs text-gray-400 pt-2 border-t border-gray-100">
            Análisis generado: {{ formatDate(iaResult?.fecha_analisis) }}
          </p>
        </div>
      </div>
    </div>
  </Teleport>
</template>
