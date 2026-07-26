#!/usr/bin/env node
// Substitui o placeholder __BUILD_VERSION__ em build/sw.js por um hash
// sha256 (12 hex) do conteúdo inteiro de build/ (exceto o próprio sw.js) —
// qualquer mudança em qualquer asset estático servido (viewer.mjs,
// viewer.html, bundle da SPA, ícones, manifest...) muda o hash e força os
// dois caches do service worker (SHELL_CACHE/API_CACHE) a serem recriados
// no próximo deploy. Roda depois de `vite build` (ver package.json), nunca
// toca frontend/static/sw.js — só o artefato gerado em build/.

import { createHash } from 'node:crypto';
import { readFileSync, readdirSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const buildDir = join(frontendRoot, 'build');
const swPath = join(buildDir, 'sw.js');
// SvelteKit embute Date.now() aqui a cada build (ver _app/version.json) —
// não reflete conteúdo de asset nenhum, então fica fora do hash para não
// invalidar o cache do SW em todo deploy que não tocou nada servido.
const versionJsonPath = join(buildDir, '_app', 'version.json');

function collectFiles(dir) {
	const out = [];
	for (const name of readdirSync(dir).sort()) {
		const full = join(dir, name);
		const st = statSync(full);
		if (st.isDirectory()) out.push(...collectFiles(full));
		else out.push(full);
	}
	return out;
}

const hash = createHash('sha256');
for (const file of collectFiles(buildDir)) {
	if (file === swPath || file === versionJsonPath) continue;
	hash.update(relative(buildDir, file));
	hash.update(readFileSync(file));
}
const version = hash.digest('hex').slice(0, 12);

const sw = readFileSync(swPath, 'utf8');
const updated = sw.replace('__BUILD_VERSION__', version);
if (updated === sw) {
	console.error('stamp:sw — placeholder __BUILD_VERSION__ não encontrado em build/sw.js');
	process.exit(1);
}
writeFileSync(swPath, updated);
console.log(`stamp:sw → VERSION = ${version}`);
