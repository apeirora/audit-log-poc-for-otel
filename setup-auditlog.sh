#!/bin/bash

set -e

echo "Step 1: Cloning opentelemetry-collector..."
if [ -d "opentelemetry-collector" ]; then
    echo "Directory opentelemetry-collector already exists, skipping clone..."
else
    git clone https://github.com/apeirora/opentelemetry-collector
fi

echo "Step 2: Checking out to main branch..."
cd opentelemetry-collector
git checkout main
cd ..

echo "Step 3: Cloning opentelemetry-collector-contrib..."
if [ -d "opentelemetry-collector-contrib" ]; then
    echo "Directory opentelemetry-collector-contrib already exists, skipping clone..."
else
    git clone https://github.com/apeirora/opentelemetry-collector-contrib
fi

echo "Step 4: Checking out contrib to main branch..."
cd opentelemetry-collector-contrib
git checkout main
cd ..

echo "Step 5: Cloning opentelemetry-go..."
if [ -d "opentelemetry-go" ]; then
    echo "Directory opentelemetry-go already exists, skipping clone..."
else
    git clone https://github.com/apeirora/opentelemetry-go
fi

echo "Step 6: Checking out to AuditLog branch..."
cd opentelemetry-go
git checkout AuditLog
cd ..

echo "Step 7: Building otelcontribcol for Linux (Docker)..."
cd opentelemetry-collector-contrib

if [ -f "bin/otelcontribcol_linux_amd64" ]; then
    echo "Linux binary already exists, skipping build..."
    echo "Found: bin/otelcontribcol_linux_amd64"
else
    echo "Building Linux binary for Docker..."
    GOOS=linux GOARCH=amd64 make otelcontribcol
    if [ $? -eq 0 ]; then
        echo "Linux build completed successfully!"
    else
        echo "Build failed!"
        exit 1
    fi
fi
cd ..

echo "Step 8: Build finished successfully, proceeding..."

echo "Step 9: Creating Docker network and starting Redis container..."
if ! docker network ls --format '{{.Name}}' | grep -q "^otel-network$"; then
    echo "Creating Docker network 'otel-network'..."
    docker network create otel-network || {
        echo "Failed to create Docker network!"
        exit 1
    }
else
    echo "Docker network 'otel-network' already exists"
fi

REDIS_NEEDS_RESTART=0
REDIS_EXISTS=0

if docker ps --format '{{.Names}}' | grep -q "^redis$"; then
    echo "Redis container is already running, checking network..."
    REDIS_NETWORK=$(docker inspect redis --format '{{range $net, $conf := .NetworkSettings.Networks}}{{$net}}{{end}}' 2>/dev/null)
    if echo "$REDIS_NETWORK" | grep -q "otel-network"; then
        echo "Redis is on the correct network, checking if it's responding..."
        if docker exec redis redis-cli ping 2>&1 | grep -q "PONG"; then
            echo "Redis is responding, reusing container..."
            REDIS_EXISTS=1
        else
            echo "Redis is not responding, will restart..."
            REDIS_NEEDS_RESTART=1
        fi
    else
        echo "Redis is not on otel-network, will restart with proper configuration..."
        REDIS_NEEDS_RESTART=1
    fi
elif docker ps -a --format '{{.Names}}' | grep -q "^redis$"; then
    echo "Redis container exists but is not running, will start with proper configuration..."
    REDIS_NEEDS_RESTART=1
    REDIS_EXISTS=1
fi

if [ $REDIS_NEEDS_RESTART -eq 1 ]; then
    echo "Stopping and removing existing Redis container..."
    docker stop redis 2>/dev/null || true
    docker rm redis 2>/dev/null || true
    echo "Creating new Redis container on otel-network..."
    docker run -d --name redis --network otel-network -p 6379:6379 redis:latest || {
        echo "Redis container failed to start!"
        exit 1
    }
elif [ $REDIS_EXISTS -eq 0 ]; then
    echo "Creating new Redis container on otel-network..."
    docker run -d --name redis --network otel-network -p 6379:6379 redis:latest || {
        echo "Redis container failed to start!"
        exit 1
    }
fi

echo "Waiting for Redis container to be ready..."
sleep 2

if ! docker ps --format '{{.Names}}' | grep -q "^redis$"; then
    echo "Error: Redis container is not running!"
    echo "Checking container status:"
    docker ps -a --filter "name=redis" --format "table {{.Names}}\t{{.Status}}"
    exit 1
fi

echo "Waiting for Redis to be accessible..."
MAX_RETRIES=30
RETRY_COUNT=0
REDIS_READY=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if docker exec redis redis-cli ping >/dev/null 2>&1; then
        if command -v redis-cli >/dev/null 2>&1; then
            if redis-cli -h localhost -p 6379 ping >/dev/null 2>&1; then
                echo "Redis is ready and accessible from host!"
                REDIS_READY=1
                break
            fi
        else
            if command -v nc >/dev/null 2>&1 && nc -z localhost 6379 2>/dev/null; then
                echo "Redis is ready and port is accessible from host!"
                REDIS_READY=1
                break
            elif command -v powershell >/dev/null 2>&1 && powershell -Command "Test-NetConnection -ComputerName localhost -Port 6379 -InformationLevel Quiet -WarningAction SilentlyContinue" 2>/dev/null | grep -q "True"; then
                echo "Redis is ready and port is accessible from host!"
                REDIS_READY=1
                break
            elif docker exec redis redis-cli ping 2>&1 | grep -q "PONG"; then
                echo "Redis is ready (verified from container, assuming host access is available)..."
                REDIS_READY=1
                break
            fi
        fi
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "Waiting for Redis... ($RETRY_COUNT/$MAX_RETRIES)"
    sleep 1
done

if [ $REDIS_READY -eq 0 ]; then
    echo "Warning: Redis may not be fully ready, but continuing anyway..."
    echo "Checking Redis container status:"
    docker ps --filter "name=redis" --format "table {{.Names}}\t{{.Status}}"
    echo "Testing Redis connection from container:"
    docker exec redis redis-cli ping || echo "Failed to ping Redis from container"
else
    echo "Waiting a moment for Redis to fully stabilize..."
    sleep 2
    echo "Performing final Redis connection test from host..."
    CONNECTION_TEST_OK=0
    if command -v redis-cli >/dev/null 2>&1; then
        if redis-cli -h localhost -p 6379 ping 2>&1 | grep -q "PONG"; then
            echo "Redis connection test successful from host!"
            CONNECTION_TEST_OK=1
        else
            echo "Warning: Redis connection test failed from host, but continuing..."
        fi
    elif command -v nc >/dev/null 2>&1; then
        if nc -z -w 2 localhost 6379 2>/dev/null; then
            echo "Redis port is accessible (tested with nc)!"
            CONNECTION_TEST_OK=1
        fi
    else
        echo "Skipping host connection test (Redis is running in container and port is mapped - assuming accessible)"
        CONNECTION_TEST_OK=1
    fi
fi

echo "Verifying Redis container port mapping..."
REDIS_PORT=$(docker port redis 6379 2>/dev/null | head -1)
if [ -n "$REDIS_PORT" ]; then
    echo "Redis is mapped to: $REDIS_PORT"
    
    echo "Performing final connection verification..."
    echo "Waiting additional 3 seconds for Redis to be fully ready..."
    sleep 3
    
    if docker exec redis redis-cli ping 2>&1 | grep -q "PONG"; then
        echo "Redis is responding from inside container"
        
        echo "Redis is running in container with port mapped to 0.0.0.0:6379"
        echo "Collector should be able to connect to 127.0.0.1:6379"
        echo "Redis is ready for collector to connect!"
    else
        echo "Error: Redis is not responding in container!"
        exit 1
    fi
else
    echo "Warning: Could not verify Redis port mapping"
    echo "Restarting Redis container with port binding to all interfaces..."
    docker stop redis 2>/dev/null || true
    docker rm redis 2>/dev/null || true
    docker run -d --name redis -p 6379:6379 redis:latest
    echo "Waiting for Redis to restart..."
    sleep 5
    if docker exec redis redis-cli ping 2>&1 | grep -q "PONG"; then
        echo "Redis restarted and responding!"
    else
        echo "Warning: Redis restart verification failed"
    fi
fi

echo "Step 10: Creating auditlog-config.yaml..."
cat > auditlog-config.yaml << 'EOF'
extensions:
  redis_storage:
    endpoint: redis:6379
    db: 0

receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318

exporters:
  debug:
    verbosity: detailed
    sending_queue:
      enabled: true
      num_consumers: 10
      queue_size: 1000
      storage: redis_storage
      batch:
        flush_timeout: 1m
        min_size: 100
        max_size: 1000

service:
  extensions: [redis_storage]
  
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [debug]
    
    metrics:
      receivers: [otlp]
      exporters: [debug]
    
    logs:
      receivers: [otlp]
      exporters: [debug]
EOF

echo "Step 11: Running collector in Docker container..."
cd opentelemetry-collector-contrib

if [ -f "bin/otelcontribcol_linux_amd64" ]; then
    COLLECTOR_DIR=$(pwd)
    CONFIG_DIR=$(cd .. && pwd)
    
    if docker ps -a --format '{{.Names}}' | grep -q "^otel-collector$"; then
        echo "Removing existing otel-collector container..."
        docker rm -f otel-collector 2>/dev/null || true
    fi
    
    echo "Checking for and killing any Windows collector processes..."
    KILLED_ANY=0
    if command -v tasklist >/dev/null 2>&1; then
        WINDOWS_PIDS=$(tasklist /FI "IMAGENAME eq otelcontribcol_windows_amd64.exe" /FO CSV 2>/dev/null | grep -v "INFO:" | cut -d',' -f2 | tr -d '"' | grep -v "^$" || true)
        if [ -n "$WINDOWS_PIDS" ]; then
            echo "Found Windows collector processes: $WINDOWS_PIDS"
            for PID in $WINDOWS_PIDS; do
                echo "Killing Windows collector process: $PID"
                taskkill /F /PID "$PID" 2>/dev/null && KILLED_ANY=1 || true
            done
        fi
        WINDOWS_PIDS2=$(tasklist /FI "IMAGENAME eq otelcontribcol.exe" /FO CSV 2>/dev/null | grep -v "INFO:" | cut -d',' -f2 | tr -d '"' | grep -v "^$" || true)
        if [ -n "$WINDOWS_PIDS2" ]; then
            echo "Found additional Windows collector processes: $WINDOWS_PIDS2"
            for PID in $WINDOWS_PIDS2; do
                echo "Killing Windows collector process: $PID"
                taskkill /F /PID "$PID" 2>/dev/null && KILLED_ANY=1 || true
            done
        fi
        if [ $KILLED_ANY -eq 1 ]; then
            echo "Waiting for processes to terminate..."
            sleep 2
        else
            echo "No Windows collector processes found"
        fi
    elif command -v ps >/dev/null 2>&1; then
        WINDOWS_PIDS=$(ps aux 2>/dev/null | grep -i "otelcontribcol" | grep -v grep | awk '{print $2}' || true)
        if [ -n "$WINDOWS_PIDS" ]; then
            echo "Found Windows collector processes: $WINDOWS_PIDS"
            for PID in $WINDOWS_PIDS; do
                echo "Killing Windows collector process: $PID"
                kill -9 "$PID" 2>/dev/null && KILLED_ANY=1 || true
            done
            if [ $KILLED_ANY -eq 1 ]; then
                echo "Waiting for processes to terminate..."
                sleep 2
            fi
        else
            echo "No Windows collector processes found"
        fi
    fi
    
    echo "Ensuring Redis is fully ready before starting collector..."
    sleep 3
    
    if docker exec redis redis-cli ping 2>&1 | grep -q "PONG"; then
        echo "Redis is confirmed ready - starting collector in Docker..."
    else
        echo "Error: Redis is not responding! Cannot start collector."
        exit 1
    fi
    
    echo "Creating collector Docker container..."
    COLLECTOR_DIR_ABS=$(cd "$COLLECTOR_DIR" && pwd -W 2>/dev/null || pwd)
    CONFIG_DIR_ABS=$(cd "$CONFIG_DIR" && pwd -W 2>/dev/null || pwd)
    
    BINARY_PATH="${COLLECTOR_DIR_ABS}/bin/otelcontribcol_linux_amd64"
    CONFIG_PATH="${CONFIG_DIR_ABS}/auditlog-config.yaml"
    
    if [ ! -f "$BINARY_PATH" ]; then
        echo "Error: Binary not found at $BINARY_PATH"
        ls -la "$(dirname "$BINARY_PATH")" 2>/dev/null || echo "Directory listing failed"
        exit 1
    fi
    
    if [ ! -f "$CONFIG_PATH" ]; then
        echo "Error: Config file not found at $CONFIG_PATH"
        exit 1
    fi
    
    if command -v cygpath >/dev/null 2>&1; then
        BINARY_PATH_WIN=$(cygpath -w "$BINARY_PATH")
        CONFIG_PATH_WIN=$(cygpath -w "$CONFIG_PATH")
    elif [ -n "$MSYSTEM" ] || [ -n "$MSYS" ]; then
        BINARY_PATH_WIN=$(echo "$BINARY_PATH" | sed -e 's|^/\([a-z]\)|\1:|' -e 's|/|\\|g')
        CONFIG_PATH_WIN=$(echo "$CONFIG_PATH" | sed -e 's|^/\([a-z]\)|\1:|' -e 's|/|\\|g')
    else
        BINARY_PATH_WIN="$BINARY_PATH"
        CONFIG_PATH_WIN="$CONFIG_PATH"
    fi
    
    echo "Binary path (Windows): $BINARY_PATH_WIN"
    echo "Config path (Windows): $CONFIG_PATH_WIN"
    
    MSYS_NO_PATHCONV=1 docker run -d --name otel-collector \
        --network otel-network \
        --entrypoint /otelcontribcol \
        -v "${BINARY_PATH_WIN}:/otelcontribcol:ro" \
        -v "${CONFIG_PATH_WIN}:/auditlog-config.yaml:ro" \
        -p 4318:4318 \
        debian:bookworm-slim \
        --config /auditlog-config.yaml
    
    if [ $? -eq 0 ]; then
        echo "Collector container started successfully!"
        sleep 5
        
        if docker ps --format '{{.Names}}' | grep -q "^otel-collector$"; then
            echo "Collector container is running"
            echo "Waiting for collector to initialize..."
            sleep 5
            
            echo "Checking collector logs..."
            if docker logs otel-collector 2>&1 | grep -qi "Everything is ready\|Starting HTTP server"; then
                echo "Collector started successfully in Docker!"
            elif docker logs otel-collector 2>&1 | grep -qi "error.*redis\|failed.*redis"; then
                echo "Warning: Collector may have Redis connection issues. Checking logs..."
                docker logs otel-collector 2>&1 | tail -20
            else
                echo "Collector is starting up..."
                docker logs otel-collector 2>&1 | tail -10
            fi
            
            echo ""
            echo "=== Collector Status ==="
            echo "Running as: Docker container (otel-collector)"
            echo "Endpoint: http://localhost:4318"
            echo ""
            echo "To view logs: docker logs -f otel-collector"
            echo "To stop: docker stop otel-collector"
            echo "To restart: docker start otel-collector"
        else
            echo "Error: Collector container stopped immediately!"
            echo "Container logs:"
            docker logs otel-collector 2>&1
            exit 1
        fi
    else
        echo "Error: Failed to start collector container!"
        exit 1
    fi
else
    echo "Error: otelcontribcol_linux_amd64 not found in bin directory"
    echo "Available files in bin:"
    ls -la bin/ || echo "bin directory not found"
    exit 1
fi

echo "Step 12: Changing to opentelemetry-go directory..."
cd ../opentelemetry-go

echo "Setup complete!"
