# Refatoração newPdfDing — índice

Esta pasta contém o **plano de documentação** da refatoração completa do newPdfDing, do stack atual (Django + HTMX) para Go + SvelteKit, espelhando a arquitetura de `pkd`. Nenhum código de produção é alterado aqui — todos os arquivos abaixo são planejamento e especificação, a serem executados etapa por etapa conforme [ETAPAS.md](ETAPAS.md). Nada fora de `refatoracao/` é tocado por este material.

## Documentos

| Arquivo | O que responde | Quando consultar |
|---|---|---|
| [00-visao-geral.md](00-visao-geral.md) | Objetivos do projeto, princípios de desenvolvimento (versionamento, frontend, dados, Docker, segurança), as 11 decisões arquiteturais fechadas, tabela stack antigo→novo, e o que muda com a eliminação do multiusuário | Antes de iniciar qualquer etapa, para entender o "porquê" por trás das escolhas |
| [01-arquitetura.md](01-arquitetura.md) | Árvore de diretórios alvo, regra de dependência entre camadas, assinatura da interface `storage.Backend`, configuração obrigatória do SQLite, mecanismo de migração de schema, e a tabela de dependências Go fixadas | Ao estruturar o repositório Go (ETAPA-1) ou ao decidir onde um novo arquivo/pacote deve viver |
| [02-modelo-de-dados.md](02-modelo-de-dados.md) | O `schema.sql` completo e final, o mapeamento de cada modelo Django para a tabela nova, as chaves de `settings` com seus defaults e valores válidos, e a regra de normalização de tags | Ao criar ou alterar o schema do banco, ou ao mapear um campo do Django antigo |
| [03-storage.md](03-storage.md) | Esquema de chaves de arquivo no filesystem, a interface `storage.Backend`/`Seeker`, a implementação `LocalBackend`, e tratamento de falhas de disco | Ao implementar `internal/storage` (ETAPA-3) |
| [04-busca-hibrida.md](04-busca-hibrida.md) | Índice FTS5, geração de embeddings sob demanda via API Gemini, formato do BLOB vetorial, fórmula de fusão RRF, e os três estados de `embedding_status` (`none`/`current`/`stale`) | Ao implementar a busca (ETAPA-6) ou o botão de embedding sob demanda |
| [05-api.md](05-api.md) | Contrato REST completo: toda rota, método, payload, resposta e status de erro possível | Ao implementar qualquer handler HTTP ou ao consumir a API pelo frontend |
| [06-frontend.md](06-frontend.md) | Stack SvelteKit fixado, rotas da SPA, padrão de rolagem infinita por cursor, tema claro/escuro, PWA, fluxo de upload com pdf.js no navegador, ponte do viewer, e o comportamento do botão de embedding na UI | Ao implementar qualquer tela ou componente do frontend (ETAPA-9, ETAPA-10) |
| [07-docker-ci-deploy.md](07-docker-ci-deploy.md) | Dockerfile de 3 estágios, workflow de CI/CD, Dependabot, lista fechada de variáveis de ambiente (incluindo as eliminadas), `.env.example` e instruções de deploy no Unraid | Ao empacotar a imagem Docker, configurar CI (ETAPA-11), ou definir/ler uma variável de ambiente |
| [08-seguranca.md](08-seguranca.md) | Autenticação por senha única, rate limiting, CSRF, headers de segurança fixos, validação de entrada, e o checklist final de segurança | Ao implementar autenticação/sessão (ETAPA-2) ou antes da validação final (ETAPA-13) |
| [09-limpeza-repositorio.md](09-limpeza-repositorio.md) | Lista fechada e exaustiva do que é apagado do repositório, o que é reescrito, o que é preservado intacto, e os comandos de varredura para provar que não sobrou resíduo | Na primeira etapa da execução (ETAPA-0), antes de qualquer código novo |
| [10-inventario-funcionalidades.md](10-inventario-funcionalidades.md) | Tabela de paridade: cada funcionalidade hoje em produção, onde está implementada, para onde vai, e em qual etapa; mais a lista de funcionalidades intencionalmente removidas | Para garantir que nenhuma funcionalidade se perde, e como checklist final (ETAPA-13) |
| [11-desempenho-viewer.md](11-desempenho-viewer.md) | Estudo crítico do travamento do viewer em PDFs grandes: perfil real do acervo, as cinco causas (thread principal, replay por tile, duplo download, ausência de Range, teto de 2 GB), o que é resolvível por configuração e por que não trocar de SDK | Antes de mexer em desempenho do viewer, na CSP de worker, ou ao reavaliar o EmbedPDF |
| [ETAPAS.md](ETAPAS.md) | O contrato de execução: as 14 etapas nomeadas, cada uma com objetivo, entregas, dependências e critério de aceitação executável | Para saber qual etapa rodar a seguir, e o que ela precisa entregar para ser considerada concluída |

## Como executar

Cada etapa é invocada num **prompt novo**, pelo **nome exato** listado em [ETAPAS.md](ETAPAS.md) — por exemplo `ETAPA-3-STORAGE`. `ETAPAS.md` é o **contrato de execução**: a etapa só é considerada concluída quando seu critério de aceitação for demonstrado (comando executado, resultado observado). As etapas têm dependências explícitas entre si — respeitar a ordem declarada em cada bloco "Depende de". Todo commit vai direto para `main`; este projeto não usa branches.

## Estado atual

Stack em produção hoje versus o stack alvo desta refatoração:

| Camada | Hoje | Alvo |
|---|---|---|
| Backend | Django 5.2.14 | Go 1.25 + chi + `modernc.org/sqlite` |
| Banco de dados | SQLite ou PostgreSQL | Somente SQLite |
| Frontend | HTMX + templates Django | SvelteKit (SPA estática) |
| Processos em segundo plano | huey + supervisord | Uma goroutine com `time.Ticker` (consumo por watch-dir) |
| Servidor HTTP | gunicorn | `net/http` |
| Viewer de PDF | pdf.js 5.5.207 | pdf.js 5.5.207 |
