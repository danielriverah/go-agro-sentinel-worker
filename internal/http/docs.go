package http

// openapiSpec is the embedded OpenAPI 3.0 description of the API, served at
// GET /api/v1/openapi.yaml and rendered by Scalar at GET /docs.
const openapiSpec = `openapi: 3.0.3
info:
  title: Agro Sentinel Worker API
  description: REST API for querying producciones, escenas and their processed files.
  version: "1.0.0"
servers:
  - url: /api/v1
paths:
  /producciones:
    get:
      summary: List active producciones
      operationId: listProducciones
      tags: [producciones]
      responses:
        "200":
          description: List of active producciones
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
      summary: Get a produccion with its escenas
      operationId: getProduccion
      tags: [producciones]
      parameters:
        - $ref: "#/components/parameters/ProduccionID"
      responses:
        "200":
          description: Produccion detail
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/ProductionDetail"
        "404":
          $ref: "#/components/responses/NotFound"
  /producciones/{id}/desbloquear:
    post:
      summary: Unblock a produccion
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
      responses:
        "200":
          description: Updated produccion
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/Production"
        "404":
          $ref: "#/components/responses/NotFound"
  /escenas/{id}/archivos/{tipo}:
    get:
      summary: Get a presigned URL for a scene file
      operationId: getEscenaArchivo
      tags: [escenas]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
            format: int64
        - name: tipo
          in: path
          required: true
          schema:
            type: string
          description: File type, e.g. ndvi, natural, multiband, params, analisis
      responses:
        "200":
          description: Presigned URL
          content:
            application/json:
              schema:
                type: object
                properties:
                  data:
                    $ref: "#/components/schemas/ArchivoResponse"
        "404":
          $ref: "#/components/responses/NotFound"
  /sync/trigger:
    post:
      summary: Trigger a sync cycle
      operationId: triggerSync
      tags: [sync]
      responses:
        "202":
          description: Sync cycle triggered
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
                        example: triggered
components:
  parameters:
    ProduccionID:
      name: id
      in: path
      required: true
      schema:
        type: integer
        format: int64
  responses:
    NotFound:
      description: Resource not found
      content:
        application/json:
          schema:
            type: object
            properties:
              error:
                type: string
  schemas:
    Production:
      type: object
      properties:
        ID: { type: integer, format: int64 }
        ProduccionID: { type: integer, format: int64 }
        Cultivo: { type: string }
        Ciclo: { type: string }
        Monitoring: { type: boolean }
        Bloqueado: { type: boolean }
        TotalEscenas: { type: integer }
        TotalEscenasValidas: { type: integer }
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
      properties:
        ID: { type: integer, format: int64 }
        ProduccionID: { type: integer, format: int64 }
        SceneID: { type: string }
        SceneDate: { type: string, format: date-time }
        Status: { type: string }
    ArchivoResponse:
      type: object
      properties:
        file_name: { type: string }
        file_type: { type: string }
        url: { type: string }
        expires_in_seconds: { type: integer }
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
