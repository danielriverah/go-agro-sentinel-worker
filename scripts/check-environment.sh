#!/bin/bash
set -e

echo "Checking environment for Agro Sentinel Worker..."

# Check Go
echo -n "Checking Go... "
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}')
    echo "OK (${GO_VERSION})"
else
    echo "FAIL: Go is not installed"
    exit 1
fi

# Check GDAL
echo -n "Checking GDAL... "
if command -v gdalinfo &> /dev/null; then
    GDAL_VERSION=$(gdalinfo --version)
    echo "OK (${GDAL_VERSION})"
else
    echo "WARN: GDAL is not installed (required for Docker builds)"
fi

# Check Docker
echo -n "Checking Docker... "
if command -v docker &> /dev/null; then
    DOCKER_VERSION=$(docker --version)
    echo "OK (${DOCKER_VERSION})"
else
    echo "WARN: Docker is not installed"
fi

# Check Docker Compose
echo -n "Checking Docker Compose... "
if command -v docker-compose &> /dev/null || docker compose version &> /dev/null; then
    echo "OK"
else
    echo "WARN: Docker Compose is not installed"
fi

# Check MySQL client (optional)
echo -n "Checking MySQL client... "
if command -v mysql &> /dev/null; then
    echo "OK"
else
    echo "WARN: MySQL client is not installed (optional)"
fi

echo ""
echo "Environment check complete!"
