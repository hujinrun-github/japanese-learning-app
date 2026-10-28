# syntax=docker/dockerfile:1.7

FROM node:22-bookworm-slim AS frontend-builder

WORKDIR /src

# Install frontend dependencies before copying the source so dependency layers
# remain cacheable when only application code changes.
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts

COPY front/react/package.json front/react/package-lock.json ./front/react/
COPY front/admin/package.json front/admin/package-lock.json ./front/admin/
RUN npm ci --prefix front/react
RUN npm ci --prefix front/admin

COPY scripts ./scripts
COPY front/react ./front/react
COPY front/admin ./front/admin

RUN npm --prefix front/react run build
RUN npm --prefix front/admin run build

FROM golang:1.25-bookworm AS go-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags='-s -w' -o /out/server ./backend/cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags='-s -w' -o /out/admin ./backend/cmd/admin

FROM alpine:3.22 AS app-runtime

RUN addgroup -S -g 1000 app \
    && adduser -S -D -H -u 1000 -G app app \
    && mkdir -p /app/data/audio \
    && chown -R app:app /app

WORKDIR /app
ENV DB_PATH=/app/data/app.db \
    AUDIO_STORE_PATH=/app/data/audio \
    STATIC_DIR=/app/front/dist/assets \
    TEMPLATE_DIR=/app/front/dist

USER app

FROM app-runtime AS server
COPY --from=go-builder --chown=app:app /out/server /app/server
COPY --from=frontend-builder --chown=app:app /src/front/dist /app/front/dist
ENTRYPOINT ["/app/server"]

FROM app-runtime AS admin-api
COPY --from=go-builder --chown=app:app /out/admin /app/admin
ENTRYPOINT ["/app/admin"]

FROM nginx:1.27-alpine AS learner-web
COPY --from=frontend-builder /src/front/dist /usr/share/nginx/html
COPY deploy/nginx/learner.conf /etc/nginx/conf.d/default.conf

FROM nginx:1.27-alpine AS admin-web
COPY --from=frontend-builder /src/front/admin/dist /usr/share/nginx/html
COPY deploy/nginx/admin.conf /etc/nginx/conf.d/default.conf
