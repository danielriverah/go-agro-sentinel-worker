# Estrategia de Configuración del Proyecto

## 📋 Visión General

El proyecto utiliza un **modelo de configuración de dos capas**:

1. **Capa Base (YAML):** `configs/config.yaml` — valores por defecto para desarrollo
2. **Capa Override (Env Vars):** Variables de entorno sobrescriben valores YAML en producción/Docker

```
┌──────────────────────────────────────┐
│   config.yaml (valores por defecto)  │
└──────────────────┬───────────────────┘
                   │
                   ▼
┌──────────────────────────────────────┐
│  os.Getenv() → applyEnvOverrides()   │
│  (sobrescribe valores selectivamente) │
└──────────────────┬───────────────────┘
                   │
                   ▼
        ┌─────────────────────┐
        │ Config final cargada │
        │ para el servicio     │
        └─────────────────────┘
```

---

## 🔧 Cómo Funciona la Carga de Configuración

### En Desarrollo Local

1. **Archivo default:** `configs/config.yaml`
   ```bash
   ./cmd/sync/main.go
   ./cmd/worker/main.go
   ./cmd/api/main.go
   ```
   Todos buscan: `cfgPath := os.Getenv("CONFIG_PATH") || "configs/config.yaml"`

2. **Overrides vía variables de entorno:**
   ```bash
   export MYSQL_HOST=localhost
   export AWS_REGION=us-west-2
   export AUTH_JWT_SECRET=mi-clave-larga-de-32-caracteres-minimo
   ./bin/sync
   ```

### En Docker (docker-compose.yml)

1. **Config path:** Fijado a `/etc/agro-sentinel/config.yaml` (copiado en Dockerfile)
2. **Variables de entorno:** Inyectadas desde `.env` o definidas en docker-compose
   ```yaml
   environment:
     CONFIG_PATH: /etc/agro-sentinel/config.yaml
     MYSQL_HOST: ${MYSQL_HOST:-}
     AWS_REGION: ${AWS_REGION:-}
     AUTH_JWT_SECRET: ${AUTH_JWT_SECRET:-}
   ```

---

## 📋 Variables de Entorno Soportadas

### AWS y S3
| Variable | Valor YAML Afectado | Default | Requerida | Nota |
|----------|------------------|---------|-----------|------|
| `AWS_REGION` | `aws.region` | — | Sí | Región general AWS |
| `AWS_ENDPOINT_URL` | `aws.endpoint` | vacío (AWS real) | No | Mock endpoint (LocalStack) |
| `S3_REGION` | `s3.region` | vacío (hereda de aws.region) | No | **Región independiente para S3** |
| `S3_BUCKET` | `s3.bucket` | — | Sí | Nombre del bucket S3 |
| `S3_PREFIX` | `s3.prefix` | `sentinel/producciones` | No | Prefijo en el bucket |
| `S3_PUBLIC_ENDPOINT` | `s3.public_endpoint` | vacío (sin reescritura) | No | URLs presigned en navegador |
| `S3_ACCESS_KEY_ID` | AWS S3 credential | — | Sí (en AWS real) | Credenciales específicas de S3 |
| `S3_SECRET_ACCESS_KEY` | AWS S3 credential | — | Sí (en AWS real) | Credenciales específicas de S3 |

### DynamoDB
| Variable | Valor YAML Afectado | Default | Requerida |
|----------|------------------|---------|-----------|
| `DYNAMODB_REGION` | `dynamodb.region` | vacío (hereda de aws.region) | No |
| `DYNAMODB_ENDPOINT` | `dynamodb.endpoint` | vacío (AWS real) | No |
| `DYNAMODB_TABLE_PRODUCCIONES` | `dynamodb.table_producciones` | `monitoring_producciones` | No |
| `DYNAMODB_TABLE_ESCENAS` | `dynamodb.table_escenas` | `monitoring_escenas` | No |
| `DYNAMODB_ACCESS_KEY_ID` | `dynamodb.access_key_id` | vacío (hereda credenciales AWS) | No |
| `DYNAMODB_SECRET_ACCESS_KEY` | `dynamodb.secret_access_key` | vacío (hereda credenciales AWS) | No |

### MySQL
| Variable | Valor YAML Afectado | Default | Requerida |
|----------|------------------|---------|-----------|
| `MYSQL_HOST` | `mysql.host` | `localhost` | Sí |
| `MYSQL_PORT` | `mysql.port` | `3306` | No |
| `MYSQL_USER` | `mysql.user` | — | Sí |
| `MYSQL_PASSWORD` | `mysql.password` | — | Sí |
| `MYSQL_DATABASE` / `MYSQL_DB` | `mysql.database` | `agro` | Sí |

### Schedulers (SYNC y WORKER)
| Variable | Valor YAML Afectado | Default | Nota |
|----------|------------------|---------|------|
| `SYNC_SCHEDULE` | `sync.schedule` | vacío | Cron: "0 6,18 * * *" ó déjalo vacío para interval_minutes |
| `SYNC_TIMEZONE` | `sync.timezone` | `America/Mexico_City` | IANA timezone |
| `WORKER_SCHEDULE` | `worker.schedule` | vacío | Cron: "0 1 * * *" |
| `WORKER_TIMEZONE` | `worker.timezone` | `America/Mexico_City` | IANA timezone |

### Autenticación
| Variable | Valor YAML Afectado | Default | Requerida |
|----------|------------------|---------|-----------|
| `AUTH_JWT_SECRET` | `auth.jwt_secret` | — | **Sí (en producción)** |
| `AUTH_TOKEN_TTL_HOURS` | `auth.token_ttl_hours` | `8` | No |
| `AUTH_DELETE_ALLOWED_USER_IDS` | `auth.delete_allowed_user_ids` | vacío (nadie puede borrar) | No |

### Logging
| Variable | Valor YAML Afectado | Default | Nota |
|----------|------------------|---------|------|
| `LOGGING_LEVEL` | `logging.level` | `info` | debug, info, warn, error |
| `LOGGING_FORMAT` | `logging.format` | `json` | json ó text |

### IA Bedrock
| Variable | Valor YAML Afectado | Default | Requerida |
|----------|------------------|---------|-----------|
| `BEDROCK_API_KEY` | `ia.bedrock.api_key` | vacío | No (en desarrollo) |
| `IA_AWS_ACCESS_KEY_ID` | `ia.bedrock.access_key_id` | vacío | No |
| `IA_AWS_SECRET_ACCESS_KEY` | `ia.bedrock.secret_access_key` | vacío | No |
| `IA_BEDROCK_MODEL_ID` | `ia.bedrock.model_id` | vacío | No (en desarrollo) |
| `IA_BEDROCK_REGION` | `ia.bedrock.region` | `us-east-1` | No |

---

## 📝 Ejemplos de Uso

### Desarrollo Local (Bash/PowerShell)
```bash
# Exportar variables
export MYSQL_HOST=localhost
export MYSQL_USER=agro_user
export MYSQL_PASSWORD=secret123
export AWS_REGION=us-west-2
export S3_BUCKET=agro-sentinel-bucket
export S3_ACCESS_KEY_ID=AKIA...
export S3_SECRET_ACCESS_KEY=...
export AUTH_JWT_SECRET=mi-clave-super-segura-de-32-caracteres-o-mas

# Ejecutar servicio
./bin/sync --auto
./bin/worker --auto
./bin/api
```

### Docker Compose (desde .env)
```bash
# .env
MYSQL_HOST=mysql
MYSQL_USER=agro_user
MYSQL_PASSWORD=secret123
AWS_REGION=us-west-2
S3_BUCKET=agro-sentinel-bucket
AUTH_JWT_SECRET=mi-clave-super-segura-de-32-caracteres-o-mas

# Ejecutar
docker-compose up
```

### Docker Compose (sin .env, en CI/CD)
```yaml
# docker-compose.ci.yml o variables inyectadas
services:
  api:
    environment:
      MYSQL_HOST: mysql-prod
      MYSQL_USER: ${PROD_MYSQL_USER}
      MYSQL_PASSWORD: ${PROD_MYSQL_PASSWORD}
      AWS_REGION: us-east-1
      S3_BUCKET: prod-sentinel-bucket
      AUTH_JWT_SECRET: ${PROD_JWT_SECRET}
```

---

## ✅ Checklist de Configuración

### Desarrollo Local
- [ ] Archivo `configs/config.yaml` existe y es válido
- [ ] Variables de entorno claves exportadas: `MYSQL_HOST`, `MYSQL_USER`, `MYSQL_PASSWORD`
- [ ] AWS configurado: `AWS_REGION`, `S3_BUCKET`, credenciales (si AWS real)
- [ ] Auth configurado: `AUTH_JWT_SECRET` (mínimo 32 caracteres)

### Producción / Docker
- [ ] `.env` tiene todas las variables requeridas (no vacías)
- [ ] `docker-compose.yml` tiene todas las referencias correctas
- [ ] Credenciales sensibles NO están en `docker-compose.yml` (usar `.env`)
- [ ] `MYSQL_HOST` apunta al host correcto (no `localhost`)
- [ ] `AUTH_JWT_SECRET` es fuerte y único
- [ ] Logs están configurados a nivel `info` o `warn`

---

## 🔒 Notas de Seguridad

1. **JWT Secret:** Mínimo 32 caracteres, preferiblemente 64+
   ```bash
   # Generar en PowerShell
   -join ((65..90) + (97..122) + (48..57) | Get-Random -Count 64 | %{[char]$_})
   
   # Generar en Bash
   openssl rand -base64 64
   ```

2. **Credenciales:** Nunca en `docker-compose.yml`, solo en `.env` (que está en `.gitignore`)

3. **DELETE_ALLOWED_USER_IDS:** Lista vacía = NADIE puede borrar (fallback seguro)

---

## 🐛 Troubleshooting

### "Error: loading config: reading config file"
```
→ CONFIG_PATH apunta a archivo inexistente
→ Solución: Verificar ruta; en Docker debe ser /etc/agro-sentinel/config.yaml
```

### "MySQL connection refused"
```
→ MYSQL_HOST no está configurado o es incorrecto
→ En Docker: usar nombre del servicio (mysql, no localhost)
```

### "S3 operation failed: missing AWS credentials"
```
→ S3_ACCESS_KEY_ID / S3_SECRET_ACCESS_KEY vacíos
→ O credenciales default chain no configurada
```

### "Auth: invalid JWT secret length"
```
→ AUTH_JWT_SECRET < 32 caracteres
→ Generar nueva clave de 64 caracteres
```

---

## 📚 Referencias

- Archivo de configuración: `internal/config/config.go`
- Ejemplo YAML: `configs/config.example.yaml`
- Variables .env: `.env.example`
- Funciones relevantes:
  - `config.Load(path)` — carga config.yaml
  - `applyEnvOverrides(cfg)` — aplica sobrescritos de env vars
