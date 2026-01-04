# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the agent with verbose output
RUN CGO_ENABLED=0 GOOS=linux go build -v -o pullbase-agent

# Final stage
FROM alpine:latest

# Install dependencies 
RUN apk add --no-cache nginx supervisor git ca-certificates openrc

WORKDIR /app

# Copy the agent binary
COPY --from=builder /app/pullbase-agent .

# Make the agent executable
RUN chmod +x /app/pullbase-agent

# Copy the CA certificate from the host if it exists
RUN if [ -f certs/ca.crt ]; then \
        cp certs/ca.crt /etc/ssl/certs/pullbase_ca.crt && \
        update-ca-certificates; \
    fi

# Create necessary directories
RUN mkdir -p /etc/pullbase/repo /etc/supervisor/conf.d /var/log/nginx /etc/nginx/conf.d /run/nginx

# Create basic nginx configuration
RUN echo 'events { worker_connections 1024; }' > /etc/nginx/nginx.conf && \
    echo 'http {' >> /etc/nginx/nginx.conf && \
    echo '  include /etc/nginx/mime.types;' >> /etc/nginx/nginx.conf && \
    echo '  default_type application/octet-stream;' >> /etc/nginx/nginx.conf && \
    echo '  server {' >> /etc/nginx/nginx.conf && \
    echo '    listen 80;' >> /etc/nginx/nginx.conf && \
    echo '    location / {' >> /etc/nginx/nginx.conf && \
    echo '      proxy_pass http://localhost:3000;' >> /etc/nginx/nginx.conf && \
    echo '    }' >> /etc/nginx/nginx.conf && \
    echo '  }' >> /etc/nginx/nginx.conf && \
    echo '}' >> /etc/nginx/nginx.conf

# Create supervisor configuration
RUN echo '[supervisord]' > /etc/supervisor/supervisord.conf && \
    echo 'nodaemon=true' >> /etc/supervisor/supervisord.conf && \
    echo 'logfile=/tmp/supervisord.log' >> /etc/supervisor/supervisord.conf && \
    echo 'pidfile=/var/run/supervisord.pid' >> /etc/supervisor/supervisord.conf && \
    echo '[unix_http_server]' >> /etc/supervisor/supervisord.conf && \
    echo 'file=/var/run/supervisor.sock' >> /etc/supervisor/supervisord.conf && \
    echo 'chmod=0700' >> /etc/supervisor/supervisord.conf && \
    echo '[supervisorctl]' >> /etc/supervisor/supervisord.conf && \
    echo 'serverurl=unix:///var/run/supervisor.sock' >> /etc/supervisor/supervisord.conf && \
    echo '[rpcinterface:supervisor]' >> /etc/supervisor/supervisord.conf && \
    echo 'supervisor.rpcinterface_factory = supervisor.rpcinterface:make_main_rpcinterface' >> /etc/supervisor/supervisord.conf && \
    echo '[include]' >> /etc/supervisor/supervisord.conf && \
    echo 'files = /etc/supervisor/conf.d/*.conf' >> /etc/supervisor/supervisord.conf

# Create separate program conf file
RUN echo '[program:nginx]' > /etc/supervisor/conf.d/nginx.conf && \
    echo 'command=nginx -g "daemon off;"' >> /etc/supervisor/conf.d/nginx.conf && \
    echo 'stdout_logfile=/tmp/nginx-stdout.log' >> /etc/supervisor/conf.d/nginx.conf && \
    echo 'stderr_logfile=/tmp/nginx-stderr.log' >> /etc/supervisor/conf.d/nginx.conf && \
    echo 'stdout_logfile_maxbytes=0' >> /etc/supervisor/conf.d/nginx.conf && \
    echo 'stderr_logfile_maxbytes=0' >> /etc/supervisor/conf.d/nginx.conf && \
    echo '[program:agent]' > /etc/supervisor/conf.d/agent.conf && \
    echo 'command=/app/pullbase-agent' >> /etc/supervisor/conf.d/agent.conf && \
    echo 'stdout_logfile=/tmp/agent-stdout.log' >> /etc/supervisor/conf.d/agent.conf && \
    echo 'stderr_logfile=/tmp/agent-stderr.log' >> /etc/supervisor/conf.d/agent.conf && \
    echo 'stdout_logfile_maxbytes=0' >> /etc/supervisor/conf.d/agent.conf && \
    echo 'stderr_logfile_maxbytes=0' >> /etc/supervisor/conf.d/agent.conf

# Start supervisor which will manage both nginx and the agent
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisord.conf"]