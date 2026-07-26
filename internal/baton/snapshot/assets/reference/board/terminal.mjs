#!/usr/bin/env node

import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

const BOARD_VERSION = 'baton.board/v1';
const ANSI_PATTERN = /\u001b(?:\[[0-?]*[ -/]*[@-~]|\][^\u0007]*(?:\u0007|\u001b\\)?|[ -/]*[@-~]?)/g;
const UNSAFE_CONTROL_PATTERN = /[\u0000-\u000c\u000e-\u001f\u007f-\u009f]/g;
const BIDI_PATTERN = /[\u061c\u200e\u200f\u202a-\u202e\u2066-\u2069]/g;
const MULTILINE_PATTERN = /[\r\n\u2028\u2029]+/g;
const MAX_TEXT = 240;

const COLORS = Object.freeze({
  dim: '\u001b[2m',
  red: '\u001b[31m',
  green: '\u001b[32m',
  yellow: '\u001b[33m',
  cyan: '\u001b[36m',
  reset: '\u001b[0m',
});

export function sanitizeTerminalText(value, max = MAX_TEXT) {
  const rendered = typeof value === 'string' ? value : String(value ?? '');
  return rendered
    .replace(ANSI_PATTERN, '')
    .replace(MULTILINE_PATTERN, ' \u21a9 ')
    .replace(UNSAFE_CONTROL_PATTERN, '\ufffd')
    .replace(BIDI_PATTERN, '\ufffd')
    .slice(0, max);
}

function paint(enabled, color, value) {
  return enabled ? `${COLORS[color]}${value}${COLORS.reset}` : value;
}
function display(value, fallback = '\u2014') {
  if (value === null || value === undefined || value === '') return fallback;
  return sanitizeTerminalText(value);
}

function operationText(operation) {
  if (!operation) return null;
  const identity = [
    operation.release,
    operation.track,
    operation.work,
  ].filter((value) => value !== null).map(display).join(' / ');
  return `${display(operation.operation)} [${display(operation.scope)}] ${identity}`;
}

function stateColor(state) {
  if (state === 'complete' || state === 'composed') return 'green';
  if (state === 'invalid' || state === 'blocked') return 'red';
  if (state === 'ready' || state === 'merge_ready' || state === 'assembly_ready') return 'yellow';
  return 'cyan';
}

function renderDiagnostic(lines, item, color) {
  const scope = [item.release, item.track, item.work]
    .filter((value) => value !== null)
    .map(display)
    .join(' / ');
  const prefix = scope ? `${scope}: ` : '';
  lines.push(`  ${paint(color, 'red', '!')} ${display(item.code)} ${prefix}${display(item.message)}`);
}

function renderWork(lines, work, color) {
  const state = `${display(work.stage)} / ${display(work.status)} / ${display(work.next_role)}`;
  const attempt = work.attempt === undefined ? '' : `  attempt=${display(work.attempt)}`;
  lines.push(
    `    ${display(work.id)}  ${paint(color, stateColor(work.status), state)}${attempt}  outcome=${display(work.outcome)}`,
  );
  lines.push(
    `      source ${display(work.source?.mode)} ${display(work.source?.ref)} @ ${display(work.source?.head)}`,
  );
  if (work.depends_on?.length > 0) {
    lines.push(`      depends ${work.depends_on.map(display).join(', ')}`);
  }
  if (work.blocker) {
    lines.push(
      `      ${paint(color, 'red', 'blocked')} ${display(work.blocker.code)}: ${display(work.blocker.summary)}`,
    );
  }
  const next = operationText(work.next_operation);
  if (next) lines.push(`      ${paint(color, 'yellow', 'next')} ${next}`);
}

function renderTrack(lines, track, color) {
  const composed = paint(color, stateColor(track.composition), display(track.composition));
  lines.push(
    `  Track ${display(track.id)}  ${composed}  materialisation=${display(track.materialisation)}`,
  );
  lines.push(`    ref ${display(track.ref)} @ ${display(track.head)}`);
  if (track.frozen_head) lines.push(`    frozen ${display(track.frozen_head)}`);
  if (track.depends_on?.length > 0) {
    lines.push(`    depends ${track.depends_on.map(display).join(', ')}`);
  }
  if (track.blockers?.length > 0) {
    lines.push(`    ${paint(color, 'red', 'waiting for')} ${track.blockers.map(display).join(', ')}`);
  }
  for (const work of track.work ?? []) renderWork(lines, work, color);
  const next = operationText(track.next_operation);
  if (next && track.next_operation.scope !== 'work') {
    lines.push(`    ${paint(color, 'yellow', 'next')} ${next}`);
  }
}

function renderAssembly(lines, assembly, color) {
  if (!assembly) {
    lines.push(`  Assembly  ${paint(color, 'red', 'invalid')}`);
    return;
  }
  const state = `${display(assembly.stage)} / ${display(assembly.status)} / ${display(assembly.next_role)}`;
  lines.push(
    `  Assembly  ${paint(color, stateColor(assembly.status), state)}  outcome=${display(assembly.outcome)}`,
  );
  if (assembly.source) {
    lines.push(
      `    source ${display(assembly.source.mode)} ${display(assembly.source.ref)} @ ${display(assembly.source.head)}`,
    );
  }
  if (assembly.blocker) {
    lines.push(
      `    ${paint(color, 'red', 'blocked')} ${display(assembly.blocker.code)}: ${display(assembly.blocker.summary)}`,
    );
  }
  const next = operationText(assembly.next_operation);
  if (next) lines.push(`    ${paint(color, 'yellow', 'next')} ${next}`);
}

function colorEnabled(mode, isTTY) {
  if (!['auto', 'always', 'never'].includes(mode)) {
    throw new TypeError('color must be auto, always, or never');
  }
  return mode === 'always' || (mode === 'auto' && isTTY === true);
}
export function renderTerminal(board, { color = 'auto', isTTY = false } = {}) {
  if (!board || board.schema_version !== BOARD_VERSION || !Array.isArray(board.releases)) {
    throw new TypeError(`board must be ${BOARD_VERSION}`);
  }
  const useColor = colorEnabled(color, isTTY);
  const lines = [];
  const validity = board.valid ? 'valid' : 'invalid';
  lines.push(
    `Baton ${display(board.schema_version)}  ${display(board.repository, 'no release')}  ${
      paint(useColor, stateColor(validity), validity)
    }`,
  );
  for (const item of board.diagnostics ?? []) renderDiagnostic(lines, item, useColor);
  if (board.releases.length === 0) {
    lines.push(paint(useColor, 'dim', 'No local refs/heads/release-wt/* releases.'));
  }
  for (const release of board.releases) {
    lines.push('');
    lines.push(
      `Release ${display(release.release)}  ${
        paint(useColor, stateColor(release.status), display(release.status))
      }`,
    );
    const revision = release.plan_revision === undefined ? '' : ` r${display(release.plan_revision)}`;
    lines.push(`  plan${revision} ${display(release.plan_digest)}`);
    lines.push(`  release ${display(release.release_ref)} @ ${display(release.release_head)}`);
    lines.push(`  target  ${display(release.target_ref)} @ ${display(release.target_head)}`);
    for (const item of release.diagnostics ?? []) renderDiagnostic(lines, item, useColor);
    for (const track of release.tracks ?? []) renderTrack(lines, track, useColor);
    renderAssembly(lines, release.assembly, useColor);
  }
  if ((board.next_operations ?? []).length > 0) {
    lines.push('');
    lines.push('Next operations');
    for (const operation of board.next_operations) {
      lines.push(`  ${operationText(operation)}`);
    }
  }
  return `${lines.join('\n')}\n`;
}

function usage() {
  return 'Usage: node reference/board/terminal.mjs [--color auto|always|never] < board.json\n';
}
function main(argv) {
  let color = 'auto';
  if (argv.length === 2 && argv[0] === '--color') {
    color = argv[1];
  } else if (argv.length !== 0) {
    process.stderr.write(usage());
    process.exitCode = 64;
    return;
  }
  try {
    const board = JSON.parse(readFileSync(0, 'utf8'));
    process.stdout.write(renderTerminal(board, { color, isTTY: Boolean(process.stdout.isTTY) }));
    process.exitCode = board.valid === false ? 2 : 0;
  } catch (error) {
    process.stderr.write(`${sanitizeTerminalText(error?.message ?? 'invalid board')}\n`);
    process.exitCode = error instanceof TypeError ? 64 : 2;
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2));
}
