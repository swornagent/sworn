#!/usr/bin/env node

// This file is used only by the explicit batongolden regeneration command.
// It imports the digest-pinned Baton RC3 JavaScript reference and never reads
// Go output.

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const [referenceRoot, gitExecutable, outputRoot] = process.argv.slice(2);
if (![referenceRoot, gitExecutable, outputRoot].every((value) => path.isAbsolute(value))) {
  throw new Error('oracle requires absolute reference, Git, and output paths');
}

const moduleAt = (name) => import(pathToFileURL(path.join(referenceRoot, name)).href);
const [actionModule, gitModule, recordModule, transitionModule] = await Promise.all([
  moduleAt('actions.mjs'),
  moduleAt('git.mjs'),
  moduleAt('records.mjs'),
  moduleAt('transition.mjs'),
]);

gitModule.configureEngineGitExecutable(gitExecutable);

const planBytes = Buffer.from(`\`\`\`baton-plan-v1
{
  "schema_version": "baton.plan/v1",
  "release": "demo-v1",
  "repository": "example/sworn",
  "target_ref": "refs/heads/release/v0.3.0",
  "release_ref": "refs/heads/release-wt/demo-v1",
  "record_root": ".baton/releases",
  "approval_ref": "test://approval/demo-v1",
  "tracks": [
    {
      "id": "T1",
      "ref": "refs/heads/track/demo-v1/T1",
      "depends_on": [],
      "touch_surfaces": ["product.txt"],
      "work": [
        {
          "id": "W1",
          "outcome": "deliver product",
          "scope": {"include": ["product.txt"], "exclude": []},
          "acceptance": [{"id": "A1", "text": "product is delivered"}],
          "checks": ["go test ./..."],
          "constraints": ["deterministic"],
          "depends_on": []
        }
      ]
    }
  ]
}
\`\`\`

# Demo
`);

const approvalBytes = Buffer.from('approved demo-v1\n');
const approvalDigest = recordModule.digestBytes(approvalBytes);
const clone = (value) => structuredClone(value);
const projection = (status) => `${status.stage}/${status.status}/${status.next_role}`;
const runGit = (repo, args, options = {}) => (
  gitModule.unsafeRunGit(repo, args, options).trim()
);

function fixedCommit(repo, tree, parents, message, identity, timestamp) {
  const args = ['commit-tree', tree];
  for (const parent of parents) args.push('-p', parent);
  const date = `@${timestamp} +0000`;
  return runGit(repo, args, {
    input: message,
    env: {
      GIT_AUTHOR_NAME: identity.name,
      GIT_AUTHOR_EMAIL: identity.email,
      GIT_AUTHOR_DATE: date,
      GIT_COMMITTER_NAME: identity.name,
      GIT_COMMITTER_EMAIL: identity.email,
      GIT_COMMITTER_DATE: date,
    },
  });
}

function initializeRepository(format) {
  const repo = mkdtempSync(path.join(tmpdir(), `baton-oracle-${format}-`));
  runGit(repo, ['init', '--quiet', '--initial-branch=main', `--object-format=${format}`, '.']);
  const blob = runGit(repo, ['hash-object', '-w', '--stdin'], { input: 'base\n' });
  const tree = runGit(repo, ['mktree'], { input: `100644 blob ${blob}\tproduct.txt\n` });
  const base = fixedCommit(
    repo,
    tree,
    [],
    'base\n',
    { name: 'Fixture', email: 'fixture@example.invalid' },
    1_000_000_000,
  );
  gitModule.unsafeAtomicUpdateRefs(repo, [
    { kind: 'create', ref: 'refs/heads/main', newHead: base },
    { kind: 'create', ref: 'refs/heads/release/v0.3.0', newHead: base },
  ]);
  return { repo, base };
}

function prepareProductCommit(repo, parent) {
  const directory = mkdtempSync(path.join(tmpdir(), 'baton-oracle-index-'));
  try {
    const index = path.join(directory, 'index');
    const env = { GIT_INDEX_FILE: index };
    runGit(repo, ['read-tree', `${parent}^{tree}`], { env });
    const blob = runGit(repo, ['hash-object', '-w', '--stdin'], { input: 'delivered\n' });
    runGit(repo, ['update-index', '--add', '--cacheinfo', `100644,${blob},product.txt`], { env });
    const tree = runGit(repo, ['write-tree'], { env });
    const timestamp = Number.parseInt(runGit(repo, ['show', '-s', '--format=%ct', parent]), 10) + 1;
    return {
      tree,
      commit: fixedCommit(
        repo,
        tree,
        [parent],
        'Implement W1\n',
        { name: 'Implementer', email: 'implementer@example.invalid' },
        timestamp,
      ),
    };
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

function prepareFixtureCommit(repo, parent, relativePath, bytes, message) {
  const directory = mkdtempSync(path.join(tmpdir(), 'baton-oracle-index-'));
  try {
    const index = path.join(directory, 'index');
    const env = { GIT_INDEX_FILE: index };
    runGit(repo, ['read-tree', `${parent}^{tree}`], { env });
    const blob = runGit(repo, ['hash-object', '-w', '--stdin'], { input: bytes });
    runGit(repo, ['update-index', '--add', '--cacheinfo', `100644,${blob},${relativePath}`], { env });
    const tree = runGit(repo, ['write-tree'], { env });
    const timestamp = Number.parseInt(runGit(repo, ['show', '-s', '--format=%ct', parent]), 10) + 1;
    return fixedCommit(
      repo,
      tree,
      [parent],
      message,
      { name: 'Fixture', email: 'fixture@example.invalid' },
      timestamp,
    );
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

function inertness(request) {
  return {
    kind: request.kind,
    repository: request.repository,
    record_root: request.record_root,
    commit: request.commit,
    decision: 'inert',
  };
}

function evidenceResolver(dispatches) {
  return (request) => {
    if (request.kind === 'approval') {
      return {
        bytes: Buffer.from(approvalBytes),
        provenance: {
          kind: 'approval',
          ref: request.ref,
          protected: true,
          decision: 'approved',
          plan_digest: request.plan_digest,
          authorizer_isolated: true,
          delivery_writable: false,
        },
      };
    }
    if (request.kind === 'verifier_dispatch') {
      const bytes = dispatches.get(request.ref);
      if (!bytes) throw new Error(`unknown verifier dispatch ${request.ref}`);
      return {
        bytes: Buffer.from(bytes),
        provenance: {
          kind: 'verifier_dispatch',
          ref: request.ref,
          protected: true,
          role: 'verifier',
          fresh_context: true,
          read_only: true,
          engine_controlled: true,
          invocation: request.invocation,
          plan_digest: request.plan_digest,
          proof_digest: request.proof_digest,
          candidate_commit: request.candidate_commit,
          product_tree: request.product_tree,
        },
      };
    }
    throw new Error(`unknown evidence kind ${request.kind}`);
  };
}

function selectWork(repo, plan, recordPathAdmission) {
  const snapshot = recordModule.captureRefSnapshot(repo, plan);
  const records = recordModule.readAuthoritativeRecordSnapshot(
    repo,
    plan,
    snapshot,
    { recordRootAdmission: recordPathAdmission },
  );
  return recordModule.selectAuthoritativeStatusFromSnapshot(plan, 'W1', records);
}

function selectAssembly(repo, plan, recordPathAdmission) {
  const snapshot = recordModule.captureRefSnapshot(repo, plan);
  const records = recordModule.readAuthoritativeRecordSnapshot(
    repo,
    plan,
    snapshot,
    { recordRootAdmission: recordPathAdmission },
  );
  return recordModule.selectAssemblyFromSnapshot(plan, records);
}

function addVerification(status, outcome, invocation, ref, bytes) {
  const next = clone(status);
  next.verification = {
    outcome,
    invocation,
    attestation_ref: ref,
    attestation_digest: recordModule.digestBytes(bytes),
    plan_digest: next.plan.digest,
    proof_digest: next.proof.digest,
    candidate_commit: next.proof.candidate_commit,
    product_tree: next.proof.product_tree,
  };
  return next;
}

function actionPair(events, name, first, retry) {
  events.push({ name, first, retry });
}

function runDelivery(format) {
  const { repo, base } = initializeRepository(format);
  try {
    const plan = recordModule.parsePlanBytes(planBytes);
    const dispatches = new Map();
    const resolveEvidence = evidenceResolver(dispatches);
    const recordPathAdmission = gitModule.resolveRecordPathAdmission(repo);
    const productAdmission = gitModule.resolveProductExclusionAdmission(repo, {
      recordPathAdmission,
      resolveBehavioralInertness: inertness,
    });
    const actions = actionModule.createBatonActions({
      repo,
      plan,
      profile: 'autonomous',
      resolveEvidence,
      resolveBehavioralInertness: inertness,
    });
    const events = [];

    const installed = actions.installApprovedPlan({ approvalDigest });
    const installedRetry = actions.installApprovedPlan({ approvalDigest });
    actionPair(events, 'installApprovedPlan', installed, installedRetry);
    const initial = clone(selectWork(repo, plan, recordPathAdmission).status);

    const materializedReceipt = actions.materializeTrack({ trackId: 'T1' });
    const materializedRetry = actions.materializeTrack({ trackId: 'T1' });
    actionPair(events, 'materializeTrack', materializedReceipt, materializedRetry);
    const materialized = clone(selectWork(repo, plan, recordPathAdmission).status);

    const designBytes = Buffer.from('# Design\n\nExact.\n');
    const design = clone(materialized);
    design.stage = 'design';
    design.status = 'ready';
    design.next_role = 'captain';
    design.outcome = 'none';
    design.design = {
      digest: recordModule.digestBytes(designBytes),
      producer_invocation: 'test:/implementer/design/1',
    };
    const designReceipt = actions.recordTransition({
      scope: 'work',
      workId: 'W1',
      result: 'DESIGN_WRITTEN',
      nextStatus: design,
      handoffs: { design: designBytes },
    });
    const designRetry = actions.recordTransition({
      scope: 'work',
      workId: 'W1',
      result: 'DESIGN_WRITTEN',
      nextStatus: design,
      handoffs: { design: designBytes },
    });
    actionPair(events, 'recordTransition:DESIGN_WRITTEN', designReceipt, designRetry);

    const proceed = clone(design);
    proceed.stage = 'implement';
    proceed.status = 'ready';
    proceed.next_role = 'implementer';
    proceed.outcome = 'proceed';
    proceed.captain = {
      outcome: 'proceed',
      invocation: 'test:/captain/review/1',
      plan_digest: plan.digest,
      design_digest: proceed.design.digest,
    };
    const proceedReceipt = actions.recordTransition({
      scope: 'work',
      workId: 'W1',
      result: 'PROCEED',
      nextStatus: proceed,
    });
    const proceedRetry = actions.recordTransition({
      scope: 'work',
      workId: 'W1',
      result: 'PROCEED',
      nextStatus: proceed,
    });
    actionPair(events, 'recordTransition:PROCEED', proceedReceipt, proceedRetry);

    const owner = gitModule.resolveRef(repo, 'refs/heads/track/demo-v1/T1');
    const candidate = prepareProductCommit(repo, owner);
    gitModule.unsafeAtomicUpdateRefs(repo, [{
      kind: 'update',
      ref: 'refs/heads/track/demo-v1/T1',
      newHead: candidate.commit,
      expectedHead: owner,
    }]);
    const identity = gitModule.productTreeIdentity(repo, candidate.commit, productAdmission);

    const proofBytes = Buffer.from('# Proof\n\nChecks pass.\n');
    const implemented = clone(proceed);
    implemented.stage = 'verify';
    implemented.status = 'ready';
    implemented.next_role = 'verifier';
    implemented.outcome = 'none';
    implemented.proof = {
      digest: recordModule.digestBytes(proofBytes),
      producer_invocation: 'test:/implementer/implement/1',
      repository: plan.metadata.repository,
      base_commit: implemented.materialization.base_commit,
      candidate_commit: candidate.commit,
      candidate_tree: candidate.tree,
      product_tree: identity.productTree,
      plan_digest: plan.digest,
      approval_digest: approvalDigest,
      design_digest: implemented.design.digest,
      captain_invocation: implemented.captain.invocation,
      components: [],
    };
    const implementedReceipt = actions.recordTransition({
      scope: 'work',
      workId: 'W1',
      result: 'IMPLEMENTED',
      nextStatus: implemented,
      handoffs: { proof: proofBytes },
    });
    const implementedRetry = actions.recordTransition({
      scope: 'work',
      workId: 'W1',
      result: 'IMPLEMENTED',
      nextStatus: implemented,
      handoffs: { proof: proofBytes },
    });
    actionPair(events, 'recordTransition:IMPLEMENTED', implementedReceipt, implementedRetry);

    const workDispatchRef = 'test://dispatch/work/1';
    const workDispatch = Buffer.from('fresh work verifier\n');
    dispatches.set(workDispatchRef, workDispatch);
    const passed = addVerification(
      implemented,
      'pass',
      'test:/verifier/work/1',
      workDispatchRef,
      workDispatch,
    );
    passed.stage = 'merge';
    passed.status = 'ready';
    passed.next_role = 'merge';
    passed.outcome = 'pass';
    const passReceipt = actions.recordTransition({
      scope: 'work',
      workId: 'W1',
      result: 'PASS',
      nextStatus: passed,
    });
    const passRetry = actions.recordTransition({
      scope: 'work',
      workId: 'W1',
      result: 'PASS',
      nextStatus: passed,
    });
    actionPair(events, 'recordTransition:PASS', passReceipt, passRetry);

    const composedReceipt = actions.composeTrack({ trackId: 'T1' });
    const composedRetry = actions.composeTrack({ trackId: 'T1' });
    actionPair(events, 'composeTrack', composedReceipt, composedRetry);
    const composed = clone(selectWork(repo, plan, recordPathAdmission).status);

    const assemblyProofBytes = Buffer.from('# Assembly proof\n\nExact composition.\n');
    const assemblyReceipt = actions.prepareAssembly({
      proofBytes: assemblyProofBytes,
      producerInvocation: 'test:/merge/assembly/1',
    });
    const assemblyRetry = actions.prepareAssembly({
      proofBytes: assemblyProofBytes,
      producerInvocation: 'test:/merge/assembly/1',
    });
    actionPair(events, 'prepareAssembly', assemblyReceipt, assemblyRetry);
    const assembly = clone(selectAssembly(repo, plan, recordPathAdmission).status);

    const assemblyDispatchRef = 'test://dispatch/assembly/1';
    const assemblyDispatch = Buffer.from('fresh assembly verifier\n');
    dispatches.set(assemblyDispatchRef, assemblyDispatch);
    const assemblyPass = addVerification(
      assembly,
      'pass',
      'test:/verifier/assembly/1',
      assemblyDispatchRef,
      assemblyDispatch,
    );
    assemblyPass.stage = 'merge';
    assemblyPass.status = 'ready';
    assemblyPass.next_role = 'merge';
    assemblyPass.outcome = 'pass';
    const assemblyPassReceipt = actions.recordTransition({
      scope: 'assembly',
      result: 'PASS',
      nextStatus: assemblyPass,
    });
    const assemblyPassRetry = actions.recordTransition({
      scope: 'assembly',
      result: 'PASS',
      nextStatus: assemblyPass,
    });
    actionPair(events, 'recordTransition:ASSEMBLY_PASS', assemblyPassReceipt, assemblyPassRetry);

    const integratedReceipt = actions.integrateRelease();
    const integratedRetry = actions.integrateRelease();
    actionPair(events, 'integrateRelease', integratedReceipt, integratedRetry);
    const mergedAssembly = clone(selectAssembly(repo, plan, recordPathAdmission).status);
    const refs = gitModule.captureHeadRefs(repo, [
      'refs/heads/release/v0.3.0',
      'refs/heads/release-wt/demo-v1',
      'refs/heads/track/demo-v1/T1',
    ]);

    return {
      action: {
        object_format: format,
        oid_hex_length: format === 'sha1' ? 40 : 64,
        base_commit: base,
        plan_digest: plan.digest,
        approval_digest: approvalDigest,
        product_candidate: candidate.commit,
        product_tree: identity.productTree,
        events,
        final_refs: refs,
      },
      statuses: {
        initial,
        materialized,
        design,
        proceed,
        implemented,
        passed,
        composed,
        assembly,
        assemblyPass,
        mergedAssembly,
      },
    };
  } finally {
    rmSync(repo, { recursive: true, force: true });
  }
}

function runRebound(format) {
  const { repo } = initializeRepository(format);
  try {
    const previousPlan = recordModule.parsePlanBytes(planBytes);
    const revisedBytes = Buffer.from(planBytes.toString().replace('# Demo\n', '# Demo revised\n'));
    const revisedPlan = recordModule.parsePlanBytes(revisedBytes);
    const oldApproval = Buffer.from('old approval\n');
    const newApproval = Buffer.from('new approval\n');
    const oldApprovalDigest = recordModule.digestBytes(oldApproval);
    const newApprovalDigest = recordModule.digestBytes(newApproval);
    const approvals = new Map([
      [oldApprovalDigest, oldApproval],
      [newApprovalDigest, newApproval],
    ]);
    const resolveEvidence = (request) => {
      const bytes = approvals.get(request.digest);
      if (request.kind !== 'approval' || !bytes) {
        throw new Error(`unknown rebound evidence ${request.kind}/${request.digest}`);
      }
      return {
        bytes: Buffer.from(bytes),
        provenance: {
          kind: 'approval',
          ref: request.ref,
          protected: true,
          decision: 'approved',
          plan_digest: request.plan_digest,
          authorizer_isolated: true,
          delivery_writable: false,
        },
      };
    };
    const previousActions = actionModule.createBatonActions({
      repo,
      plan: previousPlan,
      profile: 'guided',
      resolveEvidence,
      resolveBehavioralInertness: inertness,
    });
    previousActions.installApprovedPlan({ approvalDigest: oldApprovalDigest });
    const recordPathAdmission = gitModule.resolveRecordPathAdmission(repo);
    const previous = clone(selectWork(repo, previousPlan, recordPathAdmission).status);
    const revisedActions = actionModule.createBatonActions({
      repo,
      plan: revisedPlan,
      profile: 'guided',
      resolveEvidence,
      resolveBehavioralInertness: inertness,
    });
    const first = revisedActions.reboundPristinePlan({
      previousPlan,
      approvalDigest: newApprovalDigest,
    });
    const retry = revisedActions.reboundPristinePlan({
      previousPlan,
      approvalDigest: newApprovalDigest,
    });
    const next = clone(selectWork(repo, revisedPlan, recordPathAdmission).status);
    return {
      object_format: format,
      previous_plan_digest: previousPlan.digest,
      revised_plan_digest: revisedPlan.digest,
      previous_approval_digest: oldApprovalDigest,
      revised_approval_digest: newApprovalDigest,
      first,
      retry,
      previous,
      next,
    };
  } finally {
    rmSync(repo, { recursive: true, force: true });
  }
}

function runComposition(format) {
  const { repo, base } = initializeRepository(format);
  try {
    const left = prepareFixtureCommit(repo, base, 'left.txt', 'left\n', 'left\n');
    const right = prepareFixtureCommit(repo, base, 'right.txt', 'right\n', 'right\n');
    const recordPathAdmission = gitModule.resolveRecordPathAdmission(repo);
    const productAdmission = gitModule.resolveProductExclusionAdmission(repo, {
      recordPathAdmission,
      resolveBehavioralInertness: inertness,
    });
    const fastForward = gitModule.unsafePrepareExactComposition(repo, {
      targetRef: 'refs/heads/result/demo',
      expectedHead: base,
      candidate: left,
      productExclusionAdmission: productAdmission,
    });
    const twoParent = gitModule.unsafePrepareExactComposition(repo, {
      targetRef: 'refs/heads/result/demo',
      expectedHead: left,
      candidate: right,
      productExclusionAdmission: productAdmission,
    });
    let contained;
    try {
      gitModule.unsafePrepareExactComposition(repo, {
        targetRef: 'refs/heads/result/demo',
        expectedHead: twoParent.result,
        candidate: right,
        productExclusionAdmission: productAdmission,
      });
      contained = 'unexpected-pass';
    } catch (error) {
      contained = error.code;
    }
    return {
      object_format: format,
      oid_hex_length: format === 'sha1' ? 40 : 64,
      base,
      left,
      right,
      fast_forward: fastForward,
      two_parent: twoParent,
      contained_outcome: contained,
      result_product_tree: gitModule.productTreeIdentity(
        repo,
        twoParent.result,
        productAdmission,
      ).productTree,
    };
  } finally {
    rmSync(repo, { recursive: true, force: true });
  }
}

function transitionCase(name, previous, next, result) {
  try {
    transitionModule.unsafeValidateTransition(previous, next, result);
    return {
      name,
      result,
      source: projection(previous),
      target: projection(next),
      outcome: 'pass',
    };
  } catch (error) {
    return {
      name,
      result,
      source: projection(previous),
      target: projection(next),
      outcome: error.code ?? error.name,
    };
  }
}

function verifierStatus(status, outcome, invocation) {
  const bytes = Buffer.from(`${outcome} verifier\n`);
  return addVerification(status, outcome, invocation, `test://dispatch/${outcome}/1`, bytes);
}

function lifecycleVectors(statuses, rebound) {
  const revise = clone(statuses.design);
  revise.stage = 'design';
  revise.status = 'ready';
  revise.next_role = 'implementer';
  revise.outcome = 'revise';
  revise.captain = {
    outcome: 'revise',
    invocation: 'test:/captain/review/2',
    plan_digest: revise.plan.digest,
    design_digest: revise.design.digest,
  };

  const escalate = clone(statuses.design);
  escalate.stage = 'design';
  escalate.status = 'blocked';
  escalate.next_role = 'planner';
  escalate.outcome = 'escalate';
  escalate.captain = {
    outcome: 'escalate',
    invocation: 'test:/captain/review/3',
    plan_digest: escalate.plan.digest,
    design_digest: escalate.design.digest,
  };
  escalate.blocker = { code: 'captain_blocked', summary: 'needs an external decision' };

  const failed = verifierStatus(statuses.implemented, 'fail', 'test:/verifier/work/fail/1');
  failed.stage = 'implement';
  failed.status = 'ready';
  failed.next_role = 'implementer';
  failed.outcome = 'fail';

  const blocked = verifierStatus(statuses.implemented, 'blocked', 'test:/verifier/work/blocked/1');
  blocked.stage = 'verify';
  blocked.status = 'blocked';
  blocked.next_role = 'planner';
  blocked.outcome = 'blocked';
  blocked.blocker = { code: 'verification_blocked', summary: 'cannot finish verification' };

  const assemblyFailed = verifierStatus(
    statuses.assembly,
    'fail',
    'test:/verifier/assembly/fail/1',
  );
  assemblyFailed.stage = 'verify';
  assemblyFailed.status = 'ready';
  assemblyFailed.next_role = 'planner';
  assemblyFailed.outcome = 'fail';

  const assemblyBlocked = verifierStatus(
    statuses.assembly,
    'blocked',
    'test:/verifier/assembly/blocked/1',
  );
  assemblyBlocked.stage = 'verify';
  assemblyBlocked.status = 'blocked';
  assemblyBlocked.next_role = 'planner';
  assemblyBlocked.outcome = 'blocked';
  assemblyBlocked.blocker = {
    code: 'assembly_blocked',
    summary: 'cannot finish assembly verification',
  };

  return [
    transitionCase('design-written', statuses.materialized, statuses.design, 'DESIGN_WRITTEN'),
    transitionCase('captain-proceed', statuses.design, statuses.proceed, 'PROCEED'),
    transitionCase('captain-revise', statuses.design, revise, 'REVISE'),
    transitionCase('captain-escalate', statuses.design, escalate, 'ESCALATE'),
    transitionCase('implemented', statuses.proceed, statuses.implemented, 'IMPLEMENTED'),
    transitionCase('work-pass', statuses.implemented, statuses.passed, 'PASS'),
    transitionCase('work-fail', statuses.implemented, failed, 'FAIL'),
    transitionCase('work-blocked', statuses.implemented, blocked, 'BLOCKED'),
    transitionCase('assembly-pass', statuses.assembly, statuses.assemblyPass, 'PASS'),
    transitionCase('assembly-fail', statuses.assembly, assemblyFailed, 'FAIL'),
    transitionCase('assembly-blocked', statuses.assembly, assemblyBlocked, 'BLOCKED'),
    transitionCase('merged', statuses.assemblyPass, statuses.mergedAssembly, 'MERGED'),
    transitionCase('materialize', statuses.initial, statuses.materialized, 'MATERIALIZE'),
    transitionCase('rebound', rebound.previous, rebound.next, 'REBOUND'),
    transitionCase('no-verdict', statuses.implemented, clone(statuses.implemented), 'NO_VERDICT'),
  ];
}

function strictCase(name, bytes) {
  try {
    const parsed = recordModule.strictParseJSON(bytes, name);
    return { name, input_hex: Buffer.from(bytes).toString('hex'), outcome: 'pass', value: parsed };
  } catch (error) {
    return { name, input_hex: Buffer.from(bytes).toString('hex'), outcome: error.code ?? error.name };
  }
}

const deliveries = ['sha1', 'sha256'].map(runDelivery);
const rebounds = ['sha1', 'sha256'].map(runRebound);
const compositions = ['sha1', 'sha256'].map(runComposition);

const records = {
  schema: 'sworn.baton-golden-records/v1',
  plan: {
    input_hex: planBytes.toString('hex'),
    digest: recordModule.digestBytes(planBytes),
    release: recordModule.parsePlanBytes(planBytes).metadata.release,
  },
  strict_json: [
    strictCase('empty-object', Buffer.from('{}')),
    strictCase('duplicate-name', Buffer.from('{"a":1,"a":2}')),
    strictCase('trailing-json', Buffer.from('{} {}')),
    strictCase('lone-high-surrogate', Buffer.from('"\\ud800"')),
    strictCase('lone-low-surrogate', Buffer.from('"\\udc00"')),
    strictCase('unsafe-integer', Buffer.from('9007199254740992')),
    strictCase('nonfinite', Buffer.from('1e400')),
    strictCase('invalid-utf8', Buffer.from([0xff])),
  ],
};

const lifecycle = {
  schema: 'sworn.baton-golden-lifecycle/v1',
  cases: lifecycleVectors(deliveries[0].statuses, rebounds[0]),
};

const git = {
  schema: 'sworn.baton-golden-git/v1',
  product_tuple: 'path NUL mode NUL type NUL object LF',
  formats: compositions,
};

const actions = {
  schema: 'sworn.baton-golden-actions/v1',
  profile: 'autonomous',
  formats: deliveries.map((entry) => entry.action),
  rebounds: rebounds.map((entry) => ({
    object_format: entry.object_format,
    previous_plan_digest: entry.previous_plan_digest,
    revised_plan_digest: entry.revised_plan_digest,
    previous_approval_digest: entry.previous_approval_digest,
    revised_approval_digest: entry.revised_approval_digest,
    first: entry.first,
    retry: entry.retry,
  })),
};

mkdirSync(outputRoot, { recursive: true });
for (const [name, value] of Object.entries({ records, lifecycle, git, actions })) {
  writeFileSync(path.join(outputRoot, `${name}.json`), `${JSON.stringify(value, null, 2)}\n`, {
    mode: 0o644,
  });
}
