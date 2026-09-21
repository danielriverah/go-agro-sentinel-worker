import { nextTick, ref } from 'vue'

// Estado de navegación compartido entre vistas — persiste entre rutas sin
// depender de history.state (que Vue Router puede sobreescribir).
const productionIds = ref<number[]>([])
const sceneIds      = ref<number[]>([])

// De dónde viene el usuario, para que al volver a un listado la vista se
// posicione en el elemento que estaba viendo en lugar de saltar al principio.
// Lo escribe el guard del router en cada navegación, así funciona sin importar
// por qué botón se haya salido.
const cameFromProduccion = ref<number | null>(null)
const cameFromEscena     = ref<number | null>(null)

export function useNavContext() {
  return { productionIds, sceneIds, cameFromProduccion, cameFromEscena }
}

// Lleva el scroll al elemento marcado con data-nav-id y lo deja centrado. El
// resaltado visual lo maneja cada vista con su propio estado reactivo.
//
// Busca por atributo y no por id porque las listas responsive pintan la misma
// fila dos veces —tarjetas para móvil y tabla para escritorio— y solo una está
// visible según el ancho. Con getElementById salía la primera del DOM, que en
// escritorio es la oculta, y scrollIntoView sobre un display:none no hace nada:
// el elemento quedaba resaltado pero la pantalla no se movía.
//
// Además espera a que aparezca. Quien llama suele hacerlo desde un watch con
// immediate, que corre durante setup() cuando los datos ya venían del store,
// con el DOM inicial todavía sin montar; y en el listado agrupado puede hacer
// falta expandir un grupo colapsado, lo que provoca otro render.
export async function scrollToElement(navId: string, maxFrames = 20): Promise<boolean> {
  for (let i = 0; i < maxFrames; i++) {
    await nextTick()
    const candidates = document.querySelectorAll<HTMLElement>(
      `[data-nav-id="${CSS.escape(navId)}"]`,
    )
    // offsetParent es null cuando el elemento o un ancestro está display:none.
    const visible = [...candidates].find(el => el.offsetParent !== null)
    if (visible) {
      visible.scrollIntoView({ behavior: 'smooth', block: 'center' })
      return true
    }
    await new Promise(resolve => requestAnimationFrame(() => resolve(null)))
  }
  return false
}
