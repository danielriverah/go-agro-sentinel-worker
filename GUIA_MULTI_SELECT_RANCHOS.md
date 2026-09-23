# Guía: Multi-Select de Ranchos con Búsqueda

## 📋 Resumen

Se han creado dos componentes reutilizables para mejorar la UX al asignar roles/permisos a múltiples ranchos:

1. **RanchodMultiSelect.vue** - Tabla searchable con checkboxes para seleccionar ranchos
2. **AsignarRolAMultiplesRanchosDialog.vue** - Diálogo modal de dos pasos

## 🎯 Características

✅ **Búsqueda por nombre o ubicación** - Filtra ranchos en tiempo real  
✅ **Checkbox "Seleccionar Todo"** - Selecciona/deselecciona todos los ranchos filtrados  
✅ **Chips de selección** - Muestra y permite quitar ranchos seleccionados  
✅ **Contador** - Muestra "X de Y ranchos seleccionados"  
✅ **Flujo de dos pasos** - Primero rol, luego ranchos  
✅ **Sin dependencias externas** - Solo Tailwind + Vue 3  

## 🔧 Cómo Integrar

### 1. Usa RanchodMultiSelect en RolesPermisosView.vue

Actualiza el componente para incluir el multi-select:

```vue
<script setup lang="ts">
import RanchodMultiSelect from '@/components/RanchodMultiSelect.vue'

// En el template, reemplaza el select simple:
</script>

<template>
  <!-- Viejo (simple select) -->
  <!-- <select v-model="addRolCcId">... -->

  <!-- Nuevo (multi-select con búsqueda) -->
  <div v-if="addRolDialog" class="space-y-4">
    <div>
      <label class="text-sm font-medium">Selecciona el rol:</label>
      <select v-model.number="addRolId" class="w-full rounded border px-2 py-1">
        <option :value="null">-- Seleccionar --</option>
        <option v-for="rol in rolesList" :key="rol.rol_id" :value="rol.rol_id">
          {{ rol.nombre }}
        </option>
      </select>
    </div>

    <!-- Multi-select de ranchos -->
    <div v-if="addRolId">
      <label class="text-sm font-medium">Selecciona los ranchos:</label>
      <RanchodMultiSelect
        :centros="centros"
        :selected-ids="selectedCentroIds"
        @update:selected-ids="selectedCentroIds = $event"
      />
    </div>
  </div>
</template>
```

### 2. O usa el diálogo completo (Recomendado)

Reemplaza el flujo actual con el diálogo de dos pasos:

```vue
<script setup lang="ts">
import AsignarRolAMultiplesRanchosDialog from '@/components/AsignarRolAMultiplesRanchosDialog.vue'

const showDialog = ref(false)
const dialogLoading = ref(false)

async function asignarRolAMultiplesRanchos(rolId: number, centroIds: number[]) {
  dialogLoading.value = true
  try {
    // Hacer una petición para cada rancho
    for (const ccId of centroIds) {
      const existentes: AsignacionRol[] = [
        ...userRoles.value.map(r => ({ rol_id: r.rol_id, centro_costo_id: r.centro_costo_id })),
        { rol_id: rolId, centro_costo_id: ccId }
      ]
      await rolesApi.setUsuarioRoles(selectedUser.value!.user_id, existentes)
    }
    
    okMsg.value = `Rol asignado a ${centroIds.length} rancho(s)`
    showDialog.value = false
    await seleccionarUsuario(selectedUser.value!)
  } catch (e) {
    error.value = apiErrorMessage(e)
  } finally {
    dialogLoading.value = false
  }
}
</script>

<template>
  <!-- Botón para abrir diálogo -->
  <button 
    @click="showDialog = true"
    class="rounded bg-green-600 px-3 py-1.5 text-xs text-white hover:bg-green-700"
  >
    + Asignar Rol a Múltiples Ranchos
  </button>

  <!-- Diálogo -->
  <AsignarRolAMultiplesRanchosDialog
    :open="showDialog"
    :roles="rolesList"
    :centros="centros"
    :loading="dialogLoading"
    @close="showDialog = false"
    @assign="asignarRolAMultiplesRanchos"
  />
</template>
```

## 📊 Props y Eventos

### RanchodMultiSelect

**Props:**
- `centros: CentroCostoItem[]` - Lista de ranchos disponibles
- `selectedIds: number[]` - IDs de ranchos seleccionados
- `title?: string` - Título opcional

**Eventos:**
- `@update:selected-ids="(ids: number[]) => void"` - Emitido cuando cambia la selección

### AsignarRolAMultiplesRanchosDialog

**Props:**
- `open: boolean` - Mostrar/ocultar diálogo
- `roles: Rol[]` - Lista de roles disponibles
- `centros: CentroCostoItem[]` - Lista de ranchos
- `loading?: boolean` - Estado de carga

**Eventos:**
- `@close="() => void"` - Cerrar diálogo
- `@assign="(rolId: number, centroIds: number[]) => void"` - Asignar rol

## 🎨 Personalización

### Cambiar colores
Modifica las clases Tailwind en los componentes:
- `bg-green-600` → Tu color principal
- `focus:ring-green-100` → Tu color de acento

### Agregar más columnas en la tabla
En `RanchodMultiSelect.vue`, añade columnas al `<table>`:

```vue
<th class="px-3 py-2 text-left font-medium text-gray-700">Tu Campo</th>
<!-- en cada fila: -->
<td class="px-3 py-2.5">{{ rancho.tu_campo }}</td>
```

### Validar cantidad mínima/máxima
```vue
<button
  :disabled="selectedCentroIds.length < 1 || selectedCentroIds.length > 10"
  class="..."
>
  Asignar
</button>
```

## 🚀 Ventajas

| Antes | Después |
|-------|---------|
| Select simple, sin búsqueda | Tabla searchable con filtrado en tiempo real |
| Seleccionar 1 rancho a la vez | Seleccionar múltiples ranchos de una vez |
| No hay contexto (ubicación) | Muestra ubicación + nombre |
| Debe repetir proceso N veces | 1 diálogo para N ranchos |

## 📝 Ejemplo de Uso Completo

```vue
<script setup lang="ts">
import { ref } from 'vue'
import AsignarRolAMultiplesRanchosDialog from '@/components/AsignarRolAMultiplesRanchosDialog.vue'

const showDialog = ref(false)
const rolesList = ref([
  { rol_id: 1, nombre: 'Supervisor', permiso_ids: [...], es_sistema: true },
  { rol_id: 2, nombre: 'Operador', permiso_ids: [...], es_sistema: true }
])
const centros = ref([
  { centro_costo_id: 1, nombre: 'Rancho Norte', ubicacion: 'Sonora' },
  { centro_costo_id: 2, nombre: 'Rancho Sur', ubicacion: 'Chiapas' },
  { centro_costo_id: 3, nombre: 'Rancho Centro', ubicacion: 'Querétaro' }
])

function asignar(rolId: number, centroIds: number[]) {
  console.log(`Asignar rol ${rolId} a ranchos ${centroIds}`)
  showDialog.value = false
}
</script>

<template>
  <div class="space-y-4">
    <button @click="showDialog = true" class="px-4 py-2 bg-blue-600 text-white rounded">
      Asignar Rol
    </button>

    <AsignarRolAMultiplesRanchosDialog
      :open="showDialog"
      :roles="rolesList"
      :centros="centros"
      @close="showDialog = false"
      @assign="asignar"
    />
  </div>
</template>
```

## ✅ Checklist de Implementación

- [ ] Copiar `RanchodMultiSelect.vue` a `web/src/components/`
- [ ] Copiar `AsignarRolAMultiplesRanchosDialog.vue` a `web/src/components/`
- [ ] Importar componente en `RolesPermisosView.vue`
- [ ] Reemplazar flujo de selección simple con el nuevo diálogo
- [ ] Probar búsqueda de ranchos
- [ ] Probar seleccionar/deseleccionar múltiples
- [ ] Probar "Seleccionar Todo"
- [ ] Probar asignar rol a varios ranchos

---

**Creado:** 2026-09-23  
**Versión:** 1.0
