# newPdfDing — Dockerfile provisório de 2 estágios (sem frontend).
# O estágio de build do frontend (Node + pdf.js) e o go:embed do SvelteKit
# chegam na ETAPA-9-UI-BASE / ETAPA-11-DOCKER-CI — ver refatoracao/07-docker-ci-deploy.md.

# ── Stage 1: Go build ────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/newpdfding ./cmd/newpdfding


# ── Stage 2: Runtime (distroless, não-root) ──────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/newpdfding /newpdfding

USER nonroot

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD ["/newpdfding", "-healthcheck"]

ENTRYPOINT ["/newpdfding"]
