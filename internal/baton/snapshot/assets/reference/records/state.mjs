import {
  GitRecordError, captureHeadRefs, commitParents, isAncestor, productTreeIdentity,
  readFilesAtOID, readFirstParentHistory, verifyReleaseIntegration,
  unsafePrepareExactComposition,
} from './git.mjs';
import { ReceiptError, parsePlanBytes, parseReceiptHistoryEntry } from './receipts.mjs';

const RECORD_ROOT = '.baton/releases';
const MAX_PLAN_REVISIONS = 256;
const MAX_CANDIDATE_LINEAGE = 4096;

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

function predecessorIDs(location) {
  const position = location.track.slices.indexOf(location.slice);
  return location.track.slices.slice(0, position).map(({ id }) => id);
}

function sameIDs(left, right) {
  return left.length === right.length && left.every((id, index) => id === right[index]);
}

function slicePlanLineage(planByOID, current, sliceID) {
  const currentLocation = locations(current.parsed).get(sliceID);
  if (!currentLocation) return new Set();
  const contract = current.parsed.metadata.contracts[sliceID];
  const trackID = currentLocation.track.id;
  const predecessors = predecessorIDs(currentLocation);
  const lineage = new Set();
  let cursor = current;
  while (cursor) {
    const location = locations(cursor.parsed).get(sliceID);
    if (
      !location
      || location.track.id !== trackID
      || cursor.parsed.metadata.contracts[sliceID] !== contract
      || !sameIDs(predecessorIDs(location), predecessors)
    ) break;
    lineage.add(cursor.oid);
    const prior = cursor.parsed.metadata.previous_plan;
    cursor = prior === null ? null : planByOID.get(prior);
  }
  return lineage;
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

function validateRetirements(chain, approvals, receipts) {
  const retired = receipts.filter(({ receipt }) => (
    receipt.role === 'planner' && receipt.result === 'retired'
  ));
  const matched = new Set();
  const retiredIDs = new Set();
  for (let index = 1; index < chain.length; index += 1) {
    const prior = chain[index - 1];
    const next = chain[index];
    const priorLocations = locations(prior.parsed);
    const nextLocations = locations(next.parsed);
    for (const id of nextLocations.keys()) {
      if (retiredIDs.has(id)) {
        fail('INVALID_RETIREMENT', `retired slice ${id} cannot be re-added`);
      }
    }
    for (const id of priorLocations.keys()) {
      if (nextLocations.has(id)) continue;
      const matches = retired.filter(({ receipt }) => (
        receipt.slice === id
        && receipt.plan === next.oid
        && receipt.binds === approvals.get(next.oid).oid
        && receipt.contract === prior.parsed.metadata.contracts[id]
      ));
      if (matches.length === 0) {
        fail(
          'RETIREMENT_MISSING',
          `removed slice ${id} requires one retirement at its first absent revision`,
        );
      }
      if (matches.length !== 1) {
        fail('INVALID_RETIREMENT', `removed slice ${id} has duplicate retirements`);
      }
      matched.add(matches[0].oid);
      retiredIDs.add(id);
    }
  }
  const unmatched = retired.find((entry) => !matched.has(entry.oid));
  if (unmatched) {
    fail(
      'INVALID_RETIREMENT',
      `retirement ${unmatched.oid} does not bind one first-removal transition`,
    );
  }
}

function planInstallResult(planOID, approval, receipts) {
  let head = approval.oid;
  for (const entry of receipts) {
    const receipt = entry.receipt;
    if (
      receipt.role !== 'planner'
      || receipt.result !== 'retired'
      || receipt.plan !== planOID
      || receipt.binds !== approval.oid
    ) continue;
    if (entry.parent !== head) {
      fail(
        'INVALID_RETIREMENT',
        `retirements for plan ${planOID} do not form the exact post-approval chain`,
      );
    }
    head = entry.oid;
  }
  return head;
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

function sameInputs(left, right) {
  const leftKeys = byteSort(Object.keys(left ?? {}));
  const rightKeys = byteSort(Object.keys(right ?? {}));
  return (
    sameIDs(leftKeys, rightKeys)
    && leftKeys.every((key) => left[key] === right[key])
  );
}

function applicablePriorPass(entries, plan, sliceID, planByOID) {
  const contract = plan.parsed.metadata.contracts[sliceID];
  const lineage = slicePlanLineage(planByOID, plan, sliceID);
  const matching = entries.filter(({ receipt }) => (
    receipt.slice === sliceID
    && receipt.contract === contract
    && lineage.has(receipt.plan)
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
      if (!applicablePriorPass(priorEntries, plan, prior.id, planByOID)) {
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
    const sameLineage = sameSlice
      && slicePlanLineage(planByOID, plan, receipt.slice).has(bound?.plan);
    if (receipt.role === 'implementer' && receipt.result === 'designed') {
      const approved = bound?.role === 'planner' && bound.result === 'approved' && samePlan;
      const retry = sameLineage
        && bound.attempt === receipt.attempt - 1
        && (
          (bound.role === 'captain' && bound.result === 'revise')
          || (bound.role === 'verifier' && bound.result === 'fail')
        );
      const staleReviewRetry = sameLineage
        && bound.attempt === receipt.attempt - 1
        && (
          (bound.role === 'implementer' && bound.result === 'designed')
          || (bound.role === 'implementer' && bound.result === 'candidate')
          || (bound.role === 'captain' && bound.result === 'proceed')
          || (bound.role === 'verifier' && bound.result === 'pass')
        );
      if (
        !approved && !retry && !staleReviewRetry
      ) fail('STALE_BINDING', `design ${entry.oid} has no predecessor`);
      const hasBase = Object.hasOwn(receipt, 'base');
      const hasInputs = Object.hasOwn(receipt, 'inputs');
      if (hasBase !== hasInputs) {
        fail('STALE_BINDING', `design ${entry.oid} has incomplete reviewed-input evidence`);
      }
      if (hasInputs) {
        if (planned.slice.consumes.length === 0) {
          fail('STALE_BINDING', `design ${entry.oid} records inputs for a non-consuming slice`);
        }
        exactInputs(receipt, planned.slice.consumes, `design ${entry.oid}`);
      }
    } else if (receipt.role === 'captain') {
      if (
        !sameLineage || bound.role !== 'implementer'
        || bound.result !== 'designed' || bound.attempt !== receipt.attempt
      ) fail('STALE_BINDING', `Captain ${entry.oid} does not bind its design`);
    } else if (receipt.role === 'implementer' && receipt.result === 'candidate') {
      const proceeded = sameLineage
        && bound.role === 'captain' && bound.result === 'proceed'
        && bound.attempt === receipt.attempt;
      const retry = sameLineage
        && bound.role === 'verifier' && bound.result === 'fail'
        && bound.attempt === receipt.attempt - 1;
      const staleRetry = sameLineage
        && bound.attempt === receipt.attempt - 1
        && (
          (bound.role === 'implementer' && bound.result === 'candidate')
          || (
            bound.role === 'verifier'
            && ['pass', 'fail'].includes(bound.result)
          )
        )
        && !sameInputs(receipt.inputs, bound.inputs);
      if (
        !proceeded && !retry && !staleRetry
      ) fail('STALE_BINDING', `candidate ${entry.oid} lacks PROCEED`);
      exactInputs(receipt, planned.slice.consumes, `candidate ${entry.oid}`);
      if (planned.slice.consumes.length === 0 && Object.hasOwn(receipt, 'base')) {
        fail('STALE_BINDING', `candidate ${entry.oid} records a base for a non-consuming slice`);
      }
      if (
        entry.parent !== receipt.candidate
        || receipt.candidate === receipt.binds
        || !isAncestor(repo, receipt.binds, receipt.candidate)
        || (receipt.base && !isAncestor(repo, receipt.base, receipt.candidate))
        || productTree(repo, receipt.candidate, admission, productCache) !== receipt.product_tree
      ) fail('CHANGED_CANDIDATE', `candidate ${entry.oid} has invalid Git evidence`);
    } else if (receipt.role === 'verifier') {
      if (
        !sameLineage || bound.role !== 'implementer'
        || bound.result !== 'candidate' || bound.attempt !== receipt.attempt
        || entry.parent !== byOID.get(receipt.binds)?.oid
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

function governingDesign(history, current) {
  if (!current) return null;
  const byOID = new Map(history.entries.map((entry) => [entry.oid, entry]));
  let cursor = current;
  for (let steps = 0; cursor && steps <= history.entries.length; steps += 1) {
    if (
      cursor.receipt.role === 'implementer'
      && cursor.receipt.result === 'designed'
    ) return cursor;
    cursor = byOID.get(cursor.receipt.binds) ?? null;
  }
  return null;
}

function consumedInputForPass(repo, sliceID, history, pass, admission, productCache) {
  const receipt = pass.receipt;
  if (
    receipt.role !== 'verifier'
    || receipt.result !== 'pass'
    || receipt.slice !== sliceID
  ) fail('STALE_BINDING', `consumed slice ${sliceID} has invalid PASS authority`);
  const candidate = history.entries.find((entry) => entry.oid === receipt.binds);
  if (
    !candidate
    || candidate.receipt.role !== 'implementer'
    || candidate.receipt.result !== 'candidate'
    || pass.parent !== candidate.oid
    || candidate.parent !== candidate.receipt.candidate
    || !sameCandidate(receipt, candidate.receipt)
  ) fail('STALE_BINDING', `consumed PASS ${pass.oid} has no exact candidate chain`);
  for (const commit of [candidate.receipt.candidate, candidate.oid, pass.oid]) {
    if (
      productTree(repo, commit, admission, productCache) !== receipt.product_tree
    ) fail('CHANGED_CANDIDATE', `consumed PASS ${pass.oid} changed product identity`);
  }
  return frozen({
    slice: sliceID,
    pass_receipt: pass.oid,
    candidate_receipt: candidate.oid,
    candidate: candidate.receipt.candidate,
    product_tree: receipt.product_tree,
  });
}

function consumedInputsAtBase(
  repo,
  plan,
  base,
  consumes,
  histories,
  planByOID,
  admission,
  productCache,
) {
  const result = [];
  for (const dependency of consumes) {
    const lineage = slicePlanLineage(planByOID, plan, dependency);
    const contract = plan.parsed.metadata.contracts[dependency];
    let selected = null;
    for (const entry of histories.get(dependency).entries) {
      const receipt = entry.receipt;
      if (
        receipt.role !== 'verifier'
        || receipt.result !== 'pass'
        || receipt.contract !== contract
        || !lineage.has(receipt.plan)
        || !isAncestor(repo, entry.oid, base)
      ) continue;
      if (selected && !isAncestor(repo, selected.oid, entry.oid)) {
        fail(
          'AMBIGUOUS_AUTHORITY',
          `consumed slice ${dependency} has incomparable PASS authority`,
        );
      }
      selected = entry;
    }
    if (!selected) return null;
    result.push(consumedInputForPass(
      repo,
      dependency,
      histories.get(dependency),
      selected,
      admission,
      productCache,
    ));
  }
  return result;
}

function reviewedConsumedInputs(
  repo,
  design,
  consumes,
  histories,
  planByOID,
  admission,
  productCache,
) {
  const plan = planByOID.get(design.receipt.plan);
  if (!plan || !design.parent) {
    fail('STALE_BINDING', `design ${design.oid} has no reviewed base`);
  }
  return consumedInputsAtBase(
    repo,
    plan,
    design.parent,
    consumes,
    histories,
    planByOID,
    admission,
    productCache,
  );
}

function pinsForConsumedInputs(inputs) {
  return Object.fromEntries(inputs.map((input) => [input.slice, input.product_tree]));
}

function prepareConsumedBase(repo, ref, seed, inputs, admission) {
  let candidate = seed;
  for (const input of inputs) {
    if (
      input.pass_receipt === candidate
      || isAncestor(repo, input.pass_receipt, candidate)
    ) continue;
    candidate = unsafePrepareExactComposition(repo, {
      targetRef: ref,
      expectedHead: candidate,
      candidate: input.pass_receipt,
      productExclusionAdmission: admission,
    }).result;
  }
  return candidate;
}

function linearOneParentAncestry(repo, base, candidate) {
  let cursor = candidate;
  for (let steps = 0; steps < MAX_CANDIDATE_LINEAGE; steps += 1) {
    if (cursor === base) return true;
    const parents = commitParents(repo, cursor);
    if (parents.length !== 1) return false;
    [cursor] = parents;
  }
  fail('RESOURCE_LIMIT', 'candidate lineage exceeds the bounded history limit');
}

function exactPreparedDesignInputs(
  repo,
  release,
  design,
  histories,
  trackHistories,
  planByOID,
  approvals,
  releaseReceipts,
  admission,
  productCache,
) {
  if (!Object.hasOwn(design.receipt, 'base')) return null;
  const plan = planByOID.get(design.receipt.plan);
  const location = plan && locations(plan.parsed).get(design.receipt.slice);
  if (!location || location.slice.consumes.length === 0) {
    fail('STALE_BINDING', `design ${design.oid} has invalid reviewed-input evidence`);
  }
  const owned = trackHistories.get(location.track.id)?.owned;
  const designIndex = owned?.findIndex((entry) => entry.oid === design.oid) ?? -1;
  if (designIndex < 0) {
    fail('AMBIGUOUS_AUTHORITY', `design ${design.oid} has no owning track authority`);
  }
  const approval = approvals.get(plan.oid);
  if (!approval) fail('APPROVAL_MISSING', `design ${design.oid} has no plan approval`);
  const seed = designIndex === 0
    ? planInstallResult(plan.oid, approval, releaseReceipts)
    : owned[designIndex - 1].oid;
  if (design.receipt.base !== seed) {
    fail('STALE_BINDING', `design ${design.oid} has the wrong prior track authority`);
  }
  const inputs = consumedInputsAtBase(
    repo,
    plan,
    design.parent,
    location.slice.consumes,
    histories,
    planByOID,
    admission,
    productCache,
  );
  if (
    !inputs
    || !sameInputs(design.receipt.inputs, pinsForConsumedInputs(inputs))
  ) fail('STALE_BINDING', `design ${design.oid} has stale reviewed-input pins`);
  const expected = prepareConsumedBase(
    repo,
    `refs/heads/track/${release}/${location.track.id}`,
    seed,
    inputs,
    admission,
  );
  if (expected !== design.parent) {
    fail('STALE_BINDING', `design ${design.oid} has an inexact reviewed base`);
  }
  return inputs;
}

function validateConsumedHistories(
  repo,
  release,
  histories,
  trackHistories,
  planByOID,
  approvals,
  releaseReceipts,
  admission,
  productCache,
) {
  for (const [sliceID, history] of histories) {
    const byOID = new Map([...approvals.values()].map((entry) => [entry.oid, entry]));
    for (const entry of history.entries) byOID.set(entry.oid, entry);
    for (const entry of history.entries) {
      const receipt = entry.receipt;
      const plan = planByOID.get(receipt.plan);
      const location = plan && locations(plan.parsed).get(sliceID);
      if (!location || location.slice.consumes.length === 0) continue;
      if (receipt.role === 'implementer' && receipt.result === 'designed') {
        const bound = byOID.get(receipt.binds);
        if (!bound) continue;
        const currentInputs = exactPreparedDesignInputs(
          repo,
          release,
          entry,
          histories,
          trackHistories,
          planByOID,
          approvals,
          releaseReceipts,
          admission,
          productCache,
        );
        if (!currentInputs) continue;
        const staleRetry = (
          (bound.receipt.role === 'implementer' && bound.receipt.result === 'designed')
          || (
            bound.receipt.role === 'implementer'
            && bound.receipt.result === 'candidate'
          )
          || (
            bound.receipt.role === 'captain'
            && bound.receipt.result === 'proceed'
          )
          || (
            bound.receipt.role === 'verifier'
            && bound.receipt.result === 'pass'
          )
        );
        if (staleRetry) {
          const priorDesign = governingDesign(history, bound);
          if (!priorDesign) {
            fail('STALE_BINDING', `design ${entry.oid} has no stale review chain`);
          }
          const priorInputs = reviewedConsumedInputs(
            repo,
            priorDesign,
            location.slice.consumes,
            histories,
            planByOID,
            admission,
            productCache,
          );
          if (
            priorInputs
            && sameInputs(
              pinsForConsumedInputs(priorInputs),
              pinsForConsumedInputs(currentInputs),
            )
          ) {
            fail('STALE_BINDING', `design ${entry.oid} retries an unchanged review`);
          }
        }
      }
      if (receipt.role !== 'implementer' || receipt.result !== 'candidate') continue;
      const design = governingDesign(history, entry);
      const strictDesign = Boolean(
        design && Object.hasOwn(design.receipt, 'base'),
      );
      let reviewed = null;
      if (strictDesign) {
        reviewed = exactPreparedDesignInputs(
          repo,
          release,
          design,
          histories,
          trackHistories,
          planByOID,
          approvals,
          releaseReceipts,
          admission,
          productCache,
        );
      } else if (design) {
        reviewed = reviewedConsumedInputs(
          repo,
          design,
          location.slice.consumes,
          histories,
          planByOID,
          admission,
          productCache,
        );
      }
      if (!Object.hasOwn(receipt, 'base')) {
        if (strictDesign) {
          fail('STALE_BINDING', `candidate ${entry.oid} has no consumed-input base`);
        }
        continue;
      }
      if (
        reviewed
        && !sameInputs(receipt.inputs, pinsForConsumedInputs(reviewed))
      ) fail('STALE_BINDING', `candidate ${entry.oid} differs from its reviewed inputs`);
      if (!linearOneParentAncestry(repo, receipt.base, receipt.candidate)) {
        fail(
          'CHANGED_CANDIDATE',
          `candidate ${entry.oid} is not linear one-parent work from its base`,
        );
      }
      const inputs = consumedInputsAtBase(
        repo,
        plan,
        receipt.base,
        location.slice.consumes,
        histories,
        planByOID,
        admission,
        productCache,
      );
      if (
        !inputs
        || !sameInputs(receipt.inputs, pinsForConsumedInputs(inputs))
      ) fail('STALE_BINDING', `candidate ${entry.oid} has stale consumed pins`);
      const expected = prepareConsumedBase(
        repo,
        `refs/heads/track/${release}/${location.track.id}`,
        receipt.binds,
        inputs,
        admission,
      );
      if (expected !== receipt.base) {
        fail('CHANGED_CANDIDATE', `candidate ${entry.oid} has an inexact prepared base`);
      }
      for (const input of inputs) {
        for (const ancestor of [
          input.candidate,
          input.candidate_receipt,
          input.pass_receipt,
        ]) {
          if (!isAncestor(repo, ancestor, receipt.candidate)) {
            fail('CHANGED_CANDIDATE', `candidate ${entry.oid} omits consumed authority`);
          }
        }
      }
    }
  }
}

function deriveSlice(location, history, current, approval, planByOID) {
  const contract = current.parsed.metadata.contracts[location.slice.id];
  const lineage = slicePlanLineage(planByOID, current, location.slice.id);
  const matching = history.entries.filter(({ receipt }) => (
    receipt.contract === contract && lineage.has(receipt.plan)
  ));
  const pass = latest(matching, ({ receipt }) => receipt.role === 'verifier' && receipt.result === 'pass');
  const passCurrent = pass && !matching.some(({ receipt }) => receipt.attempt > pass.receipt.attempt);
  let currentReceipt = matching.at(-1) ?? null;
  if (
    currentReceipt
    && currentReceipt.receipt.plan !== current.oid
    && (
      (currentReceipt.receipt.role === 'captain'
        && currentReceipt.receipt.result === 'escalate')
      || (currentReceipt.receipt.role === 'verifier'
        && currentReceipt.receipt.result === 'blocked')
    )
  ) {
    currentReceipt = null;
  }
  const next = history.maximum_attempt + 1;
  const common = {
    history,
    input_pins: null,
    reviewed_pins: null,
    reviewed_base: null,
    consumed_inputs: [],
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

function deriveSlices(
  repo,
  current,
  histories,
  approvals,
  planByOID,
  admission,
  productCache,
) {
  const states = new Map();
  for (const [id, location] of locations(current.parsed)) {
    states.set(id, {
      location,
      ...deriveSlice(
        location,
        histories.get(id),
        current,
        approvals.get(current.oid),
        planByOID,
      ),
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
    let consumesReady = true;
    const pins = {};
    const inputs = [];
    for (const dependency of slice.consumes) {
      const dependencyState = states.get(dependency);
      if (dependencyState.pass) {
        pins[dependency] = dependencyState.pass.receipt.product_tree;
        inputs.push(consumedInputForPass(
          repo,
          dependency,
          histories.get(dependency),
          dependencyState.pass,
          admission,
          productCache,
        ));
      } else {
        consumesReady = false;
      }
    }
    state.input_pins = consumesReady ? pins : null;
    state.consumed_inputs = consumesReady ? inputs : [];

    let requiresFreshDesign = false;
    const externallyBlocked = state.status === 'blocked' && state.next_role === 'planner';
    const design = governingDesign(state.history, state.current_receipt);
    if (design && slice.consumes.length > 0) {
      state.reviewed_base = design.parent;
      const reviewed = reviewedConsumedInputs(
        repo,
        design,
        slice.consumes,
        histories,
        planByOID,
        admission,
        productCache,
      );
      if (reviewed) state.reviewed_pins = pinsForConsumedInputs(reviewed);
      const reviewStale = (
        !reviewed
        || !consumesReady
        || !sameInputs(state.reviewed_pins, pins)
      );
      const receipt = state.current_receipt?.receipt;
      const beforeCandidate = (
        receipt?.role === 'captain'
        || (receipt?.role === 'implementer' && receipt.result === 'designed')
      );
      const reviewEstablished = reviewed !== null;
      if (
        !externallyBlocked
        && reviewStale
        && (beforeCandidate || reviewEstablished)
      ) {
        requiresFreshDesign = true;
        state.stage = 'design';
        state.status = 'ready';
        state.next_role = 'implementer';
        state.outcome = 'stale';
        state.attempt = state.history.maximum_attempt + 1;
        state.pass = null;
        state.candidate = null;
        state.retained = false;
        state.stale_reason = 'reviewed consumed input product changed or is absent';
      }
    }

    const evidence = state.candidate?.receipt;
    if (
      !externallyBlocked && !requiresFreshDesign && evidence
      && consumesReady
      && !sameInputs(evidence.inputs, pins)
    ) {
      state.stage = 'implement';
      state.status = 'ready';
      state.next_role = 'implementer';
      state.outcome = 'stale';
      state.attempt = state.history.maximum_attempt + 1;
      state.pass = null;
      state.retained = false;
      state.stale_reason = 'consumed input lineage or product changed';
    } else if (
      !externallyBlocked
      && !requiresFreshDesign
      && evidence
      && !consumesReady
    ) {
      state.stage = 'implement';
      state.status = 'ready';
      state.next_role = 'implementer';
      state.outcome = 'stale';
      state.attempt = state.history.maximum_attempt + 1;
      state.pass = null;
      state.retained = false;
      state.stale_reason = 'consumed input product is absent';
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
      const oneSlice = plan.parsed.metadata.tracks.length === 1
        && plan.parsed.metadata.tracks[0].slices.length === 1;
      const lastSlice = plan.parsed.metadata.tracks[0]?.slices.at(-1)?.id;
      const directPass = oneSlice
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
    const lastSlice = tracks.length === 1 && tracks[0].slices.length === 1
      ? tracks[0].slices[0]
      : null;
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
  validateRetirements(plans.chain, plans.approvals, releaseHistory.receipts);

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
  validateConsumedHistories(
    repo,
    release,
    histories,
    trackHistories,
    planByOID,
    plans.approvals,
    releaseHistory.receipts,
    productExclusionAdmission,
    productCache,
  );
  const states = deriveSlices(
    repo,
    current,
    histories,
    plans.approvals,
    planByOID,
    productExclusionAdmission,
    productCache,
  );
  for (const state of states.values()) {
    state.consumed_inputs = state.consumed_inputs.map((input) => {
      const producer = states.get(input.slice);
      const source = trackHistories.get(producer.location.track.id)?.ref;
      if (!source?.head) {
        fail(
          'AMBIGUOUS_AUTHORITY',
          `consumed slice ${input.slice} has no direct producer authority`,
        );
      }
      if (!isAncestor(repo, input.pass_receipt, source.head)) {
        fail(
          'AMBIGUOUS_AUTHORITY',
          `consumed PASS ${input.pass_receipt} is absent from producer authority`,
        );
      }
      return frozen({
        ...input,
        source_ref: source.ref,
        source_head: source.head,
      });
    });
  }
  const tracks = current.parsed.metadata.tracks.map((track, index) => frozen({
    id: track.id,
    depends_on: [...track.depends_on],
    ref: names.tracks[index],
    head: captured[index + 2].head,
    authority_head: trackHistories.get(track.id).owned.at(-1)?.oid
      ?? captured[0].head,
    slices: track.slices.map((slice) => frozen(states.get(slice.id))),
  }));
  for (const track of tracks) {
    const incomplete = track.slices.find((slice) => !slice.pass);
    const active = incomplete
      ? (['verifier', 'merge'].includes(incomplete.next_role) ? incomplete : null)
      : track.slices.at(-1);
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
