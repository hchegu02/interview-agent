FROM node:22-alpine AS web-builder

WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS go-builder

WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /src/internal/httpapi/web/dist ./internal/httpapi/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S interview \
    && adduser -S -G interview interview

WORKDIR /app
COPY --from=go-builder /out/server /app/server
COPY config/config.yaml.example /app/config/config.yaml.example
COPY migrations /app/migrations
COPY seeds /app/seeds
RUN mkdir -p /var/lib/interview-agent/import-spool \
    && chown -R interview:interview /var/lib/interview-agent

USER interview
EXPOSE 8080
ENV INTERVIEW_SERVER_ADDR=:8080
ENV INTERVIEW_IMPORT_SPOOL_DIR=/var/lib/interview-agent/import-spool

ENTRYPOINT ["/app/server"]
CMD ["-config", "/app/config/config.yaml.example"]
