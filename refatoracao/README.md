# Refatoração newPdfDing — índice

Esta pasta contém a **especificação do produto** newPdfDing, resultado da refatoração completa do stack antigo (Django + HTMX) para Go + SvelteKit, espelhando a arquitetura de `pkd`. A refatoração está **concluída** e o produto está em produção; os documentos abaixo descrevem o contrato de rotas, o modelo de dados, a busca híbrida, a segurança e o empacotamento tal como implementados, e são mantidos atualizados junto com o código. O nome da pasta (`refatoracao/`) é histórico e foi mantido de propósito, para não invalidar as dezenas de citações a `refatoracao/NN-*.md` que existem em comentários de código.

## Documentos

| Arquivo | O que responde | Quando consultar |
|---|---|---|
| [00-visao-geral.md](00-visao-geral.md) | Objetivos do projeto, princípios de desenvolvimento (versionamento, frontend, dados, Docker, segurança), as 11 decisões arquiteturais fechadas, tabela stack antigo→novo, e o que muda com a eliminação do multiusuário | Para entender o "porquê" por trás das escolhas arquiteturais do produto |
| [01-arquitetura.md](01-arquitetura.md) | Árvore de diretórios alvo, regra de dependência entre camadas, assinatura da interface `storage.Backend`, configuração obrigatória do SQLite, mecanismo de migração de schema, e a tabela de dependências Go fixadas | Ao estruturar o repositório Go ou ao decidir onde um novo arquivo/pacote deve viver |
| [02-modelo-de-dados.md](02-modelo-de-dados.md) | O `schema.sql` completo e final, o mapeamento de cada modelo Django para a tabela nova, as chaves de `settings` com seus defaults e valores válidos, e a regra de normalização de tags | Ao criar ou alterar o schema do banco, ou ao mapear um campo do Django antigo |
| [03-storage.md](03-storage.md) | Esquema de chaves de arquivo no filesystem, a interface `storage.Backend`/`Seeker`, a implementação `LocalBackend`, e tratamento de falhas de disco | Ao mexer em `internal/storage` |
| [04-busca-hibrida.md](04-busca-hibrida.md) | Índice FTS5, geração de embeddings sob demanda via API Gemini, formato do BLOB vetorial, fórmula de fusão RRF, e os três estados de `embedding_status` (`none`/`current`/`stale`) | Ao mexer na busca ou no botão de embedding sob demanda |
| [05-api.md](05-api.md) | Contrato REST completo: toda rota, método, payload, resposta e status de erro possível | Ao implementar qualquer handler HTTP ou ao consumir a API pelo frontend |
| [06-frontend.md](06-frontend.md) | Stack SvelteKit fixado, rotas da SPA, padrão de rolagem infinita por cursor, tema claro/escuro, PWA, fluxo de upload com pdf.js no navegador, ponte do viewer, e o comportamento do botão de embedding na UI | Ao mexer em qualquer tela ou componente do frontend |
| [07-docker-ci-deploy.md](07-docker-ci-deploy.md) | Dockerfile de 3 estágios, workflow de CI/CD, Dependabot, lista fechada de variáveis de ambiente (incluindo as eliminadas), `.env.example` e instruções de deploy no Unraid | Ao empacotar a imagem Docker, configurar CI, ou definir/ler uma variável de ambiente |
| [08-seguranca.md](08-seguranca.md) | Autenticação por senha única, rate limiting, CSRF, headers de segurança fixos, validação de entrada, e o checklist final de segurança | Antes de mexer em autenticação/sessão, ou como checklist de segurança |
| [10-inventario-funcionalidades.md](10-inventario-funcionalidades.md) | Tabela de paridade: cada funcionalidade hoje em produção, onde está implementada, para onde vai, e em qual etapa; mais a lista de funcionalidades intencionalmente removidas | Para garantir que nenhuma funcionalidade se perde |
| [11-desempenho-viewer.md](11-desempenho-viewer.md) | Estudo crítico do travamento do viewer em PDFs grandes: perfil real do acervo, as cinco causas (thread principal, replay por tile, duplo download, ausência de Range, teto de 2 GB), o que é resolvível por configuração e por que não trocar de SDK | Antes de mexer em desempenho do viewer, na CSP de worker, ou ao reavaliar o EmbedPDF |

## Estado atual

A refatoração está **concluída**: o produto roda em produção (self-hosted, deploy no Unraid via `compose.yaml`) na stack Go + SvelteKit. A tabela abaixo preserva, como histórico, a comparação com o stack Django anterior:

| Camada | Stack antigo (Django) | Stack atual (produção) |
|---|---|---|
| Backend | Django 5.2.14 | Go 1.27 + chi + `modernc.org/sqlite` |
| Banco de dados | SQLite ou PostgreSQL | Somente SQLite |
| Frontend | HTMX + templates Django | SvelteKit (SPA estática) |
| Processos em segundo plano | huey + supervisord | Uma goroutine com `time.Ticker` (consumo por watch-dir) |
| Servidor HTTP | gunicorn | `net/http` |
| Viewer de PDF | pdf.js 5.5.207 | pdf.js 5.5.207 |

## Documentos apagados

Três documentos de plano puro (`ETAPAS.md`, `09-limpeza-repositorio.md`, `12-achatamento-cartografia-vetorial.md`) e o runbook `instrucoesFinais.md` foram apagados depois da conclusão da refatoração, por decisão do usuário. O histórico deles continua disponível no git.
