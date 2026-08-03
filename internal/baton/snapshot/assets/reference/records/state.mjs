import {
  GitRecordError, assertCandidateRecordRootUnchanged, captureHeadRefs,
  commitParents, firstParentPathChange, isAncestor, productTreeIdentity,
  readFilesAtOID, readFirstParentHistory,
  verifyReleaseIntegration,
  unsafePrepareApprovedTargetBase,
  unsafePrepareExactComposition,
  unsafePrepareProductComposition,
} from './git.mjs';
import { ReceiptError, parsePlanBytes, parseReceiptHistoryEntry } from './receipts.mjs';

const RECORD_ROOT = '.baton/releases';
const MAX_PLAN_REVISIONS = 256;
const MAX_CANDIDATE_LINEAGE = 4096;
const productBaseEvidence = new WeakMap();

/**
 * Return engine-only product-base evidence for one exact state snapshot.
 *
 * The public state projection intentionally omits raw Git bases. Actions use
 * this opaque snapshot binding so callers cannot supply or alter a base.
 */
export function unsafeProductBaseEvidence(state) {
  const evidence = productBaseEvidence.get(state);
  if (!evidence) {
    throw new TypeError('product-base evidence requires an exact Baton state snapshot');
  }
  return evidence;
}

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

function historyLimitFailure(rows, label) {
  if (rows.length === 4096) {
    fail('RESOURCE_LIMIT', `${label} exceeds the bounded first-parent history limit`);
  }
  fail('HISTORY_BOUNDARY_MISSING', `${label} is absent from the exact first-parent history`);
}

function historyEntry(rows, index) {
  const row = rows[index];
  const parent = rows[index + 1];
  if (
    !parent
    || row.parents.length !== 1
    || row.parents[0] !== parent.oid
  ) {
    fail('HISTORY_LIMIT', `cannot establish the parent tree for receipt ${row.oid}`);
  }
  try {
    return parseReceiptHistoryEntry({
      oid: row.oid,
      parents: row.parents,
      tree: row.tree,
      parent_tree: parent.tree,
      message: row.message,
    });
  } catch (error) {
    if (error instanceof ReceiptError) {
      fail(error.code, `invalid receipt ${row.oid}: ${error.message}`, error);
    }
    throw error;
  }
}

function historyAt(repo, head, exclusiveBoundary = null) {
  if (head === null) return frozen({ rows: [], receipts: [] });
  const rows = readFirstParentHistory(repo, head);
  const receipts = [];
  let boundaryIndex = rows.length;
  for (let index = 0; index < rows.length; index += 1) {
    const row = rows[index];
    if (row.oid === exclusiveBoundary) {
      boundaryIndex = index;
      break;
    }
    const parent = rows[index + 1];
    if (!parent || row.parents[0] !== parent.oid) {
      if (exclusiveBoundary !== null) {
        historyLimitFailure(rows, `history boundary ${exclusiveBoundary}`);
      }
    }
    if (!row.message.includes(Buffer.from('\nBaton-Receipt: '))) continue;
    receipts.push(historyEntry(rows, index));
  }
  if (exclusiveBoundary !== null && boundaryIndex === rows.length) {
    historyLimitFailure(rows, `history boundary ${exclusiveBoundary}`);
  }
  return frozen({
    rows: rows.slice(0, boundaryIndex).reverse(),
    receipts: receipts.reverse(),
  });
}

function assertPlanPredecessor(current, prior) {
  const next = current.parsed.metadata;
  const previous = prior.parsed.metadata;
  if (
    previous.revision !== next.revision - 1
    || previous.release !== next.release
    || previous.repository !== next.repository
    || previous.target_ref !== next.target_ref
    || previous.approval_ref === next.approval_ref
  ) {
    fail('INVALID_PLAN_HISTORY', `plan revision ${next.revision} has a broken predecessor`);
  }
}

/**
 * Read only receipts belonging to the current release epoch.
 *
 * Revision 1 is installed as target -> plan commit -> approval receipt. Its
 * approved target is therefore a deterministic exclusive floor: inherited
 * receipts below it belong to older releases, while every receipt above it is
 * parsed before any authority or ownership filtering.
 */
export function readReleaseReceiptHistory(repo, release, head) {
  if (head === null) return frozen({
    boundary: null,
    rows: [],
    receipts: [],
  });
  const current = planAt(repo, release, head);
  const rows = readFirstParentHistory(repo, head);
  const receipts = [];
  const lineage = [];
  let expected = current;
  let boundary = null;
  let boundaryIndex = rows.length;

  for (let index = 0; index < rows.length; index += 1) {
    const row = rows[index];
    if (row.oid === boundary) {
      boundaryIndex = index;
      break;
    }
    const parent = rows[index + 1];
    if (!parent || row.parents[0] !== parent.oid) {
      historyLimitFailure(rows, boundary === null
        ? `revision-1 approval for ${release}`
        : `release epoch boundary ${boundary}`);
    }
    if (!row.message.includes(Buffer.from('\nBaton-Receipt: '))) continue;

    const entry = historyEntry(rows, index);
    receipts.push(entry);
    const receipt = entry.receipt;
    if (
      boundary !== null
      || receipt.role !== 'planner'
      || receipt.result !== 'approved'
      || Object.hasOwn(receipt, 'slice')
      || receipt.plan !== expected.oid
      || receipt.release !== release
    ) continue;

    const [file] = readFilesAtOID(repo, entry.oid, [planPath(release)]);
    if (
      receipt.binds !== entry.parent
      || !file?.bytes
      || file.object !== expected.oid
    ) {
      fail('STALE_BINDING', `approval ${entry.oid} does not bind its plan commit`);
    }
    let parsed;
    try {
      parsed = parsePlanBytes(file.bytes);
    } catch (error) {
      if (error instanceof ReceiptError) fail(error.code, error.message, error);
      throw error;
    }
    const approved = frozen({ oid: file.object, parsed });
    if (
      approved.parsed.metadata.release !== release
      || approved.parsed.metadata.revision !== expected.parsed.metadata.revision
    ) {
      fail('INVALID_PLAN_HISTORY', `approval ${entry.oid} has stale plan topology`);
    }
    lineage.push({ plan: approved, approval: entry });

    const priorOID = approved.parsed.metadata.previous_plan;
    if (priorOID !== null) {
      const priorApproval = receipts.find(({ receipt: candidate }) => (
        candidate.role === 'planner'
        && candidate.result === 'approved'
        && !Object.hasOwn(candidate, 'slice')
        && candidate.plan === priorOID
        && candidate.release === release
      ));
      if (priorApproval) {
        fail('INVALID_PLAN_HISTORY', `approval for previous plan ${priorOID} is out of order`);
      }
      expected = frozen({
        oid: priorOID,
        parsed: {
          metadata: {
            ...approved.parsed.metadata,
            revision: approved.parsed.metadata.revision - 1,
          },
        },
      });
      continue;
    }

    if (approved.parsed.metadata.revision !== 1) {
      fail('INVALID_PLAN_HISTORY', 'plan history does not terminate at revision 1');
    }
    const planParents = commitParents(repo, entry.parent);
    if (
      planParents.length !== 1
      || planParents[0] !== receipt.target
    ) {
      fail(
        'INVALID_PLAN_HISTORY',
        `revision-1 approval ${entry.oid} does not install directly above its target`,
      );
    }
    const [atFloor] = readFilesAtOID(repo, receipt.target, [planPath(release)]);
    if (atFloor?.object !== null) {
      fail(
        'INVALID_PLAN_HISTORY',
        `revision-1 target ${receipt.target} already contains release ${release}`,
      );
    }
    const priorPlanPathChange = firstParentPathChange(
      repo,
      receipt.target,
      planPath(release),
    );
    if (priorPlanPathChange !== null) {
      fail(
        'INVALID_PLAN_HISTORY',
        `revision-1 plan path was already introduced at ${priorPlanPathChange}`,
      );
    }
    boundary = receipt.target;
  }

  if (boundary === null || boundaryIndex === rows.length) {
    historyLimitFailure(
      rows,
      boundary === null
        ? `revision-1 approval for ${release}`
        : `release epoch boundary ${boundary}`,
    );
  }

  const lineageOIDs = new Set(lineage.map(({ plan }) => plan.oid));
  for (let index = 0; index < lineage.length - 1; index += 1) {
    assertPlanPredecessor(lineage[index].plan, lineage[index + 1].plan);
  }
  for (const planOID of lineageOIDs) {
    const matches = receipts.filter(({ receipt }) => (
      receipt.role === 'planner'
      && receipt.result === 'approved'
      && !Object.hasOwn(receipt, 'slice')
      && receipt.plan === planOID
    ));
    if (matches.length !== 1) {
      fail(
        matches.length === 0 ? 'APPROVAL_MISSING' : 'AMBIGUOUS_APPROVAL',
        `plan ${planOID} has ${matches.length} approvals inside its release epoch`,
      );
    }
  }
  for (const entry of receipts) {
    if (entry.receipt.release !== release) {
      fail('RELEASE_RECEIPT_MISMATCH', `receipt ${entry.oid} names another release`);
    }
    if (
      entry.receipt.role === 'planner'
      && entry.receipt.result === 'approved'
      && !lineageOIDs.has(entry.receipt.plan)
    ) {
      fail('AMBIGUOUS_APPROVAL', `approval ${entry.oid} is outside the current plan lineage`);
    }
  }

  return frozen({
    boundary,
    rows: rows.slice(0, boundaryIndex).reverse(),
    receipts: receipts.reverse(),
  });
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

function productTree(repo, candidate, cache) {
  if (cache.has(candidate)) return cache.get(candidate);
  try {
    const digest = productTreeIdentity(repo, candidate).productTree;
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
      const candidateRefresh = sameLineage
        && bound.role === 'implementer' && bound.result === 'candidate'
        && bound.attempt === receipt.attempt - 1
        && sameInputs(receipt.inputs, bound.inputs)
        && linearOneParentAncestry(repo, receipt.binds, receipt.candidate)
        && historyAt(repo, receipt.candidate, receipt.binds).receipts.length === 0;
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
        !proceeded && !retry && !candidateRefresh && !staleRetry
      ) fail('STALE_BINDING', `candidate ${entry.oid} lacks PROCEED`);
      exactInputs(receipt, planned.slice.consumes, `candidate ${entry.oid}`);
      if (planned.slice.consumes.length === 0 && Object.hasOwn(receipt, 'base')) {
        fail('STALE_BINDING', `candidate ${entry.oid} records a base for a non-consuming slice`);
      }
      const approval = approvals.get(plan.oid);
      const preparedBase = approval && planned.slice.consumes.length === 0
        ? preparePlanBoundBase(
          repo,
          receipt.release,
          plan,
          planned,
          receipt.binds,
          [],
          approvals,
        )
        : null;
      if (preparedBase && !isAncestor(repo, preparedBase, receipt.candidate)) {
        fail(
          'CHANGED_CANDIDATE',
          `candidate ${entry.oid} omits its exact prepared base`,
        );
      }
      const implementationBase = preparedBase ?? receipt.base ?? receipt.binds;
      assertCandidateRecordRootUnchanged(
        repo,
        implementationBase,
        receipt.candidate,
      );
      if (
        !approval
        || (
          preparedBase === null
          && !isAncestor(repo, approval.receipt.target, receipt.candidate)
        )
        || entry.parent !== receipt.candidate
        || receipt.candidate === receipt.binds
        || !isAncestor(repo, receipt.binds, receipt.candidate)
        || (receipt.base && !isAncestor(repo, receipt.base, receipt.candidate))
        || productTree(repo, receipt.candidate, productCache) !== receipt.product_tree
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

function consumedInputForPass(repo, sliceID, history, pass, productCache) {
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
      productTree(repo, commit, productCache) !== receipt.product_tree
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
      productCache,
    ));
  }
  return result;
}

function createPassProductBaseResolver(
  repo,
  release,
  histories,
  planByOID,
  approvals,
  productCache,
) {
  const memo = new Map();
  const pending = new Set();

  function passEntry(sliceID, oid) {
    const entry = histories.get(sliceID)?.entries.find((item) => item.oid === oid);
    if (!entry) fail('AMBIGUOUS_AUTHORITY', `${sliceID} PASS ${oid} is absent`);
    return entry;
  }

  function legacyConsumedInputs(plan, candidate, consumes) {
    const result = [];
    for (const dependency of consumes) {
      const lineage = slicePlanLineage(planByOID, plan, dependency);
      const contract = plan.parsed.metadata.contracts[dependency];
      const product = candidate.receipt.inputs[dependency];
      const matches = histories.get(dependency).entries.filter(({ receipt }) => (
        receipt.role === 'verifier'
        && receipt.result === 'pass'
        && receipt.contract === contract
        && lineage.has(receipt.plan)
        && receipt.product_tree === product
      ));
      if (matches.length === 0) {
        fail(
          'AMBIGUOUS_AUTHORITY',
          `legacy candidate ${candidate.oid} has no exact ${dependency} PASS authority`,
        );
      }
      const protectedMatches = matches.filter((match) => (
        isAncestor(repo, match.oid, candidate.receipt.candidate)
      ));
      let selected = protectedMatches[0] ?? null;
      for (const match of protectedMatches.slice(1)) {
        if (isAncestor(repo, selected.oid, match.oid)) {
          selected = match;
        } else if (!isAncestor(repo, match.oid, selected.oid)) {
          selected = null;
          break;
        }
      }
      if (selected === null && matches.length === 1) {
        [selected] = matches;
      }
      if (selected === null) {
        fail(
          'AMBIGUOUS_AUTHORITY',
          `legacy candidate ${candidate.oid} has ambiguous ${dependency} PASS authorities`,
        );
      }
      result.push(consumedInputForPass(
        repo,
        dependency,
        histories.get(dependency),
        selected,
        productCache,
      ));
    }
    return result;
  }

  function baselineFor(sliceID, pass) {
    const key = `${sliceID}:${pass.oid}`;
    if (memo.has(key)) return memo.get(key);
    if (pending.has(key)) {
      fail('DEPENDENCY_CYCLE', `product-base dependency cycle reaches ${sliceID}`);
    }
    pending.add(key);
    try {
      const plan = planByOID.get(pass.receipt.plan);
      const location = plan && locations(plan.parsed).get(sliceID);
      const approval = plan && approvals.get(plan.oid);
      if (!location || !approval) {
        fail('AMBIGUOUS_AUTHORITY', `${sliceID} PASS ${pass.oid} has no approved plan`);
      }
      const candidate = histories.get(sliceID).entries.find(
        (entry) => entry.oid === pass.receipt.binds,
      );
      if (
        !candidate
        || candidate.receipt.role !== 'implementer'
        || candidate.receipt.result !== 'candidate'
      ) {
        fail('AMBIGUOUS_AUTHORITY', `${sliceID} PASS ${pass.oid} has no exact candidate`);
      }
      const priorIDs = predecessorIDs(location);
      const priorInputs = consumedInputsAtBase(
        repo,
        plan,
        pass.oid,
        priorIDs,
        histories,
        planByOID,
        productCache,
      );
      if (priorInputs === null) {
        fail('AMBIGUOUS_AUTHORITY', `${sliceID} PASS ${pass.oid} omits prior slice authority`);
      }
      let consumedInputs = [];
      if (location.slice.consumes.length > 0) {
        consumedInputs = Object.hasOwn(candidate.receipt, 'base')
          ? consumedInputsAtBase(
            repo,
            plan,
            candidate.receipt.base,
            location.slice.consumes,
            histories,
            planByOID,
            productCache,
          )
          : legacyConsumedInputs(plan, candidate, location.slice.consumes);
        if (
          consumedInputs === null
          || !sameInputs(candidate.receipt.inputs, pinsForConsumedInputs(consumedInputs))
        ) {
          fail(
            'AMBIGUOUS_AUTHORITY',
            `${sliceID} candidate ${candidate.oid} has no exact consumed PASS bindings`,
          );
        }
      }

      let baseline = approval.receipt.target;
      for (const input of [...priorInputs, ...consumedInputs]) {
        const dependencyPass = passEntry(input.slice, input.pass_receipt);
        if (input.candidate === baseline || isAncestor(repo, input.candidate, baseline)) continue;
        baseline = unsafePrepareProductComposition(repo, {
          targetRef: `refs/heads/track/${release}/${location.track.id}`,
          expectedHead: baseline,
          candidate: input.candidate,
          productBase: () => baselineFor(input.slice, dependencyPass),
        }).result;
      }
      memo.set(key, baseline);
      return baseline;
    } finally {
      pending.delete(key);
    }
  }

  return baselineFor;
}

function reviewedConsumedInputs(
  repo,
  design,
  consumes,
  histories,
  planByOID,
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
    productCache,
  );
}

function pinsForConsumedInputs(inputs) {
  return Object.fromEntries(inputs.map((input) => [input.slice, input.product_tree]));
}

function prepareConsumedBase(repo, ref, seed, inputs) {
  let candidate = seed;
  for (const input of inputs) {
    if (
      input.pass_receipt === candidate
      || isAncestor(repo, input.pass_receipt, candidate)
    ) continue;
    const prepare = input.product_base
      ? unsafePrepareProductComposition
      : unsafePrepareExactComposition;
    candidate = prepare(repo, {
      targetRef: ref,
      expectedHead: candidate,
      candidate: input.pass_receipt,
      ...(input.product_base ? { productBase: input.product_base } : {}),
    }).result;
  }
  return candidate;
}

function withPassProductBases(inputs, productBaseForPass, histories) {
  return inputs.map((input) => {
    const pass = histories.get(input.slice)?.entries.find(
      (entry) => entry.oid === input.pass_receipt,
    );
    if (!pass) {
      fail(
        'AMBIGUOUS_AUTHORITY',
        `${input.slice} PASS ${input.pass_receipt} is absent`,
      );
    }
    return {
      ...input,
      product_base: () => productBaseForPass(input.slice, pass),
    };
  });
}

function preparePlanBoundBase(
  repo,
  release,
  plan,
  location,
  authority,
  inputs,
  approvals,
) {
  const approval = approvals.get(plan.oid);
  if (!approval) fail('APPROVAL_MISSING', `plan ${plan.oid} has no approval`);
  const ref = `refs/heads/track/${release}/${location.track.id}`;
  const targetBase = unsafePrepareApprovedTargetBase(repo, {
    targetRef: ref,
    expectedHead: authority,
    approvedTarget: approval.receipt.target,
  });
  return prepareConsumedBase(repo, ref, targetBase, inputs);
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
  productBaseForPass,
  productCache,
  preparedDesignCache,
) {
  const cacheKey = `${design.oid}:${design.parent}`;
  if (preparedDesignCache.has(cacheKey)) return preparedDesignCache.get(cacheKey);
  const remember = (value) => {
    preparedDesignCache.set(cacheKey, value);
    return value;
  };
  const plan = planByOID.get(design.receipt.plan);
  const location = plan && locations(plan.parsed).get(design.receipt.slice);
  if (!location) {
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
  const targetBase = preparePlanBoundBase(
    repo,
    release,
    plan,
    location,
    seed,
    [],
    approvals,
  );
  if (location.slice.consumes.length === 0) {
    if (targetBase !== design.parent) {
      fail('STALE_BINDING', `design ${design.oid} has an inexact approved-target base`);
    }
    return remember([]);
  }
  if (!Object.hasOwn(design.receipt, 'base')) {
    if (!isAncestor(repo, approval.receipt.target, design.parent)) {
      fail('STALE_BINDING', `design ${design.oid} omits its approved target`);
    }
    return remember(null);
  }
  if (design.receipt.base !== seed) {
    fail('STALE_BINDING', `design ${design.oid} has the wrong prior track authority`);
  }
  const selectedInputs = consumedInputsAtBase(
    repo,
    plan,
    design.parent,
    location.slice.consumes,
    histories,
    planByOID,
    productCache,
  );
  const inputs = selectedInputs === null
    ? null
    : withPassProductBases(selectedInputs, productBaseForPass, histories);
  if (
    !inputs
    || !sameInputs(design.receipt.inputs, pinsForConsumedInputs(inputs))
  ) fail('STALE_BINDING', `design ${design.oid} has stale reviewed-input pins`);
  const expected = preparePlanBoundBase(
    repo,
    release,
    plan,
    location,
    seed,
    inputs,
    approvals,
  );
  if (expected !== design.parent) {
    fail('STALE_BINDING', `design ${design.oid} has an inexact reviewed base`);
  }
  return remember(inputs);
}

function validateConsumedHistories(
  repo,
  release,
  histories,
  trackHistories,
  planByOID,
  approvals,
  releaseReceipts,
  productBaseForPass,
  productCache,
) {
  const preparedDesignCache = new Map();
  for (const [sliceID, history] of histories) {
    const byOID = new Map([...approvals.values()].map((entry) => [entry.oid, entry]));
    for (const entry of history.entries) byOID.set(entry.oid, entry);
    for (const entry of history.entries) {
      const receipt = entry.receipt;
      const plan = planByOID.get(receipt.plan);
      const location = plan && locations(plan.parsed).get(sliceID);
      if (!location) continue;
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
          productBaseForPass,
          productCache,
          preparedDesignCache,
        );
        if (location.slice.consumes.length === 0) continue;
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
      if (location.slice.consumes.length === 0) continue;
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
          productBaseForPass,
          productCache,
          preparedDesignCache,
        );
      } else if (design) {
        reviewed = reviewedConsumedInputs(
          repo,
          design,
          location.slice.consumes,
          histories,
          planByOID,
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
      const selectedInputs = consumedInputsAtBase(
        repo,
        plan,
        receipt.base,
        location.slice.consumes,
        histories,
        planByOID,
        productCache,
      );
      const inputs = selectedInputs === null
        ? null
        : withPassProductBases(selectedInputs, productBaseForPass, histories);
      if (
        !inputs
        || !sameInputs(receipt.inputs, pinsForConsumedInputs(inputs))
      ) fail('STALE_BINDING', `candidate ${entry.oid} has stale consumed pins`);
      const expected = preparePlanBoundBase(
        repo,
        release,
        plan,
        location,
        receipt.binds,
        inputs,
        approvals,
      );
      if (expected !== receipt.base) {
        fail('CHANGED_CANDIDATE', `candidate ${entry.oid} has an inexact prepared base`);
      }
      assertCandidateRecordRootUnchanged(repo, expected, receipt.candidate);
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
  releaseEntries,
  planByOID,
  approvals,
  sliceEntries,
  currentPlanOID,
  tracks,
  trackProductBaseFor,
  productCache,
) {
  const byOID = new Map(releaseEntries.map((entry) => [entry.oid, entry]));
  for (const entry of approvals.values()) byOID.set(entry.oid, entry);
  for (const entry of sliceEntries) byOID.set(entry.oid, entry);
  const predecessors = new Map();
  let predecessor = null;
  for (const entry of releaseEntries.filter(({ receipt }) => (
    receipt.role === 'planner' || !Object.hasOwn(receipt, 'slice')
  ))) {
    predecessors.set(entry.oid, predecessor);
    predecessor = entry;
  }
  for (const entry of entries) {
    const receipt = entry.receipt;
    const plan = planByOID.get(receipt.plan);
    if (!plan) fail('STALE_BINDING', `assembly receipt ${entry.oid} has an unknown plan`);
    const approval = approvals.get(receipt.plan);
    if (receipt.role === 'implementer') {
      const trackIDs = plan.parsed.metadata.tracks.map((track) => track.id);
      exactInputs(receipt, trackIDs, `assembly candidate ${entry.oid}`);
      const bound = byOID.get(receipt.binds);
      if (
        bound?.oid !== predecessors.get(entry.oid)?.oid
        || entry.parent !== receipt.candidate
        || receipt.base !== receipt.target
        || !isAncestor(repo, receipt.base, receipt.candidate)
        || !isAncestor(repo, receipt.binds, receipt.candidate)
        || !isAncestor(repo, approval.receipt.target, receipt.target)
        || productTree(repo, receipt.candidate, productCache) !== receipt.product_tree
      ) fail('STALE_BINDING', `assembly candidate ${entry.oid} has invalid evidence`);
      const currentPins = Object.fromEntries(tracks.map((track) => [
        track.id,
        track.slices.at(-1)?.pass?.receipt.product_tree ?? null,
      ]));
      if (
        receipt.plan === currentPlanOID
        && sameInputs(receipt.inputs, currentPins)
      ) {
        let expectedCandidate = unsafePrepareApprovedTargetBase(repo, {
          targetRef: plan.parsed.metadata.target_ref,
          expectedHead: receipt.binds,
          approvedTarget: receipt.target,
        });
        for (const component of tracks.map((track) => ({
          authority: track.slices.at(-1).pass.oid,
          product_base: () => trackProductBaseFor(track.id),
        }))) {
          if (
            component.authority === expectedCandidate
            || isAncestor(repo, component.authority, expectedCandidate)
          ) continue;
          expectedCandidate = unsafePrepareProductComposition(repo, {
            targetRef: plan.parsed.metadata.target_ref,
            expectedHead: expectedCandidate,
            candidate: component.authority,
            productBase: component.product_base,
          }).result;
        }
        if (receipt.candidate !== expectedCandidate) {
          fail(
            'STALE_BINDING',
            `assembly candidate ${entry.oid} is not the exact product-base composition`,
          );
        }
      }
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
        || !isAncestor(repo, approval.receipt.target, receipt.target)
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
      && isAncestor(repo, target, lastSlice.pass.receipt.candidate);
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

  const releaseHistory = readReleaseReceiptHistory(repo, release, captured[0].head);
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
    const scanned = historyAt(repo, ref.head, releaseHistory.boundary).receipts;
    for (const entry of scanned) {
      if (entry.receipt.release !== release) {
        fail('AMBIGUOUS_AUTHORITY', `track ${track.id} contains a foreign receipt`);
      }
    }
    const owned = scanned.filter((entry) => (
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
      productCache,
    ));
  }
  const productBaseForPass = createPassProductBaseResolver(
    repo,
    release,
    histories,
    planByOID,
    plans.approvals,
    productCache,
  );
  validateConsumedHistories(
    repo,
    release,
    histories,
    trackHistories,
    planByOID,
    plans.approvals,
    releaseHistory.receipts,
    productBaseForPass,
    productCache,
  );
  const states = deriveSlices(
    repo,
    current,
    histories,
    plans.approvals,
    planByOID,
    productCache,
  );
  const approval = plans.approvals.get(current.oid);
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
  const passProductBaseFor = (sliceID, passOID) => {
    const pass = histories.get(sliceID)?.entries.find((entry) => entry.oid === passOID);
    if (!pass) fail('AMBIGUOUS_AUTHORITY', `${sliceID} PASS ${passOID} is absent`);
    return productBaseForPass(sliceID, pass);
  };
  const trackProductBaseFor = (trackID) => {
    const track = current.parsed.metadata.tracks.find((entry) => entry.id === trackID);
    if (!track) fail('AMBIGUOUS_AUTHORITY', `track ${trackID} is absent`);
    const first = states.get(track.slices[0].id);
    if (!first?.pass) {
      fail('AMBIGUOUS_AUTHORITY', `track ${trackID} has no first-slice PASS`);
    }
    return productBaseForPass(track.slices[0].id, first.pass);
  };
  for (const [index, track] of current.parsed.metadata.tracks.entries()) {
    const head = captured[index + 2].head;
    const incomplete = track.slices.find(({ id }) => !states.get(id).pass);
    const active = incomplete
      ? (
        ['verifier', 'merge'].includes(states.get(incomplete.id).next_role)
          ? states.get(incomplete.id)
          : null
      )
      : states.get(track.slices.at(-1).id);
    if (
      !head || !active?.candidate
      || head === active.current_receipt.oid
    ) continue;
    const awaitingCandidateVerdict = (
      active.stage === 'verify'
      && active.next_role === 'verifier'
      && active.current_receipt.oid === active.candidate.oid
    );
    if (
      !awaitingCandidateVerdict
      || !linearOneParentAncestry(repo, active.candidate.oid, head)
      || historyAt(repo, head, active.candidate.oid).receipts.length > 0
    ) fail('CHANGED_CANDIDATE', `track ${track.id} moved after its current candidate`);
    assertCandidateRecordRootUnchanged(repo, active.candidate.oid, head);
    active.stage = 'implement';
    active.status = 'ready';
    active.next_role = 'implementer';
    active.outcome = 'stale';
    active.attempt = active.history.maximum_attempt + 1;
    active.retained = false;
    active.stale_reason = 'track head changed before verification was recorded';
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
    releaseHistory.receipts,
    planByOID,
    plans.approvals,
    allSliceEntries,
    current.oid,
    tracks,
    trackProductBaseFor,
    productCache,
  );
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
    && !isAncestor(repo, approval.receipt.target, captured[1].head);
  const diagnostics = [];
  if (targetStale) diagnostics.push(diagnostic(
    'TARGET_DIVERGED',
    'The target no longer contains the approved starting point; reconcile its history.',
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

  const result = frozen({
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
  productBaseEvidence.set(result, frozen({
    pass: passProductBaseFor,
    track: trackProductBaseFor,
  }));
  return result;
}

export function isBatonStateError(error) {
  return error instanceof BatonStateError
    || error instanceof ReceiptError
    || error instanceof GitRecordError;
}
