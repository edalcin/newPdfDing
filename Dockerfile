# newPdfDing — imagem de produção: 3 estágios, runtime distroless não-root.
# Ver refatoracao/07-docker-ci-deploy.md, "Dockerfile de três estágios".

# ── Stage 1: Frontend build (Node + EmbedPDF/PDFium) ────────────────────────
FROM node:22-alpine AS frontend

WORKDIR /app/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ .

# @embedpdf/pdfium já é uma dependência transitiva declarada em package.json
# (via @embedpdf/svelte-pdf-viewer) — nada para baixar aqui; npm run build
# chama frontend/scripts/copy-embedpdf.mjs antes do vite build (ver
# 06-frontend.md, "Viewer — EmbedPDF").
RUN npm run build
# Saída: /app/frontend/build/  (adapter-static)


# ── Stage 2: Go build ────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend /app/frontend/build/ ./internal/server/web/dist/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/newpdfding ./cmd/newpdfding

# /data and /files are baked in empty here so that, on a fresh volume mount
# (named volume or empty bind mount), Docker's volume-populate-from-image
# behavior copies them in pre-owned by uid 65532 — otherwise a brand-new
# named volume is root-owned and the nonroot process below can never write
# its SQLite file or PDFs (ver refatoracao/08-seguranca.md, "Menor
# privilégio").
RUN mkdir -p /data /files


# ── Stage 3: Runtime (distroless, não-root) ──────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/newpdfding /newpdfding
COPY --from=build --chown=65532:65532 /data /data
COPY --from=build --chown=65532:65532 /files /files

USER nonroot

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD ["/newpdfding", "-healthcheck"]

ENTRYPOINT ["/newpdfding"]
