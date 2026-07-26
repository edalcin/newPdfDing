# Changelog

Todas as mudanças notáveis deste projeto são documentadas aqui.

## [Unreleased]

### Changed

- Início da refatoração completa do stack: backend Python legado + HTMX → Go + SvelteKit. Plano de execução em [`refatoracao/`](refatoracao/README.md).
- `ETAPA-0-LIMPEZA`: remoção integral do stack Python legado (aplicação web, empacotamento de dependências, runtime multi-processo antigo, frontend Node da raiz, artefatos `graphify-out/`). Ver [`refatoracao/09-limpeza-repositorio.md`](refatoracao/09-limpeza-repositorio.md).
- `ETAPA-1` a `ETAPA-12`: reescrita completa do backend (Go/chi/SQLite puro Go), frontend (SvelteKit embutido via `go:embed`), busca híbrida FTS5+embeddings/RRF, compartilhamento público, watch-dir de consumo, imagem Docker distroless de 3 estágios, CI/CD com Trivy+govulncheck+Dependabot, e comando único `-import-legacy` para o banco Django legado. Ver [`refatoracao/ETAPAS.md`](refatoracao/ETAPAS.md) para o detalhamento etapa a etapa.
- `ETAPA-13-VALIDACAO`: validação de ponta a ponta antes de encerrar a refatoração — varredura final de resíduos, checklist de segurança, tabela de paridade de funcionalidades e changelog. Ver [`refatoracao/08-seguranca.md`](refatoracao/08-seguranca.md) e [`refatoracao/10-inventario-funcionalidades.md`](refatoracao/10-inventario-funcionalidades.md).

### Fixed

- Dockerfile: `/data` e `/files` agora são criados e `chown`ados para o uid `65532` (nonroot) dentro da própria imagem, para que o comportamento de "populate on first mount" do Docker propague a permissão de escrita para volumes nomeados recém-criados. Sem isso, `docker run`/`docker compose up` com um volume nomeado novo falhava ao abrir o SQLite (`unable to open database file`) mesmo sem `--read-only` — bug pré-existente descoberto durante a `ETAPA-13-VALIDACAO` ao testar a recomendação `--read-only --cap-drop=ALL`.
- `README.md`, `compose.yaml` e `UNRAID.md` agora incluem a recomendação `--read-only --cap-drop=ALL` de [`refatoracao/08-seguranca.md`](refatoracao/08-seguranca.md#menor-privilégio), que estava documentada no plano mas ausente das instruções de deploy.
