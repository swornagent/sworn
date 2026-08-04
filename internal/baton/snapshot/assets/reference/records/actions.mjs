import { types as utilTypes } from 'node:util';

import {
  assertCandidateRecordRootUnchanged,
  captureHeadRefs,
  commitParents,
  isAncestor,
  normalizeGitIdentity,
  productTreeIdentity,
  readFilesAtOID,
  repositoryRoot,
  resolveRecordPathAdmission,
  unsafeAtomicUpdateRefs,
  unsafePrepareApprovedTargetBase,
  unsafePrepareExactComposition,
  unsafePrepareProductComposition,
  unsafePrepareMetadataCommit,
  unsafePrepareRecordTransition,
} from './git.mjs';
import {
  canonicalJSON,
  digestBytes,
  parsePlanBytes,
  parseReceiptCommitMessage,
  renderReceiptCommit,
} from './receipts.mjs';
import {
  readBatonState,
  readReleaseReceiptHistory,
  unsafeProductBaseEvidence,
} from './state.mjs';

const RECORD_ROOT = '.baton/releases';
const MAX_SUMMARY = 280;
const MAX_DETAIL = 8_192;
const MAX_CHECK_RESULTS = 1_048_576;
const MAX_CANDIDATE_LINEAGE = 4096;
const OID = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/;
const IDENTITY = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;

export class BatonActionError extends Error {
  constructor(code, message, cause) {
    super(message, cause ? { cause } : undefined);
    this.name = 'BatonActionError';
    this.code = code;
  }
}

function fail(code, message, cause) {
  throw new BatonActionError(code, message, cause);
}

function exactOptions(value, required, optional, label) {
  if (
    value === null
    || typeof value !== 'object'
    || Array.isArray(value)
    || utilTypes.isProxy(value)
    || ![Object.prototype, null].includes(Object.getPrototypeOf(value))
  ) {
    fail('INVALID_ACTION_INPUT', `${label} requires one plain options object`);
  }
  const allowed = new Set([...required, ...optional]);
  const result = Object.create(null);
  for (const key of Reflect.ownKeys(value)) {
    if (typeof key !== 'string' || !allowed.has(key)) {
      fail('INVALID_ACTION_INPUT', `${label} received an unknown option`);
    }
    const descriptor = Object.getOwnPropertyDescriptor(value, key);
    if (!descriptor?.enumerable || !Object.hasOwn(descriptor, 'value')) {
      fail('INVALID_ACTION_INPUT', `${label} options must be plain enumerable data`);
    }
    result[key] = descriptor.value;
  }
  for (const key of required) {
    if (!Object.hasOwn(result, key)) {
      fail('INVALID_ACTION_INPUT', `${label} requires ${key}`);
    }
  }
  return Object.freeze(result);
}

function text(value, label, maximum, { nonempty = true } = {}) {
  if (
    typeof value !== 'string'
    || Buffer.byteLength(value, 'utf8') > maximum
    || (nonempty && value.trim().length === 0)
  ) {
    fail(
      'INVALID_ACTION_INPUT',
      `${label} must be ${nonempty ? 'a non-empty' : 'an'} UTF-8 string of at most ${maximum} bytes`,
    );
  }
  return value;
}

function detailBytes(value = Buffer.alloc(0)) {
  if (!(typeof value === 'string' || Buffer.isBuffer(value))) {
    fail('INVALID_ACTION_INPUT', 'detail must be a string or Buffer');
  }
  const result = Buffer.from(value);
  if (result.byteLength > MAX_DETAIL) {
    fail('INVALID_ACTION_INPUT', `detail must be at most ${MAX_DETAIL} bytes`);
  }
  return result;
}

function evidenceBytes(value, label) {
  if (!(typeof value === 'string' || Buffer.isBuffer(value))) {
    fail('INVALID_ACTION_INPUT', `${label} must be a string or Buffer`);
  }
  const result = Buffer.from(value);
  if (result.byteLength > MAX_CHECK_RESULTS) {
    fail(
      'INVALID_ACTION_INPUT',
      `${label} must be at most ${MAX_CHECK_RESULTS} bytes`,
    );
  }
  return result;
}

function objectID(value, label) {
  if (typeof value !== 'string' || !OID.test(value)) {
    fail('INVALID_ACTION_INPUT', `${label} must be one full Git object ID`);
  }
  return value;
}

function identity(value, label) {
  if (typeof value !== 'string' || !IDENTITY.test(value)) {
    fail('INVALID_ACTION_INPUT', `${label} must be one portable identity`);
  }
  return value;
}

function frozen(value, seen = new WeakSet()) {
  if (value === null || typeof value !== 'object' || seen.has(value)) return value;
  seen.add(value);
  for (const nested of Object.values(value)) frozen(nested, seen);
  return Object.freeze(value);
}

function receiptResult(action, changed, details) {
  return frozen({
    kind: 'baton.action-result/v2',
    action,
    changed,
    ...details,
  });
}

function releaseRef(release) {
  return `refs/heads/release-wt/${release}`;
}

function trackRef(release, track) {
  return `refs/heads/track/${release}/${track}`;
}

function planPath(release) {
  return `${RECORD_ROOT}/${release}/plan.md`;
}

function captureMap(repo, refs) {
  return new Map(captureHeadRefs(repo, refs).map(({ ref, head }) => [ref, head]));
}

function receiptHistory(repo, release, head) {
  return readReleaseReceiptHistory(repo, release, head).receipts;
}

function fileAt(repo, commit, relativePath) {
  const [entry] = readFilesAtOID(repo, commit, [relativePath]);
  return entry;
}

function currentPlan(repo, release, releaseHead) {
  const entry = fileAt(repo, releaseHead, planPath(release));
  if (!entry?.bytes || !entry.object) {
    fail('PLAN_NOT_FOUND', `release ${release} has no current plan`);
  }
  return Object.freeze({
    parsed: parsePlanBytes(entry.bytes),
    object: entry.object,
  });
}

function findApproval(repo, release, releaseHead, planObject) {
  const approval = receiptHistory(repo, release, releaseHead).find(({ receipt }) => (
    receipt.role === 'planner'
    && receipt.result === 'approved'
    && receipt.plan === planObject
  ));
  if (
    !approval
    || approval.receipt.binds !== approval.parent
  ) {
    fail('PLAN_NOT_APPROVED', `plan ${planObject} has no applicable approval receipt`);
  }
  return approval;
}

function assertRevision(previous, next, previousObject) {
  if (next.metadata.revision !== previous.metadata.revision + 1) {
    fail('INVALID_PLAN_REVISION', 'plan revision must advance by exactly one');
  }
  if (next.metadata.previous_plan !== previousObject) {
    fail('INVALID_PLAN_REVISION', 'plan previous_plan must bind the current plan blob');
  }
  for (const field of ['release', 'repository', 'target_ref']) {
    if (next.metadata[field] !== previous.metadata[field]) {
      fail(
        'REPLACED_RELEASE_AUTHORITY',
        `plan revision cannot change ${field}; create a new release`,
      );
    }
  }
  if (next.metadata.approval_ref === previous.metadata.approval_ref) {
    fail('STALE_APPROVAL', 'plan revision requires a new protected approval reference');
  }
}

function planReceipt({
  release,
  planObject,
  planCommit,
  target,
  summary,
  detail,
}) {
  const message = renderReceiptCommit({
    subject: `baton(${release}): approve plan`,
    detail,
    receipt: {
      version: 1,
      release,
      role: 'planner',
      result: 'approved',
      plan: planObject,
      binds: planCommit,
      detail: digestBytes(Buffer.alloc(0)),
      summary,
      target,
    },
  });
  return Object.freeze({
    message,
    receipt: parseReceiptCommitMessage(message).receipt,
  });
}

function planResult({
  changed,
  parsed,
  planObject,
  approval,
  ref,
  target,
  head = approval.oid,
  retirements = [],
}) {
  return receiptResult('recordPlanRevision', changed, {
    release: parsed.metadata.release,
    revision: parsed.metadata.revision,
    plan: planObject,
    ref,
    head,
    target,
    receipt_commit: approval.oid,
    receipt: approval.receipt,
    retirements,
  });
}

function sameInputs(left, right) {
  return canonicalJSON(left ?? {}) === canonicalJSON(right ?? {});
}

function currentTrack(state, trackID) {
  const track = state.tracks.find(({ id }) => id === trackID);
  if (!track) fail('TRACK_NOT_FOUND', `plan has no track ${trackID}`);
  return track;
}

function currentSlice(state, sliceID) {
  const slice = state.slices.find(({ location }) => location.slice.id === sliceID);
  if (!slice) fail('SLICE_NOT_FOUND', `plan has no current slice ${sliceID}`);
  return slice;
}

function requireSlicePrerequisites(state, slice) {
  const { location } = slice;
  const track = currentTrack(state, location.track.id);
  const position = track.slices.findIndex(
    ({ location: item }) => item.slice.id === location.slice.id,
  );
  const required = new Set([
    ...track.depends_on.flatMap(
      (trackID) => currentTrack(state, trackID).slices.map(
        ({ location: item }) => item.slice.id,
      ),
    ),
    ...track.slices.slice(0, position).map(
      ({ location: item }) => item.slice.id,
    ),
    ...location.slice.depends_on,
    ...location.slice.consumes,
  ]);
  for (const dependency of required) {
    if (!currentSlice(state, dependency).pass) {
      fail(
        'DEPENDENCIES_NOT_READY',
        `${location.slice.id} is waiting for ${dependency} PASS`,
      );
    }
  }
}

function currentConsumedInputs(state, slice) {
  if (slice.location.slice.consumes.length === 0) return [];
  if (
    !slice.input_pins
    || slice.consumed_inputs.length !== slice.location.slice.consumes.length
  ) {
    fail(
      'DEPENDENCIES_NOT_READY',
      `${slice.location.slice.id} has no complete consumed PASS authority`,
    );
  }
  const evidence = unsafeProductBaseEvidence(state);
  return slice.consumed_inputs.map((input) => Object.freeze({
    ...input,
    product_base: () => evidence.pass(input.slice, input.pass_receipt),
  }));
}

function prepareConsumedTrackBase(repo, consumerRef, seed, inputs, gitIdentity) {
  let candidate = seed;
  for (const input of inputs) {
    if (
      input.pass_receipt === candidate
      || isAncestor(repo, input.pass_receipt, candidate)
    ) continue;
    const prepared = unsafePrepareProductComposition(repo, {
      targetRef: consumerRef,
      expectedHead: candidate,
      candidate: input.pass_receipt,
      productBase: input.product_base,
      identity: gitIdentity,
    });
    candidate = prepared.result;
  }
  return candidate;
}

function preparedTrackBase(repo, state, slice, gitIdentity) {
  const track = currentTrack(state, slice.location.track.id);
  const inputs = currentConsumedInputs(state, slice);
  const seed = track.authority_head;
  const targetBase = unsafePrepareApprovedTargetBase(repo, {
    targetRef: track.ref,
    expectedHead: seed,
    approvedTarget: state.plan.approval.receipt.target,
    identity: gitIdentity,
  });
  return Object.freeze({
    track,
    inputs,
    seed,
    base: prepareConsumedTrackBase(
      repo,
      track.ref,
      targetBase,
      inputs,
      gitIdentity,
    ),
  });
}

function requireTargetLineage(repo, state) {
  const approved = state.plan.approval.receipt.target;
  const current = state.refs.target.head;
  if (!isAncestor(repo, approved, current)) {
    fail(
      'TARGET_DIVERGED',
      'The target no longer contains the approved starting point; '
        + 'reconcile its history before continuing.',
    );
  }
}

function prepareAssemblyCandidate(repo, targetRef, binds, target, tracks, gitIdentity) {
  let candidate = unsafePrepareApprovedTargetBase(repo, {
    targetRef,
    expectedHead: binds,
    approvedTarget: target,
    identity: gitIdentity,
  });
  for (const component of tracks) {
    if (
      component.authority === candidate
      || isAncestor(repo, component.authority, candidate)
    ) continue;
    candidate = unsafePrepareProductComposition(repo, {
      targetRef,
      expectedHead: candidate,
      candidate: component.authority,
      productBase: component.product_base,
      identity: gitIdentity,
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

function consumedRefVerifications(slice, ownerRef) {
  const seen = new Set();
  const operations = [];
  for (const input of slice.consumed_inputs) {
    if (input.source_ref === ownerRef || seen.has(input.source_ref)) continue;
    seen.add(input.source_ref);
    operations.push({
      kind: 'verify',
      ref: input.source_ref,
      expectedHead: input.source_head,
    });
  }
  return operations;
}

function exactRetry(entry, {
  role,
  result,
  summary,
  detail,
  candidate,
  base,
  checks,
}) {
  if (!entry) return false;
  const receipt = entry.receipt;
  return (
    receipt.role === role
    && receipt.result === result
    && receipt.summary === summary
    && (
      candidate === null
        ? !Object.hasOwn(receipt, 'candidate')
        : receipt.candidate === candidate
    )
    && (
      (role === 'implementer' && result === 'designed')
      || (
        base === null
        ? !Object.hasOwn(receipt, 'base')
        : receipt.base === base
      )
    )
    && (checks === null || receipt.checks === checks)
    && entry.detail.equals(detail)
  );
}

function appendResult(changed, ref, entry) {
  return receiptResult('appendReceipt', changed, {
    release: entry.receipt.release,
    slice: entry.receipt.slice ?? null,
    ref,
    receipt_commit: entry.oid,
    receipt: entry.receipt,
  });
}

function actionEntry(oid, parent, message) {
  const parsed = parseReceiptCommitMessage(message);
  return Object.freeze({
    oid,
    parent,
    receipt: parsed.receipt,
    get detail() {
      return parsed.detail;
    },
  });
}

export function createBatonActions(options) {
  const admitted = exactOptions(
    options,
    ['repo', 'identity'],
    [],
    'createBatonActions',
  );
  if (typeof admitted.repo !== 'string' || admitted.repo.length === 0) {
    fail('INVALID_ACTION_INPUT', 'repo must be a non-empty path');
  }
  const repo = repositoryRoot(admitted.repo);
  let gitIdentity;
  try {
    gitIdentity = normalizeGitIdentity(admitted.identity);
  } catch (error) {
    fail('INVALID_ACTION_INPUT', error.message, error);
  }
  const recordPathAdmission = resolveRecordPathAdmission(repo);

  function stateFor(release) {
    return readBatonState(repo, release);
  }

  function recordPlanRevision(rawOptions) {
    const input = exactOptions(
      rawOptions,
      ['planBytes', 'summary'],
      ['detail'],
      'recordPlanRevision',
    );
    const parsed = parsePlanBytes(Buffer.from(input.planBytes));
    const summary = text(input.summary, 'summary', MAX_SUMMARY);
    const detail = detailBytes(input.detail);
    const release = parsed.metadata.release;
    const targetRef = parsed.metadata.target_ref;
    const ownerRef = releaseRef(release);
    const refs = captureMap(repo, [targetRef, ownerRef]);
    const target = refs.get(targetRef);
    const priorHead = refs.get(ownerRef);
    if (!target) fail('TARGET_NOT_FOUND', `target ${targetRef} does not exist`);

    let parent;
    let previousState = null;
    if (priorHead === null) {
      if (parsed.metadata.revision !== 1 || parsed.metadata.previous_plan !== null) {
        fail('INVALID_PLAN_REVISION', 'a new release must begin at plan revision 1');
      }
      if (fileAt(repo, target, planPath(release)).object !== null) {
        fail('RELEASE_ALREADY_RECORDED', `target already contains release ${release}`);
      }
      parent = target;
    } else {
      previousState = stateFor(release);
      const previous = currentPlan(repo, release, priorHead);
      if (previous.parsed.bytes.equals(parsed.bytes)) {
        const approval = findApproval(repo, release, priorHead, previous.object);
        if (!isAncestor(repo, approval.receipt.target, target)) {
          fail(
            'TARGET_DIVERGED',
            'The target no longer contains this plan\'s approved starting point; '
              + 'reconcile its history before continuing.',
          );
        }
        return planResult({
          changed: false,
          parsed: previous.parsed,
          planObject: previous.object,
          approval: {
            oid: approval.oid,
            receipt: approval.receipt,
          },
          ref: ownerRef,
          target: approval.receipt.target,
          head: priorHead,
        });
      }
      assertRevision(previous.parsed, parsed, previous.object);
      const retiredIDs = new Set(
        receiptHistory(repo, release, priorHead)
          .filter(({ receipt }) => (
            receipt.role === 'planner' && receipt.result === 'retired'
          ))
          .map(({ receipt }) => receipt.slice),
      );
      for (const slice of parsed.metadata.tracks.flatMap((track) => track.slices)) {
        if (retiredIDs.has(slice.id)) {
          fail('INVALID_RETIREMENT', `retired slice ${slice.id} cannot be re-added`);
        }
      }
      parent = priorHead;
    }

    const preparedPlan = unsafePrepareRecordTransition(repo, {
      expectedHead: parent,
      message: `baton(${release}): plan revision ${parsed.metadata.revision}`,
      recordPathAdmission,
      changes: {
        [planPath(release)]: parsed.bytes,
      },
      identity: gitIdentity,
    });
    const planObject = fileAt(repo, preparedPlan.commit, planPath(release)).object;
    if (!planObject) fail('PLAN_NOT_FOUND', 'prepared plan blob could not be resolved');
    const rendered = planReceipt({
      release,
      planObject,
      planCommit: preparedPlan.commit,
      target,
      summary,
      detail,
    });
    const preparedApproval = unsafePrepareMetadataCommit(repo, {
      expectedHead: preparedPlan.commit,
      message: rendered.message,
      identity: gitIdentity,
    });
    let nextHead = preparedApproval.commit;
    const retirements = [];
    if (previousState) {
      const retained = new Set(
        parsed.metadata.tracks.flatMap((track) => track.slices.map((slice) => slice.id)),
      );
      for (const removed of previousState.slices.filter(
        ({ location }) => !retained.has(location.slice.id),
      )) {
        const slice = removed.location.slice.id;
        const retirementMessage = renderReceiptCommit({
          subject: `baton(${release}/${slice}): retire slice`,
          detail: Buffer.alloc(0),
          receipt: {
            version: 1,
            release,
            slice,
            role: 'planner',
            result: 'retired',
            attempt: removed.history.maximum_attempt + 1,
            plan: planObject,
            contract: previousState.plan.metadata.contracts[slice],
            binds: preparedApproval.commit,
            detail: digestBytes(Buffer.alloc(0)),
            summary: `Retired ${slice} under approved plan revision ${parsed.metadata.revision}.`,
          },
        });
        const preparedRetirement = unsafePrepareMetadataCommit(repo, {
          expectedHead: nextHead,
          message: retirementMessage,
          identity: gitIdentity,
        });
        retirements.push(Object.freeze({
          slice,
          receipt_commit: preparedRetirement.commit,
          receipt: parseReceiptCommitMessage(retirementMessage).receipt,
        }));
        nextHead = preparedRetirement.commit;
      }
    }
    const operations = [
      { kind: 'verify', ref: targetRef, expectedHead: target },
      priorHead === null
        ? { kind: 'create', ref: ownerRef, newHead: nextHead }
        : {
          kind: 'update',
          ref: ownerRef,
          newHead: nextHead,
          expectedHead: priorHead,
        },
    ];
    unsafeAtomicUpdateRefs(repo, operations);
    return planResult({
      changed: true,
      parsed,
      planObject,
      approval: {
        oid: preparedApproval.commit,
        receipt: rendered.receipt,
      },
      ref: ownerRef,
      target,
      head: nextHead,
      retirements,
    });
  }

  function appendReceipt(rawOptions) {
    const input = exactOptions(
      rawOptions,
      ['release', 'role', 'result', 'summary'],
      ['slice', 'detail', 'candidate', 'base', 'checkResults'],
      'appendReceipt',
    );
    const release = identity(input.release, 'release');
    const role = text(input.role, 'role', 16);
    const result = text(input.result, 'result', 16);
    const summary = text(input.summary, 'summary', MAX_SUMMARY);
    const detail = detailBytes(input.detail);
    const sliceID = Object.hasOwn(input, 'slice')
      ? identity(input.slice, 'slice')
      : null;
    const candidate = Object.hasOwn(input, 'candidate')
      ? objectID(input.candidate, 'candidate')
      : null;
    const base = Object.hasOwn(input, 'base')
      ? objectID(input.base, 'base')
      : null;
    const evidenceRequired = (
      (role === 'implementer' && result === 'candidate')
      || role === 'verifier'
    );
    if (Object.hasOwn(input, 'checkResults') !== evidenceRequired) {
      fail(
        'INVALID_ACTION_INPUT',
        evidenceRequired
          ? `${role}/${result} requires checkResults`
          : `${role}/${result} does not accept checkResults`,
      );
    }
    const checks = evidenceRequired
      ? digestBytes(evidenceBytes(input.checkResults, 'checkResults'))
      : null;
    const state = stateFor(release);

    let ownerRef;
    let ownerHead;
    let parent;
    let receipt;
    let current;
    if (sliceID !== null) {
      const slice = currentSlice(state, sliceID);
      const location = slice.location;
      const track = currentTrack(state, location.track.id);
      ownerRef = track.ref;
      ownerHead = track.head;
      current = slice.current_receipt;
      if (
        base !== null
        && !(role === 'implementer' && result === 'candidate')
      ) fail('INVALID_ACTION_INPUT', `${role}/${result} does not accept base`);
      const actionIsEligible = (
        (
          role === 'implementer'
          && result === 'designed'
          && slice.next_role === 'implementer'
          && slice.stage === 'design'
        )
        || (
          role === 'captain'
          && ['proceed', 'revise', 'escalate'].includes(result)
          && slice.next_role === 'captain'
        )
        || (
          role === 'implementer'
          && result === 'candidate'
          && slice.next_role === 'implementer'
          && slice.stage === 'implement'
        )
        || (
          role === 'verifier'
          && ['pass', 'fail', 'blocked'].includes(result)
          && slice.next_role === 'verifier'
        )
      );
      if (!actionIsEligible && exactRetry(current, {
        role,
        result,
        summary,
        detail,
        candidate,
        base,
        checks,
      })) {
        return appendResult(false, ownerRef, current);
      }
      requireSlicePrerequisites(state, slice);

      let attempt;
      let binds;
      const common = {
        version: 1,
        release,
        slice: sliceID,
        plan: state.plan.oid,
        contract: state.plan.metadata.contracts[sliceID],
        detail: digestBytes(Buffer.alloc(0)),
        summary,
      };
      if (role === 'implementer' && result === 'designed') {
        if (slice.next_role !== 'implementer' || slice.stage !== 'design') {
          fail('ROLE_NOT_ELIGIBLE', `${sliceID} does not currently need an Implementer design`);
        }
        attempt = slice.attempt;
        binds = current.oid;
        parent = ownerHead ?? state.refs.release.head;
        const prepared = preparedTrackBase(
          repo,
          state,
          slice,
          gitIdentity,
        );
        if (parent !== prepared.base) {
          fail(
            'TRACK_BASE_NOT_PREPARED',
            `${ownerRef} does not equal the exact current approved-target and consumed-input base`,
          );
        }
        let reviewedEvidence = {};
        if (location.slice.consumes.length > 0) {
          reviewedEvidence = {
            base: prepared.seed,
            inputs: slice.input_pins,
          };
        }
        receipt = {
          ...common,
          role,
          result,
          attempt,
          binds,
          ...reviewedEvidence,
        };
      } else if (role === 'captain' && ['proceed', 'revise', 'escalate'].includes(result)) {
        if (slice.next_role !== 'captain') {
          fail('ROLE_NOT_ELIGIBLE', `${sliceID} does not currently need Captain review`);
        }
        attempt = current.receipt.attempt;
        binds = current.oid;
        receipt = { ...common, role, result, attempt, binds };
        parent = ownerHead;
        if (ownerHead !== current.oid) {
          fail('CHANGED_OWNER_HEAD', `${ownerRef} changed after its design receipt`);
        }
      } else if (role === 'implementer' && result === 'candidate') {
        if (slice.next_role !== 'implementer' || slice.stage !== 'implement') {
          fail('ROLE_NOT_ELIGIBLE', `${sliceID} does not currently need an implementation candidate`);
        }
        if (candidate === null || ownerHead !== candidate) {
          fail('CHANGED_CANDIDATE', 'candidate must be the exact captured track head');
        }
        attempt = slice.attempt;
        binds = current.oid;
        const prepared = preparedTrackBase(
          repo,
          state,
          slice,
          gitIdentity,
        );
        if (location.slice.consumes.length > 0 && base !== prepared.base) {
          fail(
            'CHANGED_CANDIDATE',
            'consuming candidate must bind the exact prepared base',
          );
        }
        if (location.slice.consumes.length === 0 && base !== null) {
          fail('INVALID_ACTION_INPUT', 'non-consuming candidate does not accept base');
        }
        if (
          location.slice.consumes.length === 0
          && !isAncestor(repo, prepared.base, candidate)
        ) {
          fail(
            'CHANGED_CANDIDATE',
            'non-consuming candidate omits the exact prepared base',
          );
        }
        if (
          base !== null
          && !linearOneParentAncestry(repo, base, candidate)
        ) {
          fail(
            'CHANGED_CANDIDATE',
            'consuming candidate must be linear one-parent work from its exact base',
          );
        }
        for (const input of prepared.inputs) {
          for (const ancestor of [
            input.candidate,
            input.candidate_receipt,
            input.pass_receipt,
          ]) {
            if (!isAncestor(repo, ancestor, candidate)) {
              fail(
                'CHANGED_CANDIDATE',
                `candidate omits consumed authority ${ancestor}`,
              );
            }
          }
        }
        assertCandidateRecordRootUnchanged(repo, prepared.base, candidate);
        const identity = productTreeIdentity(repo, candidate);
        receipt = {
          ...common,
          role,
          result,
          attempt,
          binds,
          candidate,
          ...(base !== null ? { base } : {}),
          product_tree: identity.productTree,
          inputs: slice.input_pins ?? {},
          checks,
        };
        parent = candidate;
      } else if (role === 'verifier' && ['pass', 'fail', 'blocked'].includes(result)) {
        if (slice.next_role !== 'verifier') {
          fail('ROLE_NOT_ELIGIBLE', `${sliceID} does not currently need verification`);
        }
        const evidence = current.receipt;
        if (candidate === null || candidate !== evidence.candidate) {
          fail('CHANGED_CANDIDATE', 'Verifier must bind the exact current candidate');
        }
        attempt = evidence.attempt;
        binds = current.oid;
        receipt = {
          ...common,
          role,
          result,
          attempt,
          binds,
          candidate,
          product_tree: evidence.product_tree,
          inputs: evidence.inputs,
          checks,
        };
        parent = ownerHead;
        if (ownerHead !== current.oid) {
          fail('CHANGED_OWNER_HEAD', `${ownerRef} changed after its candidate receipt`);
        }
      } else {
        fail('INVALID_ACTION_INPUT', `unsupported slice receipt ${role}/${result}`);
      }
    } else {
      ownerRef = state.refs.release.ref;
      ownerHead = state.refs.release.head;
      current = state.assembly.current_receipt;
      if (exactRetry(current, {
        role,
        result,
        summary,
        detail,
        candidate,
        base,
        checks,
      })) {
        return appendResult(false, ownerRef, current);
      }
      if (
        role !== 'verifier'
        || !['pass', 'fail', 'blocked'].includes(result)
        || state.assembly.next_role !== 'verifier'
      ) {
        fail('ROLE_NOT_ELIGIBLE', 'the assembly does not currently need a Verifier verdict');
      }
      const evidence = state.assembly.candidate?.receipt;
      if (!evidence || candidate === null || evidence.candidate !== candidate) {
        fail('CHANGED_CANDIDATE', 'Verifier must bind the exact current assembly candidate');
      }
      receipt = {
        version: 1,
        release,
        role,
        result,
        plan: state.plan.oid,
        binds: state.assembly.candidate.oid,
        detail: digestBytes(Buffer.alloc(0)),
        summary,
        candidate,
        product_tree: evidence.product_tree,
        inputs: evidence.inputs,
        checks,
      };
      parent = ownerHead;
      if (ownerHead !== state.assembly.current_receipt.oid) {
        fail('CHANGED_OWNER_HEAD', `${ownerRef} changed after its assembly candidate receipt`);
      }
    }

    requireTargetLineage(repo, state);
    const message = renderReceiptCommit({
      subject: `baton(${release}${sliceID ? `/${sliceID}` : ''}): ${role} ${result}`,
      detail,
      receipt,
    });
    const prepared = unsafePrepareMetadataCommit(repo, {
      expectedHead: parent,
      message,
      identity: gitIdentity,
    });
    const operations = [];
    if (ownerRef !== state.refs.release.ref) {
      operations.push({
        kind: 'verify',
        ref: state.refs.release.ref,
        expectedHead: state.refs.release.head,
      });
      if (
        sliceID !== null
        && role === 'implementer'
        && ['designed', 'candidate'].includes(result)
      ) operations.push(...consumedRefVerifications(currentSlice(state, sliceID), ownerRef));
    }
    operations.push(
      ownerHead === null
        ? { kind: 'create', ref: ownerRef, newHead: prepared.commit }
        : {
          kind: 'update',
          ref: ownerRef,
          newHead: prepared.commit,
          expectedHead: ownerHead,
        },
    );
    unsafeAtomicUpdateRefs(repo, operations);
    return appendResult(
      true,
      ownerRef,
      actionEntry(prepared.commit, parent, message),
    );
  }

  function prepareTrackBase(rawOptions) {
    const input = exactOptions(
      rawOptions,
      ['release', 'slice'],
      [],
      'prepareTrackBase',
    );
    const release = identity(input.release, 'release');
    const sliceID = identity(input.slice, 'slice');
    const state = stateFor(release);
    requireTargetLineage(repo, state);
    const slice = currentSlice(state, sliceID);
    requireSlicePrerequisites(state, slice);
    if (
      slice.next_role !== 'implementer'
      || !['design', 'implement'].includes(slice.stage)
    ) {
      fail(
        'ROLE_NOT_ELIGIBLE',
        `${sliceID} does not currently need an Implementer base`,
      );
    }
    const prepared = preparedTrackBase(
      repo,
      state,
      slice,
      gitIdentity,
    );
    const pins = Object.fromEntries(
      prepared.inputs.map((item) => [item.slice, item.product_tree]),
    );
    const authorities = prepared.inputs.map((item) => ({
      slice: item.slice,
      pass_receipt: item.pass_receipt,
      candidate_receipt: item.candidate_receipt,
      candidate: item.candidate,
      product_tree: item.product_tree,
    }));
    const snapshotOperations = [
      {
        kind: 'verify',
        ref: state.refs.release.ref,
        expectedHead: state.refs.release.head,
      },
      ...consumedRefVerifications(slice, prepared.track.ref),
    ];
    if (
      prepared.inputs.length === 0
      && prepared.track.head === null
      && prepared.base === prepared.seed
    ) {
      unsafeAtomicUpdateRefs(repo, [
        ...snapshotOperations,
        {
          kind: 'verify',
          ref: prepared.track.ref,
          expectedHead: prepared.track.head,
        },
      ]);
      return receiptResult('prepareTrackBase', false, {
        release,
        slice: sliceID,
        ref: prepared.track.ref,
        base: prepared.track.head ?? prepared.base,
        pins,
        authorities,
      });
    }
    const preservesCandidateRefresh = (
      slice.stage === 'implement'
      && slice.next_role === 'implementer'
      && slice.current_receipt.oid === slice.candidate?.oid
      && prepared.base === prepared.track.authority_head
      && prepared.track.head !== null
      && prepared.track.head !== prepared.track.authority_head
    );
    if (
      prepared.track.head !== null
      && prepared.track.head !== prepared.track.authority_head
      && prepared.track.head !== prepared.base
      && !preservesCandidateRefresh
    ) {
      fail(
        'CHANGED_OWNER_HEAD',
        `${prepared.track.ref} moved beyond its authoritative receipt`,
      );
    }
    if (prepared.track.head === prepared.base || preservesCandidateRefresh) {
      unsafeAtomicUpdateRefs(repo, [
        ...snapshotOperations,
        {
          kind: 'verify',
          ref: prepared.track.ref,
          expectedHead: prepared.track.head,
        },
      ]);
      return receiptResult('prepareTrackBase', false, {
        release,
        slice: sliceID,
        ref: prepared.track.ref,
        base: prepared.base,
        pins,
        authorities,
      });
    }
    const operations = [
      ...snapshotOperations,
      prepared.track.head === null
        ? {
          kind: 'create',
          ref: prepared.track.ref,
          newHead: prepared.base,
        }
        : {
          kind: 'update',
          ref: prepared.track.ref,
          newHead: prepared.base,
          expectedHead: prepared.track.head,
        },
    ];
    unsafeAtomicUpdateRefs(repo, operations);
    return receiptResult('prepareTrackBase', true, {
      release,
      slice: sliceID,
      ref: prepared.track.ref,
      base: prepared.base,
      pins,
      authorities,
    });
  }

  function prepareAssembly(rawOptions) {
    const input = exactOptions(
      rawOptions,
      ['release', 'summary'],
      ['detail', 'checkResults'],
      'prepareAssembly',
    );
    const release = identity(input.release, 'release');
    const summary = text(input.summary, 'summary', MAX_SUMMARY);
    const detail = detailBytes(input.detail);
    const state = stateFor(release);
    requireTargetLineage(repo, state);
    for (const slice of state.slices) {
      if (!slice.pass) {
        fail('SLICE_PASS_REQUIRED', `${slice.location.slice.id} has no current PASS`);
      }
    }

    const trackCandidates = [];
    const productBases = unsafeProductBaseEvidence(state);
    for (const track of state.tracks) {
      const finalPass = track.slices.at(-1)?.pass;
      if (!finalPass) fail('SLICE_PASS_REQUIRED', `track ${track.id} has no final PASS`);
      for (const slice of track.slices) {
        if (!isAncestor(repo, slice.pass.receipt.candidate, finalPass.receipt.candidate)) {
          fail('INVALID_TRACK_TOPOLOGY', `track ${track.id} candidates are not one serial lineage`);
        }
      }
      trackCandidates.push({
        id: track.id,
        candidate: finalPass.receipt.candidate,
        authority: finalPass.oid,
        product_base: () => productBases.track(track.id),
        product_tree: finalPass.receipt.product_tree,
      });
    }
    const inputs = Object.fromEntries(
      trackCandidates.map((track) => [track.id, track.product_tree]),
    );
    const target = state.refs.target.head;
    if (
      trackCandidates.length === 1
      && state.tracks[0].slices.length === 1
      && isAncestor(repo, target, trackCandidates[0].candidate)
    ) {
      return receiptResult('prepareAssembly', false, {
        release,
        direct: true,
        candidate: trackCandidates[0].candidate,
        inputs,
        receipt_commit: state.tracks[0].slices.at(-1).pass.oid,
      });
    }
    const existing = state.assembly.candidate;
    if (
      existing
      && existing.receipt.target === target
      && sameInputs(existing.receipt.inputs, inputs)
    ) {
      return receiptResult('prepareAssembly', false, {
        release,
        direct: false,
        candidate: existing.receipt.candidate,
        inputs,
        receipt_commit: existing.oid,
      });
    }

    const authorityHistory = readReleaseReceiptHistory(
      repo,
      release,
      state.refs.release.head,
    );
    const binds = authorityHistory.receipts.at(-1)?.oid ?? null;
    if (binds === null || state.refs.release.head !== binds) {
      fail('CHANGED_OWNER_HEAD', `${state.refs.release.ref} moved beyond its assembly authority`);
    }
    const candidate = prepareAssemblyCandidate(
      repo,
      state.refs.target.ref,
      binds,
      target,
      trackCandidates,
      gitIdentity,
    );
    const productIdentity = productTreeIdentity(repo, candidate);
    const checks = Object.hasOwn(input, 'checkResults')
      ? digestBytes(evidenceBytes(input.checkResults, 'checkResults'))
      : digestBytes(Buffer.from(canonicalJSON(inputs)));
    const receipt = {
      version: 1,
      release,
      role: 'implementer',
      result: 'candidate',
      plan: state.plan.oid,
      binds,
      detail: digestBytes(Buffer.alloc(0)),
      summary,
      target,
      base: target,
      candidate,
      product_tree: productIdentity.productTree,
      inputs,
      checks,
    };
    const message = renderReceiptCommit({
      subject: `baton(${release}): assembly candidate`,
      detail,
      receipt,
    });
    const prepared = unsafePrepareMetadataCommit(repo, {
      expectedHead: candidate,
      message,
      identity: gitIdentity,
    });
    unsafeAtomicUpdateRefs(repo, [
      {
        kind: 'verify',
        ref: state.refs.target.ref,
        expectedHead: target,
      },
      {
        kind: 'update',
        ref: state.refs.release.ref,
        newHead: prepared.commit,
        expectedHead: state.refs.release.head,
      },
    ]);
    const entry = actionEntry(prepared.commit, candidate, message);
    return receiptResult('prepareAssembly', true, {
      release,
      direct: false,
      candidate,
      inputs,
      receipt_commit: prepared.commit,
      receipt: entry.receipt,
    });
  }

  function mergePassedCandidate(rawOptions) {
    const input = exactOptions(
      rawOptions,
      ['release', 'summary'],
      ['detail'],
      'mergePassedCandidate',
    );
    const release = identity(input.release, 'release');
    const summary = text(input.summary, 'summary', MAX_SUMMARY);
    const detail = detailBytes(input.detail);
    const state = stateFor(release);
    if (state.assembly.outcome === 'merged') {
      return receiptResult('mergePassedCandidate', false, {
        release,
        candidate: state.assembly.candidate.receipt.candidate,
        target: state.refs.target.ref,
        result_commit: state.assembly.result_commit,
        receipt_commit: state.assembly.current_receipt.oid,
        receipt: state.assembly.current_receipt.receipt,
      });
    }
    requireTargetLineage(repo, state);

    let passed;
    if (state.assembly.pass) {
      passed = state.assembly.pass;
    } else if (state.tracks.length === 1 && state.tracks[0].slices.length === 1) {
      const finalPass = state.tracks[0].slices[0].pass;
      if (
        finalPass
        && isAncestor(repo, state.refs.target.head, finalPass.receipt.candidate)
      ) {
        passed = finalPass;
      }
    }
    if (!passed) {
      fail('ASSEMBLY_PASS_REQUIRED', 'the current exact candidate has no applicable PASS');
    }
    const candidate = passed.receipt.candidate;
    const prepared = unsafePrepareExactComposition(repo, {
      targetRef: state.refs.target.ref,
      expectedHead: state.refs.target.head,
      candidate,
      identity: gitIdentity,
    });
    const productIdentity = productTreeIdentity(repo, prepared.result);
    const receipt = {
      version: 1,
      release,
      role: 'merge',
      result: 'merged',
      plan: state.plan.oid,
      binds: passed.oid,
      detail: digestBytes(Buffer.alloc(0)),
      summary,
      target: state.refs.target.head,
      candidate,
      product_tree: productIdentity.productTree,
      result_commit: prepared.result,
    };
    const message = renderReceiptCommit({
      subject: `baton(${release}): merge passed candidate`,
      detail,
      receipt,
    });
    const preparedReceipt = unsafePrepareMetadataCommit(repo, {
      expectedHead: state.refs.release.head,
      message,
      identity: gitIdentity,
    });
    unsafeAtomicUpdateRefs(repo, [
      {
        kind: 'update',
        ref: state.refs.target.ref,
        newHead: prepared.result,
        expectedHead: state.refs.target.head,
      },
      {
        kind: 'update',
        ref: state.refs.release.ref,
        newHead: preparedReceipt.commit,
        expectedHead: state.refs.release.head,
      },
    ]);
    return receiptResult('mergePassedCandidate', true, {
      release,
      candidate,
      target: state.refs.target.ref,
      result_commit: prepared.result,
      receipt_commit: preparedReceipt.commit,
      receipt: parseReceiptCommitMessage(message).receipt,
    });
  }

  return Object.freeze({
    recordPlanRevision,
    prepareTrackBase,
    appendReceipt,
    prepareAssembly,
    mergePassedCandidate,
  });
}

export const referenceNames = Object.freeze({
  releaseRef,
  trackRef,
  planPath,
});
