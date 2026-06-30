# newPdfDing

Self-hosted PDF manager and viewer — independent fork of [PdfDing by mrmn2](https://github.com/mrmn2/PdfDing).

> **This is not an official release of PdfDing.** It is a private fork maintained independently and not affiliated with or endorsed by the upstream project or its author.

---

## What is PdfDing?

PdfDing is a self-hosted PDF manager, viewer and editor. It offers browser-based PDF reading across devices, remembers your reading position, supports multi-level tagging, collections, annotations, highlights, Markdown notes, and a clean dark-mode UI.

This fork preserves all core document-management features while removing the multi-user infrastructure to produce a simpler, single-admin deployment.

---

## Differences from upstream

| Area | Upstream PdfDing | This fork |
|---|---|---|
| **Users** | Multi-user with registration and invitations | Single admin user; no registration, no invitations |
| **Auth** | Password + OIDC/SSO + 2FA (TOTP/WebAuthn) | Password only (admin credentials via env vars) |
| **Workspaces** | Multiple workspaces per user | Single personal workspace auto-created for the admin |
| **Admin panel** | User management (create/edit/delete users, admin overview) | Instance information + tag management only |
| **Duplicate detection** | No content-hash deduplication | SHA-256 check on upload — rejects files that already exist; bulk upload skips duplicates silently |
| **Tag input** | Plain text field | Auto-complete picker — shows existing tags as you type, allows creating new ones inline |
| **Tag management** | Rename/delete per-tag (inline edit) | Admin page (`/admin/tags`) — create, rename, delete, and **substitute** (merge one tag into another across all PDFs) |
| **Container image** | Published to Docker Hub (`mrmn/pdfding`) | Built and published to GHCR (`ghcr.io/edalcin/newpdfding`) on every push to `main` |
| **Public sharing** | Full shared-PDF feature (password, expiry, max views, QR code, sessions) | Minimal share link per PDF — one UUID link, no password/expiry; owner shares/unshares from the details page; public viewer is read-only (no annotations, highlights or notes visible); admin can revoke any share at `/admin/shares` |

Everything else — PDF viewer, annotations, highlights, Markdown notes, collections, starring, archiving, progress bars, signatures, themes, dark mode, backup — is inherited unchanged from **PdfDing v1.9.0**.

---

## Running with Docker

### Minimal example

```sh
docker run -d \
  --name newpdfding \
  -e SECRET_KEY='<your-secret-key>' \
  -e ADMIN_EMAIL='admin@example.com' \
  -e ADMIN_PASSWORD='changeme' \
  -p 8000:8000 \
  -v /path/to/db:/home/nonroot/pdfding/db \
  -v /path/to/media:/home/nonroot/pdfding/media \
  ghcr.io/edalcin/newpdfding:latest
```

### Unraid deployment

```sh
docker run \
  -d \
  --name='newPdfDing' \
  --net='bridge' \
  --pids-limit 2048 \
  -e TZ="America/Sao_Paulo" \
  -e HOST_NAME='pdfding.example.com' \
  -e SECRET_KEY='<your-secret-key>' \
  -e CSRF_COOKIE_SECURE='FALSE' \
  -e SESSION_COOKIE_SECURE='FALSE' \
  -e HOST_PORT='8000' \
  -e DATABASE_TYPE='SQLITE' \
  -e DEFAULT_THEME='light' \
  -e DEFAULT_THEME_COLOR='blue' \
  -e ADMIN_EMAIL='admin@example.com' \
  -e ADMIN_PASSWORD='changeme' \
  -p '8778:8000/tcp' \
  -v '/mnt/user/Storage/appsdata/pdfding/db':'/home/nonroot/pdfding/db/':'rw' \
  -v '/mnt/user/Storage/appsdata/pdfding/media':'/home/nonroot/pdfding/media/':'rw' \
  'ghcr.io/edalcin/newpdfding:latest'
```

---

## CI / Container image

Every push to `main` triggers `.github/workflows/docker-publish.yaml`, which:

1. Builds the `Dockerfile` (multi-stage: node/Tailwind/PDF.js → poetry venv → Alpine runtime).
2. Publishes two tags to GHCR:
   - `ghcr.io/edalcin/newpdfding:latest`
   - `ghcr.io/edalcin/newpdfding:<commit-sha>`

`GITHUB_TOKEN` is sufficient — no extra secrets required.

### Make the package public (one-time)

After the first successful workflow run:

> GitHub → your profile → **Packages** → `newpdfding` → **Package settings** → **Change visibility** → **Public**

Alternatively keep it private and run `docker login ghcr.io` on Unraid with a GitHub PAT (`read:packages` scope) before pulling.

---

## Attribution

Based on **[PdfDing v1.9.0](https://github.com/mrmn2/PdfDing)** by [mrmn2](https://github.com/mrmn2).  
Licensed under AGPL-3.0 — see [`LICENSE`](./LICENSE).
