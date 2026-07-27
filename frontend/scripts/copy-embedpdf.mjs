#!/usr/bin/env node
// Copia o binário WASM do PDFium de node_modules para static/embedpdf/ antes
// de dev/build. O default do @embedpdf/snippet é buscá-lo em
// https://cdn.jsdelivr.net/npm/@embedpdf/pdfium@<v>/dist/pdfium.wasm, o que a
// CSP do projeto (connect-src 'self') bloqueia — por isso servimos do próprio
// domínio e apontamos PDFViewerConfig.wasmUrl para cá. Saída gerada: nunca
// commitada (ver frontend/.gitignore).

import { cpSync, mkdirSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const src = join(frontendRoot, 'node_modules', '@embedpdf', 'pdfium');
const dest = join(frontendRoot, 'static', 'embedpdf');

mkdirSync(dest, { recursive: true });
cpSync(join(src, 'dist', 'pdfium.wasm'), join(dest, 'pdfium.wasm'));

const { version } = JSON.parse(readFileSync(join(src, 'package.json'), 'utf8'));
console.log(`copy:embedpdf → @embedpdf/pdfium@${version} copiado para ${dest}`);
