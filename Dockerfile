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
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates python3-numpy && rm -rf /var/lib/apt/lists/*
COPY --from=builder /bin/api /bin/worker /bin/sync /usr/local/bin/
COPY configs/config.yaml /etc/agro-sentinel/config.yaml
COPY scripts/overlay_polygon.py /usr/local/bin/overlay_polygon.py
COPY scripts/calc_index.py /usr/local/bin/calc_index.py
RUN mkdir -p /tmp/agro-sentinel
WORKDIR /app
