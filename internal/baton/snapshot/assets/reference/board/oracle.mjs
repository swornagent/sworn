#!/usr/bin/env node

import { pathToFileURL } from 'node:url';

import {
  GitRecordError,
  repositoryRoot,
  unsafeRunGit,
} from '../records/git.mjs';
import {
  BatonStateError,
  isBatonStateError,
  readBatonState,
} from '../records/state.mjs';

export const BOARD_VERSION = 'baton.board/v1';
export const GRAPH_VERSION = 'baton.graph/v1';

const RELEASE_PREFIX = 'refs/heads/release-wt/';
const MAX_RELEASES = 32;
const MAX_DIAGNOSTIC_TEXT = 1_000;
const OPERATION_BY_ROLE = Object.freeze({
  planner: 'baton-plan',
  implementer: 'baton-implement',
  captain: 'baton-design-review',
  verifier: 'baton-verify',
  merge: 'baton-merge',
});

function frozen(value) {
  if (value === null || typeof value !== 'object' || Object.isFrozen(value)) return value;
  for (const item of Object.values(value)) frozen(item);
  return Object.freeze(value);
}

function safeText(value, repository = '') {
  let text = typeof value === 'string' ? value : String(value ?? '');
  if (repository) text = text.split(repository).join('<repository>');
  return text
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f]/g, '\ufffd')
    .replace(/[\r\n\u2028\u2029]+/g, ' ')
    .slice(0, MAX_DIAGNOSTIC_TEXT);
}

function diagnostic(error, repository, context = {}) {
  const code = isBatonStateError(error) || error instanceof GitRecordError
    ? error.code
    : 'BOARD_PROJECTION_FAILED';
  return frozen({
    code,
    release: context.release ?? null,
    track: context.track ?? null,
    work: context.work ?? null,
    message: safeText(error?.message ?? 'board projection failed', repository),
  });
}

function parseReleaseListing(raw) {
  let text;
  try {
    text = new TextDecoder('utf-8', { fatal: true }).decode(raw);
  } catch (error) {
    throw new GitRecordError('MALFORMED_GIT_OUTPUT', 'release refs are not valid UTF-8', error);
  }
  const seen = new Set();
  const releases = text.split('\n').filter(Boolean).map((line) => {
    const [ref, head, type, ...extra] = line.split('\t');
    const release = ref?.slice(RELEASE_PREFIX.length);
    if (
      extra.length > 0
      || !ref?.startsWith(RELEASE_PREFIX)
      || !release
      || release.includes('/')
      || type !== 'commit'
      || !/^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(head ?? '')
      || seen.has(ref)
    ) {
      throw new GitRecordError('INVALID_RELEASE_REF', `invalid release ref ${ref ?? ''}`);
    }
    seen.add(ref);
    return frozen({ release, ref, head });
  });
  if (releases.length > MAX_RELEASES) {
    throw new GitRecordError('RESOURCE_LIMIT', `repository has more than ${MAX_RELEASES} releases`);
  }
  return frozen(releases.sort((left, right) => Buffer.from(left.ref).compare(Buffer.from(right.ref))));
}

export function discoverReleaseHeads(repo) {
  return parseReleaseListing(unsafeRunGit(repo, [
    'for-each-ref',
    '--format=%(refname)%09%(objectname)%09%(objecttype)',
    'refs/heads/release-wt',
  ], {
    encoding: null,
    label: 'discover Baton release refs',
  }));
}

function operation(operationRole, scope, release, track = null, work = null) {
  const operationName = OPERATION_BY_ROLE[operationRole];
  return operationName ? frozen({
    operation: operationName,
    scope,
    release,
    track,
    work,
  }) : null;
}

function source(entry, ref, mode = 'receipt') {
  return entry ? frozen({ mode, ref, head: entry.oid }) : null;
}

function blocker(item) {
  if (item.status !== 'blocked') return null;
  return frozen({
    code: item.outcome,
    summary: item.current_receipt?.receipt.summary ?? 'External judgement is required.',
  });
}

function dependencyReady(slice, states) {
  return [...new Set([...slice.depends_on, ...slice.consumes])]
    .every((id) => states.get(id)?.pass);
}

function projectTracks(state) {
  const bySlice = new Map(state.slices.map((slice) => [slice.location.slice.id, slice]));
  const trackReady = new Map(state.tracks.map((track) => [
    track.id,
    track.slices.every((slice) => slice.pass),
  ]));
  const tracks = [];
  const nextOperations = [];
  for (const track of state.tracks) {
    const blockers = track.depends_on.filter((id) => !trackReady.get(id));
    const firstIncomplete = track.slices.find((slice) => !slice.pass) ?? null;
    let nextOperation = null;
    if (
      !state.plan.target_stale
      && blockers.length === 0
      && firstIncomplete
      && dependencyReady(firstIncomplete.location.slice, bySlice)
    ) {
      nextOperation = operation(
        firstIncomplete.next_role,
        'work',
        state.release,
        track.id,
        firstIncomplete.location.slice.id,
      );
    }
    if (nextOperation) nextOperations.push(nextOperation);
    const ready = trackReady.get(track.id);
    const finalPass = track.slices.at(-1)?.pass ?? null;
    tracks.push(frozen({
      id: track.id,
      ref: track.ref,
      head: track.head,
      depends_on: frozen([...track.depends_on]),
      blockers: frozen(blockers),
      materialisation: track.head ? 'owner' : 'baseline',
      composition: ready ? (state.assembly.candidate ? 'composed' : 'ready') : 'pending',
      frozen_head: finalPass?.receipt.candidate ?? null,
      work: frozen(track.slices.map((slice) => {
        const planSlice = slice.location.slice;
        const applicable = nextOperation?.work === planSlice.id ? nextOperation : null;
        const mode = slice.retained
          ? 'retained'
          : slice.current_receipt?.receipt.role === 'planner' ? 'plan' : 'receipt';
        return frozen({
          id: planSlice.id,
          depends_on: frozen([...planSlice.depends_on]),
          consumes: frozen([...planSlice.consumes]),
          plan_revision: state.plan.metadata.revision,
          attempt: slice.attempt,
          stage: slice.stage,
          status: slice.status,
          next_role: slice.next_role,
          outcome: slice.outcome,
          blocker: blocker(slice),
          source: source(slice.current_receipt, track.ref, mode),
          next_operation: applicable,
        });
      })),
      next_operation: nextOperation,
    }));
  }
  return frozen({ tracks, next_operations: nextOperations, ready: trackReady });
}

function projectAssembly(state, allTracksReady) {
  const item = state.assembly;
  let nextOperation = null;
  if (!state.plan.target_stale && allTracksReady) {
    nextOperation = operation(item.next_role, 'assembly', state.release);
  }
  return frozen({
    stage: item.stage,
    status: item.status,
    next_role: item.next_role,
    outcome: item.outcome,
    blocker: blocker(item),
    source: source(item.current_receipt, state.refs.release.ref),
    next_operation: nextOperation,
  });
}

function sliceGraphState(slice, projected) {
  if (slice.retained) return 'retained';
  if (slice.pass) return 'passed';
  if (slice.outcome === 'stale') return 'stale';
  if (slice.status === 'blocked') return 'blocked';
  return projected.next_operation ? 'ready' : 'waiting';
}

function assemblyGraphState(item, nextOperation) {
  if (item.pass?.receipt && Object.hasOwn(item.pass.receipt, 'slice')) {
    return 'not_required';
  }
  if (item.pass) return 'passed';
  if (item.outcome === 'stale') return 'stale';
  if (item.status === 'blocked') return 'blocked';
  return nextOperation ? 'ready' : 'waiting';
}

function graphEdges(state, nodeRanks) {
  const edges = new Map();
  function add(from, to, kind) {
    const key = `${from}\u0000${to}`;
    if (!edges.has(key)) edges.set(key, { from, to, kinds: new Set() });
    edges.get(key).kinds.add(kind);
  }
  const sliceID = (work) => `slice:${work}`;
  const tracks = new Map(state.tracks.map((track) => [track.id, track]));

  for (const track of state.tracks) {
    for (let index = 1; index < track.slices.length; index += 1) {
      add(
        sliceID(track.slices[index - 1].location.slice.id),
        sliceID(track.slices[index].location.slice.id),
        'serial',
      );
    }
    const first = sliceID(track.slices[0].location.slice.id);
    for (const dependency of track.depends_on) {
      const prerequisite = tracks.get(dependency);
      add(
        sliceID(prerequisite.slices.at(-1).location.slice.id),
        first,
        'track_dependency',
      );
    }
    for (const slice of track.slices) {
      const work = slice.location.slice;
      for (const dependency of work.depends_on) {
        add(sliceID(dependency), sliceID(work.id), 'depends_on');
      }
      for (const dependency of work.consumes) {
        add(sliceID(dependency), sliceID(work.id), 'consumes');
      }
    }
    add(sliceID(track.slices.at(-1).location.slice.id), 'assembly', 'assembly');
  }
  add('assembly', 'merge', 'verified_before_merge');

  const incoming = new Set([...edges.values()].map(({ to }) => to));
  for (const track of state.tracks) {
    for (const slice of track.slices) {
      const id = sliceID(slice.location.slice.id);
      if (!incoming.has(id)) add('plan', id, 'start');
    }
  }

  return [...edges.values()]
    .map(({ from, to, kinds }) => frozen({
      from,
      to,
      kinds: frozen([...kinds].sort((left, right) => (
        Buffer.from(left).compare(Buffer.from(right))
      ))),
    }))
    .sort((left, right) => (
      nodeRanks.get(left.from) - nodeRanks.get(right.from)
      || nodeRanks.get(left.to) - nodeRanks.get(right.to)
    ));
}

function projectGraph(state, tracks, assembly, planNextOperation) {
  const nodes = [frozen({
    id: 'plan',
    kind: 'plan',
    state: state.plan.target_stale ? 'revision_required' : 'approved',
    next_operation: planNextOperation,
  })];
  const slices = new Map(state.slices.map((slice) => [
    slice.location.slice.id,
    slice,
  ]));
  for (const track of tracks.tracks) {
    for (const work of track.work) {
      nodes.push(frozen({
        id: `slice:${work.id}`,
        kind: 'slice',
        track: track.id,
        work: work.id,
        state: sliceGraphState(slices.get(work.id), work),
        next_operation: work.next_operation,
      }));
    }
  }
  const passApplicable = Boolean(state.assembly.pass);
  const mergeReady = passApplicable && Boolean(assembly.next_operation);
  nodes.push(frozen({
    id: 'assembly',
    kind: 'assembly',
    state: assemblyGraphState(state.assembly, assembly.next_operation),
    next_operation: passApplicable ? null : assembly.next_operation,
  }));
  nodes.push(frozen({
    id: 'merge',
    kind: 'merge',
    state: state.assembly.status === 'complete'
      ? 'complete'
      : mergeReady ? 'ready' : 'waiting',
    next_operation: state.assembly.status === 'complete' || !mergeReady
      ? null
      : assembly.next_operation,
  }));
  const nodeRanks = new Map(nodes.map((node, index) => [node.id, index]));
  return frozen({
    schema_version: GRAPH_VERSION,
    nodes: frozen(nodes),
    edges: frozen(graphEdges(state, nodeRanks)),
  });
}

function releaseStatus(state, tracks) {
  if (state.assembly.status === 'complete') return 'complete';
  if (
    state.assembly.status === 'blocked'
    || state.slices.some((slice) => slice.status === 'blocked')
  ) return 'blocked';
  if ([...tracks.ready.values()].some((ready) => !ready)) return 'in_progress';
  if (state.assembly.outcome === 'pass') return 'merge_ready';
  if (state.assembly.next_role === 'verifier') return 'assembly';
  return 'assembly_ready';
}

function projectState(state) {
  const tracks = projectTracks(state);
  const allTracksReady = [...tracks.ready.values()].every(Boolean);
  const assembly = projectAssembly(state, allTracksReady);
  const planNextOperation = state.plan.target_stale
    ? operation('planner', 'release', state.release)
    : null;
  const nextOperations = state.plan.target_stale
    ? [planNextOperation]
    : allTracksReady
      ? assembly.next_operation ? [assembly.next_operation] : []
      : tracks.next_operations;
  return frozen({
    schema_version: BOARD_VERSION,
    release: state.release,
    repository: state.repository,
    valid: true,
    diagnostics: frozen([...state.diagnostics]),
    plan_digest: state.plan.digest,
    plan_object: state.plan.oid,
    plan_revision: state.plan.metadata.revision,
    release_ref: state.refs.release.ref,
    release_head: state.refs.release.head,
    target_ref: state.refs.target.ref,
    target_head: state.refs.target.head,
    status: releaseStatus(state, tracks),
    tracks: tracks.tracks,
    assembly,
    graph: projectGraph(state, tracks, assembly, planNextOperation),
    next_operations: frozen(nextOperations.filter(Boolean)),
  });
}

function invalidRelease(discovered, error, repository) {
  return frozen({
    schema_version: BOARD_VERSION,
    release: discovered.release,
    repository: null,
    valid: false,
    diagnostics: frozen([diagnostic(error, repository, { release: discovered.release })]),
    plan_digest: null,
    plan_object: null,
    plan_revision: null,
    release_ref: discovered.ref,
    release_head: discovered.head,
    target_ref: null,
    target_head: null,
    status: 'invalid',
    tracks: frozen([]),
    assembly: null,
    graph: null,
    next_operations: frozen([]),
  });
}

export function createBoardOracle({ readState = readBatonState } = {}) {
  if (typeof readState !== 'function') throw new TypeError('readState must be a function');
  return frozen({
    project(repo = process.cwd(), options = {}) {
      let root;
      try {
        root = repositoryRoot(repo);
      } catch (error) {
        return frozen({
          schema_version: BOARD_VERSION,
          repository: null,
          valid: false,
          diagnostics: frozen([diagnostic(error, '')]),
          releases: frozen([]),
          next_operations: frozen([]),
        });
      }
      let discovered;
      try {
        discovered = discoverReleaseHeads(root);
      } catch (error) {
        return frozen({
          schema_version: BOARD_VERSION,
          repository: null,
          valid: false,
          diagnostics: frozen([diagnostic(error, root)]),
          releases: frozen([]),
          next_operations: frozen([]),
        });
      }
      const releases = discovered.map((entry) => {
        let current = entry;
        for (let attempt = 0; attempt < 2; attempt += 1) {
          try {
            return projectState(readState(root, current.release, {
              expectedReleaseHead: current.head,
              captureRefs: options.captureRefs,
            }));
          } catch (error) {
            if (
              error instanceof BatonStateError
              && error.code === 'REF_SNAPSHOT_UNSTABLE'
              && attempt === 0
            ) {
              current = discoverReleaseHeads(root)
                .find(({ release }) => release === current.release) ?? current;
              continue;
            }
            return invalidRelease(current, error, root);
          }
        }
        return invalidRelease(
          current,
          new BatonStateError('REF_SNAPSHOT_UNSTABLE', `release ${current.release} kept moving`),
          root,
        );
      });
      const repositories = new Set(releases.filter((release) => release.valid)
        .map((release) => release.repository));
      const globalDiagnostics = [];
      if (repositories.size > 1) globalDiagnostics.push(frozen({
        code: 'REPOSITORY_MISMATCH',
        release: null,
        track: null,
        work: null,
        message: 'release plans disagree on repository identity',
      }));
      const valid = releases.every((release) => release.valid) && globalDiagnostics.length === 0;
      return frozen({
        schema_version: BOARD_VERSION,
        repository: repositories.size === 1 ? [...repositories][0] : null,
        valid,
        diagnostics: frozen(globalDiagnostics),
        releases: frozen(releases),
        next_operations: frozen(releases.flatMap((release) => release.next_operations)),
      });
    },
  });
}

const defaultOracle = createBoardOracle();

export function projectBoard(repo = process.cwd(), options = {}) {
  return defaultOracle.project(repo, options);
}

export function boardBytes(board) {
  if (!board || board.schema_version !== BOARD_VERSION) {
    throw new TypeError(`board must be ${BOARD_VERSION}`);
  }
  return Buffer.from(`${JSON.stringify(board)}\n`);
}

function usage() {
  return 'Usage: node reference/board/oracle.mjs [repository]\n';
}

function main(argv) {
  if (argv.includes('--help')) {
    if (argv.length !== 1) {
      process.stderr.write(usage());
      process.exitCode = 64;
      return;
    }
    process.stderr.write(usage());
    return;
  }
  if (argv.length > 1 || argv[0]?.startsWith('-')) {
    process.stderr.write(usage());
    process.exitCode = 64;
    return;
  }
  const board = projectBoard(argv[0] ?? process.cwd());
  process.stdout.write(boardBytes(board));
  process.exitCode = board.valid ? 0 : 2;
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) {
  main(process.argv.slice(2));
}
