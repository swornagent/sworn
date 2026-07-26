import {
  GitRecordError, captureHeadRefs, isAncestor, productTreeIdentity,
  readFilesAtOID, readFirstParentHistory, verifyReleaseIntegration,
} from './git.mjs';
import { ReceiptError, parsePlanBytes, parseReceiptHistoryEntry } from './receipts.mjs';

const RECORD_ROOT = '.baton/releases';
const MAX_PLAN_REVISIONS = 256;

export class BatonStateError extends Error {
  constructor(code, message, cause) {
    super(message, cause ? { cause } : undefined);
    this.name = 'BatonStateError'; this.code = code;
  }
}
function fail(code, message, cause) { throw new BatonStateError(code, message, cause); }

function frozen(value, seen = new WeakSet()) {
  if (value === null || typeof value !== 'object' || seen.has(value) || ArrayBuffer.isView(value)) {
    return value;
  }
  seen.add(value);
  for (const item of Object.values(value)) frozen(item, seen);
  return Object.freeze(value);
}

const byteSort = (values) => [...values]
  .sort((left, right) => Buffer.from(left).compare(Buffer.from(right)));

function refsFor(release, plan) {
  return {
    release: `refs/heads/release-wt/${release}`,
    target: plan.metadata.target_ref,
    tracks: plan.metadata.tracks.map((track) => `refs/heads/track/${release}/${track.id}`),
  };
}
const planPath = (release) => `${RECORD_ROOT}/${release}/plan.md`;

function locations(plan) {
  const result = new Map();
  for (const track of plan.metadata.tracks) {
    for (const slice of track.slices) result.set(slice.id, { track, slice });
  }
  return result;
}

function planAt(repo, release, commit) {
  const [file] = readFilesAtOID(repo, commit, [planPath(release)]);
  if (!file?.bytes || !file.object) fail('PLAN_NOT_FOUND', `release ${release} has no plan`);
  try {
    const parsed = parsePlanBytes(file.bytes);
    if (parsed.metadata.release !== release) {
      fail('RELEASE_PLAN_MISMATCH', `plan release does not match ${release}`);
    }
    return frozen({ oid: file.object, parsed });
  } catch (error) {
    if (error instanceof ReceiptError) fail(error.code, error.message, error);
    throw error;
  }
}

function historyAt(repo, head) {
  if (head === null) return frozen({ rows: [], receipts: [] });
  const rows = readFirstParentHistory(repo, head);
  const receipts = [];
  for (let index = 0; index < rows.length; index += 1) {
    const row = rows[index];
    if (!row.message.includes(Buffer.from('\nBaton-Receipt: '))) continue;
    const parent = rows[index + 1];
    if (!parent || row.parents.length !== 1 || row.parents[0] !== parent.oid) {
      fail('HISTORY_LIMIT', `cannot establish the parent tree for receipt ${row.oid}`);
    }
    try {
      receipts.push(parseReceiptHistoryEntry({
        oid: row.oid,
        parents: row.parents,
        tree: row.tree,
        parent_tree: parent.tree,
        message: row.message,
      }));
    } catch (error) {
      if (error instanceof ReceiptError) {
        fail(error.code, `invalid receipt ${row.oid}: ${error.message}`, error);
      }
      throw error;
    }
  }
  return frozen({ rows: [...rows].reverse(), receipts: receipts.reverse() });
}

function matchingApproval(repo, release, entry, receipts) {
  const matches = receipts.filter(({ receipt }) => (
    receipt.role === 'planner'
    && receipt.result === 'approved'
    && receipt.plan === entry.oid
    && !Object.hasOwn(receipt, 'slice')
  ));
  if (matches.length !== 1) {
    fail(
      matches.length === 0 ? 'APPROVAL_MISSING' : 'AMBIGUOUS_APPROVAL',
      `plan revision ${entry.parsed.metadata.revision} has ${matches.length} approvals`,
    );
  }
  const [approval] = matches;
  const [file] = readFilesAtOID(repo, approval.oid, [planPath(release)]);
  if (approval.receipt.binds !== approval.parent || file.object !== entry.oid) {
    fail('STALE_BINDING', `approval ${approval.oid} does not bind its plan commit`);
  }
  return approval;
}

function planChain(repo, release, current, receipts) {
  const reverse = [];
  const seen = new Set();
  let cursor = current;
  while (cursor) {
    if (seen.has(cursor.oid)) fail('INVALID_PLAN_HISTORY', 'plan history contains a cycle');
    seen.add(cursor.oid);
    reverse.push(cursor);
    const priorOID = cursor.parsed.metadata.previous_plan;
    if (priorOID === null) break;
    if (reverse.length >= MAX_PLAN_REVISIONS) {
      fail('RESOURCE_LIMIT', `plan history exceeds ${MAX_PLAN_REVISIONS} revisions`);
    }
    const approval = receipts.find(({ receipt }) => (
      receipt.role === 'planner'
      && receipt.result === 'approved'
      && receipt.plan === priorOID
      && !Object.hasOwn(receipt, 'slice')
    ));
    if (!approval) fail('INVALID_PLAN_HISTORY', `previous plan ${priorOID} has no approval`);
    const [file] = readFilesAtOID(repo, approval.oid, [planPath(release)]);
    if (!file.bytes || file.object !== priorOID) {
      fail('INVALID_PLAN_HISTORY', `approval ${approval.oid} does not contain ${priorOID}`);
    }
    let parsed;
    try {
      parsed = parsePlanBytes(file.bytes);
    } catch (error) {
      if (error instanceof ReceiptError) fail(error.code, error.message, error);
      throw error;
    }
    const next = cursor.parsed.metadata;
    const prior = parsed.metadata;
    if (
      prior.revision !== next.revision - 1
      || prior.release !== current.parsed.metadata.release
      || prior.repository !== current.parsed.metadata.repository
      || prior.target_ref !== current.parsed.metadata.target_ref
      || prior.approval_ref === next.approval_ref
    ) {
      fail('INVALID_PLAN_HISTORY', `plan revision ${next.revision} has a broken predecessor`);
    }
    const oldLocations = locations(parsed);
    for (const [id, location] of locations(cursor.parsed)) {
      const old = oldLocations.get(id);
      if (old && old.track.id !== location.track.id) {
        fail('AMBIGUOUS_AUTHORITY', `slice ${id} moved between tracks`);
      }
    }
    cursor = frozen({ oid: priorOID, parsed });
  }
  const chain = reverse.reverse();
  if (chain[0].parsed.metadata.revision !== 1) {
    fail('INVALID_PLAN_HISTORY', 'plan history does not terminate at revision 1');
  }
  const approvals = new Map(chain.map((entry) => [
    entry.oid,
    matchingApproval(repo, release, entry, receipts),
  ]));
  return { chain, approvals };
}

function validateRetirements(current, chain, approvals, receipts) {
  const active = locations(current.parsed);
  const prior = new Map();
  for (const entry of chain.slice(0, -1)) {
    for (const [id, location] of locations(entry.parsed)) prior.set(id, { entry, location });
  }
  const retired = receipts.filter(({ receipt }) => (
    receipt.role === 'planner' && receipt.result === 'retired'
  ));
  for (const entry of retired) {
    if (active.has(entry.receipt.slice)) {
      fail('INVALID_RETIREMENT', `active slice ${entry.receipt.slice} is retired`);
    }
  }
  for (const [id, old] of prior) {
    if (active.has(id)) continue;
    const matches = retired.filter(({ receipt }) => (
      receipt.slice === id
      && receipt.plan === current.oid
      && receipt.binds === approvals.get(current.oid).oid
      && receipt.contract === old.entry.parsed.metadata.contracts[id]
    ));
    if (matches.length !== 1) {
      fail('RETIREMENT_MISSING', `removed slice ${id} requires one current retirement`);
    }
  }
}

function exactInputs(receipt, keys, label) {
  if (JSON.stringify(byteSort(Object.keys(receipt.inputs))) !== JSON.stringify(byteSort(keys))) {
    fail('STALE_BINDING', `${label} input keys do not match the plan`);
  }
}

function productTree(repo, candidate, admission, cache) {
  if (cache.has(candidate)) return cache.get(candidate);
  try {
    const digest = productTreeIdentity(repo, candidate, admission).productTree;
    cache.set(candidate, digest);
    return digest;
  } catch (error) {
    fail(error.code ?? 'INVALID_CANDIDATE', `cannot validate candidate ${candidate}`, error);
  }
}

function sameCandidate(left, right) {
  return (
    left.candidate === right.candidate
    && left.product_tree === right.product_tree
    && JSON.stringify(left.inputs) === JSON.stringify(right.inputs)
  );
}

function applicablePriorPass(entries, plan, sliceID) {
  const contract = plan.parsed.metadata.contracts[sliceID];
  const matching = entries.filter(({ receipt }) => (
    receipt.slice === sliceID && receipt.contract === contract
  ));
  const pass = latest(
    matching,
    ({ receipt }) => receipt.role === 'verifier' && receipt.result === 'pass',
  );
  return pass && !matching.some(
    ({ receipt }) => receipt.attempt > pass.receipt.attempt,
  );
}

function validateSerialSliceOrder(track, entries, planByOID) {
  const priorEntries = [];
  for (const entry of entries) {
    const plan = planByOID.get(entry.receipt.plan);
    const plannedTrack = plan?.parsed.metadata.tracks.find(
      ({ id }) => id === track.id,
    );
    const position = plannedTrack?.slices.findIndex(
      ({ id }) => id === entry.receipt.slice,
    ) ?? -1;
    if (position < 0) {
      fail('AMBIGUOUS_AUTHORITY', `receipt ${entry.oid} uses the wrong track`);
    }
    for (const prior of plannedTrack.slices.slice(0, position)) {
      if (!applicablePriorPass(priorEntries, plan, prior.id)) {
        fail(
          'DEPENDENCIES_NOT_READY',
          `${entry.receipt.slice} advanced before ${prior.id} PASS`,
        );
      }
    }
    priorEntries.push(entry);
  }
}

function validateSlice(
  repo,
  location,
  entries,
  planByOID,
  approvals,
  admission,
  productCache,
) {
  const byOID = new Map([...approvals.values()].map((entry) => [entry.oid, entry]));
  const seen = new Set();
  let maximum = 0;
  for (const entry of entries) {
    const receipt = entry.receipt;
    const plan = planByOID.get(receipt.plan);
    const planned = plan && locations(plan.parsed).get(receipt.slice);
    if (!planned || planned.track.id !== location.track.id) {
      fail('AMBIGUOUS_AUTHORITY', `receipt ${entry.oid} uses the wrong track`);
    }
    if (
      receipt.contract !== plan.parsed.metadata.contracts[receipt.slice]
      || !['implementer', 'captain', 'verifier'].includes(receipt.role)
    ) {
      fail('STALE_BINDING', `receipt ${entry.oid} has stale slice bindings`);
    }
    if (receipt.attempt < maximum || receipt.attempt > maximum + 1) {
      fail('INVALID_ATTEMPT', `receipt ${entry.oid} has non-monotonic attempt`);
    }
    maximum = Math.max(maximum, receipt.attempt);
    const roleKey = receipt.role === 'implementer' ? receipt.result : 'decision';
    const identity = `${receipt.attempt}:${receipt.role}:${roleKey}`;
    if (seen.has(identity)) fail('AMBIGUOUS_RECEIPT', `${receipt.slice} repeats ${identity}`);
    seen.add(identity);
    const bound = byOID.get(receipt.binds)?.receipt;
    const sameSlice = bound?.slice === receipt.slice;
    const samePlan = bound?.plan === receipt.plan;
    if (receipt.role === 'implementer' && receipt.result === 'designed') {
      const approved = bound?.role === 'planner' && bound.result === 'approved' && samePlan;
      const retry = sameSlice
        && bound.attempt === receipt.attempt - 1
        && (
          (bound.role === 'captain' && bound.result === 'revise')
          || (bound.role === 'verifier' && bound.result === 'fail')
        );
      if (!approved && !retry) fail('STALE_BINDING', `design ${entry.oid} has no predecessor`);
    } else if (receipt.role === 'captain') {
      if (
        !sameSlice || !samePlan || bound.role !== 'implementer'
        || bound.result !== 'designed' || bound.attempt !== receipt.attempt
      ) fail('STALE_BINDING', `Captain ${entry.oid} does not bind its design`);
    } else if (receipt.role === 'implementer' && receipt.result === 'candidate') {
      const proceeded = sameSlice && samePlan
        && bound.role === 'captain' && bound.result === 'proceed'
        && bound.attempt === receipt.attempt;
      const retry = sameSlice && samePlan
        && bound.role === 'verifier' && bound.result === 'fail'
        && bound.attempt === receipt.attempt - 1;
      if (!proceeded && !retry) fail('STALE_BINDING', `candidate ${entry.oid} lacks PROCEED`);
      exactInputs(receipt, planned.slice.consumes, `candidate ${entry.oid}`);
      if (
        entry.parent !== receipt.candidate
        || receipt.candidate === receipt.binds
        || !isAncestor(repo, receipt.binds, receipt.candidate)
        || (receipt.base && !isAncestor(repo, receipt.base, receipt.candidate))
        || productTree(repo, receipt.candidate, admission, productCache) !== receipt.product_tree
      ) fail('CHANGED_CANDIDATE', `candidate ${entry.oid} has invalid Git evidence`);
    } else if (receipt.role === 'verifier') {
      if (
        !sameSlice || !samePlan || bound.role !== 'implementer'
        || bound.result !== 'candidate' || bound.attempt !== receipt.attempt
        || !sameCandidate(receipt, bound)
      ) fail('STALE_BINDING', `Verifier ${entry.oid} does not bind its candidate`);
    } else {
      fail('INVALID_RECEIPT', `slice ${receipt.slice} has an unsupported receipt`);
    }
    byOID.set(entry.oid, entry);
  }
  return frozen({ entries, maximum_attempt: maximum });
}

function latest(entries, predicate) {
  for (let index = entries.length - 1; index >= 0; index -= 1) {
    if (predicate(entries[index])) return entries[index];
  }
  return null;
}

function deriveSlice(location, history, current, approval) {
  const contract = current.parsed.metadata.contracts[location.slice.id];
  const matching = history.entries.filter(({ receipt }) => receipt.contract === contract);
  const pass = latest(matching, ({ receipt }) => receipt.role === 'verifier' && receipt.result === 'pass');
  const passCurrent = pass && !matching.some(({ receipt }) => receipt.attempt > pass.receipt.attempt);
  const currentReceipt = latest(matching, ({ receipt }) => receipt.plan === current.oid);
  const next = history.maximum_attempt + 1;
  const common = {
    history, input_pins: null,
    next_attempts: { design: next, candidate: next },
    stale_reason: null,
  };
  if (passCurrent) return {
    ...common,
    stage: 'merge', status: 'ready', next_role: 'merge', outcome: 'pass',
    attempt: pass.receipt.attempt,
    current_receipt: pass,
    candidate: history.entries.find((entry) => entry.oid === pass.receipt.binds),
    pass, retained: pass.receipt.plan !== current.oid,
  };
  if (!currentReceipt) return {
    ...common,
    stage: 'design', status: 'ready', next_role: 'implementer', outcome: 'none',
    attempt: next, current_receipt: approval, candidate: null, pass: null, retained: false,
  };
  const receipt = currentReceipt.receipt;
  const candidate = receipt.role === 'verifier'
    ? history.entries.find((entry) => entry.oid === receipt.binds)
    : receipt.result === 'candidate' ? currentReceipt : null;
  const state = {
    ...common, attempt: receipt.attempt, current_receipt: currentReceipt,
    candidate, pass: null, retained: false,
  };
  const key = `${receipt.role}/${receipt.result}`;
  const status = {
    'implementer/designed': ['design', 'ready', 'captain', 'none'],
    'implementer/candidate': ['verify', 'ready', 'verifier', 'none'],
    'captain/proceed': ['implement', 'ready', 'implementer', 'proceed'],
    'captain/revise': ['design', 'ready', 'implementer', 'revise'],
    'captain/escalate': ['design', 'blocked', 'planner', 'escalate'],
    'verifier/pass': ['merge', 'ready', 'merge', 'pass'],
    'verifier/fail': ['implement', 'ready', 'implementer', 'fail'],
    'verifier/blocked': ['verify', 'blocked', 'planner', 'blocked'],
  }[key];
  if (!status) fail('INVALID_RECEIPT', `unsupported current receipt ${key}`);
  [state.stage, state.status, state.next_role, state.outcome] = status;
  if (['captain/revise', 'verifier/fail'].includes(key)) state.attempt += 1;
  if (key === 'verifier/fail') {
    state.next_attempts = { design: next, candidate: receipt.attempt + 1 };
  }
  return state;
}

function deriveSlices(current, histories, approvals) {
  const states = new Map();
  for (const [id, location] of locations(current.parsed)) {
    states.set(id, {
      location,
      ...deriveSlice(location, histories.get(id), current, approvals.get(current.oid)),
    });
  }
  const pending = new Set();
  const done = new Set();
  function resolve(id) {
    if (done.has(id)) return states.get(id);
    if (pending.has(id)) fail('DEPENDENCY_CYCLE', `slice dependency cycle reaches ${id}`);
    pending.add(id);
    const state = states.get(id);
    const slice = state.location.slice;
    const required = [...new Set([...slice.depends_on, ...slice.consumes])];
    for (const dependency of required) resolve(dependency);
    const ready = required.every((dependency) => states.get(dependency).pass);
    const pins = Object.fromEntries(slice.consumes.map((dependency) => [
      dependency,
      states.get(dependency).pass?.receipt.product_tree ?? null,
    ]));
    state.input_pins = ready ? pins : null;
    const evidence = state.candidate?.receipt;
    if (
      evidence
      && (!ready || JSON.stringify(evidence.inputs) !== JSON.stringify(pins))
    ) {
      state.stage = 'implement';
      state.status = 'ready';
      state.next_role = 'implementer';
      state.outcome = 'stale';
      state.attempt = state.history.maximum_attempt + 1;
      state.pass = null;
      state.retained = false;
      state.stale_reason = 'dependency eligibility or consumed inputs changed';
    }
    pending.delete(id);
    done.add(id);
    return state;
  }
  for (const id of states.keys()) resolve(id);
  const trackReady = new Map(current.parsed.metadata.tracks.map((track) => [
    track.id,
    track.slices.every(({ id }) => Boolean(states.get(id).pass)),
  ]));
  for (const track of current.parsed.metadata.tracks) {
    const trackDependenciesReady = track.depends_on.every(
      (trackID) => trackReady.get(trackID),
    );
    let priorSlicesReady = true;
    for (const slice of track.slices) {
      const state = states.get(slice.id);
      const explicitDependenciesReady = [...new Set([
        ...slice.depends_on,
        ...slice.consumes,
      ])].every((id) => states.get(id).pass);
      if (
        !state.pass
        && state.status !== 'blocked'
        && !(trackDependenciesReady && priorSlicesReady && explicitDependenciesReady)
      ) {
        state.status = 'waiting';
        state.next_role = 'none';
      }
      priorSlicesReady = priorSlicesReady && Boolean(state.pass);
    }
  }
  return states;
}

function assemblyCandidate(history, entry) {
  let cursor = entry;
  while (cursor && cursor.receipt.role !== 'implementer') {
    cursor = history.byOID.get(cursor.receipt.binds);
  }
  return cursor ?? null;
}

function validateAssembly(
  repo,
  entries,
  planByOID,
  approvals,
  sliceEntries,
  admission,
  productCache,
) {
  const byOID = new Map([...approvals.values()].map((entry) => [entry.oid, entry]));
  for (const entry of sliceEntries) byOID.set(entry.oid, entry);
  let previous = null;
  for (const entry of entries) {
    const receipt = entry.receipt;
    const plan = planByOID.get(receipt.plan);
    if (!plan) fail('STALE_BINDING', `assembly receipt ${entry.oid} has an unknown plan`);
    const approval = approvals.get(receipt.plan);
    if (receipt.role === 'implementer') {
      const trackIDs = plan.parsed.metadata.tracks.map((track) => track.id);
      exactInputs(receipt, trackIDs, `assembly candidate ${entry.oid}`);
      const bound = byOID.get(receipt.binds);
      const first = bound?.oid === approval.oid;
      const retry = previous && bound?.oid === previous.oid;
      if (
        (!first && !retry)
        || entry.parent !== receipt.candidate
        || !isAncestor(repo, receipt.base, receipt.candidate)
        || !isAncestor(repo, receipt.binds, receipt.candidate)
        || receipt.target !== approval.receipt.target
        || productTree(repo, receipt.candidate, admission, productCache) !== receipt.product_tree
      ) fail('STALE_BINDING', `assembly candidate ${entry.oid} has invalid evidence`);
    } else if (receipt.role === 'verifier') {
      const bound = byOID.get(receipt.binds);
      if (
        bound?.receipt.role !== 'implementer'
        || Object.hasOwn(bound.receipt, 'slice')
        || !sameCandidate(receipt, bound.receipt)
      ) fail('STALE_BINDING', `assembly Verifier ${entry.oid} has no exact candidate`);
    } else if (receipt.role === 'merge') {
      const bound = byOID.get(receipt.binds);
      const assemblyPass = bound?.receipt.role === 'verifier'
        && bound.receipt.result === 'pass'
        && !Object.hasOwn(bound.receipt, 'slice');
      const oneTrack = plan.parsed.metadata.tracks.length === 1;
      const lastSlice = plan.parsed.metadata.tracks[0]?.slices.at(-1)?.id;
      const directPass = oneTrack
        && bound?.receipt.role === 'verifier'
        && bound.receipt.result === 'pass'
        && bound.receipt.slice === lastSlice;
      const candidate = assemblyPass ? assemblyCandidate({ byOID }, bound) : bound;
      if (
        (!assemblyPass && !directPass)
        || receipt.target !== approval.receipt.target
        || receipt.candidate !== candidate?.receipt.candidate
        || receipt.product_tree !== candidate?.receipt.product_tree
      ) fail('STALE_BINDING', `Merge ${entry.oid} has no applicable PASS`);
      try {
        verifyReleaseIntegration(repo, receipt.target, receipt.candidate, receipt.result_commit);
      } catch (error) {
        fail(error.code ?? 'INVALID_MERGE', `Merge ${entry.oid} is not exact`, error);
      }
    } else {
      fail('AMBIGUOUS_AUTHORITY', `release receipt ${entry.oid} has an invalid role`);
    }
    byOID.set(entry.oid, entry);
    previous = entry;
  }
  return { entries, byOID };
}

function deriveAssembly(repo, current, history, approval, tracks, target) {
  const entries = history.entries.filter(({ receipt }) => receipt.plan === current.oid);
  const latestEntry = entries.at(-1) ?? null;
  const allPassed = tracks.every((track) => track.slices.every((slice) => slice.pass));
  const pins = Object.fromEntries(tracks.map((track) => [
    track.id,
    track.slices.at(-1)?.pass?.receipt.product_tree ?? null,
  ]));
  const common = {
    history: history.entries,
    input_pins: pins,
    stale_reason: null,
    result_commit: null,
  };
  if (!allPassed) return frozen({
    ...common, stage: 'verify', status: 'waiting', next_role: 'none', outcome: 'none',
    current_receipt: null, candidate: null, pass: null,
  });
  if (!latestEntry) {
    const lastSlice = tracks.length === 1 ? tracks[0].slices.at(-1) : null;
    const direct = lastSlice?.pass
      && isAncestor(repo, approval.receipt.target, lastSlice.pass.receipt.candidate);
    return frozen({
      ...common,
      stage: direct ? 'merge' : 'verify',
      status: 'ready',
      next_role: 'merge',
      outcome: direct ? 'pass' : 'none',
      current_receipt: direct ? lastSlice.pass : approval,
      candidate: direct ? lastSlice.candidate : null,
      pass: direct ? lastSlice.pass : null,
    });
  }
  const candidate = assemblyCandidate(history, latestEntry);
  if (latestEntry.receipt.role === 'merge') return frozen({
    ...common, stage: 'merge', status: 'complete', next_role: 'none', outcome: 'merged',
    current_receipt: latestEntry, candidate,
    pass: history.byOID.get(latestEntry.receipt.binds),
    result_commit: latestEntry.receipt.result_commit,
  });
  const stale = candidate && (
    candidate.receipt.target !== target
    || JSON.stringify(candidate.receipt.inputs) !== JSON.stringify(pins)
  );
  if (stale) return frozen({
    ...common, stage: 'verify', status: 'ready', next_role: 'merge', outcome: 'stale',
    current_receipt: latestEntry, candidate, pass: null,
    stale_reason: 'target or track inputs changed',
  });
  if (latestEntry.receipt.role === 'implementer') return frozen({
    ...common, stage: 'verify', status: 'ready', next_role: 'verifier', outcome: 'none',
    current_receipt: latestEntry, candidate, pass: null,
  });
  if (latestEntry.receipt.role === 'verifier') {
    const result = latestEntry.receipt.result;
    return frozen({
      ...common,
      stage: result === 'pass' ? 'merge' : 'verify',
      status: result === 'blocked' ? 'blocked' : 'ready',
      next_role: result === 'blocked' ? 'planner' : result === 'pass' ? 'merge' : 'merge',
      outcome: result,
      current_receipt: latestEntry,
      candidate,
      pass: result === 'pass' ? latestEntry : null,
    });
  }
  fail('INVALID_RECEIPT', `assembly history ends at unsupported ${latestEntry.receipt.role}`);
}

function diagnostic(code, message, context = {}) {
  return frozen({
    code,
    release: context.release ?? null,
    track: context.track ?? null,
    work: context.work ?? null,
    message,
  });
}

/**
 * Read one release from exact Git refs. The result is mutation-free structural
 * evidence for board projection and action preconditions.
 */
export function readBatonState(
  repo,
  release,
  {
    expectedReleaseHead = null,
    captureRefs = captureHeadRefs,
    productExclusionAdmission = null,
  } = {},
) {
  if (typeof release !== 'string' || typeof captureRefs !== 'function') {
    throw new TypeError('readBatonState requires a release string and captureRefs function');
  }
  const releaseName = `refs/heads/release-wt/${release}`;
  const [initial] = captureRefs(repo, [releaseName]);
  if (!initial.head) fail('REF_NOT_FOUND', `release ref ${releaseName} does not exist`);
  if (expectedReleaseHead && initial.head !== expectedReleaseHead) {
    fail('REF_SNAPSHOT_UNSTABLE', `release ref ${releaseName} moved before capture`);
  }
  const current = planAt(repo, release, initial.head);
  const names = refsFor(release, current.parsed);
  const captured = captureRefs(repo, [names.release, names.target, ...names.tracks]);
  if (captured[0].head !== initial.head) {
    fail('REF_SNAPSHOT_UNSTABLE', `release ref ${releaseName} moved during capture`);
  }
  if (!captured[1].head) fail('REF_NOT_FOUND', `target ref ${names.target} does not exist`);

  const releaseHistory = historyAt(repo, captured[0].head);
  for (const entry of releaseHistory.receipts) {
    if (entry.receipt.release !== release) {
      fail('RELEASE_RECEIPT_MISMATCH', `receipt ${entry.oid} names another release`);
    }
  }
  const plans = planChain(repo, release, current, releaseHistory.receipts);
  const planByOID = new Map(plans.chain.map((entry) => [entry.oid, entry]));
  validateRetirements(current, plans.chain, plans.approvals, releaseHistory.receipts);

  const releasePlannerOIDs = new Set(
    releaseHistory.receipts
      .filter(({ receipt }) => receipt.role === 'planner')
      .map((entry) => entry.oid),
  );
  const priorOwners = new Map();
  for (const entry of plans.chain) {
    for (const [id, location] of locations(entry.parsed)) priorOwners.set(id, location.track.id);
  }
  const trackHistories = new Map();
  const claimed = new Map();
  for (let index = 0; index < current.parsed.metadata.tracks.length; index += 1) {
    const track = current.parsed.metadata.tracks[index];
    const ref = captured[index + 2];
    const owned = historyAt(repo, ref.head).receipts.filter((entry) => (
      Object.hasOwn(entry.receipt, 'slice')
      && !releasePlannerOIDs.has(entry.oid)
      && priorOwners.get(entry.receipt.slice) === track.id
    ));
    for (const entry of owned) {
      if (
        entry.receipt.release !== release
        || (claimed.has(entry.oid) && claimed.get(entry.oid) !== track.id)
      ) fail('AMBIGUOUS_AUTHORITY', `track ${track.id} contains a foreign receipt`);
      claimed.set(entry.oid, track.id);
    }
    validateSerialSliceOrder(track, owned, planByOID);
    trackHistories.set(track.id, { ref, owned });
  }

  const productCache = new Map();
  const histories = new Map();
  for (const [id, location] of locations(current.parsed)) {
    const entries = trackHistories.get(location.track.id).owned
      .filter((entry) => entry.receipt.slice === id);
    histories.set(id, validateSlice(
      repo,
      location,
      entries,
      planByOID,
      plans.approvals,
      productExclusionAdmission,
      productCache,
    ));
  }
  const states = deriveSlices(current, histories, plans.approvals);
  const tracks = current.parsed.metadata.tracks.map((track, index) => frozen({
    id: track.id,
    depends_on: [...track.depends_on],
    ref: names.tracks[index],
    head: captured[index + 2].head,
    slices: track.slices.map((slice) => frozen(states.get(slice.id))),
  }));
  for (const track of tracks) {
    const active = track.slices.find((slice) => (
      slice.pass || ['verifier', 'merge'].includes(slice.next_role)
    ));
    if (
      track.head && active?.candidate
      && productTree(repo, track.head, productExclusionAdmission, productCache)
        !== active.candidate.receipt.product_tree
    ) fail('CHANGED_CANDIDATE', `track ${track.id} moved after its current candidate`);
  }

  const assemblyEntries = releaseHistory.receipts.filter(({ receipt }) => (
    (receipt.role === 'implementer' && receipt.result === 'candidate'
      && !Object.hasOwn(receipt, 'slice'))
    || (receipt.role === 'verifier' && !Object.hasOwn(receipt, 'slice'))
    || receipt.role === 'merge'
  ));
  for (const entry of releaseHistory.receipts) {
    if (
      entry.receipt.role !== 'planner'
      && !assemblyEntries.includes(entry)
      && !claimed.has(entry.oid)
    ) {
      fail('AMBIGUOUS_AUTHORITY', `receipt ${entry.oid} is on the wrong authority`);
    }
  }
  const allSliceEntries = [...histories.values()].flatMap((history) => history.entries);
  const assemblyHistory = validateAssembly(
    repo,
    assemblyEntries,
    planByOID,
    plans.approvals,
    allSliceEntries,
    productExclusionAdmission,
    productCache,
  );
  const approval = plans.approvals.get(current.oid);
  const assembly = deriveAssembly(
    repo,
    current,
    assemblyHistory,
    approval,
    tracks,
    captured[1].head,
  );
  if (
    assembly.status === 'complete'
    && !isAncestor(repo, assembly.result_commit, captured[1].head)
  ) fail('MOVED_TARGET', `target ${names.target} no longer contains the recorded merge`);

  // A completed Merge necessarily advances the target beyond the plan's
  // approved starting point. Once the recorded result is proven to remain in
  // the target ancestry above, that movement is the successful terminal
  // outcome, not a reason to revise the plan.
  const targetStale = assembly.status !== 'complete'
    && approval.receipt.target !== captured[1].head;
  const diagnostics = [];
  if (targetStale) diagnostics.push(diagnostic(
    'TARGET_MOVED',
    'the approved target moved; record a new plan revision',
    { release },
  ));
  for (const track of tracks) {
    if (!track.head) diagnostics.push(diagnostic(
      'TRACK_REF_ABSENT',
      `track ${track.id} may be materialized from approved facts`,
      { release, track: track.id },
    ));
    for (const slice of track.slices) {
      if (slice.stale_reason) diagnostics.push(diagnostic(
        'STALE_INPUTS',
        `${slice.location.slice.id} is recoverable: ${slice.stale_reason}`,
        { release, track: track.id, work: slice.location.slice.id },
      ));
    }
  }
  if (assembly.stale_reason) {
    diagnostics.push(diagnostic('STALE_ASSEMBLY', assembly.stale_reason, { release }));
  }

  return frozen({
    release,
    repository: current.parsed.metadata.repository,
    plan: {
      oid: current.oid,
      digest: current.parsed.digest,
      metadata: current.parsed.metadata,
      approval,
      approval_oid: approval.oid,
      target_stale: targetStale,
      history: plans.chain.map((entry) => ({
        oid: entry.oid,
        revision: entry.parsed.metadata.revision,
        approval: plans.approvals.get(entry.oid),
        plan: entry.parsed,
      })),
    },
    refs: {
      release: captured[0],
      target: captured[1],
      tracks: captured.slice(2).map((entry, index) => ({
        id: current.parsed.metadata.tracks[index].id,
        ...entry,
      })),
    },
    tracks,
    slices: [...states.values()],
    assembly,
    diagnostics,
  });
}

export function isBatonStateError(error) {
  return error instanceof BatonStateError
    || error instanceof ReceiptError
    || error instanceof GitRecordError;
}
