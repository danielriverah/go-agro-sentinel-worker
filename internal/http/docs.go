package http

// openapiSpec is the embedded OpenAPI 3.0 description of the API, served at
// GET /api/v1/openapi.yaml and rendered by Scalar at GET /docs.
const openapiSpec = `openapi: 3.0.3
info:
  title: Agro Sentinel Worker API
  description: |
    REST API for monitoring satellite imagery processing (producciones, escenas)
    and controlling the worker/sync daemons.

    **Tipos de archivo disponibles por escena** (campo 'tipo' en archivos):
    - 'multiband' — GeoTIFF multiband (bandas crudas, base para índices)
    - 'rgb' / 'natural' — composición de color natural (R-G-B)
    - 'false_color' — falso color (NIR-R-G)
    - 'ndvi' — índice de vegetación NDVI (escala de grises o paleta verde)
    - 'evi' — índice EVI
    - 'savi' — índice SAVI
    - 'ndre' — índice NDRE (rojo-borde, útil para estrés)
    - 'gndvi' — índice GNDVI
    - 'nbr' — índice NBR (quemadura)
    - 'ndmi' — índice NDMI (humedad)
    - 'params' — JSON con estadísticas de todos los índices por zona
    - 'ia_req' — JSON de entrada al modelo Bedrock (prompt estructurado)
    - 'ia_result' — JSON de respuesta del modelo Bedrock
  version: "1.1.0"
servers:
  - url: /api/v1

tags:
  - name: auth
    description: Autenticación JWT — login y cambio de contraseña
  - name: producciones
    description: Producciones de cultivo monitoreadas
  - name: escenas
    description: Escenas Sentinel-2 por producción
  - name: ia
    description: Análisis IA con AWS Bedrock
  - name: sync
    description: Sincronización con DynamoDB y S3
  - name: worker
    description: Control del worker de procesamiento

paths:
  /auth/login:
    post:
      summary: Iniciar sesión
      description: |
        Valida credenciales llamando 'fn_validate_login' en MySQL (el hash SHA2-256 lo
        verifica el procedimiento). Retorna un JWT HS256 válido por 'token_ttl_hours'
        horas (default 8 h).

        **Rutas públicas** (no requieren token): '/auth/login', '/health', '/docs', '/api/v1/openapi.yaml'.
        **Todas las demás rutas** requieren el header: 'Authorization: Bearer <token>'.
      operationId: login
      tags: [auth]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [username, password]
              properties:
                username:
                  type: string
                  example: daniel
                password:
                  type: string
                  format: password
                  example: "********"
      responses:
        "200":
          description: Sesión iniciada correctamente
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/LoginResponse"
        "400":
          description: Body inválido o campos vacíos
        "401":
          description: Credenciales inválidas (usuario no existe o contraseña incorrecta — mismo mensaje a propósito)
        "403":
          description: Cuenta desactivada
        "500":
          description: Error de base de datos

  /auth/change-password:
    post:
      summary: Cambiar contraseña
      description: |
        El usuario autenticado cambia su propia contraseña. Requiere la contraseña
        actual para confirmar identidad antes de actualizar. Llama 'sp_change_password'
        en MySQL, que regenera el salt y el hash.
      operationId: changePassword
      tags: [auth]
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [password_actual, password_nueva]
              properties:
                password_actual:
                  type: string
                  format: password
                  description: Contraseña vigente
                password_nueva:
                  type: string
                  format: password
                  description: Nueva contraseña (mínimo 8 caracteres)
      responses:
        "200":
          description: Contraseña actualizada
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: object
                    properties:
                      status:
                        type: string
                        example: contraseña actualizada
        "400":
          description: Body inválido, campos vacíos o contraseña nueva menor a 8 caracteres
        "401":
          description: No autenticado o contraseña actual incorrecta
        "403":
          description: Cuenta desactivada
        "500":
          description: Error de base de datos

  /producciones:
    get:
      summary: Listar producciones activas
      description: |
        Retorna todas las producciones en monitoreo. Incluye coordenadas del tile
        (tile_center_lat/lon) para centrar el mapa, y el polígono del cultivo (poligono_json).
      operationId: listProducciones
      tags: [producciones]
      responses:
        "200":
          description: Lista de producciones activas
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items:
                      $ref: "#/components/schemas/Production"

  /producciones/{id}:
    get:
      summary: Detalle de una producción con sus escenas
      description: |
        Retorna la producción junto con todas sus escenas ordenadas por fecha.
        Cada escena incluye flags de procesamiento (truth_tif_exists, params_exists, ia_exists, usable)
        y cloud_cover para decidir si mostrarla en el mapa.
      operationId: getProduccion
      tags: [producciones]
      parameters:
        - $ref: "#/components/parameters/ProduccionID"
      responses:
        "200":
          description: Detalle de producción
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/ProductionDetail"
        "404":
          $ref: "#/components/responses/NotFound"

  /producciones/{id}/poligono:
    put:
      summary: Reemplazar el polígono de monitoreo
      description: |
        Reescribe el polígono usado para enmascarar y medir la producción.

        Solo modifica s3_monitoring_producciones (poligono, pbox, polygon_bbox).
        El tile de descarga (tile_bbox, tile_center) NO se mueve, para que los
        multiband ya generados sigan alineados, y las tablas del ERP quedan
        intactas — el polígono original permanece recuperable desde ahí.

        Las escenas ya procesadas no se vuelven a encolar: el nuevo polígono
        aplica a lo que se procese de aquí en adelante.

        El anillo se valida en el servidor (mínimo 3 vértices, sin
        auto-intersecciones, separación mínima de 5 m entre vértices
        consecutivos, y todos los vértices dentro del tile) y se normaliza a
        sentido antihorario según la regla de la mano derecha de GeoJSON.
      operationId: putProduccionPoligono
      tags: [producciones]
      parameters:
        - $ref: "#/components/parameters/ProduccionID"
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [poligono]
              properties:
                poligono:
                  type: array
                  description: Vértices como [lon, lat]; no necesita venir cerrado
                  minItems: 3
                  items:
                    type: array
                    minItems: 2
                    maxItems: 2
                    items: { type: number, format: double }
      responses:
        "200":
          description: Polígono actualizado
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: object
                    properties:
                      produccion:
                        $ref: "#/components/schemas/Production"
                      vertices: { type: integer }
                      area_hectareas: { type: number, format: double }
        "404":
          $ref: "#/components/responses/NotFound"
        "422":
          description: |
            Geometría inválida — auto-intersección, vértices demasiado juntos,
            menos de 3 vértices, o el polígono sale del tile de descarga.
          content:
            application/json:
              schema:
                type: object
                properties:
                  error: { type: string }

  /producciones/{id}/desbloquear:
    post:
      summary: Desbloquear una producción
      description: |
        Limpia el flag 'bloqueado' de la producción (lo pone en false).
        Una producción se bloquea automáticamente cuando el análisis IA detecta
        estado 'critico'. Este endpoint permite retomar el monitoreo.
      operationId: desbloquearProduccion
      tags: [producciones]
      parameters:
        - $ref: "#/components/parameters/ProduccionID"
      requestBody:
        required: false
        content:
          application/json:
            schema:
              type: object
              properties:
                usuario:
                  type: string
                  description: Identificador del usuario que desbloquea (para auditoría)
      responses:
        "200":
          description: Producción actualizada
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/Production"
        "404":
          $ref: "#/components/responses/NotFound"

  /escenas/{id}:
    get:
      summary: Detalle de una escena
      operationId: getEscena
      tags: [escenas]
      parameters:
        - $ref: "#/components/parameters/EscenaID"
      responses:
        "200":
          description: Escena encontrada
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/Scene"
        "404":
          $ref: "#/components/responses/NotFound"

  /escenas/{id}/archivos:
    get:
      summary: Listar todos los archivos de una escena
      description: |
        Retorna todos los archivos indexados de la escena con URLs presignadas (válidas 15 min).
        Usar este endpoint para saber qué combinaciones de bandas están disponibles antes de
        mostrarlas en el mapa (no todas las escenas tienen todos los tipos).
      operationId: listEscenaArchivos
      tags: [escenas]
      parameters:
        - $ref: "#/components/parameters/EscenaID"
      responses:
        "200":
          description: Lista de archivos con URLs presignadas
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: array
                    items:
                      $ref: "#/components/schemas/ArchivoItem"

  /escenas/{id}/archivos/{tipo}:
    get:
      summary: URL presignada para un archivo específico de la escena
      description: |
        Retorna una URL presignada de S3 válida por 15 minutos para el archivo solicitado.
        Usar el 'tipo' que corresponde al índice o imagen deseada (ver descripción de la API
        para la lista completa de tipos disponibles).
      operationId: getEscenaArchivo
      tags: [escenas]
      parameters:
        - $ref: "#/components/parameters/EscenaID"
        - name: tipo
          in: path
          required: true
          schema:
            type: string
            example: ndvi
          description: |
            Tipo de archivo. Valores: multiband, rgb, natural, false_color, ndvi, evi, savi,
            ndre, gndvi, nbr, ndmi, params, ia_req, ia_result
      responses:
        "200":
          description: URL presignada
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/ArchivoResponse"
        "404":
          $ref: "#/components/responses/NotFound"

  /escenas/{id}/analisis:
    get:
      summary: Resultado IA de una escena
      description: |
        Retorna el último análisis generado por el modelo Bedrock para esta escena.
        Incluye estado_clave (optimo/bueno/alerta/critico), nivel de riesgo y motivo.
        Retorna 404 si la escena no ha sido analizada aún (ia_exists=false o analysis=false).
      operationId: getEscenaAnalisis
      tags: [ia]
      parameters:
        - $ref: "#/components/parameters/EscenaID"
      responses:
        "200":
          description: Resultado del análisis IA
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/IAResult"
        "404":
          $ref: "#/components/responses/NotFound"

  /escenas/{id}/analizar:
    post:
      summary: Disparar análisis IA de una escena
      description: |
        Inicia el pipeline de análisis IA en background (descarga ia_req.json → invoca Bedrock
        → persiste resultado → actualiza producción). Retorna 202 inmediatamente.

        **Requisito**: la escena debe tener 'ia_exists=true' (ia_req.json generado por el worker).

        **Modo dry-run** ('?dry_run=true'): descarga ia_req.json y retorna el prompt completo
        que se enviaría a Bedrock sin hacer ninguna llamada al modelo. Útil para inspección.
      operationId: triggerEscenaAnalisis
      tags: [ia]
      parameters:
        - $ref: "#/components/parameters/EscenaID"
        - name: dry_run
          in: query
          required: false
          schema:
            type: boolean
          description: Si true, retorna el prompt ensamblado sin llamar al modelo.
      responses:
        "200":
          description: Resultado del dry-run con el prompt ensamblado
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/IADryRunResult"
        "202":
          description: Análisis disparado en background
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: object
                    properties:
                      status: { type: string, example: triggered }
                      escena_id: { type: integer, format: int64 }
                      s3_key: { type: string }
        "409":
          description: Análisis ya en progreso para esta escena
        "422":
          description: La escena no tiene ia_req.json todavía (ia_exists=false)
        "503":
          description: Bedrock IA no configurado (faltan credenciales)

  /sync/status:
    get:
      summary: Estado del scheduler de sincronización
      description: |
        Retorna el schedule configurado (expresión cron y timezone), la última vez que
        se ejecutó y la próxima ejecución programada.
      operationId: syncStatus
      tags: [sync]
      responses:
        "200":
          description: Estado del sync
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/SyncStatus"

  /sync/trigger:
    post:
      summary: Disparar sincronización manual
      description: |
        Ejecuta un ciclo de sync en background y retorna 202 inmediatamente.
        El sync actualiza producciones y escenas desde DynamoDB y descarga
        metadatos de S3.
      operationId: triggerSync
      tags: [sync]
      responses:
        "202":
          description: Sync disparado
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: object
                    properties:
                      status: { type: string, example: triggered }

  /worker/status:
    get:
      summary: Estado del worker de procesamiento
      description: |
        Retorna el estado del worker global (--auto) y de cualquier worker por-producción
        activo. Cuando el worker está durmiendo entre ejecuciones de cron, 'global.phase'
        es 'sleeping' y 'global.next_schedule_at' indica cuándo correrá de nuevo.
      operationId: workerStatus
      tags: [worker]
      responses:
        "200":
          description: Estado del worker
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: object
                    properties:
                      global:
                        nullable: true
                        allOf:
                          - $ref: "#/components/schemas/WorkerStatus"
                        description: Estado del --auto (null si no está corriendo)
                      productions:
                        type: array
                        nullable: true
                        items:
                          $ref: "#/components/schemas/WorkerStatus"
                        description: Workers por-producción activos

  /worker/run-production/{id}:
    post:
      summary: Procesar una producción completa bajo demanda
      description: |
        Encola el procesamiento de todas las escenas pendientes de la producción.
        El worker --auto lo detecta en ≤30 segundos y ejecuta 'ProcessProduction'.

        **Usar cuando**:
        - Se activa una nueva producción que tiene escenas históricas pendientes
        - Se limpió una producción y se quiere regenerar desde cero

        Retorna 409 si el worker global o esa producción ya están procesando.
        El progreso puede verse en 'GET /worker/status'.
      operationId: runWorkerProduction
      tags: [worker]
      parameters:
        - $ref: "#/components/parameters/ProduccionID"
      responses:
        "202":
          description: Procesamiento encolado — el worker lo ejecutará en ≤30 s
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: object
                    properties:
                      status: { type: string, example: queued }
                      produccion_id: { type: integer, format: int64 }
                      message: { type: string }
        "409":
          description: Ya hay un worker activo para ese contexto

  /worker/cancel:
    post:
      summary: Cancelar un worker en ejecución
      description: |
        Envía SIGTERM al proceso del worker identificado por el lock file.
        El worker procesa la señal limpiamente (termina la escena en curso y libera el lock).
        Omitir 'produccion_id' para cancelar el --auto global.
      operationId: cancelWorker
      tags: [worker]
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                produccion_id:
                  type: integer
                  format: int64
                  description: ID de producción a cancelar. Omitir para cancelar --auto.
      responses:
        "200":
          description: SIGTERM enviado
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: object
                    properties:
                      pid: { type: integer }
                      signal: { type: string, example: SIGTERM }
        "404":
          description: No hay lock activo

  /worker/unlock:
    post:
      summary: Forzar liberación de un lock de worker
      description: |
        Elimina el archivo de lock y su state file sin importar si el proceso sigue vivo.
        Usar solo cuando 'cancel' no funcionó o el proceso ya murió y el lock quedó huérfano.
        Omitir 'produccion_id' para desbloquear el --auto global.
      operationId: unlockWorker
      tags: [worker]
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                produccion_id:
                  type: integer
                  format: int64
                  description: ID de producción a desbloquear. Omitir para el lock global.
      responses:
        "200":
          description: Lock eliminado
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    type: object
                    properties:
                      unlocked: { type: string, description: Ruta del lock eliminado }

components:
  parameters:
    ProduccionID:
      name: id
      in: path
      required: true
      description: produccion_id (FK → producciones.produccion_id)
      schema:
        type: integer
        format: int64
    EscenaID:
      name: id
      in: path
      required: true
      description: s3_monitoring_escena_id
      schema:
        type: integer
        format: int64

  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
      description: |
        JWT emitido por 'POST /api/v1/auth/login'. Incluir en cada request como:
        'Authorization: Bearer <token>'

  responses:
    NotFound:
      description: Recurso no encontrado
      content:
        application/json:
          schema:
            type: object
            properties:
              error:
                type: string

  schemas:
    LoginResponse:
      type: object
      description: Respuesta exitosa del endpoint de login
      properties:
        token:
          type: string
          description: JWT firmado con HS256, válido por token_ttl_hours horas
          example: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
        expires_at:
          type: string
          format: date-time
          description: Hora de expiración del token (UTC ISO 8601)
        username:
          type: string
          example: daniel
    Production:
      type: object
      description: Producción de cultivo en monitoreo satelital
      properties:
        ID:
          type: integer
          format: int64
          description: s3_monitoring_produccion_id (ID interno de monitoreo)
        ProduccionID:
          type: integer
          format: int64
          description: FK → producciones.produccion_id (ID del ERP)
        Folio:
          type: string
          description: Número de folio del ERP
        Rancho:
          type: string
          description: Nombre del rancho (centros_costos.nombre)
        Cosecha:
          type: string
          description: Temporada de cosecha (ej. 2025-2026)
        Prefix:
          type: string
          description: Prefijo de S3 donde se almacenan los archivos de esta producción
        Monitoring:
          type: boolean
          description: Si la producción está en monitoreo activo
        Bloqueado:
          type: boolean
          description: true cuando el análisis IA detectó estado crítico; usar /desbloquear para limpiar
        IAuto:
          type: boolean
          description: Si se dispara análisis IA automáticamente al completar cada escena
        PosibleCosecha:
          type: boolean
          description: Flag actualizado por el modelo IA cuando detecta posible cosecha
        TileCenterLat:
          type: number
          format: double
          nullable: true
          description: Latitud del centro del tile Sentinel-2 (para centrar el mapa)
        TileCenterLon:
          type: number
          format: double
          nullable: true
          description: Longitud del centro del tile
        TileEdgeMeters:
          type: integer
          description: Tamaño del tile en metros (tipicamente 109800)
        PoligonoJSON:
          type: object
          nullable: true
          description: GeoJSON del polígono del cultivo (para overlay en el mapa)
        TileBBoxJSON:
          type: object
          nullable: true
          description: Bounding box del tile Sentinel-2 como GeoJSON
        FechaPlantacion:
          type: string
          format: date-time
          nullable: true
        FechaFin:
          type: string
          format: date-time
          nullable: true
        TifCompleteAt:
          type: string
          format: date-time
          nullable: true
          description: Fecha en que se completó el procesamiento de todos los TIFs
        IaCompleteAt:
          type: string
          format: date-time
          nullable: true
          description: Fecha en que se completó el análisis IA de todas las escenas

    ProductionDetail:
      allOf:
        - $ref: "#/components/schemas/Production"
        - type: object
          properties:
            escenas:
              type: array
              items:
                $ref: "#/components/schemas/Scene"

    Scene:
      type: object
      description: Escena Sentinel-2 de una producción
      properties:
        ID:
          type: integer
          format: int64
          description: s3_monitoring_escena_id
        MonitoringProduccionID:
          type: integer
          format: int64
          description: FK → s3_monitoring_producciones.s3_monitoring_produccion_id
        SceneName:
          type: string
          description: Nombre de la escena (ej. S2C_14QLJ_20260602_0_L2A)
        Fecha:
          type: string
          format: date-time
          nullable: true
          description: Fecha de captura de la escena
        CloudCover:
          type: number
          format: double
          nullable: true
          description: Porcentaje de nubosidad del tile completo (0-100)
        ProductionCloud:
          type: number
          format: double
          nullable: true
          description: Porcentaje de nubosidad dentro del polígono del cultivo (más relevante)
        Status:
          type: string
          enum: [PENDING, PROCESSING, COMPLETED, FAILED, SKIPPED]
          description: Estado de procesamiento de la escena
        Usable:
          type: boolean
          description: true si la nubosidad es aceptable y el procesamiento completó correctamente
        TruthTifExists:
          type: boolean
          description: true si el multiband.tif fue generado
        ParamsExists:
          type: boolean
          description: true si el params.json con índices fue generado
        IaExists:
          type: boolean
          description: true si el ia_req.json fue generado (requerido para análisis IA)
        Analysis:
          type: boolean
          description: true si el análisis Bedrock ya se ejecutó para esta escena
        LatestIaRiesgoNivel:
          type: string
          enum: [bajo, medio, alto, ""]
          description: Nivel de riesgo del último análisis IA (vacío si no hay análisis)
        LatestIaFechaAnalisis:
          type: string
          format: date-time
          nullable: true
        MultibandRefEscenaID:
          type: integer
          format: int64
          nullable: true
          description: ID de la escena fuente del multiband reutilizado (null = propio)

    ArchivoItem:
      type: object
      description: Archivo indexado de una escena con URL presignada
      properties:
        tipo:
          type: string
          description: Tipo de archivo (ndvi, rgb, params, ia_req, etc.)
        s3_key:
          type: string
          description: Clave S3 completa
        extension:
          type: string
          description: Extensión del archivo (tif, json, png)
        size_bytes:
          type: integer
          format: int64
        url:
          type: string
          nullable: true
          description: URL presignada válida por 15 minutos (null si s3_key está vacío)

    ArchivoResponse:
      type: object
      description: URL presignada para un archivo específico
      properties:
        file_name: { type: string }
        file_type: { type: string }
        url: { type: string, description: URL presignada válida 15 minutos }
        expires_in_seconds: { type: integer, example: 900 }

    SyncStatus:
      type: object
      properties:
        schedule:
          type: string
          description: Descripción del schedule (ej. "0 5,22 * * *")
        timezone:
          type: string
          description: Timezone del cron (ej. America/Mexico_City)
        last_run:
          type: string
          format: date-time
          nullable: true
          description: Última vez que se ejecutó el sync
        next_run:
          type: string
          format: date-time
          nullable: true
          description: Próxima ejecución programada
        report:
          $ref: "#/components/schemas/SyncReport"

    SyncReport:
      type: object
      nullable: true
      description: |
        Resultado del último ciclo de sync. Permite saber si MySQL quedó al día
        con DynamoDB, no sólo cuándo corrió el ciclo. Está al día cuando
        prods_skipped y errors vienen vacíos.
      properties:
        started_at: { type: string, format: date-time }
        finished_at: { type: string, format: date-time }
        prods_in_dynamo:
          type: integer
          description: Producciones con estatus OPEN encontradas en DynamoDB
        prods_upserted:
          type: integer
          description: Producciones insertadas o actualizadas correctamente
        prods_monitoring:
          type: integer
          description: Producciones con monitoring=1 tras el ciclo
        escenas_inserted:
          type: integer
          description: Escenas nuevas bajadas de DynamoDB a MySQL
        prods_skipped:
          type: array
          description: Producciones que el sync vio pero no pudo sincronizar
          items:
            type: object
            properties:
              produccion_id: { type: integer, format: int64 }
              folio: { type: string }
              reason:
                type: string
                enum: [sin_poligono, sin_fecha_plantacion, no_existe_en_erp, bloqueada, fin_monitoreo]
        errors:
          type: array
          items: { type: string }

    WorkerStatus:
      type: object
      description: Estado de un worker de procesamiento
      properties:
        mode:
          type: string
          enum: [auto, manual-all, production, scene, regen, trigger-production]
          description: |
            Modo de ejecución:
            - auto: cron diario procesando todas las escenas pendientes
            - trigger-production: producción específica disparada desde la API
            - production: corrido manualmente con --production
        pid:
          type: integer
          description: PID del proceso del worker
        phase:
          type: string
          enum: [processing, sleeping]
          description: |
            - processing: activamente procesando escenas (lock adquirido)
            - sleeping: esperando el próximo cron (lock liberado, triggers activos)
        started_at:
          type: string
          format: date-time
        current_scene:
          type: string
          nullable: true
          description: Nombre de la escena que está procesando en este momento
        current_produccion_id:
          type: integer
          format: int64
          nullable: true
        scenes_done:
          type: integer
          description: Escenas completadas en esta ejecución
        scenes_failed:
          type: integer
          description: Escenas que fallaron en esta ejecución
        scenes_total:
          type: integer
          description: Total de escenas a procesar (0 = desconocido cuando es --auto)
        next_schedule_at:
          type: string
          format: date-time
          nullable: true
          description: Próxima ejecución del cron (solo cuando phase=sleeping)
        last_completed_at:
          type: string
          format: date-time
          nullable: true
          description: Última vez que terminó una ejecución completa

    IAResult:
      type: object
      description: Resultado del análisis IA de una escena
      properties:
        s3_monitoring_escena_id:
          type: integer
          format: int64
        estado_clave:
          type: string
          enum: [optimo, bueno, alerta, critico]
          description: Estado resumido del cultivo
        estado_general:
          type: string
          description: Descripción textual del estado generada por el modelo
        riesgo_nivel:
          type: string
          enum: [bajo, medio, alto]
        riesgo_motivo:
          type: string
          description: Justificación del nivel de riesgo
        fecha_analisis:
          type: string
          format: date-time
          nullable: true
        json_original:
          type: string
          description: Respuesta JSON completa del modelo (para auditoría)

    IADryRunResult:
      type: object
      description: Resultado del dry-run — muestra el prompt sin llamar al modelo
      properties:
        s3_key:
          type: string
          description: Clave S3 del ia_req.json usado
        ia_req_json:
          type: string
          description: Contenido raw del ia_req.json
        system_prompt:
          type: string
          description: Prompt de sistema enviado a Bedrock
        user_message:
          type: string
          description: Mensaje de usuario enviado a Bedrock
        full_bedrock_payload:
          type: object
          description: Payload completo que se enviaría a Bedrock
`

// scalarHTML renders the Scalar API reference UI, pointed at the embedded
// OpenAPI spec served from GET /api/v1/openapi.yaml.
const scalarHTML = `<!doctype html>
<html>
  <head>
    <title>Agro Sentinel Worker API</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <script id="api-reference" data-url="/api/v1/openapi.yaml"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>`
