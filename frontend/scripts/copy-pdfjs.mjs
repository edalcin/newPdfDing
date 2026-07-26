#!/usr/bin/env node
// Copia os assets de runtime do pdf.js de node_modules/pdfjs-dist para
// static/pdfjs/ antes de dev/build — SvelteKit serve frontend/static/* na
// raiz do site, então static/pdfjs/pdf.mjs vira /pdfjs/pdf.mjs (ver
// refatoracao/06-frontend.md, "Viewer — ponte postMessage"). Saída gerada:
// nunca commitada (ver frontend/.gitignore).

import { cpSync, mkdirSync, readFileSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const src = join(frontendRoot, 'node_modules', 'pdfjs-dist');
const dest = join(frontendRoot, 'static', 'pdfjs');

// Arquivos achatados para a raiz de static/pdfjs/ — viewer.mjs importa
// './pdf.mjs' e './pdf.worker.mjs' como caminhos relativos simples.
const FILES = ['build/pdf.mjs', 'build/pdf.worker.mjs', 'web/pdf_viewer.mjs', 'web/pdf_viewer.css'];
// Diretórios copiados recursivamente, preservando o nome (necessários para
// texto não-latino e fallback de fontes incorporadas).
const DIRS = ['cmaps', 'standard_fonts'];

mkdirSync(dest, { recursive: true });
for (const file of FILES) cpSync(join(src, file), join(dest, basename(file)));
for (const dir of DIRS) cpSync(join(src, dir), join(dest, dir), { recursive: true });

const { version } = JSON.parse(readFileSync(join(src, 'package.json'), 'utf8'));
console.log(`copy:pdfjs → pdfjs-dist@${version} copiado para ${dest}`);
