# newPdfDing

Self-hosted PDF manager and viewer, forked from [PdfDing by mrmn2](https://github.com/mrmn2/PdfDing).

## Attribution

Based on **PdfDing v1.9.0** by [mrmn2](https://github.com/mrmn2/PdfDing).  
Licensed under AGPL-3.0 (see `LICENSE`).  
This is an independent fork, not affiliated with or endorsed by the upstream project.

## How the image is built

Every push to `main` triggers `.github/workflows/docker-publish.yaml`, which:
1. Builds the `Dockerfile` (multi-stage: node/tailwind/pdfjs → poetry venv → alpine runtime).
2. Publishes two tags to GitHub Container Registry:
   - `ghcr.io/edalcin/newpdfding:latest`
   - `ghcr.io/edalcin/newpdfding:<commit-sha>`

No extra secrets required — `GITHUB_TOKEN` is sufficient.

## UNRAID deployment

Replace the old image reference in your `docker run` command:

```sh
docker run \
  -d \
  --name='newPdfDing' \
  --net='bridge' \
  --pids-limit 2048 \
  -e TZ="America/Sao_Paulo" \
  -e HOST_OS="Unraid" \
  -e HOST_HOSTNAME="Asilo" \
  -e HOST_CONTAINERNAME="newPdfDing" \
  -e 'HOST_NAME'='pdfding.dalc.in' \
  -e 'SECRET_KEY'='<your-secret-key>' \
  -e 'CSRF_COOKIE_SECURE'='FALSE' \
  -e 'SESSION_COOKIE_SECURE'='FALSE' \
  -e 'HOST_PORT'='8000' \
  -e 'DATABASE_TYPE'='SQLITE' \
  -e 'DEFAULT_THEME'='light' \
  -e 'DEFAULT_THEME_COLOR'='blue' \
  -e 'ADMIN_EMAIL'='admin@example.com' \
  -e 'ADMIN_PASSWORD'='changeme' \
  -l net.unraid.docker.managed=dockerman \
  -l net.unraid.docker.webui='https://pdfding.dalc.in' \
  -l net.unraid.docker.icon='https://web.dalc.in/infra/pdfdingIcon.png' \
  -p '8778:8000/tcp' \
  -v '/mnt/user/Storage/appsdata/pdfding/db':'/home/nonroot/pdfding/db/':'rw' \
  -v '/mnt/user/Storage/appsdata/pdfding/media':'/home/nonroot/pdfding/media/':'rw' \
  'ghcr.io/edalcin/newpdfding:latest'
```

Only two things changed from the original command:
- `--name` → `newPdfDing`
- image ref → `ghcr.io/edalcin/newpdfding:latest`

## One-time post-first-build step

After the first successful workflow run, set the GHCR package to **Public** so UNRAID pulls without credentials:

GitHub → your profile → **Packages** → `newpdfding` → **Package settings** → **Change visibility** → **Public**

Alternative: keep the package private and run `docker login ghcr.io` on UNRAID with a GitHub PAT that has `read:packages` scope before issuing `docker run`.
