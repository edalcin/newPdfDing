# newPdfDing — imagem de produção: 3 estágios, runtime distroless não-root.
# Ver refatoracao/07-docker-ci-deploy.md, "Dockerfile de três estágios".

# ── Stage 1: Frontend build (Node + pdf.js) ─────────────────────────────────
FROM node:22-alpine AS frontend

WORKDIR /app/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ .

# pdf.js 5.5.207 já é uma dependência declarada em package.json (pdfjs-dist)
# — nada para baixar aqui; npm run build chama frontend/scripts/copy-pdfjs.mjs
# antes do vite build (ver 06-frontend.md, "Viewer — ponte postMessage").
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


# ── Stage 3: Runtime (distroless, não-root) ──────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/newpdfding /newpdfding

USER nonroot

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD ["/newpdfding", "-healthcheck"]

ENTRYPOINT ["/newpdfding"]
