FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ \
    ./cmd/control-api ./cmd/redirect ./cmd/analytics-api ./cmd/render-gateway

FROM node:22-alpine AS dashboard
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
ENV NEXT_PUBLIC_CONTROL_API="" \
    NEXT_PUBLIC_ANALYTICS_API="" \
    NEXT_PUBLIC_REDIRECT_URL="/s" \
    NEXT_PUBLIC_DEV_AUTH=true \
    NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM node:22-alpine
RUN apk add --no-cache bash tini
WORKDIR /app
COPY --from=backend /out/ /usr/local/bin/
COPY --from=dashboard --chown=node:node /app/ ./
COPY migrations/postgres/001_initial.sql deploy/render/schema.sql /migrations/
COPY deploy/render/start.sh /usr/local/bin/uzip-start
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    UZIP_MIGRATION_DIR=/migrations \
    GOMEMLIMIT=48MiB
USER node
EXPOSE 10000
ENTRYPOINT ["/sbin/tini", "--", "/bin/bash", "/usr/local/bin/uzip-start"]
