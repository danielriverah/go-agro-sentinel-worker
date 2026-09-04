### Task 15: Docker and docker-compose

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `.dockerignore`
- Create: `scripts/check-environment.sh`

**Interfaces:**
- Consumes: all 3 binaries from `cmd/`
- Produces: Docker images that run api, worker, sync with GDAL installed

- [ ] **Step 1: Create multi-stage Dockerfile**

```dockerfile
# Build stage
FROM golang:1.24-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/api ./cmd/api
RUN CGO_ENABLED=0 go build -o /bin/worker ./cmd/worker
RUN CGO_ENABLED=0 go build -o /bin/sync ./cmd/sync

# Runtime stage
FROM ghcr.io/osgeo/gdal:ubuntu-small-3.9.3
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /bin/api /bin/worker /bin/sync /usr/local/bin/
COPY configs/config.example.yaml /etc/agro-sentinel/config.yaml
WORKDIR /app
```

- [ ] **Step 2: Create docker-compose.yml**

```yaml
services:
  api:
    build: .
    command: api
    ports:
      - "6000:6000"
    environment:
      CONFIG_PATH: /etc/agro-sentinel/config.yaml
      MYSQL_HOST: mysql
      MYSQL_PASSWORD: ${MYSQL_PASSWORD:-devpass}
      AWS_ENDPOINT_URL: http://localstack:4566
      AWS_REGION: us-east-1
      AWS_ACCESS_KEY_ID: test
      AWS_SECRET_ACCESS_KEY: test
    depends_on:
      mysql:
        condition: service_healthy

  worker:
    build: .
    command: worker
    environment:
      CONFIG_PATH: /etc/agro-sentinel/config.yaml
      MYSQL_HOST: mysql
      MYSQL_PASSWORD: ${MYSQL_PASSWORD:-devpass}
      AWS_ENDPOINT_URL: http://localstack:4566
      AWS_REGION: us-east-1
      AWS_ACCESS_KEY_ID: test
      AWS_SECRET_ACCESS_KEY: test
    depends_on:
      mysql:
        condition: service_healthy

  sync:
    build: .
    command: sync
    environment:
      CONFIG_PATH: /etc/agro-sentinel/config.yaml
      MYSQL_HOST: mysql
      MYSQL_PASSWORD: ${MYSQL_PASSWORD:-devpass}
      AWS_ENDPOINT_URL: http://localstack:4566
      AWS_REGION: us-east-1
      AWS_ACCESS_KEY_ID: test
      AWS_SECRET_ACCESS_KEY: test
    depends_on:
      mysql:
        condition: service_healthy

  mysql:
    image: mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_PASSWORD:-devpass}
      MYSQL_DATABASE: agro
    ports:
      - "3306:3306"
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 5s
      retries: 10

  localstack:
    image: localstack/localstack:latest
    ports:
      - "4566:4566"
    environment:
      SERVICES: s3,sqs,dynamodb
```

- [ ] **Step 3: Create .dockerignore**

```
.git
bin/
tmp/
*.exe
docs/
```

- [ ] **Step 4: Create check-environment.sh**

Script that verifies GDAL, Go, and other dependencies are available.

- [ ] **Step 5: Build and test locally**

```bash
docker compose build
docker compose up -d mysql localstack
docker compose up api
# Test: curl http://localhost:6000/health
```

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "feat: Docker multi-stage build and docker-compose for local development"
```

---

