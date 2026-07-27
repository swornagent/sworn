import { createHash } from 'node:crypto';

export const RECEIPT_VERSION = 1;
export const PLAN_VERSION = 'baton.plan/v2';
export const RECEIPT_TRAILER = 'Baton-Receipt: ';
export const DETAIL_BEGIN = 'Baton-Detail-Begin';
export const DETAIL_END = 'Baton-Detail-End';
export const RECEIPT_LIMITS = Object.freeze({
  receiptBytes: 2_048,
  detailBytes: 8_192,
  messageBytes: 12_288,
  planBytes: 1_048_576,
  depth: 64,
  tracks: 64,
  slices: 1_024,
  listItems: 256,
});

const MAX_SAFE_INTEGER = 9_007_199_254_740_991;
const IDENTITY = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;
const OBJECT_ID = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/;
const DIGEST = /^sha256:[0-9a-f]{64}$/;
const HEAD_REF = /^refs\/heads\/(?!.*(?:^|\/)\.\.?($|\/))(?!.*\/\/)(?!.*@\{)(?!.*[~^:?*\[\\\s])[^/][^\u0000-\u001f\u007f]*$/;
const RESULT_BY_ROLE = Object.freeze({
  planner: new Set(['approved', 'retired']),
  implementer: new Set(['designed', 'candidate']),
  captain: new Set(['proceed', 'revise', 'escalate']),
  verifier: new Set(['pass', 'fail', 'blocked']),
  merge: new Set(['merged']),
});

export class ReceiptError extends Error {
  constructor(code, message, cause) {
    super(message, cause ? { cause } : undefined);
    this.name = 'ReceiptError';
    this.code = code;
  }
}

function fail(code, message, cause) {
  throw new ReceiptError(code, message, cause);
}

function bytes(value, label, maximum) {
  const result = Buffer.isBuffer(value) ? Buffer.from(value) : Buffer.from(value);
  if (result.byteLength > maximum) {
    fail('RESOURCE_LIMIT', `${label} exceeds ${maximum} bytes`);
  }
  return result;
}

function utf8(value, label, maximum) {
  const input = bytes(value, label, maximum);
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(input);
  } catch (error) {
    fail('INVALID_UTF8', `${label} is not valid UTF-8`, error);
  }
}

function validUnicode(value, label) {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(index + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) {
        fail('INVALID_UNICODE', `${label} contains a lone high surrogate`);
      }
      index += 1;
    } else if (code >= 0xdc00 && code <= 0xdfff) {
      fail('INVALID_UNICODE', `${label} contains a lone low surrogate`);
    }
  }
  return value;
}

class StrictJSONParser {
  constructor(text) {
    this.text = text;
    this.offset = 0;
  }

  parse() {
    this.space();
    const value = this.value();
    this.space();
    if (this.offset !== this.text.length) {
      fail('TRAILING_JSON', `JSON has trailing input at byte ${this.offset}`);
    }
    return value;
  }

  space() {
    while (/[\t\n\r ]/.test(this.text[this.offset] ?? '')) this.offset += 1;
  }

  value(depth = 0) {
    if (depth > RECEIPT_LIMITS.depth) fail('RESOURCE_LIMIT', 'JSON is too deeply nested');
    this.space();
    const character = this.text[this.offset];
    if (
      this.text.startsWith('NaN', this.offset)
      || this.text.startsWith('Infinity', this.offset)
      || this.text.startsWith('-Infinity', this.offset)
    ) {
      fail('NONFINITE_NUMBER', `non-finite number at byte ${this.offset}`);
    }
    if (character === '{') return this.object(depth);
    if (character === '[') return this.array(depth);
    if (character === '"') return this.string();
    if (character === '-' || (character >= '0' && character <= '9')) return this.number();
    for (const [token, value] of [['true', true], ['false', false], ['null', null]]) {
      if (this.text.startsWith(token, this.offset)) {
        this.offset += token.length;
        return value;
      }
    }
    fail('INVALID_JSON', `unexpected token at byte ${this.offset}`);
  }

  object(depth) {
    this.offset += 1;
    this.space();
    const result = {};
    const names = new Set();
    if (this.text[this.offset] === '}') {
      this.offset += 1;
      return result;
    }
    for (;;) {
      if (this.text[this.offset] !== '"') {
        fail('INVALID_JSON', `expected object name at byte ${this.offset}`);
      }
      const name = this.string();
      if (names.has(name)) fail('DUPLICATE_NAME', `duplicate object name ${JSON.stringify(name)}`);
      names.add(name);
      this.space();
      if (this.text[this.offset] !== ':') fail('INVALID_JSON', `expected ':' at byte ${this.offset}`);
      this.offset += 1;
      Object.defineProperty(result, name, {
        value: this.value(depth + 1),
        enumerable: true,
        configurable: true,
        writable: true,
      });
      this.space();
      if (this.text[this.offset] === '}') {
        this.offset += 1;
        return result;
      }
      if (this.text[this.offset] !== ',') fail('INVALID_JSON', `expected ',' at byte ${this.offset}`);
      this.offset += 1;
      this.space();
    }
  }

  array(depth) {
    this.offset += 1;
    this.space();
    const result = [];
    if (this.text[this.offset] === ']') {
      this.offset += 1;
      return result;
    }
    for (;;) {
      result.push(this.value(depth + 1));
      this.space();
      if (this.text[this.offset] === ']') {
        this.offset += 1;
        return result;
      }
      if (this.text[this.offset] !== ',') fail('INVALID_JSON', `expected ',' at byte ${this.offset}`);
      this.offset += 1;
      this.space();
    }
  }

  string() {
    const start = this.offset;
    this.offset += 1;
    let escaped = false;
    for (; this.offset < this.text.length; this.offset += 1) {
      const character = this.text[this.offset];
      if (!escaped && character === '"') {
        this.offset += 1;
        let value;
        try {
          value = JSON.parse(this.text.slice(start, this.offset));
        } catch (error) {
          fail('INVALID_JSON', `invalid string at byte ${start}`, error);
        }
        return validUnicode(value, 'JSON string');
      }
      if (!escaped && character.charCodeAt(0) < 0x20) {
        fail('INVALID_JSON', `unescaped control at byte ${this.offset}`);
      }
      escaped = !escaped && character === '\\';
    }
    fail('INVALID_JSON', `unterminated string at byte ${start}`);
  }

  number() {
    const remaining = this.text.slice(this.offset);
    const match = remaining.match(/^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?/);
    if (!match) fail('INVALID_JSON', `invalid number at byte ${this.offset}`);
    const raw = match[0];
    this.offset += raw.length;
    const next = this.text[this.offset];
    if (next !== undefined && !/[\t\n\r ,}\]]/.test(next)) {
      fail('INVALID_JSON', `invalid number delimiter at byte ${this.offset}`);
    }
    const value = Number(raw);
    if (!Number.isFinite(value)) fail('NONFINITE_NUMBER', `non-finite number ${raw}`);
    if (!Number.isSafeInteger(value)) fail('UNSAFE_INTEGER', `number is not a safe integer: ${raw}`);
    if (Math.abs(value) > MAX_SAFE_INTEGER) fail('UNSAFE_INTEGER', `unsafe integer ${raw}`);
    return value;
  }
}

export function strictParseJSON(value, label = 'JSON', maximum = RECEIPT_LIMITS.planBytes) {
  return new StrictJSONParser(utf8(value, label, maximum)).parse();
}

export function digestBytes(value) {
  return `sha256:${createHash('sha256').update(value).digest('hex')}`;
}

function plainObject(value, label) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    fail('INVALID_SHAPE', `${label} must be an object`);
  }
  return value;
}

function exactKeys(value, required, optional, label) {
  plainObject(value, label);
  const allowed = new Set([...required, ...optional]);
  for (const key of Object.keys(value)) {
    if (!allowed.has(key)) fail('UNKNOWN_FIELD', `${label} has unknown field ${key}`);
  }
  for (const key of required) {
    if (!Object.hasOwn(value, key)) fail('MISSING_FIELD', `${label} is missing ${key}`);
  }
}

function string(value, label, { min = 1, max = 1_000, pattern } = {}) {
  if (typeof value !== 'string' || value.length < min || value.length > max) {
    fail('INVALID_FIELD', `${label} must be a string of ${min}-${max} characters`);
  }
  validUnicode(value, label);
  if (pattern && !pattern.test(value)) fail('INVALID_FIELD', `${label} is invalid`);
  return value;
}

function identity(value, label) {
  return string(value, label, { max: 128, pattern: IDENTITY });
}

function objectID(value, label) {
  return string(value, label, { max: 64, pattern: OBJECT_ID });
}

function digest(value, label) {
  return string(value, label, { max: 71, pattern: DIGEST });
}

function integer(value, label, minimum = 1) {
  if (!Number.isSafeInteger(value) || value < minimum) {
    fail('INVALID_FIELD', `${label} must be a safe integer >= ${minimum}`);
  }
  return value;
}

function array(value, label, { nonempty = false } = {}) {
  if (!Array.isArray(value) || (nonempty && value.length === 0)) {
    fail('INVALID_FIELD', `${label} must be ${nonempty ? 'a non-empty' : 'an'} array`);
  }
  if (value.length > RECEIPT_LIMITS.listItems) fail('RESOURCE_LIMIT', `${label} is too long`);
  return value;
}

function uniqueStrings(value, label, validator = identity) {
  const seen = new Set();
  return array(value, label).map((item, index) => {
    const parsed = validator(item, `${label}[${index}]`);
    if (seen.has(parsed)) fail('DUPLICATE_IDENTITY', `${label} repeats ${parsed}`);
    seen.add(parsed);
    return parsed;
  });
}

function repositoryPath(value, label) {
  string(value, label, { max: 512 });
  if (
    value.startsWith('/')
    || value.startsWith('\\')
    || value.endsWith('/')
    || value.includes('//')
    || value.includes('\\')
    || /[\u0000-\u001f\u007f]/.test(value)
    || value.split('/').some((part) => part === '' || part === '.' || part === '..')
    || value === '.git'
    || value.startsWith('.git/')
  ) {
    fail('INVALID_PATH', `${label} is not a canonical repository path`);
  }
  return value;
}

function freeze(value) {
  if (value === null || typeof value !== 'object' || Object.isFrozen(value)) return value;
  // Byte arrays are exposed only through copy-returning getters below.
  if (ArrayBuffer.isView(value)) return value;
  for (const item of Object.values(value)) freeze(item);
  return Object.freeze(value);
}

function canonicalValue(value, label = 'value') {
  if (value === null || typeof value === 'boolean' || typeof value === 'string') {
    if (typeof value === 'string') validUnicode(value, label);
    return value;
  }
  if (typeof value === 'number') {
    if (!Number.isSafeInteger(value)) fail('UNSAFE_INTEGER', `${label} is not a safe integer`);
    return value;
  }
  if (Array.isArray(value)) {
    return value.map((item, index) => canonicalValue(item, `${label}[${index}]`));
  }
  plainObject(value, label);
  const result = {};
  for (const key of Object.keys(value).sort()) {
    Object.defineProperty(result, key, {
      value: canonicalValue(value[key], `${label}.${key}`),
      enumerable: true,
      configurable: true,
      writable: true,
    });
  }
  return result;
}

export function canonicalJSON(value) {
  return JSON.stringify(canonicalValue(value));
}

function validateScope(value, label) {
  exactKeys(value, ['include', 'exclude'], [], label);
  const result = {
    include: uniqueStrings(value.include, `${label}.include`, repositoryPath),
    exclude: uniqueStrings(value.exclude, `${label}.exclude`, repositoryPath),
  };
  if (result.include.length === 0) fail('INVALID_FIELD', `${label}.include cannot be empty`);
  return result;
}

function validateAcceptance(value, label) {
  const ids = new Set();
  return array(value, label, { nonempty: true }).map((item, index) => {
    const itemLabel = `${label}[${index}]`;
    exactKeys(item, ['id', 'text'], [], itemLabel);
    const id = identity(item.id, `${itemLabel}.id`);
    if (ids.has(id)) fail('DUPLICATE_IDENTITY', `${label} repeats ${id}`);
    ids.add(id);
    return { id, text: string(item.text, `${itemLabel}.text`, { max: 4_096 }) };
  });
}

function validateSlice(value, trackID, label) {
  exactKeys(
    value,
    ['id', 'outcome', 'scope', 'acceptance', 'checks', 'constraints', 'depends_on', 'consumes'],
    [],
    label,
  );
  const slice = {
    id: identity(value.id, `${label}.id`),
    outcome: string(value.outcome, `${label}.outcome`, { max: 4_096 }),
    scope: validateScope(value.scope, `${label}.scope`),
    acceptance: validateAcceptance(value.acceptance, `${label}.acceptance`),
    checks: uniqueStrings(value.checks, `${label}.checks`, (item, itemLabel) => (
      string(item, itemLabel, { max: 2_048 })
    )),
    constraints: uniqueStrings(value.constraints, `${label}.constraints`, (item, itemLabel) => (
      string(item, itemLabel, { max: 2_048 })
    )),
    depends_on: uniqueStrings(value.depends_on, `${label}.depends_on`),
    consumes: uniqueStrings(value.consumes, `${label}.consumes`),
  };
  return {
    slice,
    contract: digestBytes(Buffer.from(canonicalJSON({ track: trackID, ...slice }))),
  };
}

function pathOverlap(left, right) {
  return left === right || left.startsWith(`${right}/`) || right.startsWith(`${left}/`);
}

function assertAcyclic(nodes, edges, label) {
  const state = new Map();
  function visit(node) {
    if (state.get(node) === 1) fail('DEPENDENCY_CYCLE', `${label} contains a cycle at ${node}`);
    if (state.get(node) === 2) return;
    state.set(node, 1);
    for (const dependency of edges.get(node) ?? []) visit(dependency);
    state.set(node, 2);
  }
  for (const node of nodes) visit(node);
}

function dependencyClosures(nodes, edges) {
  const closures = new Map();
  function resolve(node) {
    if (closures.has(node)) return closures.get(node);
    const closure = new Set();
    for (const dependency of edges.get(node) ?? []) {
      closure.add(dependency);
      for (const inherited of resolve(dependency)) closure.add(inherited);
    }
    closures.set(node, closure);
    return closure;
  }
  for (const node of nodes) resolve(node);
  return closures;
}

export function validatePlanMetadata(value) {
  exactKeys(
    value,
    [
      'schema_version',
      'release',
      'revision',
      'previous_plan',
      'repository',
      'target_ref',
      'approval_ref',
      'tracks',
    ],
    [],
    'plan',
  );
  if (value.schema_version !== PLAN_VERSION) fail('INVALID_FIELD', `plan.schema_version must be ${PLAN_VERSION}`);
  const release = identity(value.release, 'plan.release');
  const revision = integer(value.revision, 'plan.revision');
  const previousPlan = value.previous_plan === null
    ? null
    : objectID(value.previous_plan, 'plan.previous_plan');
  if ((revision === 1) !== (previousPlan === null)) {
    fail('INVALID_FIELD', 'plan revision 1 alone must use previous_plan null');
  }
  const repository = string(value.repository, 'plan.repository', { max: 256 });
  if (repository.startsWith('/') || repository.startsWith('\\') || /^[A-Za-z]:[\\/]/.test(repository)) {
    fail('INVALID_FIELD', 'plan.repository must be a portable identity');
  }
  const targetRef = string(value.target_ref, 'plan.target_ref', { max: 250, pattern: HEAD_REF });
  const approvalRef = string(value.approval_ref, 'plan.approval_ref', { max: 500 });
  const tracks = array(value.tracks, 'plan.tracks', { nonempty: true });
  if (tracks.length > RECEIPT_LIMITS.tracks) fail('RESOURCE_LIMIT', 'plan has too many tracks');

  const trackIDs = new Set();
  const sliceIDs = new Set();
  const contracts = {};
  let sliceCount = 0;
  const parsedTracks = tracks.map((track, trackIndex) => {
    const label = `plan.tracks[${trackIndex}]`;
    exactKeys(track, ['id', 'depends_on', 'slices'], [], label);
    const id = identity(track.id, `${label}.id`);
    if (trackIDs.has(id)) fail('DUPLICATE_IDENTITY', `plan repeats track ${id}`);
    trackIDs.add(id);
    const parsedSlices = array(track.slices, `${label}.slices`, { nonempty: true })
      .map((slice, sliceIndex) => {
        const parsed = validateSlice(slice, id, `${label}.slices[${sliceIndex}]`);
        if (sliceIDs.has(parsed.slice.id)) fail('DUPLICATE_IDENTITY', `plan repeats slice ${parsed.slice.id}`);
        sliceIDs.add(parsed.slice.id);
        contracts[parsed.slice.id] = parsed.contract;
        sliceCount += 1;
        return parsed.slice;
      });
    return {
      id,
      depends_on: uniqueStrings(track.depends_on, `${label}.depends_on`),
      slices: parsedSlices,
    };
  });
  if (sliceCount > RECEIPT_LIMITS.slices) fail('RESOURCE_LIMIT', 'plan has too many slices');

  const trackEdges = new Map();
  for (const track of parsedTracks) {
    for (const dependency of track.depends_on) {
      if (!trackIDs.has(dependency) || dependency === track.id) {
        fail('INVALID_DEPENDENCY', `track ${track.id} has invalid dependency ${dependency}`);
      }
    }
    trackEdges.set(track.id, track.depends_on);
  }
  assertAcyclic(trackIDs, trackEdges, 'track dependencies');

  const sliceEdges = new Map();
  for (const track of parsedTracks) {
    for (const slice of track.slices) {
      for (const dependency of [...slice.depends_on, ...slice.consumes]) {
        if (!sliceIDs.has(dependency) || dependency === slice.id) {
          fail('INVALID_DEPENDENCY', `slice ${slice.id} has invalid dependency ${dependency}`);
        }
      }
      sliceEdges.set(slice.id, [...new Set([...slice.depends_on, ...slice.consumes])]);
    }
  }
  assertAcyclic(sliceIDs, sliceEdges, 'slice dependencies');

  // The executable delivery graph also contains edges that are implicit in
  // the plan shape. A slice is ordered after the previous slice in its track,
  // and the first slice in a dependent track waits for the final slice in
  // every prerequisite track. Validate those edges together with depends_on
  // and consumes so individually acyclic layers cannot form a combined
  // deadlock.
  const tracksByID = new Map(parsedTracks.map((track) => [track.id, track]));
  const deliveryEdges = new Map([...sliceIDs].map((sliceID) => [sliceID, new Set()]));
  for (const track of parsedTracks) {
    for (let index = 0; index < track.slices.length; index += 1) {
      const slice = track.slices[index];
      const dependencies = deliveryEdges.get(slice.id);
      for (const dependency of [...slice.depends_on, ...slice.consumes]) {
        dependencies.add(dependency);
      }
      if (index > 0) dependencies.add(track.slices[index - 1].id);
    }
    const firstSlice = track.slices[0];
    for (const dependencyID of track.depends_on) {
      const dependencyTrack = tracksByID.get(dependencyID);
      deliveryEdges.get(firstSlice.id).add(dependencyTrack.slices.at(-1).id);
    }
  }
  assertAcyclic(sliceIDs, deliveryEdges, 'delivery graph');
  const deliveryClosures = dependencyClosures(sliceIDs, deliveryEdges);

  for (let leftIndex = 0; leftIndex < parsedTracks.length; leftIndex += 1) {
    for (let rightIndex = leftIndex + 1; rightIndex < parsedTracks.length; rightIndex += 1) {
      const left = parsedTracks[leftIndex];
      const right = parsedTracks[rightIndex];
      for (const leftSlice of left.slices) {
        for (const rightSlice of right.slices) {
          const ordered = (
            deliveryClosures.get(leftSlice.id).has(rightSlice.id)
            || deliveryClosures.get(rightSlice.id).has(leftSlice.id)
          );
          if (ordered) continue;
          for (const leftPath of leftSlice.scope.include) {
            for (const rightPath of rightSlice.scope.include) {
              if (pathOverlap(leftPath, rightPath)) {
                fail('PARALLEL_TOUCH_CONFLICT', `independent tracks overlap at ${leftPath} and ${rightPath}`);
              }
            }
          }
        }
      }
    }
  }

  return freeze({
    schema_version: PLAN_VERSION,
    release,
    revision,
    previous_plan: previousPlan,
    repository,
    target_ref: targetRef,
    approval_ref: approvalRef,
    tracks: parsedTracks,
    contracts,
  });
}

export function parsePlanBytes(value) {
  const input = bytes(value, 'plan', RECEIPT_LIMITS.planBytes);
  const open = Buffer.from('```baton-plan-v2\n');
  const close = Buffer.from('\n```\n');
  if (!input.subarray(0, open.length).equals(open)) fail('INVALID_PLAN_FENCE', 'plan must begin at byte zero');
  const closeAt = input.indexOf(close, open.length);
  if (closeAt === -1 || input.indexOf(close, closeAt + close.length) !== -1) {
    fail('INVALID_PLAN_FENCE', 'plan must contain one closed baton-plan-v2 block');
  }
  const metadataBytes = input.subarray(open.length, closeAt);
  const metadata = validatePlanMetadata(strictParseJSON(metadataBytes, 'plan metadata'));
  const markdown = utf8(input.subarray(closeAt + close.length), 'plan Markdown', RECEIPT_LIMITS.planBytes);
  const frozenBytes = Buffer.from(input);
  return Object.freeze({
    metadata,
    markdown,
    digest: digestBytes(frozenBytes),
    get bytes() {
      return Buffer.from(frozenBytes);
    },
  });
}

function validateInputs(value, label) {
  plainObject(value, label);
  const result = {};
  for (const key of Object.keys(value).sort()) {
    identity(key, `${label} key`);
    result[key] = digest(value[key], `${label}.${key}`);
  }
  if (Object.keys(result).length > RECEIPT_LIMITS.listItems) {
    fail('RESOURCE_LIMIT', `${label} has too many inputs`);
  }
  return result;
}

function assertRoleFields(receipt) {
  const present = (field) => Object.hasOwn(receipt, field);
  const requireFields = (...fields) => {
    const missing = fields.filter((field) => !present(field));
    if (missing.length > 0) {
      fail(
        'MISSING_FIELD',
        `${receipt.role}/${receipt.result} receipt requires ${missing.join(', ')}`,
      );
    }
  };
  const forbidFields = (...fields) => {
    const unexpected = fields.filter(present);
    if (unexpected.length > 0) {
      fail(
        'INVALID_FIELD',
        `${receipt.role}/${receipt.result} receipt forbids ${unexpected.join(', ')}`,
      );
    }
  };
  const candidateEvidence = ['candidate', 'product_tree', 'inputs', 'checks'];
  const mergeEvidence = ['target', 'candidate', 'product_tree', 'result_commit'];

  if (receipt.role === 'planner') {
    if (receipt.result === 'approved') {
      if (present('slice')) fail('INVALID_FIELD', 'planner/approved receipt is release-scoped');
      requireFields('target');
    } else {
      requireFields('slice');
    }
    forbidFields('base', 'candidate', 'product_tree', 'inputs', 'checks', 'result_commit');
    return;
  }
  if (receipt.role === 'implementer') {
    if (receipt.result === 'designed') {
      requireFields('slice');
      forbidFields('target', 'base', ...candidateEvidence, 'result_commit');
    } else {
      requireFields(...candidateEvidence);
      forbidFields('result_commit');
      if (!present('slice')) requireFields('target', 'base');
    }
    return;
  }
  if (receipt.role === 'captain') {
    requireFields('slice');
    forbidFields(
      'target',
      'base',
      'candidate',
      'product_tree',
      'inputs',
      'checks',
      'result_commit',
    );
    return;
  }
  if (receipt.role === 'verifier') {
    requireFields(...candidateEvidence);
    forbidFields('base', 'result_commit');
    return;
  }
  if (receipt.role === 'merge') {
    if (present('slice')) fail('INVALID_FIELD', 'merge receipt is release-scoped');
    requireFields(...mergeEvidence);
    forbidFields('base', 'checks');
  }
}

export function validateReceipt(value) {
  exactKeys(
    value,
    ['version', 'release', 'role', 'result', 'plan', 'binds', 'detail', 'summary'],
    [
      'slice',
      'attempt',
      'contract',
      'target',
      'base',
      'candidate',
      'product_tree',
      'inputs',
      'checks',
      'result_commit',
    ],
    'receipt',
  );
  if (value.version !== RECEIPT_VERSION) fail('INVALID_FIELD', 'receipt.version must be 1');
  const role = string(value.role, 'receipt.role', { max: 16 });
  const results = RESULT_BY_ROLE[role];
  if (!results || !results.has(value.result)) {
    fail('INVALID_FIELD', `receipt result ${value.result} is invalid for ${role}`);
  }
  const parsed = {
    version: RECEIPT_VERSION,
    release: identity(value.release, 'receipt.release'),
    role,
    result: value.result,
    plan: objectID(value.plan, 'receipt.plan'),
    binds: objectID(value.binds, 'receipt.binds'),
    detail: digest(value.detail, 'receipt.detail'),
    summary: string(value.summary, 'receipt.summary', { max: 280 }),
  };
  const hasSlice = Object.hasOwn(value, 'slice');
  for (const field of ['slice', 'attempt', 'contract']) {
    if (Object.hasOwn(value, field) !== hasSlice) {
      fail('INVALID_FIELD', 'receipt slice, attempt, and contract must appear together');
    }
  }
  if (hasSlice) {
    parsed.slice = identity(value.slice, 'receipt.slice');
    parsed.attempt = integer(value.attempt, 'receipt.attempt');
    parsed.contract = digest(value.contract, 'receipt.contract');
  } else if (role === 'captain' || (role === 'implementer' && value.result === 'designed')) {
    fail('MISSING_FIELD', `${role} receipt requires slice identity`);
  }
  for (const field of ['target', 'base', 'candidate', 'result_commit']) {
    if (Object.hasOwn(value, field)) parsed[field] = objectID(value[field], `receipt.${field}`);
  }
  for (const field of ['product_tree', 'checks']) {
    if (Object.hasOwn(value, field)) parsed[field] = digest(value[field], `receipt.${field}`);
  }
  if (Object.hasOwn(value, 'inputs')) parsed.inputs = validateInputs(value.inputs, 'receipt.inputs');
  assertRoleFields(parsed);
  return freeze(canonicalValue(parsed));
}

export function parseReceiptBytes(value) {
  const input = bytes(value, 'receipt', RECEIPT_LIMITS.receiptBytes);
  if (input.includes(0x0a) || input.includes(0x0d)) {
    fail('INVALID_RECEIPT', 'receipt must be canonical one-line JSON');
  }
  const parsed = validateReceipt(strictParseJSON(input, 'receipt', RECEIPT_LIMITS.receiptBytes));
  const canonical = canonicalJSON(parsed);
  if (!input.equals(Buffer.from(canonical))) fail('NON_CANONICAL_RECEIPT', 'receipt JSON is not canonical');
  return parsed;
}

function normalizedDetail(value) {
  const input = bytes(value, 'receipt detail', RECEIPT_LIMITS.detailBytes);
  if (input.includes(0x00) || input.includes(0x0d)) {
    fail('INVALID_DETAIL', 'receipt detail must use valid LF-only UTF-8 without NUL');
  }
  const text = utf8(input, 'receipt detail', RECEIPT_LIMITS.detailBytes);
  if (text.includes(DETAIL_BEGIN) || text.includes(DETAIL_END)) {
    fail('INVALID_DETAIL', 'receipt detail cannot contain marker text');
  }
  return Buffer.from(text);
}

export function renderReceiptCommit({ subject, detail = Buffer.alloc(0), receipt }) {
  const parsedSubject = string(subject, 'commit subject', { max: 200 });
  if (/[\r\n\u0000]/.test(parsedSubject)) fail('INVALID_DETAIL', 'commit subject must be one line');
  const detailBytes = normalizedDetail(detail);
  const next = { ...receipt, detail: digestBytes(detailBytes) };
  const receiptBytes = Buffer.from(canonicalJSON(validateReceipt(next)));
  if (receiptBytes.byteLength > RECEIPT_LIMITS.receiptBytes) {
    fail('RESOURCE_LIMIT', `receipt exceeds ${RECEIPT_LIMITS.receiptBytes} bytes`);
  }
  return Buffer.concat([
    Buffer.from(`${parsedSubject}\n\n${DETAIL_BEGIN}\n`),
    detailBytes,
    Buffer.from(`\n${DETAIL_END}\n\n${RECEIPT_TRAILER}`),
    receiptBytes,
    Buffer.from('\n'),
  ]);
}

export function parseReceiptCommitMessage(value) {
  const input = bytes(value, 'receipt commit message', RECEIPT_LIMITS.messageBytes);
  if (input.includes(0x00) || input.includes(0x0d)) {
    fail('INVALID_RECEIPT_COMMIT', 'receipt commit message must be LF-only UTF-8 without NUL');
  }
  const text = utf8(input, 'receipt commit message', RECEIPT_LIMITS.messageBytes);
  if (!text.endsWith('\n')) fail('INVALID_RECEIPT_COMMIT', 'receipt commit message must end with LF');
  const beginToken = `\n\n${DETAIL_BEGIN}\n`;
  const endToken = `\n${DETAIL_END}\n\n${RECEIPT_TRAILER}`;
  const begin = text.indexOf(beginToken);
  const end = text.indexOf(endToken, begin + beginToken.length);
  if (
    begin <= 0
    || end < begin
    || text.indexOf(beginToken, begin + beginToken.length) !== -1
    || text.indexOf(endToken, end + endToken.length) !== -1
  ) {
    fail('INVALID_RECEIPT_COMMIT', 'receipt commit message has invalid detail markers');
  }
  const subject = text.slice(0, begin);
  if (subject.includes('\n') || subject.length > 200) {
    fail('INVALID_RECEIPT_COMMIT', 'receipt commit subject must be one bounded line');
  }
  const detailText = text.slice(begin + beginToken.length, end);
  const detail = normalizedDetail(Buffer.from(detailText));
  const trailerStart = end + endToken.length;
  const trailer = text.slice(trailerStart, -1);
  if (trailer.includes('\n')) fail('INVALID_RECEIPT_COMMIT', 'receipt trailer must be the final line');
  const receipt = parseReceiptBytes(Buffer.from(trailer));
  if (receipt.detail !== digestBytes(detail)) {
    fail('STALE_BINDING', 'receipt detail digest does not match the exact detail bytes');
  }
  return Object.freeze({
    subject,
    receipt,
    get detail() {
      return Buffer.from(detail);
    },
  });
}

export function parseReceiptHistoryEntry(value) {
  exactKeys(value, ['oid', 'parents', 'tree', 'parent_tree', 'message'], [], 'history entry');
  const oid = objectID(value.oid, 'history entry.oid');
  const parents = uniqueStrings(value.parents, 'history entry.parents', objectID);
  if (parents.length !== 1) fail('INVALID_HISTORY', 'receipt commit must have exactly one parent');
  const tree = objectID(value.tree, 'history entry.tree');
  const parentTree = objectID(value.parent_tree, 'history entry.parent_tree');
  if (tree !== parentTree) fail('PRODUCT_MUTATION', 'receipt commit must be metadata-only');
  const parsed = parseReceiptCommitMessage(value.message);
  const detail = parsed.detail;
  return Object.freeze({
    oid,
    parent: parents[0],
    tree,
    subject: parsed.subject,
    receipt: parsed.receipt,
    get detail() {
      return Buffer.from(detail);
    },
  });
}
