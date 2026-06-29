# Análise: Migração do Frontend — SvelteKit + shadcn-svelte e Alternativas

> Status: análise somente — nenhuma mudança no código.
> Atualizado: 2026-06-29

---

## Premissas revisadas

- **Sem multi-usuário**: sistema passa a ter um único administrador; senha passada via variável de ambiente `ADMIN_PASSWORD` no Docker. Remove django-allauth, MFA, OIDC, Profile multi-tenant, workspaces por usuário, e o sistema de sharing entre usuários.
- **Sem TipTap**: editor de texto rico fora do escopo.
- **DB preservado**: schema, migrations e dados de produção intocados.

---

## Stack atual

| Camada | Tecnologia |
|--------|-----------|
| Backend | Django 5.2 + Python 3.12 (Alpine) |
| Auth | django-allauth (email, OIDC, MFA: TOTP/WebAuthn) |
| Frontend rendering | Django templates + HTMX 2 + Alpine.js 3.15 |
| CSS | Tailwind v4 (compilado no build Docker) |
| PDF viewer | PDF.js 5.5 (embutido) |
| DB | SQLite (padrão) ou PostgreSQL |
| Tarefa assíncrona | Huey (thumbnails, backup) |
| Armazenamento | Local ou MinIO/S3 |
| Docker runtime | ~54 MB (python:3.12-alpine + venv) |

---

## Impacto de remover multi-usuário

Remover multi-usuário é a mudança de maior alavancagem. Elimina:

| Componente removível | Impacto |
|---------------------|---------|
| django-allauth (+ MFA + OIDC) | Remove a maior dependência; ~10 URLs de auth desaparecem |
| `Profile` model | Preferências viram settings globais ou env vars |
| `Workspace` model | Única workspace hardcoded |
| SharedPdf / SharedCollection | Feature de compartilhamento entre usuários desaparece |
| User management views | Eliminadas |
| Signup / password reset flows | Eliminados |

Auth substituta: middleware Django que lê `os.environ['ADMIN_PASSWORD']`, compara com POST `/login`, seta cookie de sessão. ~30 linhas, sem dependência externa.

---

## Stack proposta original (SvelteKit full)

| Camada | Nova tecnologia |
|--------|----------------|
| Frontend | SvelteKit (modo SPA) |
| Componentes UI | shadcn-svelte |
| Ícones | Boxicons |
| PWA | vite-plugin-pwa (Workbox) |
| Tema | dark/light toggle nativo no cliente |
| Listas | Rolagem infinita (IntersectionObserver) |
| Backend | Django simplificado — expõe REST API |
| DB | Mantido — zero mudança |

---

## Pontos positivos (mantidos)

### 1. DX e manutenibilidade do frontend
- Svelte compila para JS vanilla sem runtime; bundle menor que React/Vue
- TypeScript end-to-end; componentes testáveis via Vitest
- shadcn-svelte: componentes acessíveis via Tailwind; sem runtime de UI pesado

### 2. Docker menor (runtime)
- Backend: gunicorn + Django simplificado (sem allauth, sem MFA libs)
- Frontend build: artefato estático nginx:alpine (~8 MB) ou whitenoise no Django
- Runtime total estimado: ~40-45 MB (Alpine sem allauth/fido2/qrcode)

### 3. PWA real
- vite-plugin-pwa adiciona manifest, service worker, cache offline
- Instalável na tela inicial; cache de PDFs visualizados via Cache API

### 4. Rolagem infinita nativa
- IntersectionObserver + fetch; endpoint `get_next_pdf_overview_page/<int:page>/` reusado
- Elimina paginação clicável

### 5. Tema claro/escuro sem reload
- Toggle via `localStorage` + CSS variables; sem round-trip ao servidor

### 6. Separação clara de responsabilidades
- Django vira API pura; frontend independente com CI próprio

---

## Pontos negativos / riscos (revisados)

### 1. Custo de reescrita — menor com single-user, mas ainda significativo
- Sem multi-usuário: remove ~30% das views e templates
- Restam: ~100 URLs de PDF/collection/workspace; ~700 linhas de views
- Cada view precisa de serializer DRF; estimativa: **3-6 semanas** (vs. 2-4 meses anterior)

### 2. Auth — **risco eliminado**
- ~~allauth headless, MFA, OIDC~~ — tudo removido
- Substituto: middleware simples com env var. Sem bloqueador.

### 3. PDF.js integração
- Mantém-se o risco: viewer em iframe ou componente Svelte; postMessage para anotações
- Mitigação: manter viewer como rota Django separada (rota `/pdf/view/<id>`) servida por Django template estático; Svelte abre em iframe. Custo baixo.

### 4. i18n — simplificada com single-user
- Single-user: idioma pode ser fixo (en) ou env var `LANG`; sem per-profile language choice
- Elimina necessidade de `svelte-i18n` para múltiplos idiomas

### 5. HTMX inline-edit patterns a reimplementar
- Edição inline de campos (nome, tags, descrição) precisa virar componente Svelte com PATCH
- Sem allauth para se preocupar, este é agora o principal trabalho

### 6. Container adicional (mínimo)
- Opção A: nginx:alpine serve SvelteKit build; Django API no mesmo container com gunicorn — 1 container total
- Opção B: 2 containers (nginx + gunicorn) — compose mais simples que parece

### 7. Testes e2e a reescrever
- 12 arquivos Playwright; com single-user ficam mais simples (sem login multi-user)
- Estimativa: ~1 semana para reescrever o suite completo

### 8. shadcn-svelte — maturação
- Port comunitário; componentes avançados podem precisar de ajuste
- Mitigação: usar apenas primitivos (Button, Input, Dialog, Table) que são estáveis

---

## Alternativas ordenadas por custo

### Opção A — Dias: Melhorar stack atual (mínimo esforço)

**Manter Django + HTMX + Alpine. Adicionar:**
- Auth simplificada: remover allauth, adicionar middleware env var (~30 linhas)
- `django-pwa`: manifest + service worker básico → PWA instalável
- `htmx.ext.intersect.js`: infinite scroll com `hx-trigger="intersect"` nos cards, sem JS extra
- Boxicons: CDN ou npm, substituição CSS pura dos ícones atuais
- Tema: toggle client-only via `localStorage` + 1 endpoint PATCH assíncrono

**Obtém**: auth simplificada, PWA, infinite scroll, novos ícones, tema sem reload.  
**Não obtém**: SvelteKit, shadcn-svelte, arquitetura SPA.  
**Custo**: ~3-5 dias.

---

### Opção B — Semanas: Inertia.js + Svelte (recomendada)

**Como funciona**: Django views retornam `Inertia.render('Page', props={...})` ao invés de HTML. Frontend Svelte recebe props como componente. Sem REST API separada. Auth permanece cookie de sessão Django — nenhuma mudança de paradigma.

**Stack**:
- `django-inertia` adapter no backend
- SvelteKit (modo SPA via `@inertiajs/svelte`)
- shadcn-svelte, Boxicons
- vite-plugin-pwa
- IntersectionObserver para infinite scroll
- Tailwind v4 mantido

**Vantagens sobre SvelteKit full**:
- Zero REST API (Django views mantidas, só o retorno muda)
- Auth: mesmo cookie de sessão Django → zero mudança de auth
- Routing: continua server-side no Django (URLs mantidas)
- PDF viewer: manter como rota Django separada sem alteração

**O que é removido do backend**:
- allauth + MFA + OIDC → middleware simples com env var
- Templates HTML (exceto PDF viewer e erros 4xx)
- Alpine.js, HTMX

**Custo estimado com single-user**:
- Remover allauth + simplificar auth: 1-2 dias
- Instalar Inertia + configurar SvelteKit: 1 dia
- Migrar views uma a uma (sem REST API): ~2-3 semanas
- Reescrever e2e: ~1 semana
- **Total: ~3-4 semanas**

**Riscos restantes**: shadcn-svelte maturação; PDF viewer iframe communication.

---

### Opção C — Meses: SvelteKit SPA + Django REST

Arquitetura completa: Django expõe REST API (DRF), SvelteKit SPA consome. Com single-user, auth é trivial (env var → JWT ou session cookie). Maior esforço mas máxima separação.

**Custo com single-user**: ~6-8 semanas (antes: 2-4 meses).  
**Vantagem sobre Inertia**: frontend completamente desacoplado; pode hospedar separado.  
**Custo adicional vs Inertia**: escrever serializers DRF para cada modelo.

---

## Banco de dados: impacto zero

DB permanece intocado em todas as opções. Django ORM continua como única camada de acesso. UUIDs, FileFields, migrations — sem mudança. Dados de produção preservados.

---

## Recomendação direta

**Com as premissas revisadas (single-user + sem TipTap), a recomendação muda:**

**Escolha Opção B — Inertia.js + Svelte.**

Motivo: a remoção do multi-usuário eliminou o único bloqueador crítico (auth allauth headless). Inertia preserva o que Django faz bem (routing, ORM, session auth, PDF serving) e entrega o que foi pedido (SvelteKit, shadcn-svelte, Boxicons, PWA, infinite scroll, tema). Sem REST API separada = metade do trabalho de Opção C com 90% do resultado.

| Critério | Opção A (melhorar atual) | Opção B (Inertia — recomendada) | Opção C (SPA full) |
|---------|--------------------------|--------------------------------|-------------------|
| Custo | ~3-5 dias | ~3-4 semanas | ~6-8 semanas |
| SvelteKit | Não | Sim | Sim |
| shadcn-svelte | Não | Sim | Sim |
| PWA | Sim | Sim | Sim |
| Infinite scroll | Sim | Sim | Sim |
| Tema s/ reload | Sim | Sim | Sim |
| REST API necessária | Não | Não | Sim |
| Auth risco | Zero | Zero | Baixo |
| Docker size | ~40 MB (sem allauth) | ~42 MB | ~42 MB |
| Risco funcional | Muito baixo | Baixo | Médio |
