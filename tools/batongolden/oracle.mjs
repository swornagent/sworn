#!/usr/bin/env node

import { createHash } from 'node:crypto';
import {
  chmodSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '../..');
const referenceRoot = path.join(
  root,
  'internal/baton/snapshot/assets/reference/records',
);
const {
  canonicalJSON,
  digestBytes,
  parsePlanBytes,
  parseReceiptBytes,
  parseReceiptCommitMessage,
  renderReceiptCommit,
  strictParseJSON,
} = await import(path.join(referenceRoot, 'receipts.mjs'));
const { createBatonActions } = await import(path.join(referenceRoot, 'actions.mjs'));
const { readBatonState } = await import(path.join(referenceRoot, 'state.mjs'));
const {
  configureEngineGitExecutable,
  productTreeIdentity,
  readFirstParentHistory,
} = await import(path.join(referenceRoot, 'git.mjs'));

const git = '/usr/bin/git';
const gitIdentity = Object.freeze({
  name: 'Golden Baton Engine',
  email: 'engine@example.test',
});
configureEngineGitExecutable(git);

const output = path.resolve(process.argv[2] ?? path.join(here, 'testdata/corpus'));
mkdirSync(output, { recursive: true });

function sha256(value) {
  return createHash('sha256').update(value).digest('hex');
}

function writeJSON(name, value) {
  const body = `${JSON.stringify(value, null, 2)}\n`;
  const target = path.join(output, name);
  writeFileSync(target, body, { mode: 0o644 });
  chmodSync(target, 0o644);
  return { file: name, sha256: sha256(body), bytes: Buffer.byteLength(body) };
}

function runGit(repo, args, options = {}) {
  const home = path.join(repo, '.oracle-home');
  mkdirSync(home, { recursive: true });
  const environment = {
    HOME: home,
    XDG_CONFIG_HOME: path.join(home, 'xdg'),
    LANG: 'C',
    LC_ALL: 'C',
    GIT_CONFIG_NOSYSTEM: '1',
    GIT_CONFIG_SYSTEM: '/dev/null',
    GIT_CONFIG_GLOBAL: '/dev/null',
    GIT_TERMINAL_PROMPT: '0',
    ...(options.env ?? {}),
  };
  if (options.identity) {
    environment.GIT_AUTHOR_NAME = options.identity.name;
    environment.GIT_AUTHOR_EMAIL = options.identity.email;
    environment.GIT_AUTHOR_DATE = options.date;
    environment.GIT_COMMITTER_NAME = options.identity.name;
    environment.GIT_COMMITTER_EMAIL = options.identity.email;
    environment.GIT_COMMITTER_DATE = options.date;
  }
  const result = spawnSync(git, ['-C', repo, ...args], {
    input: options.input,
    encoding: options.binary ? null : 'utf8',
    env: environment,
    maxBuffer: 64 * 1024 * 1024,
  });
  if (result.status !== 0) {
    throw new Error(`git ${args.join(' ')} failed: ${String(result.stderr)}`);
  }
  return options.binary ? result.stdout : result.stdout.trim();
}

function createRepository(objectFormat) {
  const repo = mkdtempSync(path.join(tmpdir(), `sworn-baton-oracle-${objectFormat}-`));
  const init = spawnSync(git, ['init', '--quiet', `--object-format=${objectFormat}`, repo], {
    encoding: 'utf8',
    env: {
      HOME: path.join(repo, '.oracle-home'),
      XDG_CONFIG_HOME: path.join(repo, '.oracle-home', 'xdg'),
      LANG: 'C',
      LC_ALL: 'C',
      GIT_CONFIG_NOSYSTEM: '1',
      GIT_CONFIG_SYSTEM: '/dev/null',
      GIT_CONFIG_GLOBAL: '/dev/null',
    },
  });
  if (init.status !== 0) throw new Error(init.stderr);
  writeFileSync(path.join(repo, 'base.txt'), 'base\n');
  runGit(repo, ['add', '--', 'base.txt']);
  runGit(repo, ['commit', '--quiet', '-m', 'base'], {
    identity: gitIdentity,
    date: '1000000000 +0000',
  });
  runGit(repo, ['branch', '-M', 'main']);
  return repo;
}

function commitFile(repo, parent, ref, relativePath, contents, timestamp) {
  const privateDir = mkdtempSync(path.join(tmpdir(), 'sworn-baton-oracle-index-'));
  const index = path.join(privateDir, 'index');
  const env = { GIT_INDEX_FILE: index };
  try {
    runGit(repo, ['read-tree', `${parent}^{tree}`], { env });
    const blob = runGit(repo, ['hash-object', '-w', '--stdin'], {
      input: contents,
    });
    runGit(repo, ['update-index', '--add', '--cacheinfo', `100644,${blob},${relativePath}`], {
      env,
    });
    const tree = runGit(repo, ['write-tree'], { env });
    const commit = runGit(repo, ['commit-tree', tree, '-p', parent], {
      input: `product ${relativePath}\n`,
      date: `${timestamp} +0000`,
      identity: gitIdentity,
    });
    runGit(repo, ['update-ref', ref, commit, parent]);
    return commit;
  } finally {
    rmSync(privateDir, { recursive: true, force: true });
  }
}

function planBytes(release, tracks) {
  const metadata = {
    schema_version: 'baton.plan/v2',
    release,
    revision: 1,
    previous_plan: null,
    repository: 'golden/sworn',
    target_ref: 'refs/heads/main',
    approval_ref: `golden://approval/${release}/1`,
    tracks,
  };
  return Buffer.from(
    `\`\`\`baton-plan-v2\n${JSON.stringify(metadata, null, 2)}\n\`\`\`\n\nGolden plan.\n`,
  );
}

function slice(id, include, dependsOn = [], consumes = []) {
  return {
    id,
    outcome: `Deliver ${id}.`,
    scope: { include: [include], exclude: [] },
    acceptance: [{ id: `A-${id}`, text: `${id} is exact.` }],
    checks: [`check ${id}`],
    constraints: ['deterministic'],
    depends_on: dependsOn,
    consumes,
  };
}

function projectReceipt(receipt) {
  return JSON.parse(canonicalJSON(receipt));
}

function projectResult(result) {
  const value = {
    kind: result.kind,
    action: result.action,
    changed: result.changed,
  };
  for (const key of [
    'release', 'revision', 'plan', 'ref', 'head', 'target', 'slice',
    'direct', 'candidate', 'result_commit', 'receipt_commit',
  ]) {
    if (Object.hasOwn(result, key)) value[key] = result[key];
  }
  if (Object.hasOwn(result, 'inputs')) value.inputs = result.inputs;
  if (Object.hasOwn(result, 'receipt')) value.receipt = projectReceipt(result.receipt);
  if (Object.hasOwn(result, 'retirements')) {
    value.retirements = result.retirements.map((item) => ({
      slice: item.slice,
      receipt_commit: item.receipt_commit,
      receipt: projectReceipt(item.receipt),
    }));
  }
  return value;
}

function projectState(state) {
  return {
    release: state.release,
    plan: {
      oid: state.plan.oid,
      digest: state.plan.digest,
      revision: state.plan.metadata.revision,
      approval_oid: state.plan.approval_oid,
      target_stale: state.plan.target_stale,
      contracts: state.plan.metadata.contracts,
    },
    refs: {
      release: state.refs.release,
      target: state.refs.target,
      tracks: state.refs.tracks,
    },
    slices: state.slices.map((item) => ({
      id: item.location.slice.id,
      track: item.location.track.id,
      stage: item.stage,
      status: item.status,
      next_role: item.next_role,
      outcome: item.outcome,
      attempt: item.attempt,
      maximum_attempt: item.history.maximum_attempt,
      input_pins: item.input_pins,
      current_receipt: item.current_receipt?.oid ?? null,
      candidate: item.candidate?.receipt.candidate ?? null,
      pass: item.pass?.oid ?? null,
      retained: item.retained,
      stale_reason: item.stale_reason,
    })),
    assembly: {
      stage: state.assembly.stage,
      status: state.assembly.status,
      next_role: state.assembly.next_role,
      outcome: state.assembly.outcome,
      input_pins: state.assembly.input_pins,
      current_receipt: state.assembly.current_receipt?.oid ?? null,
      candidate: state.assembly.candidate?.receipt.candidate ?? null,
      pass: state.assembly.pass?.oid ?? null,
      stale_reason: state.assembly.stale_reason,
      result_commit: state.assembly.result_commit,
    },
    diagnostics: state.diagnostics.map((item) => ({
      code: item.code,
      release: item.release,
      track: item.track,
      work: item.work,
      message: item.message,
    })),
  };
}

function executeFlow(objectFormat) {
  const repo = createRepository(objectFormat);
  try {
    const release = `golden-${objectFormat}`;
    const tracks = [
      {
        id: 'T1',
        depends_on: [],
        slices: [slice('S1', 'one.txt')],
      },
      {
        id: 'T2',
        depends_on: [],
        slices: [slice('S2', 'two.txt')],
      },
    ];
    const plan = planBytes(release, tracks);
    const actions = createBatonActions({ repo, identity: gitIdentity });
    const results = [];
    const states = [];
    results.push(projectResult(actions.recordPlanRevision({
      planBytes: plan,
      summary: 'Approve the exact golden plan.',
      detail: Buffer.from('protected approval'),
    })));
    states.push({ label: 'approved', state: projectState(readBatonState(repo, release)) });

    for (const [track, sliceID, file, timestamp] of [
      ['T1', 'S1', 'one.txt', 1000000100],
      ['T2', 'S2', 'two.txt', 1000000200],
    ]) {
      results.push(projectResult(actions.appendReceipt({
        release,
        slice: sliceID,
        role: 'implementer',
        result: 'designed',
        summary: `Design ${sliceID}.`,
        detail: Buffer.from(`design ${sliceID}`),
      })));
      results.push(projectResult(actions.appendReceipt({
        release,
        slice: sliceID,
        role: 'captain',
        result: 'proceed',
        summary: `Proceed ${sliceID}.`,
        detail: Buffer.from(`review ${sliceID}`),
      })));
      const ref = `refs/heads/track/${release}/${track}`;
      const parent = runGit(repo, ['rev-parse', '--verify', ref]);
      const candidate = commitFile(repo, parent, ref, file, `${sliceID}\n`, timestamp);
      results.push(projectResult(actions.appendReceipt({
        release,
        slice: sliceID,
        role: 'implementer',
        result: 'candidate',
        summary: `Candidate ${sliceID}.`,
        detail: Buffer.from(`implementation ${sliceID}`),
        candidate,
        checkResults: Buffer.from(`checks ${sliceID}\n`),
      })));
      results.push(projectResult(actions.appendReceipt({
        release,
        slice: sliceID,
        role: 'verifier',
        result: 'pass',
        summary: `Pass ${sliceID}.`,
        detail: Buffer.from(`verification ${sliceID}`),
        candidate,
        checkResults: Buffer.from(`fresh checks ${sliceID}\n`),
      })));
      states.push({ label: `passed-${sliceID}`, state: projectState(readBatonState(repo, release)) });
    }

    results.push(projectResult(actions.prepareAssembly({
      release,
      summary: 'Prepare exact assembly.',
      detail: Buffer.from('ordered composition'),
    })));
    let state = readBatonState(repo, release);
    states.push({ label: 'assembly-candidate', state: projectState(state) });
    const assemblyCandidate = state.assembly.candidate.receipt.candidate;
    results.push(projectResult(actions.appendReceipt({
      release,
      role: 'verifier',
      result: 'pass',
      summary: 'Pass exact assembly.',
      detail: Buffer.from('fresh assembly verification'),
      candidate: assemblyCandidate,
      checkResults: Buffer.from('assembly checks\n'),
    })));
    states.push({ label: 'assembly-pass', state: projectState(readBatonState(repo, release)) });
    results.push(projectResult(actions.mergePassedCandidate({
      release,
      summary: 'Merge exact passed assembly.',
      detail: Buffer.from('deterministic merge'),
    })));
    state = readBatonState(repo, release);
    states.push({ label: 'merged', state: projectState(state) });

    const target = runGit(repo, ['rev-parse', '--verify', 'refs/heads/main']);
    const releaseHead = runGit(repo, ['rev-parse', '--verify', `refs/heads/release-wt/${release}`]);
    const product = productTreeIdentity(repo, target);
    const history = readFirstParentHistory(repo, releaseHead).slice(0, 8).map((row) => ({
      oid: row.oid,
      parents: row.parents,
      tree: row.tree,
      message_sha256: sha256(row.message),
    }));
    return {
      actions: { object_format: objectFormat, results },
      state: { object_format: objectFormat, states },
      git: {
        object_format: objectFormat,
        target,
        release_head: releaseHead,
        target_tree: product.candidateTree,
        product_tree: product.productTree,
        product_entries: product.entries.map((entry) => ({
          path: entry.path,
          mode: entry.mode,
          type: entry.type,
          object: entry.object,
        })),
        first_parent_history: history,
      },
    };
  } finally {
    rmSync(repo, { recursive: true, force: true });
  }
}

function outcome(action) {
  try {
    return { ok: true, value: action() };
  } catch (error) {
    return { ok: false, code: error.code ?? error.name };
  }
}

const parserPlan = planBytes('golden-parser', [{
  id: 'T1',
  depends_on: [],
  slices: [slice('S1', 'one.txt')],
}]);
const parsedPlan = parsePlanBytes(parserPlan);
const oid = 'a'.repeat(40);
const contract = parsedPlan.metadata.contracts.S1;
const sliceID = 'S1';
const attempt = 1;
const receiptTemplate = {
  version: 1,
  release: 'golden-parser',
  slice: sliceID,
  role: 'implementer',
  result: 'designed',
  attempt,
  plan: oid,
  contract,
  binds: 'b'.repeat(40),
  detail: digestBytes(Buffer.alloc(0)),
  summary: 'Design exact parser fixture.',
};
const rendered = renderReceiptCommit({
  subject: 'baton(golden-parser/S1): implementer designed',
  detail: Buffer.from('golden detail'),
  receipt: receiptTemplate,
});
const receipts = {
  plan: {
    bytes_base64: parserPlan.toString('base64'),
    digest: parsedPlan.digest,
    metadata: parsedPlan.metadata,
    markdown: parsedPlan.markdown,
  },
  strict_json: [
    ['canonical', outcome(() => strictParseJSON(Buffer.from('{"a":1,"b":[true,null]}')))],
    ['duplicate', outcome(() => strictParseJSON(Buffer.from('{"a":1,"a":2}')))],
    ['fraction', outcome(() => strictParseJSON(Buffer.from('1.5')))],
    ['unsafe', outcome(() => strictParseJSON(Buffer.from('9007199254740992')))],
    ['lone_surrogate', outcome(() => strictParseJSON(Buffer.from('"\\ud800"')))],
  ].map(([name, result]) => ({ name, ...result })),
  receipt: {
    canonical: canonicalJSON(receiptTemplate),
    parsed: projectReceipt(parseReceiptBytes(Buffer.from(canonicalJSON(receiptTemplate)))),
    rendered_base64: rendered.toString('base64'),
    parsed_commit: {
      subject: parseReceiptCommitMessage(rendered).subject,
      detail_base64: parseReceiptCommitMessage(rendered).detail.toString('base64'),
      receipt: projectReceipt(parseReceiptCommitMessage(rendered).receipt),
    },
    noncanonical: outcome(() => parseReceiptBytes(Buffer.from(
      JSON.stringify(receiptTemplate),
    ))),
  },
  invalid_plans: [
    ['v1', outcome(() => parsePlanBytes(Buffer.from('```baton-plan-v1\n{}\n```\n')))],
    ['duplicate_fence', outcome(() => parsePlanBytes(Buffer.concat([parserPlan, Buffer.from('\n```\n')])) )],
  ].map(([name, result]) => ({ name, ...result })),
};

const flows = [executeFlow('sha1'), executeFlow('sha256')];
const files = [];
files.push(writeJSON('receipts.json', receipts));
files.push(writeJSON('actions.json', { flows: flows.map((flow) => flow.actions) }));
files.push(writeJSON('state.json', { flows: flows.map((flow) => flow.state) }));
files.push(writeJSON('git.json', { flows: flows.map((flow) => flow.git) }));

const references = ['actions.mjs', 'git.mjs', 'receipts.mjs', 'state.mjs'].map((name) => {
  const bytes = readFileSync(path.join(referenceRoot, name));
  return { file: name, sha256: sha256(bytes), bytes: bytes.length };
});
writeJSON('manifest.json', {
  schema: 'sworn.batongolden/v2',
  baton: '1.0.0-rc.14',
  generator: 'exact embedded Baton JavaScript reference',
  oracle_sha256: sha256(readFileSync(fileURLToPath(import.meta.url))),
  references,
  files,
});
