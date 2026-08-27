# Próximos passos

> Handoff da sessão de 2026-08-27. Ponto de partida para a próxima sessão.
> Estado do repositório neste documento: `main` em `54f5c66`, árvore limpa, tudo empurrado.

---

## ⚠️ Antes de qualquer coisa

**A imagem publicada (`ghcr.io/edalcin/newpdfding:latest`) contém o defeito que derrubou o servidor UNRAID. NÃO aperte "Reembedar pendentes" em `/admin` até as correções da seção 2 estarem no ar.**

O caminho manual (ícone de cérebro em cada card da lista) é seguro: enfileira um documento por vez.

---

## 1. O incidente

O usuário trocou o modelo para **Gemini Embedding 2** em Configurações → IA e disparou `POST /api/admin/reembed` para **160+ documentos**. O servidor UNRAID caiu no meio do processo.

### Evidência coletada da rede (host `192.168.1.10`)

| Camada | Resultado |
|---|---|
| ICMP (ping) | responde, 0 ms |
| ARP | MAC `e8-48-b8-ca-5a-6f` resolvido — NIC viva |
| TCP porta 22 | conexão **aceita** |
| Banner SSH | **nunca chega**, nem com `ConnectTimeout=60` |

Kernel vivo, userspace travado: o `sshd` aceita o socket mas não emite a string de versão. Padrão de thrash de memória ou filesystem raiz travado. **Não houve acesso ao host** — diagnóstico é 100% por leitura de código e continua sendo *hipótese*, não fato confirmado.

### Evidência que ainda falta (colher assim que a máquina subir)

```bash
dmesg | grep -iE 'oom|killed process|out of memory'    # o veredito
cp /var/log/syslog /mnt/user/<share>/syslog-incidente   # UNRAID: syslog vive em RAM, some no reboot
mount | grep -w /tmp                                     # confirma se /tmp é tmpfs (RAM)
docker stats --no-stream newpdfding                      # RSS do container em regime normal
```

Se o `dmesg` não mostrar OOM, a hipótese principal cai e é preciso investigar disco (WAL do SQLite, `docker.img` cheio) antes de assumir memória.

### Causa provável (ordenada por força da evidência)

**1. `/tmp` é RAM em UNRAID — `internal/server/consumer.go:146`**

```go
tmp, err := os.CreateTemp("", "npd-*.pdf")   // "" = $TMPDIR ou /tmp
io.Copy(tmp, rc)                             // copia o PDF INTEIRO
```

Roda uma vez por documento sem texto extraído, dentro de `textFor` → `extractPDFTextFromStorage`. O container roda `--read-only` (recomendado pelo próprio `UNRAID.md:14`), com apenas `/data` e `/files` graváveis — então `/tmp` só funciona se for tmpfs, e **tmpfs é RAM**. Em UNRAID o rootfs já é RAM, então encher tmpfs derruba o sistema operacional, não só o container.

Agravantes no mesmo caminho (`consumer.go:118-134`): `GetPlainText()` → `buf.ReadFrom()` → `buf.String()` é memória ilimitada **e** copia tudo duas vezes. `MAX_UPLOAD_MB` default é 200 (`internal/config/config.go:62`), então um scan grande sozinho passa de 600 MB de pico.

**2. Sem limite de memória no container**

Nem `compose.yaml` nem o template de `UNRAID.md` definem limite. É por isso que o **host** caiu em vez de o Docker matar só o container. Um `mem_limit` transforma "servidor derrubado" em "container reiniciado".

**3. Polling de varredura completa — `frontend/src/routes/admin/+page.svelte:79-88`**

`pollInfo` chama `GET /admin/info` **a cada 5 s** enquanto houver pendentes, ou seja por horas. Cada chamada carrega o `body` **completo** de todos os documentos para RAM (`internal/store/semantic.go:179`), e `buildEmbedText` usa só os **primeiros 2000 bytes** (`semantic.go:26,29-34`). Megabytes carregados para usar 2 KB, 720 vezes por hora.

Com 160 arquivos isso **provavelmente não derruba a máquina sozinho** — é churn de GC empilhado sobre o pico de extração. Com o acervo de ~20.000 do documento de projeto, seria fatal por si só.

**Correção de rumo registrada:** na primeira análise desta sessão eu estimei o cenário de 20.000 PDFs e apontei o polling como causa principal. Com 160 documentos o vetor principal é a extração em tmpfs. O polling é agravante.

---

## 2. Trabalho pendente

Nada disto foi aplicado. As três primeiras tarefas são independentes e tocam arquivos disjuntos — podem ir em paralelo. A quarta depende de decisão de projeto (ver seção 3).

### 2.1 `TmpfsExtraction` — `internal/server/consumer.go`

**Alvo:** `extractPDFText` (~118) e `extractPDFTextFromStorage` (~139). Não tocar em `semantic.go`, `embedqueue.go`, `handlers_admin.go`, nem no frontend.

**Mudança:**
1. O temporário sai de `/tmp` e vai para um subdiretório do volume gravável de verdade (`s.cfg.Files`, ex. `<Files>/tmp`), criado com 0o750 seguindo o padrão de `internal/storage/local.go`. Manter o `defer os.Remove`. Se for trivial em poucas linhas, limpar `npd-*.pdf` órfãos na inicialização.
2. Teto explícito de bytes na extração, com constante nomeada e comentada. Usar **2 MB**, o mesmo teto que o cliente já aplica (`TEXT_LIMIT_BYTES` em `frontend/src/lib/pdf-process.ts:11`) — hoje servidor e navegador têm limites diferentes. Usar `io.LimitReader`/`io.CopyN` + `bytes.Buffer.Grow` para não crescer dobrando nem copiar duas vezes.

**Aceite:** `go build ./...`, `go vet ./...`, `go test ./internal/server/ -run 'Consumer|Text'`.

### 2.2 `StatusMemory` — `internal/store/semantic.go`

**Alvo:** `attachEmbeddingStatus` (~168), `allWithEmbeddingStatus` (~241), `Stats` (~267), `PendingEmbeddingIDs` (~283). **Não** mudar `buildEmbedText`, `contentHash` nem `embedBodyChars`.

**Mudança:**
1. Trocar `SELECT pdf_id, body FROM pdf_text WHERE pdf_id IN (...)` por `substr(body, 1, embedBodyChars)` no próprio SQL, parando de trafegar bodies inteiros.
2. Reescrever o caminho de acervo inteiro para **streaming**: uma query com `LEFT JOIN pdf_text` e `LEFT JOIN pdf_embeddings`, sem cláusula `IN`, processando uma linha por vez. `Stats()` só **conta**; `PendingEmbeddingIDs()` acumula **só os ids**. Distinguir "sem linha em `pdf_embeddings`" (`none`) de hash divergente (`stale`) com scan anulável. `attachEmbeddingStatus` continua com `IN` para o caminho paginado de 25 itens (ali é legítimo) e só ganha o `substr`. Remover `allWithEmbeddingStatus` se ficar sem uso.

**Invariante que não pode quebrar:** `substr()` do SQLite conta **caracteres**, o Go trunca por **bytes**. Como todo caractere UTF-8 ocupa ≥ 1 byte, 2000 caracteres contêm ≥ 2000 bytes, e o truncamento por byte que `buildEmbedText` já faz em seguida devolve exatamente o mesmo prefixo. **Se o hash mudar de valor, todo o acervo vira `stale` outra vez.**

**Bônus resolvido de graça:** o `IN (?,?,?...)` com um placeholder por documento estoura `SQLITE_MAX_VARIABLE_NUMBER` num acervo grande. O streaming elimina a cláusula.

**Aceite:** `go test ./internal/store/ ./internal/server/ -run 'Embed|Reembed|Stats|Admin|Search'`, e nenhum documento pode mudar de status por causa desta refatoração.

### 2.3 `PollingFix` — `frontend/src/routes/admin/+page.svelte`

**Alvo:** `reembed` e `pollInfo`. Reaproveitar o store que já existe: `frontend/src/lib/embed-jobs.svelte.ts` (`embedJobs`, já faz polling de `GET /api/embed/jobs` a cada 1500 ms e para sozinho quando a fila esvazia). Não tocar em `.go`, nem em `pdf-card.svelte`, nem na home.

**Mudança:** três defeitos meus, todos em `pollInfo`:
1. Fonte de progresso passa a ser `embedJobs` (mapa em memória). `/admin/info` no máximo **uma vez ao final**, quando a fila esvaziar.
2. `setInterval` nunca é limpo no desmonte — sair da página deixa o timer batendo para sempre. Limpar no teardown (`$effect` com cleanup ou `onDestroy`).
3. Nada impede dois timers: dois cliques criam dois intervalos.

Mostrar também quantos jobs faltam e quantos falharam (já está no mapa) — hoje a mensagem é estática.

**Aceite:** `npm run check` 0 erros/0 warnings, `npm run build` ok.

### 2.4 Limite de memória no deploy

`compose.yaml` e a tabela de `UNRAID.md` ganham limite de memória, para que um runaway futuro seja um restart de container e nunca uma queda de host. Decidir o valor a partir do `docker stats` em regime normal (colher — ver seção 1).

### 2.5 Pedido novo do usuário: reembedar um-a-um na lista

**Descoberta importante — metade disto já existe.** `EmbedButton` (`frontend/src/lib/components/embed-button.svelte`) já é renderizado em **todos** os layouts de card da lista: `pdf-card.svelte:63, 83, 111`. Depois da troca de modelo o ícone fica âmbar (`bxs-brain text-amber-500`), clicável, com tooltip "clique para reembedar", e enfileira **um** documento via `POST /api/pdfs/{id}/embed`. A legenda dos ícones já está na home.

**O que falta de verdade:** com 160 documentos todos desatualizados, não há como *achar* os pendentes. `handleListPDFs` (`internal/server/handlers_pdfs.go:104-133`) filtra por `tag`, `starred`, `archived` e `q` — **não** por `embedding_status`.

Não implementar antes de resolver a decisão da seção 3. Provavelmente o pacote certo é:
- filtro "só pendentes" na listagem;
- rótulo visível (não só ícone) nos layouts de lista/compacto, onde há espaço — o usuário não percebeu que o botão existia.

---

## 3. Decisões abertas

**Filtro por `embedding_status`: servidor ou cliente?** `embedding_status` é derivado em Go, não é coluna — então `WHERE` em SQL não sai de graça, e filtrar no servidor interage com a paginação por cursor (`internal/store/cursor.go`). Filtrar no cliente é trivial mas mostra um conjunto incompleto, porque a lista usa scroll infinito com cursor. **Não decidido.** Examinar `cursor.go` + `ListParams` antes de escolher.

**A ação em massa continua existindo?** O usuário pediu o caminho manual, não pediu para remover o massivo. Manter os dois, com o massivo corrigido, foi a suposição desta sessão — confirmar.

---

## 4. Já entregue nesta sessão (não refazer)

| Commit | O que |
|---|---|
| `3313206` | `POST /api/admin/reembed` + botão "Reembedar pendentes" em `/admin`; `enqueueBulk` com envio bloqueante (auto-regula a fila, não fica limitado ao buffer de 256); guarda de dimensão em `dotProduct` (vetor de outro modelo pontuava similaridade inventada sobre o prefixo comum, capaz de passar do piso 0,30); testes `TestReembedAfterModelSwitch`, `TestReembedWithoutGeminiKey`, `TestDotProductRejectsDimensionMismatch` |
| `129b547` | 14 vulnerabilidades do Dependabot + 1 achada só pelo `npm audit` (`@sveltejs/kit` ReDoS); `chi` → 5.3.1; linha do Go → 1.26 com `toolchain` fixo no `go.mod`; `govulncheck` no CI deixa de ser `continue-on-error` |
| `cc92c4d` | Fila do Dependabot zerada: `pdfjs-dist` 5→6 (major, **zero mudança de código** — a v6 tornou `canvas` obrigatório em `RenderParameters` e o código já passava), `@embedpdf` 2.15.0, `sqlite` 1.57.0, Go 1.27, `node:24-alpine`, actions v7, `trivy-action` sai de `@master` para tag fixa |
| `54f5c66` | `trivy-action@v0.36.0` (eu havia referenciado sem o `v` e o job falhou) |

**Estado externo:** 0 PRs abertos, 0 alertas do Dependabot, só o branch `main` no remoto, CI verde em `54f5c66`.

**Recusas deliberadas (não relitigar sem motivo novo):**
- `typescript` fica em **6.0.3**: a 7.0.2 viola o peer range de `@sveltejs/kit` e `svelte-check` (`^5.3.3 || ^6.0.0`).
- `node` fica na linha **24** (LTS ativa), não 26 ("Current"). `.github/dependabot.yml` ignora major do `node` para o PR não voltar toda vez.
- Os 12 alertas de `django`/`sqlparse`/`pypdf` em `uv.lock` foram dispensados como `not_used`: a stack Django saiu em `5b292f0` e não há mais nenhum arquivo Python no repositório.

---

## 5. Ambiente — incantações que funcionam aqui

```bash
# Toolchain: go.mod fixa toolchain go1.27.0; GOTOOLCHAIN=auto baixa sozinho
go build ./... && go vet ./... && go test ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...     # deve dizer "No vulnerabilities found"

cd frontend && npm run check && npm run build              # 0 erros / 0 warnings
npm audit --audit-level=low                                # deve dizer 0

# Imagem real (o dist NÃO é versionado; o Docker constrói o frontend)
docker build -t newpdfding:verify .
docker run --rm -p 8899:8000 -e ADMIN_PASSWORD=... -e GEMINI_API_KEY=... \
  -e DB_PATH=/data/db.sqlite -e FILES=/files newpdfding:verify
# DB_PATH e FILES são obrigatórios, senão: "configuration error: DB_PATH is required"
```

Notas de ambiente descobertas na prática:
- O binário `docker` no PATH deste shell não é executável direto pelo supervisor de processos; usar `C:/Program Files/Docker/Docker/resources/bin/docker.exe`.
- Cookies de sessão são `Secure`: num container em `http://127.0.0.1`, um reload perde a sessão. Logar de novo na página, sem `tab.goto`.
- O viewer EmbedPDF renderiza páginas como `<img>` com blob URL dentro de `embedpdf-container` (**shadow DOM**), não em `<canvas>` — procurar canvas dá falso negativo.
- `has_text` vem `false` na **listagem** e `true` no **detalhe** do mesmo PDF. Verificado como **pré-existente** (idêntico na imagem anterior `sha-328d8ab`); não é regressão do bump do pdfjs. Não investigado a fundo, não pedido.
- Verificar hipótese de regressão comparando com a imagem publicada anterior funciona muito bem: `docker pull ghcr.io/edalcin/newpdfding:sha-<commit>` e subir as duas em portas diferentes.

---

## 6. Ordem sugerida para a próxima sessão

1. Recuperar o servidor (console/IPMI ou power cycle) e **colher a evidência da seção 1** antes de reiniciar — o syslog do UNRAID vive em RAM e some no reboot.
2. Confirmar ou derrubar a hipótese de OOM com o `dmesg`.
3. Aplicar 2.1, 2.2 e 2.3 em paralelo; verificar na imagem Docker de verdade, não só em `go test`.
4. Aplicar 2.4 com o valor tirado do `docker stats`.
5. Resolver a decisão da seção 3 e então fazer 2.5.
6. Só depois disso reembedar o acervo — e a primeira tentativa deve ser pelo caminho manual, alguns documentos, olhando `docker stats`.
