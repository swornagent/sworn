export const STATE_LABELS = Object.freeze({
  assembly: 'Checking the complete release',
  assembly_ready: 'Ready for the final check',
  blocked: 'Needs a decision',
  complete: 'Complete',
  composed: 'Combined',
  in_progress: 'In progress',
  invalid: 'Needs attention',
  merge_ready: 'Ready to merge',
  pending: 'Waiting',
  ready: 'Ready for the next step',
  revision_required: 'Plan update needed',
  valid: 'Current',
  waiting: 'Waiting',
});

export const ROLE_LABELS = Object.freeze({
  lead: 'design review',
  implementer: 'implementation',
  merge: 'merge',
  planner: 'planning',
  verifier: 'verification',
});

export const OPERATION_LABELS = Object.freeze({
  'protocol-design-review': 'Review the approach',
  'protocol-implement': 'Design or build the work',
  'protocol-merge': 'Merge what passed',
  'protocol-plan': 'Plan or update the work',
  'protocol-verify': 'Check the finished work',
});

export const DIAGNOSTIC_LABELS = Object.freeze({
  BOARD_PROJECTION_FAILED: 'Protocol could not build a trustworthy board from the saved records.',
  GIT_COMMAND_FAILED: 'Protocol could not read this repository.',
  INVALID_RELEASE_REF: 'One release reference is not valid.',
  REF_SNAPSHOT_UNSTABLE: 'The release changed while Protocol was reading it. Try again.',
  STALE_ASSEMBLY: 'The complete release changed and needs to be checked again.',
  TARGET_DIVERGED: 'The target history changed. Reconcile it before continuing.',
  TRACK_REF_ABSENT: 'This track has not started yet.',
});
