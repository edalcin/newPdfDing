# Frontend

Este documento fixa a stack do frontend SvelteKit, a lista fechada de rotas da SPA, o padrão único de rolagem infinita, o mecanismo de tema claro/escuro, a estratégia de PWA/service worker, o fluxo de upload com processamento client-side via pdf.js, a ponte `postMessage` do viewer, e a máquina de estados do botão de embedding sob demanda. Fecha com a tabela de paridade de UI, que mapeia cada rota nova às telas e views do produto atual — o inventário completo de funcionalidades, com etapa de implementação e as funcionalidades removidas, vive em [Inventário de Funcionalidades](10-inventario-funcionalidades.md) e não é repetido aqui.

## Stack fixado

| Camada | Tecnologia | Papel / configuração |
|---|---|---|
| Framework | SvelteKit 2 | Base da SPA |
| Adapter | `@sveltejs/adapter-static` | `fallback: 'index.html'`; SPA pura — `export const ssr = false;` e `export const prerender = false;` em `frontend/src/routes/+layout.ts` |
| Build tool | Vite | Bundler e dev server |
| Linguagem | TypeScript | Tipagem estática em todo o frontend |
| CSS | Tailwind CSS 4 | Via Vite (ver [tabela stack antigo → novo](00-visao-geral.md)) |
| Componentes de UI | shadcn-svelte | Configurado via `frontend/components.json`; componentes copiados para `frontend/src/lib/components/ui` — não é dependência npm tradicional |
| Ícones | Boxicons | Ex.: ícone `bx-brain` no botão de embedding |
| Editor de notas | TipTap | Substitui o textarea Markdown atual das notas; salva o conteúdo como Markdown via `turndown` |
| Viewer de PDF | pdf.js 5.5.207 | Mesma versão hoje em produção |

## Divergência de `pkd`

`pkd` usa Svelte 5 puro com Vite, sem SvelteKit. Aqui o briefing exige SvelteKit, então usa-se `@sveltejs/adapter-static` para chegar ao mesmo resultado final: uma pasta estática (`frontend/build`) embutida no binário Go via `go:embed`. A divergência de framework não muda a forma de empacotamento nem o resultado observável para o usuário.

## Saída de build e integração com `go:embed`

- `npm run build` gera `frontend/build` (ver árvore de diretórios completa em [Arquitetura](01-arquitetura.md)).
- No Dockerfile (ver [Docker, CI e Deploy](07-docker-ci-deploy.md)), `frontend/build` é copiado para `internal/server/web/dist`.
- `internal/server/server.go` embute essa pasta inteira com `//go:embed all:web/dist`.
- `http.FileServer` serve os arquivos estáticos, com fallback para `index.html` em qualquer rota que não comece por `/api` — necessário porque o roteamento das telas é feito no cliente pela SPA (`fallback: 'index.html'` do `adapter-static`).

## Rotas da SPA

Lista fechada — nenhuma rota além destas onze:

| Rota | Tela |
|---|---|
| `/` | Biblioteca |
| `/pdf/[id]` | Detalhes do PDF |
| `/viewer/[id]` | Viewer |
| `/highlights` | Visão geral de destaques |
| `/comments` | Visão geral de comentários |
| `/collections` | Coleções |
| `/tags` | Tags |
| `/settings` | Preferências |
| `/admin` | Administração |
| `/login` | Login |
| `/s/[share]` | Viewer público (somente leitura) |

## Rolagem infinita

Não existe paginação numerada em lugar nenhum da SPA — item removido do produto atual (ver [Inventário de Funcionalidades](10-inventario-funcionalidades.md)). Padrão único, usado em toda lista (biblioteca, destaques, comentários, etc.):

- Uma sentinela ao final da lista, observada por `IntersectionObserver`.
- Ao entrar em viewport, dispara `GET ...?cursor=<último id>&limit=50` contra o endpoint correspondente (ver parâmetros de paginação em [API](05-api.md)).
- **Formato do cursor**: `{created_at}|{id}` do último item da página atual, codificado em base64url.
- **Comparação no servidor**: `(created_at, id) < (?, ?)` — estável sob inserção concorrente durante a rolagem, ao contrário de `OFFSET`, que pode repetir ou pular itens quando novos registros são inseridos entre uma página e outra.

## Tema claro/escuro

- Toggle na barra superior da SPA, com três estados: `system` | `light` | `dark`.
- Persistido no servidor em `settings['ui.theme']` (ver [Modelo de dados](02-modelo-de-dados.md)) e espelhado em `localStorage` para evitar flash de tema incorreto durante o carregamento.
- Aplicado via atributo `data-theme` em `<html>`, definido por um script inline em `frontend/src/app.html`, executado antes da hidratação do Svelte, lendo o valor espelhado em `localStorage`.
- **Nenhuma cor de tema é configurável** — existe uma única paleta, definida em `frontend/tailwind.config.ts`. Elimina os campos `custom_theme_color`/`custom_theme_color_secondary`/`theme_color` do `Profile` Django atual, além da variável de ambiente de cor de tema padrão do produto hoje, cuja eliminação está documentada em [Docker, CI e Deploy](07-docker-ci-deploy.md).

## PWA e service worker

- Manifest em `frontend/static/manifest.webmanifest`: `display: standalone`, ícones 192×192 e 512×512 `maskable`.
- Service worker próprio em `frontend/static/sw.js`, com as mesmas estratégias de cache de `pkd`:

| Recurso | Estratégia de cache |
|---|---|
| `index.html` | Network-first |
| `GET /api/pdfs` | Network-first, com cache de fallback para uso offline |
| Assets estáticos (JS, CSS, ícones, pdf.js) | Cache-first |
| Mutações (`POST` / `PATCH` / `PUT` / `DELETE`) | Network-only — responde `503` quando offline |

- Dois caches versionados, permitindo invalidação independente do shell da aplicação e das respostas de API a cada deploy.
- **Timeout de rede fixo em 8s** (`AbortController`) em toda chamada de `fetch` feita pelo service worker — tanto no `network-first` quanto no `network-only` das mutações. Sem isso, uma conexão que trava (não falha, só nunca resolve) deixa a página pendurada indefinidamente em vez de cair no fallback de cache/offline; o abort força a rejeição da promise mesmo quando o `fetch` está preso na camada de rede/navegador, não só numa resposta lenta. Bug real encontrado e corrigido em produção — reproduzido com login nunca completando enquanto o SW estava sem esse guard.

## Upload — processamento no navegador

Componente de upload que, **antes** de enviar o `POST`, executa pdf.js no navegador para:

1. Contar o número de páginas do PDF.
2. Renderizar a página 1 em `<canvas>` e exportar dois PNGs: 400px de largura (thumbnail) e 1000px de largura (preview).
3. Concatenar o texto de todas as páginas via `getTextContent()` do pdf.js.

O texto extraído é limitado a **2 MB**. Os três artefatos derivados (thumbnail PNG, preview PNG, texto) mais o PDF original são enviados num único `multipart/form-data` para `POST /api/pdfs` (payload completo em [API](05-api.md)).

**Degradação graciosa**: se o pdf.js falhar no navegador (PDF corrompido ou protegido por senha), o componente envia somente o arquivo original — sem thumbnail e sem texto — e o servidor aceita o upload mesmo assim, gravando o PDF sem os artefatos derivados.

## Busca e filtro por tag na biblioteca

A rota `/` traz, acima da grade de PDFs, uma caixa de busca única (léxica + semântica, mesma caixa da [Busca híbrida](04-busca-hibrida.md) — sem seletor de modo) e a lista completa de tags (`GET /api/tags`) como pílulas clicáveis.

- **Busca**: `input` com debounce de 300ms; a cada digitação, reinicia o timer e só dispara `GET /api/pdfs?q=<termo>` quando o usuário para de digitar, evitando uma requisição por tecla.
- **Tags**: cada pílula mostra o nome e a contagem de PDFs (`TagWithCount.count`). Clicar seleciona a tag como filtro (`?tag=<nome>`); clicar de novo na mesma pílula remove o filtro. Seleção única — não há combinação de múltiplas tags no filtro da biblioteca.
- Busca e filtro de tag combinam-se livremente com os demais filtros da listagem (coleção, favoritos, arquivados) — mesma regra de composição descrita em [Busca híbrida](04-busca-hibrida.md), "Filtros combinados com a busca".
- Mensagem vazia diferenciada: "Nenhum PDF ainda. Envie um acima." só aparece sem nenhum filtro ativo; com busca ou tag ativos e zero resultados, a mensagem passa a "Nenhum PDF encontrado.".

## TagPicker — combobox de tags

Componente `frontend/src/lib/components/tag-picker.svelte`, usado no campo Tags da página de detalhes (`/pdf/[id]`) para substituir a digitação livre por um combobox com sugestão:

1. Busca a lista completa de tags (`GET /api/tags`) uma vez, ao montar.
2. Enquanto o usuário digita, filtra as tags existentes por substring (case-insensitive), excluindo as já selecionadas, mostrando até 8 sugestões num dropdown.
3. Se o texto digitado não bate exatamente com nenhuma tag existente, o dropdown inclui a opção "Criar tag "<texto>"".
4. Clicar numa sugestão, clicar em "Criar tag", ou pressionar `Enter`/`,` com texto digitado adiciona a tag como um chip removível (botão "×" em cada chip) e limpa o campo de digitação.
5. Toda adição/remoção salva imediatamente via `PATCH /api/pdfs/{id}` (campo `tags`) — não há botão "Salvar" separado nem necessidade de perder o foco do campo.

Criar uma tag "nova" pelo combobox não chama nenhum endpoint extra de criação: o backend já cria qualquer tag inexistente ao processar o `PATCH` (`ensureTags`, ver [Modelo de dados](02-modelo-de-dados.md), "Regra de normalização de tags") — o combobox só decide *o que sugerir*, nunca *como persistir*.

## Viewer — ponte `postMessage`

pdf.js é embutido como asset estático em `frontend/static/pdfjs/`: `viewer.html`/`viewer.mjs`/`viewer.css` são código próprio deste repositório (não o webapp pré-compilado oficial — este não expõe pontos de extensão para a ponte `postMessage` abaixo sem patch); os arquivos do motor (`pdf.mjs`, `pdf.worker.mjs`, `pdf_viewer.mjs`/`.css`, `cmaps/`, `standard_fonts/`) vêm do pacote npm `pdfjs-dist` (mesma versão 5.5.207 já fixada em `frontend/package.json`), copiados por `frontend/scripts/copy-pdfjs.mjs` a cada `npm run build`/`npm run dev` — nunca versionados (ver estágio de build em [Docker, CI e Deploy](07-docker-ci-deploy.md)). O resultado é aberto dentro de um `<iframe>` pela rota `/viewer/[id]`. A comunicação entre a SPA e o iframe do viewer é feita por `postMessage`:

| Ação | Gatilho | Efeito |
|---|---|---|
| Salvar página atual | Mudança de página no viewer, com debounce de 2 s | `PATCH /api/pdfs/{id}` com `current_page` |
| Criar comentário | Ação do usuário no viewer | `POST /api/pdfs/{id}/annotations` com `kind: "comment"` |
| Criar destaque | Seleção de texto no viewer | `POST /api/pdfs/{id}/annotations` com `kind: "highlight"` |
| Aplicar assinatura | Usuário escolhe assinatura salva | Insere a imagem da assinatura (data URL PNG) na página atual do viewer |
| Modo invertido | Toggle na SPA | Aplica inversão de cores ao conteúdo renderizado pelo viewer |
| Manter tela ligada | Toggle na SPA | Solicita `navigator.wakeLock` a partir do contexto que hospeda o iframe |

No modo compartilhado (`/s/[share]`), a ponte é montada em **modo somente-leitura**: nenhuma das ações acima que grava estado (salvar página, criar anotação, aplicar assinatura) fica disponível.

### Backfill de texto na abertura

Além da ponte `postMessage`, o host do viewer roda um passo independente ao carregar: se `GET /api/pdfs/{id}` devolver `has_text: false`, o host busca o próprio arquivo (`/api/pdfs/{id}/file`), extrai o texto com o mesmo pdf.js do fluxo de upload (`extractTextFromUrl`, reaproveitando o laço de `processPDF` — ver [Upload — processamento no navegador](#upload--processamento-no-navegador)) e envia via `POST /api/pdfs/{id}/text` (contrato em [API](05-api.md); mecânica de reindexação em [Busca híbrida](04-busca-hibrida.md)). Melhor-esforço, silencioso em qualquer falha (PDF corrompido/protegido, ou a própria requisição) — nunca bloqueia nem atrasa a abertura do viewer, e uma tentativa que falha simplesmente é repetida na próxima abertura do mesmo documento.

## Botão de embedding — máquina de estados

Um botão por documento, presente em dois lugares: no card da biblioteca (ícone `bx-brain` do Boxicons, sem rótulo) e na página de detalhes `/pdf/[id]` (ícone + rótulo). O estado é dirigido **exclusivamente** pelo campo derivado `embedding_status`, devolvido por `GET /api/pdfs` e `GET /api/pdfs/{id}` (cálculo em [Busca híbrida](04-busca-hibrida.md), contrato em [API](05-api.md)) — nunca por estado local inferido no frontend.

### Estados persistidos (`embedding_status`)

| `embedding_status` | Botão habilitado? | Rótulo | Tooltip |
|---|---|---|---|
| `none` | Sim | "Embedar" | — |
| `current` | **Não** | "Embedado" | Data de `pdf_embeddings.created_at` |
| `stale` | Sim | "Reembedar" | "o conteúdo mudou desde o último embedding" |

### Estado transitório (durante a requisição)

Enquanto `POST /api/pdfs/{id}/embed` está em curso, o botão entra em estado de carregamento e fica desabilitado, independentemente do estado persistido que o antecedeu.

### Tratamento da resposta

| Resposta HTTP | Comportamento da UI |
|---|---|
| `200` | Atualiza o estado local do documento para `current`, sem recarregar a lista |
| `412` (`GEMINI_API_KEY` ausente) | Mostra aviso persistente de que a busca semântica não está configurada; desabilita o botão em **todos** os documentos da sessão |
| `422` (documento sem texto extraído) | Mostra toast com a mensagem de erro; deixa o botão habilitado para nova tentativa |
| `502` (falha da API Gemini) | Mostra toast com a mensagem de erro; deixa o botão habilitado para nova tentativa |

`404` e `409` (ver [API](05-api.md)) não deveriam ocorrer a partir de um clique legítimo — o botão só fica clicável quando `embedding_status` é `none` ou `stale`, e o estado de carregamento impede um segundo clique no mesmo documento enquanto o primeiro está em curso.

**Não existe ação de "embedar todos"** — nem na biblioteca, nem na tela de administração (`/admin`). Embedar é sempre uma ação de um documento por vez, disparada por um clique explícito (decisão 5 em [Visão geral](00-visao-geral.md)).

## Paridade de UI

Tabela de mapeamento entre cada rota nova da SPA e as telas/views do produto atual. O inventário completo de funcionalidades — com a coluna "Etapa" e a lista de funcionalidades intencionalmente removidas — vive em [Inventário de Funcionalidades](10-inventario-funcionalidades.md); esta tabela cobre apenas a camada de apresentação (rota → componente → view antiga → funcionalidades da tela), sem repetir aquele conteúdo.

| Rota nova | Arquivo de rota (SvelteKit) | View(s)/URL(s) Django equivalente(s) | Funcionalidades da tela |
|---|---|---|---|
| `/` | `frontend/src/routes/+page.svelte` | `pdf_overview` (`Overview`), `pdf_overview_query` (`OverviewQuery`), `get_next_pdf_overview_page`, `add_pdf` (`Add`), `bulk_add_pdfs` (`BulkAdd`), `bulk_actions` (`BulkActions`), `star` (`Star`), `archive` (`Archive`), `serve_thumbnail` (`ServeThumbnail`) | Biblioteca com 4 layouts (compact/list/grid/minimal), 7 ordenações, caixa de busca híbrida com debounce (ver [Busca e filtro por tag na biblioteca](#busca-e-filtro-por-tag-na-biblioteca)), pílulas de tag clicáveis para filtro, modo árvore de tags, upload individual e em lote, estrela, arquivar, ações em lote, rolagem infinita (substitui a paginação numerada) |
| `/pdf/[id]` | `frontend/src/routes/pdf/[id]/+page.svelte` | `pdf_details` (`Details`), `edit_pdf` (`Edit`), `get_notes` (`GetNotes`), `show_preview` (`ShowPreview`), `serve_preview` (`ServePreview`), `download_pdf` (`Download`), `delete_pdf` (`Delete`), `share_pdf`/`unshare_pdf` (`SharePdf`/`UnsharePdf`) | Detalhes, notas em Markdown sanitizado (editadas em TipTap), descrição, renomear, mover de coleção, subdiretórios, baixar, excluir, criar/revogar link de compartilhamento, botão de embedding, tags via combobox `TagPicker` (ver [TagPicker — combobox de tags](#tagpicker--combobox-de-tags)) |
| `/viewer/[id]` | `frontend/src/routes/viewer/[id]/+page.svelte` | `view_pdf` (`ViewerView`), `serve_pdf` (`Serve`), `update_page` (`UpdatePage`), `update_pdf` (`UpdatePdf`) | Viewer pdf.js, progresso de leitura e página atual, barras de progresso, salvar PDF editado com incremento de `revision`, criar comentário/destaque, aplicar assinatura, modo invertido, manter tela ligada |
| `/highlights` | `frontend/src/routes/highlights/+page.svelte` | `pdf_highlight_overview` (`HighlightOverview`), `pdf_details_highlight_overview` (`DetailsHighlightOverview`), `export_annotations` (`ExportAnnotations`) | Visão geral de destaques com rolagem infinita, visão de destaques por PDF, exportação YAML/JSON, ordenação de anotações |
| `/comments` | `frontend/src/routes/comments/+page.svelte` | `pdf_comment_overview` (`CommentOverview`), `pdf_details_comment_overview` (`DetailsCommentOverview`), `export_annotations` (`ExportAnnotations`) | Visão geral de comentários com rolagem infinita, visão de comentários por PDF, exportação YAML/JSON, ordenação de anotações |
| `/collections` | `frontend/src/routes/collections/+page.svelte` | `create_collection` (`collection_views.Create`), `collection_details` (`workspace_views.CollectionDetails`), `edit_collection` (`collection_views.Edit`), `delete_collection` (`collection_views.Delete`) | CRUD de coleções; contagem de PDFs por coleção (`pdf_count`, ver [API](05-api.md)); coleção padrão protegida contra exclusão |
| `/tags` | `frontend/src/routes/tags/+page.svelte` | `edit_tag` (`EditTag`), `delete_tag` (`DeleteTag`), `admin/views.py:RenameTag`, `:SubstituteTag` | Criar/editar/excluir tag, renomear e fundir tags (administração de tags) |
| `/settings` | `frontend/src/routes/settings/+page.svelte` | Campos de `Profile` (`users/models.py`), `users/views.py:Signatures` | Tema claro/escuro/sistema, layout padrão, ordenação padrão de PDFs e de anotações, modo invertido, manter tela ligada, barras de progresso, modo árvore de tags, assinaturas (chaves em [Modelo de dados](02-modelo-de-dados.md)) |
| `/admin` | `frontend/src/routes/admin/+page.svelte` | `admin/views.py:Information`, `:RevokeShare` | Informações da instância (contagens, espaço em disco, agregado de `embedding_status`), revogação de compartilhamento, reindexação do FTS5 (`POST /api/admin/reindex`) — substitui `regenerate_thumbnails.py`, já que a geração de thumbnail passa a ocorrer no navegador |
| `/login` | `frontend/src/routes/login/+page.svelte` | Login do `django-allauth` | Autenticação por senha única (`ADMIN_PASSWORD`), substitui o login multiusuário |
| `/s/[share]` | `frontend/src/routes/s/[share]/+page.svelte` | `view_shared_pdf` (`ViewSharedPdf`), `serve_shared_pdf` (`ServeSharedPdf`) | Viewer público somente-leitura, contador de visualizações do compartilhamento |

Itens do inventário sem tela própria na SPA — consumo por watch-dir, `/healthz` — não têm interface dedicada e estão documentados em [Storage](03-storage.md) e em [Inventário de Funcionalidades](10-inventario-funcionalidades.md), não repetidos aqui.
