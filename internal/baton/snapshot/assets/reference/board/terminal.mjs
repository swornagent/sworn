#!/usr/bin/env node

import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

import {
  DIAGNOSTIC_LABELS,
  OPERATION_LABELS,
  ROLE_LABELS,
  STATE_LABELS,
} from './presentation.mjs';

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

function tokenLabel(value) {
  if (value === null || value === undefined || value === '') return '\u2014';
  return STATE_LABELS[value] ?? display(value).replaceAll('_', ' ');
}

function roleLabel(value) {
  return ROLE_LABELS[value] ?? tokenLabel(value);
}

function operationLabel(value) {
  return OPERATION_LABELS[value] ?? tokenLabel(value);
}

function operationText(operation) {
  if (!operation) return null;
  const identity = [
    operation.release,
    operation.track,
    operation.work,
  ].filter((value) => value !== null).map(display).join(' / ');
  return `${operationLabel(operation.operation)} \u2014 ${identity} (${display(operation.operation)})`;
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
  const explanation = DIAGNOSTIC_LABELS[item.code] ?? display(item.message);
  lines.push(`  ${paint(color, item.code === 'TRACK_REF_ABSENT' ? 'cyan' : 'red', '!')} ${explanation}`);
  const original = DIAGNOSTIC_LABELS[item.code] ? ` \u2014 ${display(item.message)}` : '';
  lines.push(
    `    Technical details: ${display(item.code)}${scope ? ` \u00b7 ${scope}` : ''}${original}`,
  );
}

function renderWork(lines, work, color) {
  const headline = work.blocker
    ? 'Needs a decision'
    : work.status === 'ready' && work.next_role
      ? `Ready for ${roleLabel(work.next_role)}`
      : tokenLabel(work.status);
  const attempt = work.attempt === undefined ? '' : ` \u00b7 attempt ${display(work.attempt)}`;
  lines.push(
    `    ${display(work.id)} \u2014 ${paint(color, stateColor(work.status), headline)}${attempt}`,
  );
  if (work.depends_on?.length > 0) {
    lines.push(`      Follows: ${work.depends_on.map(display).join(', ')}`);
  }
  if (work.blocker) {
    lines.push(
      `      ${paint(color, 'red', 'What happened:')} ${display(work.blocker.summary)}`,
    );
    lines.push(`      Technical details: ${display(work.blocker.code)}`);
  }
  const next = operationText(work.next_operation);
  if (next) lines.push(`      ${paint(color, 'yellow', 'Next:')} ${next}`);
  lines.push(
    `      Technical details: stage=${display(work.stage)} \u00b7 status=${display(work.status)} \u00b7 next_role=${display(work.next_role)} \u00b7 outcome=${display(work.outcome)}`,
  );
  lines.push(
    `      Source: ${display(work.source?.mode)} ${display(work.source?.ref)} @ ${display(work.source?.head)}`,
  );
}

function renderTrack(lines, track, color) {
  const composed = paint(color, stateColor(track.composition), tokenLabel(track.composition));
  lines.push(
    `  Track ${display(track.id)} \u2014 ${composed}`,
  );
  if (track.depends_on?.length > 0) {
    lines.push(`    Follows: ${track.depends_on.map(display).join(', ')}`);
  }
  if (track.blockers?.length > 0) {
    lines.push(`    ${paint(color, 'yellow', 'Waiting for:')} ${track.blockers.map(display).join(', ')}`);
  }
  for (const work of track.work ?? []) renderWork(lines, work, color);
  const next = operationText(track.next_operation);
  if (next && track.next_operation.scope !== 'work') {
    lines.push(`    ${paint(color, 'yellow', 'Next:')} ${next}`);
  }
  lines.push(
    `    Technical details: materialisation=${display(track.materialisation)} \u00b7 ref=${display(track.ref)} @ ${display(track.head)}`,
  );
  if (track.frozen_head) lines.push(`    Frozen head: ${display(track.frozen_head)}`);
}

function renderAssembly(lines, assembly, color) {
  if (!assembly) {
    lines.push(`  Complete release \u2014 ${paint(color, 'red', 'State unavailable')}`);
    return;
  }
  const state = assembly.status === 'ready' && assembly.next_role
    ? `Ready for ${roleLabel(assembly.next_role)}`
    : tokenLabel(assembly.status);
  lines.push(
    `  Complete release \u2014 ${paint(color, stateColor(assembly.status), state)}`,
  );
  if (assembly.blocker) {
    lines.push(
      `    ${paint(color, 'red', 'What happened:')} ${display(assembly.blocker.summary)}`,
    );
    lines.push(`    Technical details: ${display(assembly.blocker.code)}`);
  }
  const next = operationText(assembly.next_operation);
  if (next) lines.push(`    ${paint(color, 'yellow', 'Next:')} ${next}`);
  lines.push(
    `    Technical details: stage=${display(assembly.stage)} \u00b7 status=${display(assembly.status)} \u00b7 next_role=${display(assembly.next_role)} \u00b7 outcome=${display(assembly.outcome)}`,
  );
  if (assembly.source) {
    lines.push(
      `    Source: ${display(assembly.source.mode)} ${display(assembly.source.ref)} @ ${display(assembly.source.head)}`,
    );
  }
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
  lines.push(`Baton board \u2014 ${paint(useColor, stateColor(validity), tokenLabel(validity))}`);
  lines.push(`Repository: ${display(board.repository, 'No active release')}`);
  for (const item of board.diagnostics ?? []) renderDiagnostic(lines, item, useColor);
  if (board.releases.length === 0) {
    lines.push(paint(useColor, 'dim', 'No active Baton release was found.'));
    lines.push('Next: ask an agent to start with baton-plan.');
  }
  for (const release of board.releases) {
    lines.push('');
    lines.push(
      `Release ${display(release.release)} \u2014 ${
        paint(useColor, stateColor(release.status), tokenLabel(release.status))
      }`,
    );
    for (const item of release.diagnostics ?? []) {
      if (item.code !== 'TRACK_REF_ABSENT') renderDiagnostic(lines, item, useColor);
    }
    for (const track of release.tracks ?? []) renderTrack(lines, track, useColor);
    renderAssembly(lines, release.assembly, useColor);
    lines.push('  Release technical details');
    lines.push(
      `    Plan revision ${display(release.plan_revision)} \u00b7 fingerprint ${display(release.plan_digest)}`,
    );
    lines.push(`    Release ref ${display(release.release_ref)} @ ${display(release.release_head)}`);
    lines.push(`    Target ref  ${display(release.target_ref)} @ ${display(release.target_head)}`);
  }
  if ((board.next_operations ?? []).length > 0) {
    lines.push('');
    lines.push('Next steps');
    for (const operation of board.next_operations) {
      lines.push(`  ${operationText(operation)}`);
    }
  }
  lines.push('');
  lines.push(`Board technical details: format=${display(board.schema_version)} \u00b7 state=${validity}`);
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
    process.stderr.write(
      `Baton could not read this board. ${sanitizeTerminalText(error?.message ?? 'invalid board')}\n`,
    );
    process.exitCode = error instanceof TypeError ? 64 : 2;
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2));
}
