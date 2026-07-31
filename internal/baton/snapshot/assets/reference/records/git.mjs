import { createHash } from 'node:crypto';
import {
  accessSync,
  constants,
  existsSync,
  lstatSync,
  mkdtempSync,
  readdirSync,
  realpathSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import {
  execFileSync,
  spawn,
  spawnSync,
} from 'node:child_process';
import { fileURLToPath } from 'node:url';

export class GitRecordError extends Error {
  constructor(code, message, cause) {
    super(message, cause ? { cause } : undefined);
    this.name = 'GitRecordError';
    this.code = code;
  }
}

const NULL_DEVICE = process.platform === 'win32' ? 'NUL' : '/dev/null';
const RECORD_ROOT_V1 = '.baton/releases';
const MAX_HEAD_REFS = 128;
const MAX_BATCH_PATHS = 1025;
const MAX_BATCH_FILE_BYTES = 262_144;
const MAX_BATCH_TOTAL_BYTES = MAX_BATCH_PATHS * MAX_BATCH_FILE_BYTES;
const MAX_RECORD_TREE_ENTRIES = MAX_BATCH_PATHS;
const MAX_RECORD_TREE_BYTES = 8 * 1024 * 1024;
const MAX_RECORD_CHANGES = 1025;
const MAX_RECORD_VALUE_BYTES = 262_144;
const MAX_RECORD_TOTAL_BYTES = 64 * 1024 * 1024;
const MAX_RECORD_MESSAGE_BYTES = 1000;
const MAX_REF_HELPER_REQUEST_BYTES = 512 * 1024;
const MAX_REF_HELPER_OUTPUT_BYTES = 512 * 1024;
const REF_HELPER_TIMEOUT_MS = 10_000;
const REF_HELPER_MARKER = '--baton-exact-ref-helper-v1';
const recordPathAdmissions = new WeakMap();
let configuredGitExecutable;

function defaultGitCandidates() {
  if (process.platform === 'win32') {
    return [
      'C:\\Program Files\\Git\\cmd\\git.exe',
      'C:\\Program Files\\Git\\bin\\git.exe',
      'C:\\Program Files (x86)\\Git\\cmd\\git.exe',
    ];
  }
  if (process.platform === 'darwin') {
    return [
      '/usr/bin/git',
      '/opt/homebrew/bin/git',
      '/usr/local/bin/git',
    ];
  }
  return ['/usr/bin/git', '/bin/git', '/usr/local/bin/git'];
}

function validateGitExecutable(executable) {
  if (typeof executable !== 'string' || !path.isAbsolute(executable)) {
    throw new GitRecordError(
      'INVALID_GIT_EXECUTABLE',
      'the trusted Git executable must be an absolute path',
    );
  }
  try {
    accessSync(executable, constants.X_OK);
    const stat = statSync(executable);
    if (!stat.isFile()) {
      throw new Error('not a regular file');
    }
    return realpathSync(executable);
  } catch (error) {
    throw new GitRecordError(
      'INVALID_GIT_EXECUTABLE',
      `the trusted Git executable is unavailable: ${executable}`,
      error,
    );
  }
}

/**
 * Pin Git explicitly for platforms or installations outside the trusted
 * built-in locations. The caller establishes trust; PATH is never searched.
 */
export function configureEngineGitExecutable(executable) {
  const resolved = validateGitExecutable(executable);
  if (configuredGitExecutable && configuredGitExecutable !== resolved) {
    throw new GitRecordError(
      'GIT_EXECUTABLE_ALREADY_CONFIGURED',
      `trusted Git is already fixed to ${configuredGitExecutable}`,
    );
  }
  configuredGitExecutable = resolved;
  return configuredGitExecutable;
}

export function gitExecutablePath() {
  if (configuredGitExecutable) return configuredGitExecutable;
  for (const candidate of defaultGitCandidates()) {
    if (!existsSync(candidate)) continue;
    configuredGitExecutable = validateGitExecutable(candidate);
    return configuredGitExecutable;
  }
  throw new GitRecordError(
    'GIT_EXECUTABLE_NOT_FOUND',
    'no trusted Git executable was found; call configureEngineGitExecutable with an absolute path',
  );
}

function gitEnvironmentForExecutable(executable, extra = {}, internal = {}) {
  const allowedOverrides = new Set([
    'GIT_INDEX_FILE',
    'GIT_AUTHOR_NAME',
    'GIT_AUTHOR_EMAIL',
    'GIT_AUTHOR_DATE',
    'GIT_COMMITTER_NAME',
    'GIT_COMMITTER_EMAIL',
    'GIT_COMMITTER_DATE',
  ]);
  const environment = {
    LANG: 'C',
    LC_ALL: 'C',
    PATH: path.dirname(executable),
    HOME: tmpdir(),
    XDG_CONFIG_HOME: tmpdir(),
    GIT_CONFIG_NOSYSTEM: '1',
    GIT_CONFIG_SYSTEM: NULL_DEVICE,
    GIT_CONFIG_GLOBAL: NULL_DEVICE,
    GIT_ATTR_NOSYSTEM: '1',
    GIT_NO_REPLACE_OBJECTS: '1',
    GIT_LITERAL_PATHSPECS: '1',
    GIT_TERMINAL_PROMPT: '0',
    GIT_PAGER: 'cat',
    GIT_PROTOCOL_FROM_USER: '0',
  };
  if (process.platform === 'win32') {
    for (const key of ['SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT']) {
      if (process.env[key]) environment[key] = process.env[key];
    }
  }
  for (const [key, value] of Object.entries(extra)) {
    if (allowedOverrides.has(key)) environment[key] = value;
  }
  for (const [key, value] of Object.entries(internal)) {
    if (
      key === 'GIT_DIR'
      || key === 'GIT_OBJECT_DIRECTORY'
      || key === 'GIT_INDEX_FILE'
    ) {
      environment[key] = value;
    }
  }
  return environment;
}

function gitEnvironment(extra = {}, internal = {}) {
  return gitEnvironmentForExecutable(gitExecutablePath(), extra, internal);
}

function executeGitWithHooks(
  executable,
  repo,
  hooksDirectory,
  args,
  options = {},
  internal = {},
) {
  try {
    return execFileSync(executable, [
      '-c',
      `core.hooksPath=${hooksDirectory}`,
      '-c',
      'core.fsmonitor=false',
      ...args,
    ], {
      cwd: repo,
      encoding: Object.hasOwn(options, 'encoding') ? options.encoding : 'utf8',
      input: options.input,
      env: gitEnvironmentForExecutable(executable, options.env, internal),
      maxBuffer: options.maxBuffer ?? 128 * 1024 * 1024,
      stdio: ['pipe', 'pipe', 'pipe'],
    });
  } catch (error) {
    const stderr = error?.stderr?.toString?.().trim();
    throw new GitRecordError(
      options.code ?? 'GIT_COMMAND_FAILED',
      `${options.label ?? `git ${args[0]}`} failed${stderr ? `: ${stderr}` : ''}`,
      error,
    );
  }
}

function executeGit(repo, args, options = {}, internal = {}) {
  const hooksDirectory = mkdtempSync(path.join(tmpdir(), 'baton-git-hooks-'));
  try {
    return executeGitWithHooks(
      gitExecutablePath(),
      repo,
      hooksDirectory,
      args,
      options,
      internal,
    );
  } finally {
    rmSync(hooksDirectory, { recursive: true, force: true });
  }
}

function gitExitStatus(repo, args, label) {
  const executable = gitExecutablePath();
  const hooksDirectory = mkdtempSync(path.join(tmpdir(), 'baton-git-hooks-'));
  try {
    const result = spawnSync(executable, [
      '-c',
      `core.hooksPath=${hooksDirectory}`,
      '-c',
      'core.fsmonitor=false',
      ...args,
    ], {
      cwd: repo,
      encoding: null,
      env: gitEnvironmentForExecutable(executable),
      maxBuffer: MAX_REF_HELPER_OUTPUT_BYTES,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    if (result.error || result.signal || !Number.isInteger(result.status)) {
      throw new GitRecordError(
        'INVALID_HEAD_OBJECT',
        `${label} failed without a trustworthy exit status`,
        result.error,
      );
    }
    return result.status;
  } finally {
    rmSync(hooksDirectory, { recursive: true, force: true });
  }
}

function runGit(repo, args, options = {}) {
  return executeGit(repo, args, options);
}

export function unsafeRunGit(repo, args, options = {}) {
  return runGit(repo, args, options);
}

export function repositoryRoot(repo = process.cwd()) {
  return runGit(repo, ['rev-parse', '--show-toplevel'], {
    label: 'resolve repository root',
  }).trim();
}

export function resolveRef(repo, ref) {
  return runGit(repo, ['rev-parse', '--verify', `${ref}^{commit}`], {
    code: 'REF_NOT_FOUND',
    label: `resolve ${ref}`,
  }).trim();
}

export function refExists(repo, ref) {
  try {
    resolveRef(repo, ref);
    return true;
  } catch (error) {
    if (error instanceof GitRecordError && error.code === 'REF_NOT_FOUND') {
      return false;
    }
    throw error;
  }
}

function assertExactHeadRef(ref) {
  const components = typeof ref === 'string' ? ref.split('/') : [];
  if (
    typeof ref !== 'string'
    || !ref.startsWith('refs/heads/')
    || ref.length > 1024
    || /[\u0000-\u0020\u007f~^:?*\\]/.test(ref)
    || ref.includes('[')
    || ref.includes('..')
    || ref.includes('@{')
    || ref.includes('//')
    || ref.endsWith('/')
    || ref.endsWith('.')
    || components.some((component) => (
      component.length === 0
      || component.startsWith('.')
      || component.endsWith('.lock')
    ))
  ) {
    throw new GitRecordError(
      'INVALID_HEAD_REF',
      `invalid exact branch ref ${String(ref)}`,
    );
  }
  return ref;
}

/**
 * Capture up to 128 exact branch heads in input order. Direct present refs use
 * one batch; refs omitted by Git need bounded probes to distinguish genuine
 * absence from a dangling symbolic ref.
 */
export function captureHeadRefs(repo, refs) {
  if (!Array.isArray(refs) || refs.length > MAX_HEAD_REFS) {
    throw new GitRecordError(
      'INVALID_REF_BATCH',
      `head capture requires an array of at most ${MAX_HEAD_REFS} refs`,
    );
  }
  if (refs.length === 0) return Object.freeze([]);
  const exactRefs = refs.map(assertExactHeadRef);
  if (new Set(exactRefs).size !== exactRefs.length) {
    throw new GitRecordError('DUPLICATE_REF', 'head capture refs must be unique');
  }
  let raw;
  try {
    raw = runGit(
      repo,
      [
        'for-each-ref',
        '--format=%(refname)%09%(objectname)%09%(objecttype)%09%(symref)',
        ...exactRefs,
      ],
      { encoding: null, label: 'capture exact branch heads' },
    );
  } catch (error) {
    throw new GitRecordError(
      'INVALID_HEAD_OBJECT',
      'one or more captured branches do not point directly to a commit',
      error,
    );
  }
  let rendered;
  try {
    rendered = new TextDecoder('utf-8', { fatal: true }).decode(raw);
  } catch (error) {
    throw new GitRecordError(
      'MALFORMED_GIT_OUTPUT',
      'branch head capture was not valid UTF-8',
      error,
    );
  }
  const requested = new Set(exactRefs);
  const captured = new Map();
  for (const line of rendered.split('\n').filter(Boolean)) {
    const fields = line.split('\t');
    if (fields.length !== 4) {
      throw new GitRecordError('MALFORMED_GIT_OUTPUT', 'branch head capture was malformed');
    }
    const [ref, head, type, symbolicTarget] = fields;
    if (!requested.has(ref)) continue;
    if (
      captured.has(ref)
      || symbolicTarget !== ''
      || type !== 'commit'
      || !/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(head)
    ) {
      throw new GitRecordError(
        'INVALID_HEAD_OBJECT',
        `branch ${ref} does not point directly to a commit`,
      );
    }
    captured.set(ref, head);
  }
  for (const ref of exactRefs) {
    if (captured.has(ref)) continue;
    const symbolicStatus = gitExitStatus(
      repo,
      ['symbolic-ref', '-q', ref],
      `inspect exact branch representation ${ref}`,
    );
    if (symbolicStatus === 0) {
      throw new GitRecordError(
        'INVALID_HEAD_OBJECT',
        `branch ${ref} does not point directly to a commit`,
      );
    }
    if (symbolicStatus !== 1) {
      throw new GitRecordError(
        'INVALID_HEAD_OBJECT',
        `branch ${ref} does not point directly to a commit`,
      );
    }
    const existenceStatus = gitExitStatus(
      repo,
      ['show-ref', '--verify', '--quiet', ref],
      `inspect exact branch existence ${ref}`,
    );
    if (existenceStatus !== 1) {
      throw new GitRecordError(
        'INVALID_HEAD_OBJECT',
        `branch ${ref} does not point directly to a commit`,
      );
    }
  }
  return Object.freeze(exactRefs.map((ref) => Object.freeze({
    ref,
    head: captured.get(ref) ?? null,
  })));
}

function assertObjectId(value, label) {
  if (!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(value)) {
    throw new GitRecordError('INVALID_REF_OID', `${label} must be a full commit OID`);
  }
  return value;
}

function assertObjectIdForFormat(value, label, objectFormat) {
  const objectId = assertObjectId(value, label);
  const width = objectFormat === 'sha256' ? 64 : 40;
  if (objectId.length !== width) {
    throw new GitRecordError(
      'INVALID_REF_OID',
      `${label} must match the repository object format`,
    );
  }
  return objectId;
}

function validateRefTransactionOperations(operations, objectFormat) {
  if (
    !Array.isArray(operations)
    || operations.length === 0
    || operations.length > MAX_HEAD_REFS
  ) {
    throw new GitRecordError(
      'INVALID_REF_TRANSACTION',
      `ref transaction requires between 1 and ${MAX_HEAD_REFS} operations`,
    );
  }
  if (!['sha1', 'sha256'].includes(objectFormat)) {
    throw new GitRecordError(
      'UNSUPPORTED_OBJECT_FORMAT',
      `unsupported Git object format ${objectFormat}`,
    );
  }
  const nullObjectId = '0'.repeat(objectFormat === 'sha256' ? 64 : 40);
  const refs = new Set();
  const commands = [];
  const receipt = [];
  const preState = [];
  const desiredState = [];
  let meaningful = false;
  for (const [index, operation] of operations.entries()) {
    if (
      operation === null
      || typeof operation !== 'object'
      || Array.isArray(operation)
      || !['create', 'update', 'verify'].includes(operation.kind)
    ) {
      throw new GitRecordError(
        'INVALID_REF_TRANSACTION',
        `ref transaction operation ${index} is malformed`,
      );
    }
    const ref = assertExactHeadRef(operation.ref);
    if (refs.has(ref)) {
      throw new GitRecordError('DUPLICATE_REF', `ref transaction repeats ${ref}`);
    }
    refs.add(ref);
    if (operation.kind === 'create') {
      if (Object.keys(operation).sort().join(',') !== 'kind,newHead,ref') {
        throw new GitRecordError(
          'INVALID_REF_TRANSACTION',
          `create operation ${index} has unknown fields`,
        );
      }
      const newHead = assertObjectIdForFormat(
        operation.newHead,
        'new ref head',
        objectFormat,
      );
      const copy = Object.freeze({ kind: 'create', ref, newHead });
      receipt.push(copy);
      preState.push(Object.freeze({ ref, head: null }));
      desiredState.push(Object.freeze({ ref, head: newHead }));
      commands.push(`create ${ref} ${newHead}`);
      meaningful = true;
      continue;
    }
    if (operation.kind === 'verify') {
      if (Object.keys(operation).sort().join(',') !== 'expectedHead,kind,ref') {
        throw new GitRecordError(
          'INVALID_REF_TRANSACTION',
          `verify operation ${index} has unknown fields`,
        );
      }
      const expectedHead = operation.expectedHead === null
        ? null
        : assertObjectIdForFormat(
          operation.expectedHead,
          'expected ref head',
          objectFormat,
        );
      const copy = Object.freeze({ kind: 'verify', ref, expectedHead });
      receipt.push(copy);
      preState.push(Object.freeze({ ref, head: expectedHead }));
      desiredState.push(Object.freeze({ ref, head: expectedHead }));
      commands.push(`verify ${ref} ${expectedHead ?? nullObjectId}`);
      continue;
    }
    if (Object.keys(operation).sort().join(',') !== 'expectedHead,kind,newHead,ref') {
      throw new GitRecordError(
        'INVALID_REF_TRANSACTION',
        `update operation ${index} has unknown fields`,
      );
    }
    const expectedHead = assertObjectIdForFormat(
      operation.expectedHead,
      'expected ref head',
      objectFormat,
    );
    const newHead = assertObjectIdForFormat(
      operation.newHead,
      'new ref head',
      objectFormat,
    );
    const copy = Object.freeze({
      kind: 'update',
      ref,
      newHead,
      expectedHead,
    });
    receipt.push(copy);
    preState.push(Object.freeze({ ref, head: expectedHead }));
    desiredState.push(Object.freeze({ ref, head: newHead }));
    commands.push(`update ${ref} ${newHead} ${expectedHead}`);
    if (newHead !== expectedHead) meaningful = true;
  }
  return Object.freeze({
    commands: Object.freeze(commands),
    receipt: Object.freeze(receipt),
    preState: Object.freeze(preState),
    desiredState: Object.freeze(desiredState),
    meaningful,
  });
}

function exactRefVectorMatches(observed, expected) {
  return observed.length === expected.length && observed.every((entry, index) => (
    entry.ref === expected[index].ref && entry.head === expected[index].head
  ));
}

function cleanupRefTransactionHooks(hooksDirectory) {
  rmSync(hooksDirectory, { recursive: true, force: true });
}

/**
 * Raw exact ref transaction primitive. Safe callers should use
 * createBatonActions; this exists for the engine implementation and
 * adversarial conformance fixtures.
 */
export function unsafeAtomicUpdateRefs(repo, operations) {
  const objectFormat = runGit(repo, ['rev-parse', '--show-object-format'], {
    label: 'resolve ref transaction object format',
  }).trim();
  const prepared = validateRefTransactionOperations(operations, objectFormat);
  const hooksDirectory = realpathSync(
    mkdtempSync(path.join(tmpdir(), 'baton-ref-hooks-')),
  );
  const request = Buffer.from(JSON.stringify({
    gitExecutable: gitExecutablePath(),
    hooksDirectory,
    objectFormat,
    operations: prepared.receipt,
  }));
  let helperOutcome = null;
  try {
    const helperOutput = execFileSync(
      process.execPath,
      [fileURLToPath(import.meta.url), REF_HELPER_MARKER],
      {
        cwd: repo,
        encoding: null,
        input: request,
        env: gitEnvironment(),
        maxBuffer: MAX_REF_HELPER_OUTPUT_BYTES,
        timeout: REF_HELPER_TIMEOUT_MS,
        killSignal: 'SIGKILL',
        stdio: ['pipe', 'pipe', 'pipe'],
      },
    );
    if (helperOutput.byteLength !== 0) {
      helperOutcome = new Error('exact-ref helper emitted unexpected output');
    }
  } catch (error) {
    helperOutcome = error;
  }

  let observed = null;
  let reconciliationError = null;
  try {
    observed = captureHeadRefs(
      repo,
      prepared.receipt.map((operation) => operation.ref),
    );
  } catch (error) {
    reconciliationError = error;
  }

  let result;
  if (observed && exactRefVectorMatches(observed, prepared.desiredState)) {
    result = prepared.receipt;
  } else if (
    observed
    && prepared.meaningful
    && exactRefVectorMatches(observed, prepared.preState)
  ) {
    result = new GitRecordError(
      'ATOMIC_REF_UPDATE_FAILED',
      'exact Baton ref transaction lost without partial advancement',
      helperOutcome,
    );
  } else {
    result = new GitRecordError(
      'ATOMIC_REF_UPDATE_FAILED',
      'exact Baton ref transaction has an ambiguous outcome; '
        + 'authoritative recovery is required before retry',
      reconciliationError ?? helperOutcome,
    );
  }

  try {
    cleanupRefTransactionHooks(hooksDirectory);
  } catch {
    // Exact-ref reconciliation is authoritative; cleanup cannot change it.
  }
  if (result instanceof Error) {
    throw result;
  }
  return result;
}

function exactObjectKeys(value, keys) {
  return (
    value !== null
    && typeof value === 'object'
    && !Array.isArray(value)
    && Object.keys(value).sort().join(',') === [...keys].sort().join(',')
  );
}

async function readBoundedHelperRequest() {
  const chunks = [];
  let size = 0;
  for await (const chunk of process.stdin) {
    size += chunk.byteLength;
    if (size > MAX_REF_HELPER_REQUEST_BYTES) {
      throw new Error('exact-ref helper request exceeded its byte bound');
    }
    chunks.push(chunk);
  }
  let rendered;
  try {
    rendered = new TextDecoder('utf-8', { fatal: true }).decode(Buffer.concat(chunks));
  } catch (error) {
    throw new Error('exact-ref helper request was not valid UTF-8', { cause: error });
  }
  let request;
  try {
    request = JSON.parse(rendered);
  } catch (error) {
    throw new Error('exact-ref helper request was not valid JSON', { cause: error });
  }
  if (!exactObjectKeys(
    request,
    ['gitExecutable', 'hooksDirectory', 'objectFormat', 'operations'],
  )) {
    throw new Error('exact-ref helper request fields differ');
  }
  const executable = validateGitExecutable(request.gitExecutable);
  if (executable !== request.gitExecutable) {
    throw new Error('exact-ref helper Git executable is not canonical');
  }
  if (
    typeof request.hooksDirectory !== 'string'
    || !path.isAbsolute(request.hooksDirectory)
    || realpathSync(request.hooksDirectory) !== request.hooksDirectory
    || !statSync(request.hooksDirectory).isDirectory()
    || readdirSync(request.hooksDirectory).length !== 0
  ) {
    throw new Error('exact-ref helper hooks directory is not exact and empty');
  }
  const prepared = validateRefTransactionOperations(
    request.operations,
    request.objectFormat,
  );
  return Object.freeze({
    gitExecutable: executable,
    hooksDirectory: request.hooksDirectory,
    objectFormat: request.objectFormat,
    prepared,
  });
}

function helperGitArguments(hooksDirectory, args) {
  return [
    '-c',
    `core.hooksPath=${hooksDirectory}`,
    '-c',
    'core.fsmonitor=false',
    ...args,
  ];
}

function helperGitStatus(request, args) {
  const result = spawnSync(
    request.gitExecutable,
    helperGitArguments(request.hooksDirectory, args),
    {
      cwd: process.cwd(),
      encoding: null,
      env: gitEnvironmentForExecutable(request.gitExecutable),
      maxBuffer: MAX_REF_HELPER_OUTPUT_BYTES,
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  );
  if (result.error || result.signal || !Number.isInteger(result.status)) {
    throw new Error('exact-ref helper inspection lacked a trustworthy exit status', {
      cause: result.error,
    });
  }
  return result.status;
}

function helperCaptureDirectRefs(request) {
  const refs = request.prepared.receipt.map((operation) => operation.ref);
  let raw;
  try {
    raw = execFileSync(
      request.gitExecutable,
      helperGitArguments(request.hooksDirectory, [
        'for-each-ref',
        '--format=%(refname)%09%(objectname)%09%(objecttype)%09%(symref)',
        ...refs,
      ]),
      {
        cwd: process.cwd(),
        encoding: null,
        env: gitEnvironmentForExecutable(request.gitExecutable),
        maxBuffer: MAX_REF_HELPER_OUTPUT_BYTES,
        stdio: ['ignore', 'pipe', 'pipe'],
      },
    );
  } catch (error) {
    throw new Error('exact-ref helper batch inspection failed', { cause: error });
  }
  let rendered;
  try {
    rendered = new TextDecoder('utf-8', { fatal: true }).decode(raw);
  } catch (error) {
    throw new Error('exact-ref helper batch inspection was not valid UTF-8', {
      cause: error,
    });
  }
  const requested = new Set(refs);
  const captured = new Map();
  for (const line of rendered.split('\n').filter(Boolean)) {
    const fields = line.split('\t');
    if (fields.length !== 4) {
      throw new Error('exact-ref helper batch inspection was malformed');
    }
    const [ref, head, type, symbolicTarget] = fields;
    if (
      !requested.has(ref)
      || captured.has(ref)
      || symbolicTarget !== ''
      || type !== 'commit'
      || !/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(head)
    ) {
      throw new Error('exact-ref helper observed an invalid branch representation');
    }
    captured.set(ref, head);
  }
  return captured;
}

function recheckPreparedRefState(request) {
  for (const expected of request.prepared.preState) {
    const symbolicStatus = helperGitStatus(
      request,
      ['symbolic-ref', '-q', expected.ref],
    );
    if (symbolicStatus !== 1) {
      throw new Error('exact-ref helper observed a symbolic or unreadable branch');
    }
    const existenceStatus = helperGitStatus(
      request,
      ['show-ref', '--verify', '--quiet', expected.ref],
    );
    const expectedStatus = expected.head === null ? 1 : 0;
    if (existenceStatus !== expectedStatus) {
      throw new Error('exact-ref helper observed unexpected branch existence');
    }
  }
  const captured = helperCaptureDirectRefs(request);
  for (const expected of request.prepared.preState) {
    if (expected.head === null) {
      if (captured.has(expected.ref)) {
        throw new Error('exact-ref helper observed an unexpected branch');
      }
    } else if (captured.get(expected.ref) !== expected.head) {
      throw new Error('exact-ref helper observed a moved or invalid branch');
    }
  }
}

function createBoundedLineReader(stream) {
  const lines = [];
  let buffered = Buffer.alloc(0);
  let total = 0;
  let ended = false;
  let failure = null;
  let wake = null;
  const notify = () => {
    if (wake) {
      const resolve = wake;
      wake = null;
      resolve();
    }
  };
  stream.on('data', (chunk) => {
    total += chunk.byteLength;
    if (total > MAX_REF_HELPER_OUTPUT_BYTES) {
      failure = new Error('exact-ref Git protocol output exceeded its byte bound');
      notify();
      return;
    }
    buffered = Buffer.concat([buffered, chunk]);
    let newline = buffered.indexOf(0x0a);
    while (newline !== -1) {
      lines.push(buffered.subarray(0, newline));
      buffered = buffered.subarray(newline + 1);
      newline = buffered.indexOf(0x0a);
    }
    notify();
  });
  stream.on('error', (error) => {
    failure = error;
    notify();
  });
  stream.on('end', () => {
    ended = true;
    notify();
  });
  const waitForChange = async () => {
    await new Promise((resolve) => {
      wake = resolve;
    });
  };
  return Object.freeze({
    async next() {
      while (lines.length === 0 && !ended && !failure) {
        await waitForChange();
      }
      if (failure) throw failure;
      if (lines.length === 0) {
        throw new Error('exact-ref Git protocol ended before acknowledgement');
      }
      try {
        return new TextDecoder('utf-8', { fatal: true }).decode(lines.shift());
      } catch (error) {
        throw new Error('exact-ref Git protocol was not valid UTF-8', { cause: error });
      }
    },
    requireNoQueued() {
      if (failure) throw failure;
      if (lines.length !== 0 || buffered.byteLength !== 0) {
        throw new Error('exact-ref Git protocol emitted extra output');
      }
    },
    async requireEnd() {
      while (!ended && !failure) {
        await waitForChange();
      }
      if (failure) throw failure;
      if (lines.length !== 0 || buffered.byteLength !== 0) {
        throw new Error('exact-ref Git protocol emitted extra output');
      }
    },
  });
}

function writeProtocolLine(child, line) {
  return new Promise((resolve, reject) => {
    child.stdin.write(`${line}\n`, (error) => {
      if (error) reject(error);
      else resolve();
    });
  });
}

async function runExactRefHelper() {
  const request = await readBoundedHelperRequest();
  const child = spawn(
    request.gitExecutable,
    helperGitArguments(request.hooksDirectory, [
      'update-ref',
      '--no-deref',
      '--stdin',
    ]),
    {
      cwd: process.cwd(),
      env: gitEnvironmentForExecutable(request.gitExecutable),
      stdio: ['pipe', 'pipe', 'pipe'],
    },
  );
  const stdout = createBoundedLineReader(child.stdout);
  const stderrChunks = [];
  let stderrBytes = 0;
  let stderrOverflow = false;
  child.stderr.on('data', (chunk) => {
    stderrBytes += chunk.byteLength;
    if (stderrBytes > MAX_REF_HELPER_OUTPUT_BYTES) {
      stderrOverflow = true;
      child.stdin.end();
    } else {
      stderrChunks.push(chunk);
    }
  });
  const exit = new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('exit', (code, signal) => resolve({ code, signal }));
  });
  let startSent = false;
  let committed = false;
  try {
    await writeProtocolLine(child, 'start');
    startSent = true;
    if (await stdout.next() !== 'start: ok') {
      throw new Error('exact-ref Git protocol rejected start');
    }
    stdout.requireNoQueued();
    for (const command of request.prepared.commands) {
      await writeProtocolLine(child, command);
    }
    await writeProtocolLine(child, 'prepare');
    if (await stdout.next() !== 'prepare: ok') {
      throw new Error('exact-ref Git protocol rejected prepare');
    }
    stdout.requireNoQueued();
    recheckPreparedRefState(request);
    await writeProtocolLine(child, 'commit');
    if (await stdout.next() !== 'commit: ok') {
      throw new Error('exact-ref Git protocol rejected commit');
    }
    committed = true;
    child.stdin.end();
    const outcome = await exit;
    await stdout.requireEnd();
    if (
      stderrOverflow
      || stderrChunks.length !== 0
      || outcome.code !== 0
      || outcome.signal !== null
    ) {
      throw new Error('exact-ref Git process failed after commit acknowledgement');
    }
  } catch (error) {
    if (!committed && startSent && child.exitCode === null && child.signalCode === null) {
      try {
        await writeProtocolLine(child, 'abort');
        if (await stdout.next() !== 'abort: ok') {
          throw new Error('exact-ref Git protocol rejected abort');
        }
      } catch {}
    }
    if (!child.stdin.destroyed) child.stdin.end();
    if (child.exitCode === null && child.signalCode === null) {
      await exit.catch(() => {});
    }
    throw error;
  }
}

async function runExactRefHelperMain() {
  try {
    await runExactRefHelper();
  } catch (error) {
    const message = error instanceof Error ? error.message : 'unknown helper failure';
    process.stderr.write(`${message.slice(0, 4096)}\n`);
    process.exitCode = 1;
  }
}

export function isAncestor(repo, ancestor, descendant) {
  try {
    runGit(repo, ['merge-base', '--is-ancestor', ancestor, descendant], {
      code: 'NOT_ANCESTOR',
      label: `check ancestry ${ancestor} -> ${descendant}`,
    });
    return true;
  } catch (error) {
    if (error instanceof GitRecordError && error.code === 'NOT_ANCESTOR') {
      return false;
    }
    throw error;
  }
}

export function readFileAtOID(repo, ref, relativePath) {
  assertRepositoryPath(relativePath);
  if (!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(ref)) {
    throw new GitRecordError(
      'INVALID_REF_OID',
      'singular file reads require a full captured commit OID',
    );
  }
  try {
    return runGit(repo, ['show', `${ref}:${relativePath}`], {
      encoding: null,
      code: 'RECORD_NOT_FOUND',
      label: `read ${relativePath} at ${ref}`,
    });
  } catch (error) {
    if (error instanceof GitRecordError) throw error;
    throw new GitRecordError('RECORD_NOT_FOUND', `cannot read ${relativePath} at ${ref}`, error);
  }
}

function readFilesAtOIDWithin(repo, refOID, paths, maxPaths) {
  if (!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(refOID)) {
    throw new GitRecordError(
      'INVALID_REF_OID',
      'batched file reads require a full captured commit OID',
    );
  }
  if (!Array.isArray(paths) || paths.length > maxPaths) {
    throw new GitRecordError(
      'INVALID_PATH_BATCH',
      `batched file reads require an array of at most ${maxPaths} paths`,
    );
  }
  if (paths.length === 0) return Object.freeze([]);
  const exactPaths = paths.map(assertRepositoryPath);
  const expressions = exactPaths.map((relativePath) => `${refOID}:${relativePath}`);
  const input = `${expressions.join('\n')}\n`;
  if (Buffer.byteLength(input) > 4 * 1024 * 1024) {
    throw new GitRecordError('PATH_BATCH_TOO_LARGE', 'batched file read input exceeds 4 MiB');
  }
  const raw = runGit(repo, ['cat-file', '--batch'], {
    encoding: null,
    input,
    maxBuffer: MAX_BATCH_TOTAL_BYTES + (8 * 1024 * 1024),
    code: 'BATCH_READ_FAILED',
    label: `read ${exactPaths.length} files at ${refOID}`,
  });
  const entries = [];
  let offset = 0;
  let totalBytes = 0;
  for (let index = 0; index < exactPaths.length; index += 1) {
    const newline = raw.indexOf(0x0a, offset);
    if (newline < 0) {
      throw new GitRecordError('MALFORMED_GIT_OUTPUT', 'cat-file batch header is incomplete');
    }
    const header = raw.subarray(offset, newline).toString('utf8');
    offset = newline + 1;
    if (header === `${expressions[index]} missing`) {
      entries.push(Object.freeze({
        path: exactPaths[index],
        object: null,
        size: null,
        bytes: null,
      }));
      continue;
    }
    const match = header.match(/^([0-9a-f]{40}|[0-9a-f]{64}) ([a-z]+) ([0-9]+)$/);
    if (!match || match[2] !== 'blob') {
      throw new GitRecordError(
        'MALFORMED_GIT_OUTPUT',
        `cat-file returned an invalid blob header for ${exactPaths[index]}`,
      );
    }
    const size = Number.parseInt(match[3], 10);
    if (
      !Number.isSafeInteger(size)
      || size < 0
      || size > MAX_BATCH_FILE_BYTES
      || totalBytes + size > MAX_BATCH_TOTAL_BYTES
    ) {
      throw new GitRecordError(
        'BATCH_READ_LIMIT_EXCEEDED',
        `batched file content exceeds its bounded size at ${exactPaths[index]}`,
      );
    }
    const end = offset + size;
    if (end >= raw.length || raw[end] !== 0x0a) {
      throw new GitRecordError(
        'MALFORMED_GIT_OUTPUT',
        `cat-file returned incomplete content for ${exactPaths[index]}`,
      );
    }
    const bytes = Buffer.from(raw.subarray(offset, end));
    offset = end + 1;
    totalBytes += size;
    entries.push(Object.freeze({
      path: exactPaths[index],
      object: match[1],
      size,
      bytes,
    }));
  }
  if (offset !== raw.length) {
    throw new GitRecordError('MALFORMED_GIT_OUTPUT', 'cat-file returned unexpected trailing data');
  }
  return Object.freeze(entries);
}

/**
 * Read up to 1025 files from one captured commit OID with one cat-file process.
 * Each frozen entry contains exact bytes, or null fields when the path is
 * absent. Individual files and aggregate output are bounded.
 */
export function readFilesAtOID(repo, refOID, paths) {
  return readFilesAtOIDWithin(repo, refOID, paths, MAX_BATCH_PATHS);
}

function assertRepositoryPath(relativePath) {
  const segments = typeof relativePath === 'string' ? relativePath.split('/') : [];
  if (
    typeof relativePath !== 'string'
    || relativePath.length === 0
    || path.isAbsolute(relativePath)
    || relativePath.includes('\\')
    || /[\u0000-\u001f\u007f]/.test(relativePath)
    || segments[0] === '.git'
    || segments.some((segment) => segment === '' || segment === '.' || segment === '..')
  ) {
    throw new GitRecordError('INVALID_REPOSITORY_PATH', `invalid repository path ${String(relativePath)}`);
  }
  return relativePath;
}

function assertRelativeRoot(root) {
  if (
    typeof root !== 'string'
    || root.length === 0
    || path.isAbsolute(root)
    || root.includes('\\')
  ) {
    throw new GitRecordError('INVALID_RECORD_ROOT', 'record root must be a non-empty repository-relative path');
  }
  const segments = root.split('/');
  if (
    segments.some((segment) => segment === '' || segment === '.' || segment === '..')
    || segments[0] === '.git'
  ) {
    throw new GitRecordError('INVALID_RECORD_ROOT', `record root is not canonical: ${root}`);
  }
  return segments;
}

export function assertCanonicalRecordRoot(repo, root) {
  const repository = repositoryRoot(repo);
  const segments = assertRelativeRoot(root);
  let cursor = repository;
  for (const segment of segments) {
    cursor = path.join(cursor, segment);
    try {
      const stat = lstatSync(cursor);
      if (stat.isSymbolicLink()) {
        throw new GitRecordError('SYMLINKED_RECORD_ROOT', `record root traverses symlink ${cursor}`);
      }
    } catch (error) {
      if (error instanceof GitRecordError) throw error;
      if (error?.code === 'ENOENT') break;
      throw new GitRecordError('INVALID_RECORD_ROOT', `cannot inspect record root ${cursor}`, error);
    }
  }
  return segments.join('/');
}

/**
 * Admit the fixed v1 record path for structural reads and changed-path checks.
 * This capability says nothing about whether record bytes affect product
 * behavior and therefore cannot authorize product-tree exclusion.
 */
export function resolveRecordPathAdmission(repo) {
  const repository = realpathSync(repositoryRoot(repo));
  assertCanonicalRecordRoot(repository, RECORD_ROOT_V1);
  const admission = Object.freeze(Object.create(null));
  recordPathAdmissions.set(admission, {
    repository,
    root: RECORD_ROOT_V1,
  });
  return admission;
}

/**
 * Admit captured-object reads below the fixed v1 record root without
 * consulting launch-worktree entries. Callers receive no write capability;
 * every downstream read still validates the root and path at an exact commit.
 */
export function resolveCapturedRecordPathAdmission(repo) {
  const repository = realpathSync(repositoryRoot(repo));
  const admission = Object.freeze(Object.create(null));
  recordPathAdmissions.set(admission, {
    repository,
    root: RECORD_ROOT_V1,
  });
  return admission;
}

function recordPathAdmissionData(admission) {
  return (
    admission !== null
    && typeof admission === 'object'
    && recordPathAdmissions.get(admission)
  );
}

function requireRecordPathAdmission(repo, admission) {
  const admitted = recordPathAdmissionData(admission);
  if (!admitted) {
    throw new GitRecordError(
      'RECORD_PATH_ADMISSION_REQUIRED',
      'structural record access requires a fixed record-path admission',
    );
  }
  const repository = realpathSync(repositoryRoot(repo));
  if (repository !== admitted.repository) {
    throw new GitRecordError(
      'RECORD_ROOT_ADMISSION_MISMATCH',
      'the record-path admission belongs to a different repository',
    );
  }
  return admitted.root;
}

function parseTreeEntry(buffer) {
  const tab = buffer.indexOf(0x09);
  if (tab < 0) {
    throw new GitRecordError('MALFORMED_GIT_TREE', 'git ls-tree entry has no path separator');
  }
  const metadata = buffer.subarray(0, tab).toString('ascii').split(' ');
  if (metadata.length !== 3) {
    throw new GitRecordError('MALFORMED_GIT_TREE', 'git ls-tree entry has malformed metadata');
  }
  const filePath = new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(tab + 1));
  return {
    mode: metadata[0],
    type: metadata[1],
    object: metadata[2],
    path: filePath,
  };
}

export function assertRecordRootAtRef(repo, ref, recordRoot, options = {}) {
  const root = assertRelativeRoot(recordRoot).join('/');
  const commit = resolveRef(repo, ref);
  const segments = root.split('/');
  for (let index = 0; index < segments.length; index += 1) {
    const prefix = segments.slice(0, index + 1).join('/');
    const raw = runGit(repo, ['ls-tree', '-z', commit, '--', prefix], {
      encoding: null,
      label: `inspect record root ${prefix} at ${commit}`,
    });
    if (raw.length === 0) {
      if (options.allowMissing === true) return root;
      throw new GitRecordError(
        'RECORD_ROOT_NOT_FOUND',
        `record root ${root} does not exist at ${commit}`,
      );
    }
    const nul = raw.indexOf(0);
    if (nul < 0 || nul !== raw.length - 1) {
      throw new GitRecordError('MALFORMED_GIT_TREE', `ambiguous tree entry for ${prefix}`);
    }
    const entry = parseTreeEntry(raw.subarray(0, nul));
    if (entry.path !== prefix) {
      throw new GitRecordError('MALFORMED_GIT_TREE', `unexpected tree entry ${entry.path} for ${prefix}`);
    }
    if (entry.mode === '120000') {
      throw new GitRecordError(
        'SYMLINKED_RECORD_ROOT',
        `record root ${root} traverses a symlink at ${prefix} in ${commit}`,
      );
    }
    if (entry.type !== 'tree') {
      throw new GitRecordError(
        'INVALID_RECORD_ROOT',
        `record root component ${prefix} is not a directory in ${commit}`,
      );
    }
  }
  return root;
}

export function readRecordTreeAtOID(repo, refOID, admission, subtree) {
  if (!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(refOID)) {
    throw new GitRecordError(
      'INVALID_REF_OID',
      'record-tree inventory requires a full captured commit OID',
    );
  }
  const root = requireRecordPathAdmission(repo, admission);
  const prefix = assertRepositoryPath(subtree);
  if (prefix !== root && !prefix.startsWith(`${root}/`)) {
    throw new GitRecordError(
      'NON_RECORD_PATH',
      `record-tree inventory must remain below ${root}`,
    );
  }
  assertRecordRootAtRef(repo, refOID, root, { allowMissing: true });
  const raw = runGit(repo, ['ls-tree', '-r', '-z', refOID, '--', prefix], {
    encoding: null,
    maxBuffer: MAX_RECORD_TREE_BYTES,
    code: 'RECORD_TREE_INVENTORY_FAILED',
    label: `inventory record tree ${prefix} at ${refOID}`,
  });
  const entries = [];
  let offset = 0;
  while (offset < raw.length) {
    const nul = raw.indexOf(0, offset);
    if (nul < 0) {
      throw new GitRecordError(
        'MALFORMED_GIT_TREE',
        'record-tree inventory is not NUL terminated',
      );
    }
    if (nul > offset) {
      entries.push(Object.freeze(parseTreeEntry(raw.subarray(offset, nul))));
      if (entries.length > MAX_RECORD_TREE_ENTRIES) {
        throw new GitRecordError(
          'RECORD_TREE_INVENTORY_LIMIT',
          `record-tree inventory exceeds ${MAX_RECORD_TREE_ENTRIES} entries`,
        );
      }
    }
    offset = nul + 1;
  }
  return Object.freeze(entries);
}

export function productTreeIdentity(repo, commit) {
  const candidate = resolveRef(repo, commit);
  const root = RECORD_ROOT_V1;
  assertRecordRootAtRef(repo, candidate, root, { allowMissing: true });
  const candidateTree = runGit(repo, ['rev-parse', `${candidate}^{tree}`], {
    label: `resolve candidate tree ${candidate}`,
  }).trim();
  const raw = runGit(repo, ['ls-tree', '-r', '-z', candidate], {
    encoding: null,
    label: `read candidate tree ${candidate}`,
  });
  const entries = [];
  let offset = 0;
  while (offset < raw.length) {
    const nul = raw.indexOf(0, offset);
    if (nul < 0) {
      throw new GitRecordError('MALFORMED_GIT_TREE', 'git ls-tree output is not NUL terminated');
    }
    if (nul > offset) entries.push(parseTreeEntry(raw.subarray(offset, nul)));
    offset = nul + 1;
  }
  const productEntries = entries
    .filter((entry) => entry.path !== root && !entry.path.startsWith(`${root}/`))
    .sort((left, right) => Buffer.from(left.path).compare(Buffer.from(right.path)));
  const hash = createHash('sha256');
  for (const entry of productEntries) {
    hash.update(entry.path);
    hash.update('\0');
    hash.update(entry.mode);
    hash.update('\0');
    hash.update(entry.type);
    hash.update('\0');
    hash.update(entry.object);
    hash.update('\n');
  }
  return {
    candidate,
    candidateTree,
    productTree: `sha256:${hash.digest('hex')}`,
    entries: productEntries,
  };
}

export function assertCandidateRecordRootUnchanged(repo, base, candidate) {
  const exactBase = resolveRef(repo, base);
  const exactCandidate = resolveRef(repo, candidate);
  const root = RECORD_ROOT_V1;
  assertRecordRootAtRef(repo, exactBase, root, { allowMissing: true });
  assertRecordRootAtRef(repo, exactCandidate, root, { allowMissing: true });
  const raw = runGit(repo, [
    'diff-tree',
    '--no-commit-id',
    '--raw',
    '-z',
    exactBase,
    exactCandidate,
    '--',
    root,
  ], {
    encoding: null,
    label: `compare reserved record root between ${exactBase} and ${exactCandidate}`,
  });
  if (raw.length > MAX_RECORD_TREE_BYTES) {
    throw new GitRecordError(
      'RECORD_TREE_INVENTORY_LIMIT',
      `record-root comparison exceeds ${MAX_RECORD_TREE_BYTES} bytes`,
    );
  }
  if (raw.length !== 0) {
    throw new GitRecordError(
      'RESERVED_RECORD_ROOT_CHANGED',
      `candidate ${exactCandidate} changes reserved record root ${root} from base ${exactBase}`,
    );
  }
  return Object.freeze({ base: exactBase, candidate: exactCandidate, root });
}

export function assertCandidate(repo, base, candidate) {
  const exactBase = resolveRef(repo, base);
  const exactCandidate = resolveRef(repo, candidate);
  if (!isAncestor(repo, exactBase, exactCandidate)) {
    throw new GitRecordError(
      'INVALID_CANDIDATE_ANCESTRY',
      `candidate ${exactCandidate} does not descend from base ${exactBase}`,
    );
  }
  return { base: exactBase, candidate: exactCandidate };
}

export function commitParents(repo, commit) {
  const line = runGit(repo, ['rev-list', '--parents', '-n', '1', commit], {
    label: `read parents of ${commit}`,
  }).trim();
  return line.split(/\s+/).slice(1);
}

function splitNul(raw) {
  const values = [];
  let offset = 0;
  while (offset < raw.length) {
    const nul = raw.indexOf(0, offset);
    if (nul < 0) {
      throw new GitRecordError('MALFORMED_GIT_OUTPUT', 'Git output is not NUL terminated');
    }
    values.push(raw.subarray(offset, nul));
    offset = nul + 1;
  }
  return values;
}

export function changedPathsBetween(repo, base, candidate) {
  const exactBase = resolveRef(repo, base);
  const exactCandidate = resolveRef(repo, candidate);
  const raw = runGit(
    repo,
    [
      'diff-tree',
      '--no-commit-id',
      '--name-only',
      '-r',
      '-z',
      '--no-renames',
      '--no-ext-diff',
      '--no-textconv',
      '--ignore-submodules=none',
      exactBase,
      exactCandidate,
    ],
    { encoding: null, label: `read changed paths ${exactBase}..${exactCandidate}` },
  );
  const decoder = new TextDecoder('utf-8', { fatal: true });
  let paths;
  try {
    paths = splitNul(raw)
      .filter((value) => value.length > 0)
      .map((value) => assertRepositoryPath(decoder.decode(value)));
  } catch (error) {
    if (error instanceof GitRecordError) throw error;
    throw new GitRecordError(
      'INVALID_REPOSITORY_PATH',
      'changed paths are not canonical UTF-8 repository paths',
      error,
    );
  }
  return [...new Set(paths)].sort((left, right) => (
    Buffer.from(left).compare(Buffer.from(right))
  ));
}

/**
 * Return the newest first-parent commit at or below `head` that changed one
 * exact repository path. The query is output-bounded and never reads commit
 * messages, so callers can prove path-introduction history without
 * interpreting inherited receipt text.
 */
export function firstParentPathChange(repo, head, relativePath) {
  const exactHead = resolveRef(repo, head);
  const exactPath = assertRepositoryPath(relativePath);
  const result = runGit(
    repo,
    [
      'rev-list',
      '--first-parent',
      '--full-history',
      '--max-count=1',
      exactHead,
      '--',
      exactPath,
    ],
    {
      maxBuffer: 1024,
      label: `read first-parent path history for ${exactPath}`,
    },
  ).trim();
  if (result === '') return null;
  if (!/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(result)) {
    throw new GitRecordError(
      'MALFORMED_GIT_OUTPUT',
      `first-parent path history for ${exactPath} is malformed`,
    );
  }
  return result;
}

function repositoryObjectDirectory(repo) {
  const common = runGit(
    repo,
    ['rev-parse', '--path-format=absolute', '--git-common-dir'],
    { label: 'resolve Git common directory' },
  ).trim();
  const objectDirectory = path.join(common, 'objects');
  try {
    if (!statSync(objectDirectory).isDirectory()) throw new Error('not a directory');
    return realpathSync(objectDirectory);
  } catch (error) {
    throw new GitRecordError(
      'INVALID_GIT_OBJECT_DIRECTORY',
      `cannot use Git object directory ${objectDirectory}`,
      error,
    );
  }
}

function withEngineGitContext(repo, operation) {
  const temporary = mkdtempSync(path.join(tmpdir(), 'baton-git-context-'));
  const gitDirectory = path.join(temporary, 'repository.git');
  try {
    const objectFormat = runGit(
      repo,
      ['rev-parse', '--show-object-format=storage'],
      { label: 'resolve repository object format' },
    ).trim();
    runGit(
      temporary,
      ['init', '--quiet', '--bare', `--object-format=${objectFormat}`, gitDirectory],
      { label: 'create engine-owned Git context' },
    );
    const context = {
      cwd: repo,
      attributesFile: path.join(temporary, 'attributes'),
      environment: {
        GIT_DIR: gitDirectory,
        GIT_OBJECT_DIRECTORY: repositoryObjectDirectory(repo),
        GIT_INDEX_FILE: path.join(temporary, 'index'),
      },
    };
    return operation(context);
  } finally {
    rmSync(temporary, { recursive: true, force: true });
  }
}

function runEngineGit(context, args, options = {}) {
  return executeGit(context.cwd, args, options, context.environment);
}

function runEngineMergeTree(context, args, label) {
  const executable = gitExecutablePath();
  const hooksDirectory = mkdtempSync(path.join(tmpdir(), 'baton-git-hooks-'));
  try {
    const result = spawnSync(executable, [
      '-c',
      `core.hooksPath=${hooksDirectory}`,
      '-c',
      'core.fsmonitor=false',
      ...args,
    ], {
      cwd: context.cwd,
      encoding: 'utf8',
      env: gitEnvironmentForExecutable(executable, {}, context.environment),
      maxBuffer: 128 * 1024 * 1024,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    if (result.error || result.signal || !Number.isInteger(result.status)) {
      throw new GitRecordError(
        'GIT_COMMAND_FAILED',
        `${label} failed without a trustworthy exit status`,
        result.error,
      );
    }
    if (result.status === 0) return result.stdout;
    const stderr = result.stderr?.trim();
    if (result.status === 1) {
      throw new GitRecordError(
        'COMPOSITION_CONFLICT',
        `${label} conflicted${stderr ? `: ${stderr}` : ''}`,
      );
    }
    throw new GitRecordError(
      'GIT_COMMAND_FAILED',
      `${label} failed with status ${result.status}${stderr ? `: ${stderr}` : ''}`,
    );
  } finally {
    rmSync(hooksDirectory, { recursive: true, force: true });
  }
}

function treePaths(context, commit) {
  const raw = runEngineGit(
    context,
    ['ls-tree', '-r', '--name-only', '-z', commit],
    { encoding: null, label: `enumerate paths at ${commit}` },
  );
  return splitNul(raw).filter((value) => value.length > 0);
}

function mergeAttributesAtSource(context, source, paths) {
  if (paths.length === 0) return [];
  const input = Buffer.concat(paths.flatMap((entry) => [entry, Buffer.from([0])]));
  runEngineGit(
    context,
    ['read-tree', source],
    { label: `seed exact merge attributes at ${source}` },
  );
  const raw = runEngineGit(
    context,
    ['check-attr', '-z', '--stdin', '--cached', 'merge'],
    {
      encoding: null,
      input,
      code: 'UNTRUSTED_MERGE_ATTRIBUTES',
      label: `inspect merge attributes at ${source}`,
    },
  );
  const fields = splitNul(raw);
  if (fields.length % 3 !== 0) {
    throw new GitRecordError(
      'MALFORMED_GIT_OUTPUT',
      `Git returned malformed merge attributes for ${source}`,
    );
  }
  const attributes = [];
  for (let index = 0; index < fields.length; index += 3) {
    attributes.push({
      path: fields[index],
      value: fields[index + 2].toString('utf8'),
    });
  }
  return attributes;
}

function installBuiltInMergeAttributes(context, expected, candidate, productBase = null) {
  const unique = new Map();
  const sources = [expected, candidate, ...(productBase === null ? [] : [productBase])];
  for (const entry of sources.flatMap((source) => treePaths(context, source))) {
    unique.set(entry.toString('hex'), entry);
  }
  const paths = [...unique.values()];
  const builtIn = new Set(['unspecified', 'set', 'unset', 'text', 'binary', 'union']);
  let expectedAttributes = [];
  for (const source of sources) {
    const attributes = mergeAttributesAtSource(context, source, paths);
    if (source === expected) expectedAttributes = attributes;
    for (const { path: filePathBytes, value } of attributes) {
      if (!builtIn.has(value)) {
        const filePath = filePathBytes.toString('utf8');
        throw new GitRecordError(
          'UNTRUSTED_MERGE_DRIVER',
          `custom merge driver ${value} applies to ${filePath} at ${source}`,
        );
      }
    }
  }

  const decoder = new TextDecoder('utf-8', { fatal: true });
  let rendered;
  try {
    rendered = expectedAttributes
      .filter(({ value }) => value !== 'unspecified')
      .map(({ path: filePathBytes, value }) => {
        const filePath = assertRepositoryPath(decoder.decode(filePathBytes));
        const attribute = (
          value === 'set'
            ? 'merge'
            : value === 'unset'
              ? '-merge'
              : `merge=${value}`
        );
        return `${JSON.stringify(filePath)} ${attribute}`;
      })
      .join('\n');
  } catch (error) {
    if (error instanceof GitRecordError) throw error;
    throw new GitRecordError(
      'INVALID_REPOSITORY_PATH',
      'merge attributes apply to a non-canonical repository path',
      error,
    );
  }
  writeFileSync(context.attributesFile, `${rendered}${rendered ? '\n' : ''}`);
  runEngineGit(
    context,
    ['config', '--local', 'core.attributesFile', context.attributesFile],
    { label: 'install engine-owned merge attributes' },
  );
}

function restoreFirstParentRecordRoot(context, expected, recordRoot, label) {
  const indexedRaw = runEngineGit(
    context,
    ['ls-files', '-z', '--', recordRoot],
    {
      encoding: null,
      label: `enumerate merged ${recordRoot}`,
    },
  );
  if (indexedRaw.length > MAX_RECORD_TREE_BYTES) {
    throw new GitRecordError(
      'RECORD_TREE_INVENTORY_LIMIT',
      `merged record-tree inventory exceeds ${MAX_RECORD_TREE_BYTES} bytes`,
    );
  }
  const indexed = splitNul(indexedRaw).filter((entry) => entry.length > 0);
  if (indexed.length > MAX_RECORD_TREE_ENTRIES) {
    throw new GitRecordError(
      'RECORD_TREE_INVENTORY_LIMIT',
      `merged record-tree inventory exceeds ${MAX_RECORD_TREE_ENTRIES} entries`,
    );
  }

  const raw = runEngineGit(
    context,
    ['ls-tree', '-r', '-z', expected, '--', recordRoot],
    {
      encoding: null,
      label: `read exact first-parent ${recordRoot}`,
    },
  );
  if (raw.length > MAX_RECORD_TREE_BYTES) {
    throw new GitRecordError(
      'RECORD_TREE_INVENTORY_LIMIT',
      `first-parent record-tree inventory exceeds ${MAX_RECORD_TREE_BYTES} bytes`,
    );
  }
  const entries = splitNul(raw)
    .filter((entry) => entry.length > 0)
    .map((entry) => parseTreeEntry(entry));
  if (entries.length > MAX_RECORD_TREE_ENTRIES) {
    throw new GitRecordError(
      'RECORD_TREE_INVENTORY_LIMIT',
      `first-parent record-tree inventory exceeds ${MAX_RECORD_TREE_ENTRIES} entries`,
    );
  }
  if (indexed.length > 0 || entries.length > 0) {
    const decoder = new TextDecoder('utf-8', { fatal: true });
    let removals;
    try {
      removals = indexed.map((entry) => (
        `0 ${'0'.repeat(expected.length)}\t${assertRepositoryPath(decoder.decode(entry))}\0`
      ));
    } catch (error) {
      if (error instanceof GitRecordError) throw error;
      throw new GitRecordError(
        'INVALID_REPOSITORY_PATH',
        `merged ${recordRoot} contains a non-canonical path`,
        error,
      );
    }
    const replacements = entries.map((entry) => (
      `${entry.mode} ${entry.type} ${entry.object}\t${assertRepositoryPath(entry.path)}\0`
    ));
    const input = Buffer.from([...removals, ...replacements].join(''));
    runEngineGit(
      context,
      ['update-index', '-z', '--index-info'],
      {
        encoding: null,
        input,
        label: `restore exact first-parent ${recordRoot}`,
      },
    );
  }
  return runEngineGit(
    context,
    ['write-tree'],
    { label: `write ${label} tree with first-parent records` },
  ).trim();
}

function normalizedCandidateForRecordRoot(
  context,
  expected,
  candidate,
  recordRoot,
  label,
) {
  runEngineGit(
    context,
    ['read-tree', candidate],
    { label: `seed ${label} candidate record-root normalization` },
  );
  const tree = restoreFirstParentRecordRoot(
    context,
    expected,
    recordRoot,
    `${label} candidate`,
  );
  const timestamp = commitTimestamp(context.cwd, candidate);
  const date = `@${timestamp} +0000`;
  const parents = commitParents(context.cwd, candidate)
    .flatMap((parent) => ['-p', parent]);
  return runEngineGit(
    context,
    ['commit-tree', tree, ...parents],
    {
      input: `Baton engine-owned record normalization for ${candidate}\n`,
      env: {
        GIT_AUTHOR_NAME: 'Baton Merge',
        GIT_AUTHOR_EMAIL: 'merge@baton.invalid',
        GIT_AUTHOR_DATE: date,
        GIT_COMMITTER_NAME: 'Baton Merge',
        GIT_COMMITTER_EMAIL: 'merge@baton.invalid',
        GIT_COMMITTER_DATE: date,
      },
      label: `create ${label} candidate with first-parent records`,
    },
  ).trim();
}

function sameRecordRootEntry(context, expected, candidate, recordRoot) {
  const raw = runEngineGit(
    context,
    [
      'diff-tree',
      '--no-commit-id',
      '--raw',
      '-z',
      expected,
      candidate,
      '--',
      recordRoot,
    ],
    {
      encoding: null,
      label: `compare exact ${recordRoot} tree entries`,
    },
  );
  if (raw.length > MAX_RECORD_TREE_BYTES) {
    throw new GitRecordError(
      'RECORD_TREE_INVENTORY_LIMIT',
      `record-root comparison exceeds ${MAX_RECORD_TREE_BYTES} bytes`,
    );
  }
  return raw.length === 0;
}

function exactBaseMergeSide(context, source, productBase, side) {
  const tree = runEngineGit(
    context,
    ['rev-parse', '--verify', `${source}^{tree}`],
    { label: `resolve exact-base ${side} tree` },
  ).trim();
  const date = `@${commitTimestamp(context.cwd, productBase) + 1} +0000`;
  return runEngineGit(
    context,
    ['commit-tree', tree, '-p', productBase],
    {
      input: `Baton exact-base ${side} for ${source}\n`,
      env: {
        GIT_AUTHOR_NAME: 'Baton Merge',
        GIT_AUTHOR_EMAIL: 'merge@baton.invalid',
        GIT_AUTHOR_DATE: date,
        GIT_COMMITTER_NAME: 'Baton Merge',
        GIT_COMMITTER_EMAIL: 'merge@baton.invalid',
        GIT_COMMITTER_DATE: date,
      },
      label: `create exact-base ${side}`,
    },
  ).trim();
}

function deterministicMergeTreeInContext(
  context,
  expected,
  candidate,
  label,
  productBase = null,
  recordRoot = null,
) {
  runEngineGit(
    context,
    ['read-tree', expected],
    { label: `seed engine-owned merge index at ${expected}` },
  );
  installBuiltInMergeAttributes(context, expected, candidate, productBase);
  const recordRootsMatch = recordRoot !== null
    && sameRecordRootEntry(context, expected, candidate, recordRoot);
  const mergeCandidate = recordRoot === null || recordRootsMatch
    ? candidate
    : normalizedCandidateForRecordRoot(
      context,
      expected,
      candidate,
      recordRoot,
      label,
    );
  const mergeExpected = productBase === null
    ? expected
    : exactBaseMergeSide(context, expected, productBase, 'expected');
  const mergePassed = productBase === null
    ? mergeCandidate
    : exactBaseMergeSide(context, mergeCandidate, productBase, 'candidate');
  const mergedTree = runEngineMergeTree(
    context,
    [
      'merge-tree',
      '--write-tree',
      '--no-messages',
      mergeExpected,
      mergePassed,
    ],
    `compute deterministic ${label} tree`,
  ).trim();
  if (recordRoot === null || recordRootsMatch) return mergedTree;
  runEngineGit(
    context,
    ['read-tree', mergedTree],
    { label: `seed ${label} record-root restoration` },
  );
  return restoreFirstParentRecordRoot(context, expected, recordRoot, label);
}

function deterministicMergeTree(repo, expected, candidate, label, recordRoot = null) {
  return withEngineGitContext(
    repo,
    (context) => deterministicMergeTreeInContext(
      context,
      expected,
      candidate,
      label,
      null,
      recordRoot,
    ),
  );
}

function verifyExactComposition(
  repo,
  expectedTarget,
  candidate,
  observedResult,
  label,
  recordRoot = null,
) {
  const expected = resolveRef(repo, expectedTarget);
  const passed = resolveRef(repo, candidate);
  const observed = resolveRef(repo, observedResult);
  if (observed === passed && isAncestor(repo, expected, passed)) {
    return { mode: 'fast-forward', expected, candidate: passed, observed };
  }
  const parents = commitParents(repo, observed);
  if (
    parents.length === 2
    && parents[0] === expected
    && parents[1] === passed
    && isAncestor(repo, expected, observed)
    && isAncestor(repo, passed, observed)
  ) {
    const deterministicTree = deterministicMergeTree(
      repo,
      expected,
      passed,
      label,
      recordRoot,
    );
    const observedTree = runGit(repo, ['rev-parse', `${observed}^{tree}`], {
      label: `resolve ${label} result tree`,
    }).trim();
    if (observedTree !== deterministicTree) {
      throw new GitRecordError(
        'FORGED_COMPOSITION_TREE',
        `${label} result ${observed} has the expected parents but not the deterministic merge tree`,
      );
    }
    return { mode: 'two-parent', expected, candidate: passed, observed };
  }
  throw new GitRecordError(
    'UNEXPECTED_COMPOSITION_TOPOLOGY',
    `${label} result ${observed} is neither the exact fast-forward nor a two-parent composition of ${expected} and ${passed}`,
  );
}

export function verifyTrackComposition(repo, expectedReleaseHead, frozenTrackHead, observedResult) {
  return verifyExactComposition(
    repo,
    expectedReleaseHead,
    frozenTrackHead,
    observedResult,
    'track composition',
    RECORD_ROOT_V1,
  );
}

export function verifyReleaseIntegration(repo, expectedTarget, assemblyCandidate, observedResult) {
  return verifyExactComposition(
    repo,
    expectedTarget,
    assemblyCandidate,
    observedResult,
    'release integration',
    RECORD_ROOT_V1,
  );
}

function commitTimestamp(repo, commit) {
  const rendered = runGit(repo, ['show', '-s', '--format=%ct', commit], {
    label: `read timestamp for ${commit}`,
  }).trim();
  const parsed = Number.parseInt(rendered, 10);
  if (!Number.isSafeInteger(parsed) || parsed < 0) {
    throw new GitRecordError('INVALID_COMMIT_TIMESTAMP', `invalid Git timestamp ${rendered}`);
  }
  return parsed;
}

/**
 * Create one deterministic metadata-only child without moving a ref.
 *
 * Baton receipts live in commit messages, so their commit must reuse the
 * parent's exact tree. The caller still has to compare-and-swap the intended
 * owner ref with unsafeAtomicUpdateRefs.
 */
export function unsafePrepareMetadataCommit(repo, {
  expectedHead,
  message,
}) {
  const expected = resolveRef(repo, expectedHead);
  const input = Buffer.from(message);
  if (
    input.byteLength === 0
    || input.byteLength > 12_288
    || input.includes(0)
    || input.includes(0x0d)
  ) {
    throw new GitRecordError(
      'INVALID_COMMIT_MESSAGE',
      'metadata commit message must be 1-12288 bytes of LF-only data without NUL',
    );
  }
  const tree = runGit(repo, ['rev-parse', '--verify', `${expected}^{tree}`], {
    label: 'resolve metadata commit tree',
  }).trim();
  const date = `@${commitTimestamp(repo, expected) + 1} +0000`;
  const commit = runGit(repo, ['commit-tree', tree, '-p', expected], {
    input,
    env: {
      GIT_AUTHOR_NAME: 'Baton Receipts',
      GIT_AUTHOR_EMAIL: 'receipts@baton.invalid',
      GIT_AUTHOR_DATE: date,
      GIT_COMMITTER_NAME: 'Baton Receipts',
      GIT_COMMITTER_EMAIL: 'receipts@baton.invalid',
      GIT_COMMITTER_DATE: date,
    },
    label: 'create metadata receipt commit',
  }).trim();
  const parents = commitParents(repo, commit);
  if (
    parents.length !== 1
    || parents[0] !== expected
    || runGit(repo, ['rev-parse', '--verify', `${commit}^{tree}`], {
      label: 'verify metadata commit tree',
    }).trim() !== tree
  ) {
    throw new GitRecordError(
      'INVALID_METADATA_COMMIT',
      'prepared metadata commit did not preserve its exact parent and tree',
    );
  }
  return Object.freeze({ expected, tree, commit });
}

/**
 * Return bounded first-parent commit envelopes in newest-first order.
 * Commit messages cannot contain NUL, so the closed NUL protocol is
 * unambiguous and avoids one Git process per receipt.
 */
export function readFirstParentHistory(repo, head, { maxCount = 4096 } = {}) {
  if (!Number.isSafeInteger(maxCount) || maxCount < 1 || maxCount > 4096) {
    throw new GitRecordError(
      'INVALID_HISTORY_LIMIT',
      'first-parent history limit must be an integer from 1 to 4096',
    );
  }
  const exact = resolveRef(repo, head);
  const raw = runGit(
    repo,
    [
      'log',
      '--first-parent',
      `--max-count=${maxCount}`,
      '-z',
      '--format=%H%x00%P%x00%T%x00%B%x00',
      exact,
    ],
    {
      encoding: null,
      maxBuffer: 32 * 1024 * 1024,
      label: 'read first-parent receipt history',
    },
  );
  let rendered;
  try {
    rendered = new TextDecoder('utf-8', { fatal: true }).decode(raw);
  } catch (error) {
    throw new GitRecordError(
      'MALFORMED_GIT_OUTPUT',
      'first-parent history was not valid UTF-8',
      error,
    );
  }
  const fields = rendered.split('\x00');
  if (fields.at(-1) !== '') {
    throw new GitRecordError(
      'MALFORMED_GIT_OUTPUT',
      'first-parent history was not terminated',
    );
  }
  fields.pop();
  const records = [];
  while (fields.length > 0) {
    if (fields.length < 5) {
      throw new GitRecordError(
        'MALFORMED_GIT_OUTPUT',
        'first-parent history envelope is malformed',
      );
    }
    const [oid, parentsRaw, tree, message, separator] = fields.splice(0, 5);
    if (separator !== '') {
      throw new GitRecordError(
        'MALFORMED_GIT_OUTPUT',
        'first-parent history record separator is malformed',
      );
    }
    if (
      !/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(oid)
      || !/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(tree)
    ) {
      throw new GitRecordError(
        'MALFORMED_GIT_OUTPUT',
        'first-parent history contains an invalid object identity',
      );
    }
    const parents = parentsRaw === '' ? [] : parentsRaw.split(' ');
    if (!parents.every((parent) => /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(parent))) {
      throw new GitRecordError(
        'MALFORMED_GIT_OUTPUT',
        'first-parent history contains an invalid parent identity',
      );
    }
    records.push(Object.freeze({
      oid,
      parents: Object.freeze(parents),
      tree,
      message: Buffer.from(message, 'utf8'),
    }));
  }
  return Object.freeze(records);
}

function deterministicCompositionCommit(context, repo, targetRef, expected, candidate, tree) {
  const timestamp = Math.max(
    commitTimestamp(repo, expected),
    commitTimestamp(repo, candidate),
  ) + 1;
  const date = `@${timestamp} +0000`;
  return runEngineGit(
    context,
    ['commit-tree', tree, '-p', expected, '-p', candidate],
    {
      input: `Baton exact composition of ${candidate} into ${targetRef}\n`,
      env: {
        GIT_AUTHOR_NAME: 'Baton Merge',
        GIT_AUTHOR_EMAIL: 'merge@baton.invalid',
        GIT_AUTHOR_DATE: date,
        GIT_COMMITTER_NAME: 'Baton Merge',
        GIT_COMMITTER_EMAIL: 'merge@baton.invalid',
        GIT_COMMITTER_DATE: date,
      },
      label: 'create deterministic composition commit',
    },
  ).trim();
}

function validateCompositionTargetRef(repo, targetRef) {
  runGit(repo, ['check-ref-format', targetRef], {
    code: 'INVALID_TARGET_REF',
    label: `validate target ref ${targetRef}`,
  });
  if (!targetRef.startsWith('refs/heads/')) {
    throw new GitRecordError(
      'INVALID_TARGET_REF',
      'composition target must be a full refs/heads ref',
    );
  }
}

function prepareTwoParentComposition(
  repo,
  targetRef,
  expected,
  candidate,
  productBase,
  recordRoot,
) {
  return withEngineGitContext(repo, (context) => {
    const tree = deterministicMergeTreeInContext(
      context,
      expected,
      candidate,
      'composition',
      productBase,
      recordRoot,
    );
    const result = deterministicCompositionCommit(
      context,
      repo,
      targetRef,
      expected,
      candidate,
      tree,
    );
    return Object.freeze({ result, tree });
  });
}

function verifyPreparedComposition(repo, expected, candidate, prepared, label) {
  const parents = commitParents(repo, prepared.result);
  if (
    parents.length !== 2
    || parents[0] !== expected
    || parents[1] !== candidate
  ) {
    throw new GitRecordError(
      'UNEXPECTED_COMPOSITION_TOPOLOGY',
      `${label} result ${prepared.result} does not preserve the exact ordered parents`,
    );
  }
  const observedTree = runGit(repo, ['rev-parse', `${prepared.result}^{tree}`], {
    label: `resolve ${label} result tree`,
  }).trim();
  if (observedTree !== prepared.tree) {
    throw new GitRecordError(
      'FORGED_COMPOSITION_TREE',
      `${label} result ${prepared.result} does not preserve its deterministic tree`,
    );
  }
}

function boundedFirstParentContains(repo, head, expected) {
  const rendered = runGit(
    repo,
    ['rev-list', '--first-parent', '--max-count=4096', head],
    {
      maxBuffer: 512 * 1024,
      label: 'read bounded first-parent identities',
    },
  );
  if (!rendered.endsWith('\n')) {
    throw new GitRecordError(
      'MALFORMED_GIT_OUTPUT',
      'bounded first-parent identities were not terminated',
    );
  }
  const identities = rendered.slice(0, -1).split('\n');
  if (
    identities.length < 1
    || identities.length > 4096
    || !identities.every((oid) => /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(oid))
  ) {
    throw new GitRecordError(
      'MALFORMED_GIT_OUTPUT',
      'bounded first-parent identities were malformed',
    );
  }
  return identities.includes(expected);
}

export function unsafePrepareExactComposition(repo, {
  targetRef,
  expectedHead,
  candidate,
}) {
  validateCompositionTargetRef(repo, targetRef);
  const expected = resolveRef(repo, expectedHead);
  const passed = resolveRef(repo, candidate);
  productTreeIdentity(repo, expected);
  productTreeIdentity(repo, passed);
  const recordRoot = RECORD_ROOT_V1;
  let mode;
  let result;
  if (isAncestor(repo, expected, passed)) {
    mode = 'fast-forward';
    result = passed;
  } else if (isAncestor(repo, passed, expected)) {
    throw new GitRecordError(
      'CANDIDATE_ALREADY_CONTAINED',
      `candidate ${passed} is already contained by expected target ${expected}`,
    );
  } else {
    mode = 'two-parent';
    const prepared = prepareTwoParentComposition(
      repo,
      targetRef,
      expected,
      passed,
      null,
      recordRoot,
    );
    verifyPreparedComposition(repo, expected, passed, prepared, 'composition');
    result = prepared.result;
  }
  productTreeIdentity(repo, result);
  return Object.freeze({
    mode,
    expected,
    candidate: passed,
    result,
  });
}

/**
 * Retry a conflicting exact composition from one engine-derived product base.
 *
 * This is an internal reference-engine primitive. Public actions never accept
 * a merge base: state reconstruction derives and validates the exact base
 * before calling this function.
 */
export function unsafePrepareProductComposition(repo, {
  targetRef,
  expectedHead,
  candidate,
  productBase,
}) {
  if (typeof productBase !== 'function') {
    throw new GitRecordError(
      'PRODUCT_BASE_RESOLVER_REQUIRED',
      'product composition requires an engine-owned lazy product-base resolver',
    );
  }
  try {
    const prepared = unsafePrepareExactComposition(repo, {
      targetRef,
      expectedHead,
      candidate,
    });
    return prepared;
  } catch (error) {
    if (!(error instanceof GitRecordError) || error.code !== 'COMPOSITION_CONFLICT') {
      throw error;
    }
  }

  validateCompositionTargetRef(repo, targetRef);
  const expected = resolveRef(repo, expectedHead);
  const passed = resolveRef(repo, candidate);
  productTreeIdentity(repo, expected);
  productTreeIdentity(repo, passed);
  const suppliedBase = assertObjectIdForFormat(
    productBase(),
    'product base',
    expected.length === 64 ? 'sha256' : 'sha1',
  );
  const base = resolveRef(repo, suppliedBase);
  productTreeIdentity(repo, base);
  const recordRoot = RECORD_ROOT_V1;
  const prepared = prepareTwoParentComposition(
    repo,
    targetRef,
    expected,
    passed,
    base,
    recordRoot,
  );
  verifyPreparedComposition(repo, expected, passed, prepared, 'composition');
  const { result } = prepared;
  productTreeIdentity(repo, result);
  return Object.freeze({
    mode: 'two-parent',
    expected,
    candidate: passed,
    productBase: base,
    result,
  });
}

// Keep current authority as the first parent. Add the approved target only
// when that authority does not already contain it.
export function unsafePrepareApprovedTargetBase(repo, {
  targetRef,
  expectedHead,
  approvedTarget,
}) {
  const expected = resolveRef(repo, expectedHead);
  const target = resolveRef(repo, approvedTarget);
  if (target === expected || isAncestor(repo, target, expected)) return expected;
  if (isAncestor(repo, expected, target)) {
    if (boundedFirstParentContains(repo, target, expected)) {
      return unsafePrepareExactComposition(repo, {
        targetRef,
        expectedHead: expected,
        candidate: target,
      }).result;
    }
    validateCompositionTargetRef(repo, targetRef);
    productTreeIdentity(repo, expected);
    productTreeIdentity(repo, target);
    const recordRoot = RECORD_ROOT_V1;
    const prepared = prepareTwoParentComposition(
      repo,
      targetRef,
      expected,
      target,
      null,
      recordRoot,
    );
    verifyPreparedComposition(repo, expected, target, prepared, 'composition');
    const { result } = prepared;
    productTreeIdentity(repo, result);
    return result;
  }
  return unsafePrepareExactComposition(repo, {
    targetRef,
    expectedHead: expected,
    candidate: target,
  }).result;
}

export function unsafeApplyExactComposition(repo, options) {
  const prepared = unsafePrepareExactComposition(repo, options);
  const {
    targetRef,
  } = options;
  const {
    mode,
    expected,
    candidate: passed,
    result,
  } = prepared;
  const current = resolveRef(repo, targetRef);
  if (current === result) {
    return Object.freeze({
      mode,
      expected,
      candidate: passed,
      result,
      changed: false,
    });
  }
  if (current !== expected) {
    throw new GitRecordError(
      'STALE_TARGET',
      `expected ${targetRef} at ${expected}, observed ${current}`,
    );
  }
  unsafeAtomicUpdateRefs(repo, [{
    kind: 'update',
    ref: targetRef,
    newHead: result,
    expectedHead: expected,
  }]);
  return Object.freeze({
    mode,
    expected,
    candidate: passed,
    result,
    changed: true,
  });
}

export function assertStructuralRecordOnlyTransition(
  repo,
  before,
  after,
  admission,
  expectedPaths = [],
) {
  const root = requireRecordPathAdmission(repo, admission);
  const exactBefore = resolveRef(repo, before);
  const exactAfter = resolveRef(repo, after);
  const parents = commitParents(repo, exactAfter);
  if (parents.length !== 1 || parents[0] !== exactBefore) {
    throw new GitRecordError(
      'UNEXPECTED_RECORD_TRANSITION',
      `record transition ${exactAfter} is not a direct child of ${exactBefore}`,
    );
  }
  try {
    assertRecordRootAtRef(repo, exactAfter, root);
  } catch (error) {
    if (
      error instanceof GitRecordError
      && [
        'RECORD_ROOT_NOT_FOUND',
        'INVALID_RECORD_ROOT',
        'SYMLINKED_RECORD_ROOT',
      ].includes(error.code)
    ) {
      throw new GitRecordError(
        'RECORD_ROOT_REPLACED',
        `record transition deleted or replaced ${root}`,
        error,
      );
    }
    throw error;
  }
  const paths = changedPathsBetween(repo, exactBefore, exactAfter);
  if (paths.some((changedPath) => !recordPathAllowed(changedPath, root))) {
    throw new GitRecordError('NON_RECORD_CHANGE', 'record transition contains a product path');
  }
  const expected = [...expectedPaths]
    .map(assertRepositoryPath)
    .sort((left, right) => Buffer.from(left).compare(Buffer.from(right)));
  if (expected.length > 0 && !isDeepStringArray(paths, expected)) {
    throw new GitRecordError(
      'INCOMPLETE_RECORD_TRANSITION',
      `record transition paths ${JSON.stringify(paths)} do not equal ${JSON.stringify(expected)}`,
    );
  }
  return { before: exactBefore, after: exactAfter, paths };
}

function isDeepStringArray(left, right) {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

function recordPathAllowed(relativePath, recordRoot) {
  return relativePath === recordRoot || relativePath.startsWith(`${recordRoot}/`);
}

function assertRecordRootPreservedInTree(repo, tree, root) {
  const raw = runGit(repo, ['ls-tree', '-z', tree, '--', root], {
    encoding: null,
    label: `inspect preserved record root ${root}`,
  });
  if (raw.length === 0) {
    throw new GitRecordError(
      'RECORD_ROOT_REPLACED',
      `record transition deleted ${root}`,
    );
  }
  const nul = raw.indexOf(0);
  if (nul < 0 || nul !== raw.length - 1) {
    throw new GitRecordError(
      'MALFORMED_GIT_TREE',
      `ambiguous tree entry for ${root}`,
    );
  }
  const entry = parseTreeEntry(raw.subarray(0, nul));
  if (entry.path !== root || entry.type !== 'tree' || entry.mode === '120000') {
    throw new GitRecordError(
      'RECORD_ROOT_REPLACED',
      `record transition replaced ${root}`,
    );
  }
}

export function unsafePrepareRecordTransition(repo, {
  expectedHead,
  message,
  recordPathAdmission,
  changes,
}) {
  const root = requireRecordPathAdmission(repo, recordPathAdmission);
  const expected = resolveRef(repo, expectedHead);
  const expectedProduct = productTreeIdentity(repo, expected);
  assertRecordRootAtRef(repo, expected, root, { allowMissing: true });
  if (!changes || typeof changes !== 'object' || Array.isArray(changes)) {
    throw new GitRecordError('EMPTY_RECORD_TRANSITION', 'a record transition requires at least one path change');
  }
  const changePaths = Object.keys(changes);
  if (changePaths.length === 0) {
    throw new GitRecordError('EMPTY_RECORD_TRANSITION', 'a record transition requires at least one path change');
  }
  if (changePaths.length > MAX_RECORD_CHANGES) {
    throw new GitRecordError(
      'RECORD_CHANGE_LIMIT',
      `a record transition may change at most ${MAX_RECORD_CHANGES} paths`,
    );
  }
  if (typeof message !== 'string' || message.trim().length === 0) {
    throw new GitRecordError('INVALID_COMMIT_MESSAGE', 'a record transition requires a non-empty commit message');
  }
  const commitMessage = message.trim();
  if (Buffer.byteLength(commitMessage, 'utf8') > MAX_RECORD_MESSAGE_BYTES) {
    throw new GitRecordError(
      'COMMIT_MESSAGE_LIMIT',
      `record transition messages may be at most ${MAX_RECORD_MESSAGE_BYTES} UTF-8 bytes`,
    );
  }
  const preparedChanges = [];
  let aggregateBytes = 0;
  for (const relativePath of changePaths) {
    if (
      path.isAbsolute(relativePath)
      || relativePath.includes('\\')
      || relativePath.split('/').some((segment) => segment === '' || segment === '.' || segment === '..')
      || relativePath === root
      || !recordPathAllowed(relativePath, root)
    ) {
      throw new GitRecordError(
        'NON_RECORD_CHANGE',
        `record transition attempted to change non-record path ${relativePath}`,
      );
    }
    const value = changes[relativePath];
    let byteLength = 0;
    if (value !== null) {
      if (Buffer.isBuffer(value)) {
        byteLength = value.byteLength;
      } else if (typeof value === 'string') {
        byteLength = Buffer.byteLength(value, 'utf8');
      } else {
        throw new GitRecordError(
          'INVALID_RECORD_VALUE',
          `record value for ${relativePath} must be a string, Buffer, or null`,
        );
      }
      if (byteLength > MAX_RECORD_VALUE_BYTES) {
        throw new GitRecordError(
          'RECORD_VALUE_LIMIT',
          `record value for ${relativePath} exceeds ${MAX_RECORD_VALUE_BYTES} bytes`,
        );
      }
      aggregateBytes += byteLength;
      if (aggregateBytes > MAX_RECORD_TOTAL_BYTES) {
        throw new GitRecordError(
          'RECORD_TOTAL_LIMIT',
          `record transition values exceed ${MAX_RECORD_TOTAL_BYTES} aggregate bytes`,
        );
      }
    }
    preparedChanges.push([relativePath, value]);
  }

  const temporary = mkdtempSync(path.join(tmpdir(), 'baton-record-index-'));
  const indexFile = path.join(temporary, 'index');
  const env = { GIT_INDEX_FILE: indexFile };
  try {
    runGit(repo, ['read-tree', expected], { env, label: 'seed record transition index' });
    for (const [relativePath, value] of preparedChanges) {
      if (value === null) {
        runGit(repo, ['update-index', '--force-remove', '--', relativePath], {
          env,
          label: `remove record ${relativePath}`,
        });
        continue;
      }
      const bytes = Buffer.isBuffer(value) ? value : Buffer.from(value, 'utf8');
      const object = runGit(repo, ['hash-object', '-w', '--stdin'], {
        input: bytes,
        label: `write record blob ${relativePath}`,
      }).trim();
      runGit(repo, ['update-index', '--add', '--cacheinfo', `100644,${object},${relativePath}`], {
        env,
        label: `stage record ${relativePath}`,
      });
    }
    const tree = runGit(repo, ['write-tree'], { env, label: 'write record transition tree' }).trim();
    assertRecordRootPreservedInTree(repo, tree, root);
    const date = `@${commitTimestamp(repo, expected) + 1} +0000`;
    const commit = runGit(repo, ['commit-tree', tree, '-p', expected], {
      input: `${commitMessage}\n`,
      env: {
        GIT_AUTHOR_NAME: 'Baton Records',
        GIT_AUTHOR_EMAIL: 'records@baton.invalid',
        GIT_AUTHOR_DATE: date,
        GIT_COMMITTER_NAME: 'Baton Records',
        GIT_COMMITTER_EMAIL: 'records@baton.invalid',
        GIT_COMMITTER_DATE: date,
      },
      label: 'create record transition commit',
    }).trim();
    const nextProduct = productTreeIdentity(repo, commit);
    if (nextProduct.productTree !== expectedProduct.productTree) {
      throw new GitRecordError(
        'PRODUCT_CHANGED_DURING_RECORD_TRANSITION',
        'record transition changed product identity',
      );
    }
    return Object.freeze({
      expected,
      commit,
      paths: Object.freeze(preparedChanges.map(([relativePath]) => relativePath).sort()),
    });
  } finally {
    rmSync(temporary, { recursive: true, force: true });
  }
}

export function unsafeCommitRecordTransition(repo, {
  ref,
  expectedHead,
  message,
  recordPathAdmission,
  changes,
  createRef,
}) {
  const exactRef = assertExactHeadRef(ref);
  let ownerRef;
  if (createRef !== undefined) {
    if (
      createRef === null
      || typeof createRef !== 'object'
      || Array.isArray(createRef)
      || Object.keys(createRef).length !== 1
      || typeof createRef.ref !== 'string'
    ) {
      throw new GitRecordError(
        'INVALID_CREATE_REF',
        'createRef must be exactly { ref: \"refs/heads/...\" }',
      );
    }
    ownerRef = assertExactHeadRef(createRef.ref);
    if (ownerRef === exactRef) {
      throw new GitRecordError(
        'INVALID_CREATE_REF',
        'createRef must differ from the updated record ref',
      );
    }
  }
  const expected = resolveRef(repo, expectedHead);
  const current = resolveRef(repo, exactRef);
  if (current !== expected) {
    throw new GitRecordError(
      'STALE_WRITER',
      `expected ${exactRef} at ${expected}, observed ${current}`,
    );
  }
  const prepared = unsafePrepareRecordTransition(repo, {
    expectedHead: expected,
    message,
    recordPathAdmission,
    changes,
  });
  const operations = [{
    kind: 'update',
    ref: exactRef,
    newHead: prepared.commit,
    expectedHead: expected,
  }];
  if (ownerRef) {
    operations.push({ kind: 'create', ref: ownerRef, newHead: prepared.commit });
  }
  try {
    unsafeAtomicUpdateRefs(repo, operations);
  } catch (error) {
    const code = ownerRef ? 'ATOMIC_REF_UPDATE_FAILED' : 'STALE_WRITER';
    const message = (
      error instanceof GitRecordError
      && error.message.includes('ambiguous outcome')
    )
      ? `record ref transaction has an ambiguous outcome for ${exactRef}; `
        + 'authoritative recovery is required before retry'
      : `record ref transaction lost for ${exactRef}`;
    throw new GitRecordError(code, message, error);
  }
  return prepared.commit;
}

if (
  process.argv.length === 3
  && process.argv[1] === fileURLToPath(import.meta.url)
  && process.argv[2] === REF_HELPER_MARKER
) {
  await runExactRefHelperMain();
}
