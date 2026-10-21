FROM node:24.15.0-alpine3.22 AS frontend
WORKDIR /src
COPY package*.json ./
RUN npm ci
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
COPY api/ /src/api/
RUN npm run typecheck && npm run build

FROM golang:1.25.0-alpine3.22 AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/web/dist ./internal/adapters/webui/dist/
RUN CGO_ENABLED=0 go build -tags production -trimpath -o /out/wikit ./cmd/wikit
RUN CGO_ENABLED=0 go build -trimpath -o /out/acceptance ./tests/acceptance
RUN mkdir -p /empty/media /empty/state

FROM scratch AS runtime
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=backend /out/wikit /wikit
COPY --from=backend --chown=65532:65532 /empty/media /var/lib/wikit/media
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/wikit"]
CMD ["serve"]

# CI-only driver; never shipped in the default production target.
FROM runtime AS acceptance
COPY --from=backend /out/acceptance /acceptance
COPY --from=backend --chown=65532:65532 /empty/state /state
ENTRYPOINT ["/acceptance"]

FROM runtime AS production
