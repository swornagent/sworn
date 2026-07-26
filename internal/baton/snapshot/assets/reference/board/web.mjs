#!/usr/bin/env node

import { createServer } from 'node:http';
import { pathToFileURL } from 'node:url';

import {
  boardBytes,
  projectBoard,
} from './oracle.mjs';

const HTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="color-scheme" content="light">
  <title>Baton relay board</title>
  <link rel="stylesheet" href="/style.css">
  <script src="/app.js" defer></script>
</head>
<body>
  <header class="masthead">
    <a class="skip-link" href="#board">Skip to board</a>
    <div class="brand">
      <span class="brand-mark" aria-hidden="true"></span>
      <div>
        <p class="eyebrow">The work, handed forward</p>
        <p class="wordmark">Baton</p>
      </div>
    </div>
    <p id="freshness" class="freshness" role="status" aria-live="polite">Connecting</p>
  </header>
  <main id="board" class="board" tabindex="-1">
    <section class="loading" aria-labelledby="loading-title">
      <p class="eyebrow">Local release board</p>
      <h1 id="loading-title">Reading committed state</h1>
      <p>The board follows release and track refs, never whichever worktree happens to be open.</p>
    </section>
  </main>
  <footer class="footer">
    <p>Read-only. Refreshed from committed Baton records.</p>
  </footer>
</body>
</html>
`;

const APP_JS = `'use strict';

const SVG_NS = 'http://www.w3.org/2000/svg';
const GRAPH_VERSION = 'baton.graph/v1';
const boardRoot = document.getElementById('board');
const freshness = document.getElementById('freshness');
let lastBoard = null;
let activeGraphs = [];
let graphSequence = 0;
let graphDrawQueued = false;

function text(value, fallback) {
  if (value === null || value === undefined || value === '') return fallback || '—';
  return String(value);
}

function element(tag, className, value) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (value !== undefined) node.textContent = text(value);
  return node;
}

function svgElement(tag, className) {
  const node = document.createElementNS(SVG_NS, tag);
  if (className) node.setAttribute('class', className);
  return node;
}

function stateClass(value) {
  const safe = String(value || '').toLowerCase().replace(/[^a-z0-9_-]/g, '');
  return 'state state--' + (safe || 'unknown');
}

function statePill(value) {
  return element('span', stateClass(value), text(value));
}

function labelledValue(label, value, mono) {
  const item = element('div', 'fact');
  item.append(element('dt', 'fact-label', label));
  item.append(element('dd', mono ? 'fact-value mono' : 'fact-value', text(value)));
  return item;
}

function operationIdentity(operation) {
  return [operation.release, operation.track, operation.work]
    .filter(function (value) { return value !== null; })
    .map(function (value) { return text(value); })
    .join(' / ');
}

function operationCard(operation) {
  const card = element('article', 'operation');
  const lead = element('div', 'operation-lead');
  lead.append(element('span', 'operation-name mono', operation.operation));
  lead.append(element('span', 'operation-scope', operation.scope));
  card.append(lead);
  card.append(element('p', 'operation-target mono', operationIdentity(operation)));
  return card;
}

function blockerBlock(blocker) {
  const block = element('div', 'blocker');
  block.append(element('p', 'blocker-code mono', blocker.code));
  block.append(element('p', 'blocker-summary', blocker.summary));
  return block;
}

function sourceLine(source) {
  const row = element('p', 'source mono');
  row.append(element('span', 'source-mode', text(source && source.mode)));
  row.append(document.createTextNode(
    '  ' + text(source && source.ref) + ' @ ' + text(source && source.head)
  ));
  return row;
}

function relationLine(label, values, className) {
  if (!Array.isArray(values) || values.length === 0) return null;
  const row = element('p', 'relation relation--' + className);
  row.append(element('span', 'relation-label', label));
  row.append(document.createTextNode(' '));
  values.forEach(function (value, index) {
    if (index > 0) row.append(document.createTextNode(', '));
    row.append(element('strong', 'relation-id mono', value));
  });
  return row;
}

function graphKey(work) {
  return 'slice:' + text(work);
}

function addEdge(graph, from, to, kind) {
  if (!from || !to || from === to) return;
  const identity = from + '|' + to + '|' + kind;
  if (graph.edgeKeys.has(identity)) return;
  graph.edgeKeys.add(identity);
  graph.edges.push({ from: from, to: to, kind: kind });
}

function registerNode(graph, key, node) {
  graph.nodes.set(key, node);
  node.setAttribute('data-relay-node', 'true');
}

function projectedSliceInputs(graph, target, kind) {
  const found = [];
  const seen = new Set();
  graph.projectedEdges.forEach(function (edge) {
    if (edge.to !== target || !edge.kinds.includes(kind)) return;
    const source = graph.projectedNodes.get(edge.from);
    if (!source || source.kind !== 'slice' || seen.has(source.work)) return;
    seen.add(source.work);
    found.push(source.work);
  });
  return found;
}

function projectedTrackFeeds(graph, trackID) {
  const found = [];
  const seen = new Set();
  graph.projectedEdges.forEach(function (edge) {
    if (!edge.kinds.includes('track_dependency')) return;
    const source = graph.projectedNodes.get(edge.from);
    const target = graph.projectedNodes.get(edge.to);
    if (
      !source
      || !target
      || source.kind !== 'slice'
      || target.kind !== 'slice'
      || target.track !== trackID
      || seen.has(source.track)
    ) return;
    seen.add(source.track);
    found.push(source.track);
  });
  return found;
}

function workNode(work, index, graph) {
  const nodeID = graphKey(work.id);
  const projected = graph.projectedNodes.get(nodeID);
  const dependencies = projectedSliceInputs(graph, nodeID, 'depends_on');
  const consumes = projectedSliceInputs(graph, nodeID, 'consumes');
  const item = element('li', 'work-node');
  const details = element('details', 'work-leg');
  const summary = element('summary', 'work-summary');
  const identity = element('span', 'work-identity');
  identity.append(element('span', 'leg-number', 'Leg ' + String(index + 1).padStart(2, '0')));
  identity.append(element('span', 'work-id mono', work.id));
  summary.append(identity);
  summary.append(statePill(projected.state));
  if (projected.next_operation) {
    const summaryRoute = element('span', 'leg-route');
    summaryRoute.append(element('span', 'next-label', 'Next'));
    summaryRoute.append(document.createTextNode(' '));
    summaryRoute.append(element(
      'span',
      'lifecycle-role mono',
      projected.next_operation.operation
    ));
    summary.append(summaryRoute);
  }
  const feeds = [];
  if (dependencies.length > 0) {
    feeds.push('after ' + dependencies.join(', '));
  }
  if (consumes.length > 0) {
    feeds.push('uses ' + consumes.join(', '));
  }
  if (feeds.length > 0) summary.append(element('span', 'work-feeds mono', feeds.join(' · ')));
  details.append(summary);

  const facts = element('dl', 'leg-facts');
  facts.append(labelledValue('Attempt', work.attempt));
  facts.append(labelledValue('Plan revision', work.plan_revision));
  facts.append(labelledValue('Recorded stage', work.stage));
  facts.append(labelledValue('Recorded outcome', work.outcome));
  details.append(facts);
  details.append(sourceLine(work.source));

  const dependencyLine = relationLine('Depends on', dependencies, 'depends');
  const consumesLine = relationLine('Consumes', consumes, 'consumes');
  if (dependencyLine) details.append(dependencyLine);
  if (consumesLine) details.append(consumesLine);
  if (work.blocker) details.append(blockerBlock(work.blocker));
  if (projected.next_operation) {
    const next = element('div', 'next-inline');
    next.append(element('span', 'next-label', 'Next handoff'));
    next.append(element('span', 'mono', projected.next_operation.operation));
    details.append(next);
  }
  if (typeof details.addEventListener === 'function') {
    details.addEventListener('toggle', scheduleGraphDraw);
  }
  item.append(details);
  registerNode(graph, graphKey(work.id), item);
  return item;
}

function trackLane(track, graph) {
  const lane = element('section', 'track-lane');
  lane.setAttribute('aria-labelledby', 'lane-label-' + graph.index + '-' + graph.laneSequence);
  const stem = element('header', 'track-stem');
  const title = element('div');
  title.append(element('p', 'eyebrow', 'Lane'));
  const laneName = element('h4', 'track-id mono', track.id);
  laneName.setAttribute('id', 'lane-label-' + graph.index + '-' + graph.laneSequence);
  title.append(laneName);
  stem.append(title);
  stem.append(element(
    'p',
    'track-authority',
    String((track.work || []).length) + ' legs · ' + text(track.materialisation) + ' authority'
  ));
  const feeds = projectedTrackFeeds(graph, track.id);
  if (feeds.length > 0) {
    stem.append(element('p', 'track-dependency mono', 'fed by ' + feeds.join(', ')));
  }
  lane.append(stem);

  const workList = element('ol', 'work-list');
  workList.setAttribute('aria-label', 'Ordered slices for lane ' + text(track.id));
  (track.work || []).forEach(function (work, index) {
    workList.append(workNode(work, index, graph));
  });
  lane.append(workList);

  const exact = element('details', 'track-record');
  exact.append(element('summary', 'record-summary', 'Lane record'));
  const facts = element('dl', 'track-facts');
  facts.append(labelledValue('Ref', track.ref, true));
  facts.append(labelledValue('Head', track.head, true));
  if (track.frozen_head) facts.append(labelledValue('Frozen', track.frozen_head, true));
  exact.append(facts);
  if (Array.isArray(track.blockers) && track.blockers.length > 0) {
    exact.append(element('p', 'track-waiting mono', 'Recorded blockers ' + track.blockers.join(', ')));
  }
  lane.append(exact);
  graph.laneSequence += 1;
  return lane;
}

function assemblyNode(assembly, graph) {
  const projectedAssembly = graph.projectedNodes.get('assembly');
  const projectedMerge = graph.projectedNodes.get('merge');
  const stack = element('div', 'cadence-stack');
  const exchange = element('details', 'cadence-node final-exchange');
  const summary = element('summary', 'cadence-summary');
  const label = element('span', 'cadence-label');
  label.append(element('span', 'eyebrow', 'Final exchange'));
  label.append(element('strong', 'cadence-title', 'Assembly'));
  summary.append(label);
  summary.append(statePill(projectedAssembly.state));
  exchange.append(summary);
  if (!assembly) {
    exchange.append(element('p', 'empty-copy', 'Assembly state is unavailable.'));
  } else {
    const route = element('p', 'lifecycle');
    route.append(element('span', 'lifecycle-stage', text(assembly.stage)));
    if (projectedAssembly.next_operation) {
      route.append(document.createTextNode(' → '));
      route.append(element(
        'span',
        'lifecycle-role mono',
        projectedAssembly.next_operation.operation
      ));
    }
    exchange.append(route);
    exchange.append(element('p', 'outcome', 'Outcome ' + text(assembly.outcome)));
    if (assembly.source) exchange.append(sourceLine(assembly.source));
    if (assembly.blocker) exchange.append(blockerBlock(assembly.blocker));
    if (projectedAssembly.next_operation) {
      const next = element('div', 'next-inline');
      next.append(element('span', 'next-label', 'Next handoff'));
      next.append(element('span', 'mono', projectedAssembly.next_operation.operation));
      exchange.append(next);
    }
  }
  if (typeof exchange.addEventListener === 'function') {
    exchange.addEventListener('toggle', scheduleGraphDraw);
  }
  stack.append(exchange);
  registerNode(graph, 'assembly', exchange);

  const finish = element('article', 'cadence-node finish-node');
  finish.append(element('span', 'finish-stripe'));
  finish.append(element('p', 'eyebrow', 'Finish'));
  finish.append(element('h4', 'cadence-title', 'Merge'));
  finish.append(statePill(projectedMerge.state));
  finish.append(element(
    'p',
    'finish-copy',
    projectedMerge.next_operation
      ? text(projectedMerge.next_operation.operation) + ' is the recorded handoff.'
      : 'Awaits a recorded merge handoff.'
  ));
  stack.append(finish);
  registerNode(graph, 'merge', finish);
  return stack;
}

function diagnosticCard(item) {
  const card = element('article', 'diagnostic');
  const head = element('div', 'diagnostic-heading');
  head.append(element('span', 'diagnostic-mark', '!'));
  head.append(element('p', 'diagnostic-code mono', item.code));
  card.append(head);
  const scope = [item.release, item.track, item.work]
    .filter(function (value) { return value !== null; })
    .join(' / ');
  if (scope) card.append(element('p', 'diagnostic-scope mono', scope));
  card.append(element('p', 'diagnostic-message', item.message));
  return card;
}

function legendItem(kind, label) {
  const item = element('span', 'legend-item legend-item--' + kind);
  item.append(element('span', 'legend-line'));
  item.append(document.createTextNode(label));
  return item;
}

function projectGraph(release) {
  const projection = release.graph;
  if (
    !projection
    || projection.schema_version !== GRAPH_VERSION
    || !Array.isArray(projection.nodes)
    || !Array.isArray(projection.edges)
  ) return null;
  const projectedNodes = new Map();
  for (const node of projection.nodes) {
    if (!node || typeof node.id !== 'string' || projectedNodes.has(node.id)) return null;
    projectedNodes.set(node.id, node);
  }
  if (
    projectedNodes.get('plan')?.kind !== 'plan'
    || projectedNodes.get('assembly')?.kind !== 'assembly'
    || projectedNodes.get('merge')?.kind !== 'merge'
  ) return null;
  const expectedSlices = new Set();
  for (const track of release.tracks || []) {
    for (const work of track.work || []) {
      const id = graphKey(work.id);
      const node = projectedNodes.get(id);
      if (
        !node
        || node.kind !== 'slice'
        || node.track !== track.id
        || node.work !== work.id
      ) return null;
      expectedSlices.add(id);
    }
  }
  for (const node of projection.nodes) {
    if (node.kind === 'slice' && !expectedSlices.has(node.id)) return null;
  }
  for (const edge of projection.edges) {
    if (
      !edge
      || !projectedNodes.has(edge.from)
      || !projectedNodes.has(edge.to)
      || !Array.isArray(edge.kinds)
      || edge.kinds.some(function (kind) { return typeof kind !== 'string'; })
    ) return null;
  }
  return { nodes: projectedNodes, edges: projection.edges };
}

function createGraph(release) {
  const index = graphSequence++;
  const projected = projectGraph(release);
  const graph = {
    index: index,
    markerId: 'relay-arrow-' + index,
    nodes: new Map(),
    projectedNodes: projected ? projected.nodes : new Map(),
    projectedEdges: projected ? projected.edges : [],
    edges: [],
    edgeKeys: new Set(),
    canvas: null,
    svg: null,
    laneSequence: 0,
    release: release,
    valid: projected !== null,
  };
  if (!projected) return graph;
  projected.edges.forEach(function (edge) {
    const kinds = edge.kinds;
    if (kinds.includes('start')) addEdge(graph, edge.from, edge.to, 'branch');
    if (kinds.includes('serial')) addEdge(graph, edge.from, edge.to, 'handoff');
    if (kinds.includes('track_dependency') || kinds.includes('depends_on')) {
      addEdge(graph, edge.from, edge.to, 'depends');
    }
    if (kinds.includes('consumes')) addEdge(graph, edge.from, edge.to, 'consumes');
    if (kinds.includes('assembly')) addEdge(graph, edge.from, edge.to, 'converge');
    if (kinds.includes('verified_before_merge')) addEdge(graph, edge.from, edge.to, 'finish');
  });
  return graph;
}

function relayMap(release) {
  const graph = createGraph(release);
  const wrapper = element('section', 'relay-map');
  const heading = element('header', 'relay-heading');
  const title = element('div');
  title.append(element('p', 'eyebrow', 'Committed route'));
  title.append(element('h3', 'relay-title', 'Release relay'));
  heading.append(title);
  const legend = element('div', 'graph-legend');
  legend.setAttribute('role', 'group');
  legend.setAttribute('aria-label', 'Relationship legend');
  legend.append(legendItem('handoff', 'Handoff'));
  legend.append(legendItem('depends', 'Depends on'));
  legend.append(legendItem('consumes', 'Consumes'));
  heading.append(legend);
  wrapper.append(heading);
  wrapper.append(element(
    'p',
    'relay-caption',
    'Each lane runs forward. Cross-lane lines are declared dependencies; dashed lines are consumed inputs.'
  ));
  if (!graph.valid) {
    wrapper.append(element(
      'p',
      'graph-unavailable',
      'The committed graph projection is unavailable. No route is shown.'
    ));
    return wrapper;
  }

  const viewport = element('div', 'graph-viewport');
  viewport.setAttribute('tabindex', '0');
  viewport.setAttribute('role', 'region');
  viewport.setAttribute('aria-label', 'Scrollable release relay graph for ' + text(release.release));
  const canvas = element('div', 'graph-canvas');
  graph.canvas = canvas;
  const links = svgElement('svg', 'graph-links');
  links.setAttribute('aria-hidden', 'true');
  links.setAttribute('focusable', 'false');
  graph.svg = links;
  canvas.append(links);

  const start = element('div', 'start-block');
  start.append(element('span', 'start-signal', 'Start'));
  start.append(element('span', 'baton-mark'));
  start.append(element('p', 'eyebrow', 'Plan'));
  start.append(element('strong', 'start-title', 'r' + text(release.plan_revision, '—')));
  start.append(statePill(graph.projectedNodes.get('plan').state));
  canvas.append(start);
  registerNode(graph, 'plan', start);

  const tracks = element('div', 'track-field');
  (release.tracks || []).forEach(function (track) {
    tracks.append(trackLane(track, graph));
  });
  canvas.append(tracks);

  const cadence = assemblyNode(release.assembly, graph);
  canvas.append(cadence);

  viewport.append(canvas);
  wrapper.append(viewport);
  activeGraphs.push(graph);
  return wrapper;
}

function releaseSection(release) {
  const section = element('section', 'release');
  const header = element('header', 'release-heading');
  const title = element('div', 'release-title');
  title.append(element('p', 'eyebrow', 'Release'));
  title.append(element('h2', 'release-name', release.release));
  header.append(title);
  section.append(header);

  const record = element('details', 'release-record');
  record.append(element(
    'summary',
    'record-summary',
    'Exact ref record · plan r' + text(release.plan_revision)
  ));
  const facts = element('dl', 'release-facts');
  facts.append(labelledValue('Plan digest', release.plan_digest, true));
  facts.append(labelledValue('Plan object', release.plan_object, true));
  facts.append(labelledValue('Release ref', release.release_ref, true));
  facts.append(labelledValue('Release head', release.release_head, true));
  facts.append(labelledValue('Target ref', release.target_ref, true));
  facts.append(labelledValue('Target head', release.target_head, true));
  record.append(facts);
  section.append(record);

  if (Array.isArray(release.diagnostics) && release.diagnostics.length > 0) {
    const diagnostics = element('div', 'diagnostics');
    release.diagnostics.forEach(function (item) { diagnostics.append(diagnosticCard(item)); });
    section.append(diagnostics);
  }
  section.append(relayMap(release));
  return section;
}

function nodeBox(node, canvasBox) {
  const box = node.getBoundingClientRect();
  return {
    left: box.left - canvasBox.left,
    right: box.right - canvasBox.left,
    top: box.top - canvasBox.top,
    bottom: box.bottom - canvasBox.top,
    centerX: box.left - canvasBox.left + box.width / 2,
    centerY: box.top - canvasBox.top + box.height / 2,
  };
}

function graphPath(source, target, edge, index) {
  const verticalRelation = (
    edge.kind === 'depends'
    || edge.kind === 'consumes'
    || edge.kind === 'finish'
  ) && Math.abs(target.centerY - source.centerY) > 44;
  if (verticalRelation) {
    const targetIsBelow = target.centerY > source.centerY;
    const offset = edge.kind === 'consumes' ? 12 : edge.kind === 'depends' ? -12 : 0;
    const startX = source.centerX + offset;
    const startY = targetIsBelow ? source.bottom : source.top;
    const endX = target.centerX + offset;
    const endY = targetIsBelow ? target.top : target.bottom;
    const pull = Math.max(24, Math.abs(endY - startY) * 0.48);
    const startControl = targetIsBelow ? startY + pull : startY - pull;
    const endControl = targetIsBelow ? endY - pull : endY + pull;
    return 'M ' + startX + ' ' + startY
      + ' C ' + startX + ' ' + startControl
      + ', ' + endX + ' ' + endControl
      + ', ' + endX + ' ' + endY;
  }
  const startX = source.right;
  const startY = source.centerY;
  const endX = target.left;
  const endY = target.centerY;
  const distance = endX - startX;
  if (distance > 4) {
    const pull = Math.max(16, distance * 0.42);
    return 'M ' + startX + ' ' + startY
      + ' C ' + (startX + pull) + ' ' + startY
      + ', ' + (endX - pull) + ' ' + endY
      + ', ' + endX + ' ' + endY;
  }
  const upper = Math.max(16, Math.min(source.top, target.top) - 28 - (index % 3) * 12);
  const exit = startX + 30 + (index % 2) * 10;
  const entry = endX - 30 - (index % 2) * 10;
  return 'M ' + startX + ' ' + startY
    + ' C ' + exit + ' ' + startY + ', ' + exit + ' ' + upper + ', ' + exit + ' ' + upper
    + ' L ' + entry + ' ' + upper
    + ' C ' + entry + ' ' + upper + ', ' + entry + ' ' + endY + ', ' + endX + ' ' + endY;
}

function drawGraph(graph) {
  if (
    !graph.canvas
    || !graph.svg
    || typeof graph.canvas.getBoundingClientRect !== 'function'
  ) return;
  const canvasBox = graph.canvas.getBoundingClientRect();
  if (!canvasBox.width || !canvasBox.height) return;
  graph.svg.setAttribute('viewBox', '0 0 ' + canvasBox.width + ' ' + canvasBox.height);
  graph.svg.setAttribute('width', String(canvasBox.width));
  graph.svg.setAttribute('height', String(canvasBox.height));

  const defs = svgElement('defs');
  const marker = svgElement('marker');
  marker.setAttribute('id', graph.markerId);
  marker.setAttribute('viewBox', '0 0 8 8');
  marker.setAttribute('refX', '7');
  marker.setAttribute('refY', '4');
  marker.setAttribute('markerWidth', '7');
  marker.setAttribute('markerHeight', '7');
  marker.setAttribute('orient', 'auto-start-reverse');
  const arrow = svgElement('path');
  arrow.setAttribute('d', 'M 0 0 L 8 4 L 0 8 z');
  arrow.setAttribute('class', 'graph-arrow');
  marker.append(arrow);
  defs.append(marker);
  const paths = [defs];
  graph.edges.forEach(function (edge, index) {
    const sourceNode = graph.nodes.get(edge.from);
    const targetNode = graph.nodes.get(edge.to);
    if (!sourceNode || !targetNode) return;
    const path = svgElement('path', 'graph-link graph-link--' + edge.kind);
    path.setAttribute(
      'd',
      graphPath(nodeBox(sourceNode, canvasBox), nodeBox(targetNode, canvasBox), edge, index)
    );
    path.setAttribute('marker-end', 'url(#' + graph.markerId + ')');
    paths.push(path);
  });
  graph.svg.replaceChildren.apply(graph.svg, paths);
}

function drawAllGraphs() {
  graphDrawQueued = false;
  activeGraphs.forEach(drawGraph);
}

function scheduleGraphDraw() {
  if (graphDrawQueued || typeof window.requestAnimationFrame !== 'function') return;
  graphDrawQueued = true;
  window.requestAnimationFrame(drawAllGraphs);
}

function render(board) {
  activeGraphs = [];
  const fragment = document.createDocumentFragment();
  const intro = element('section', 'intro');
  const copy = element('div');
  copy.append(element('p', 'eyebrow', 'Baton board · exact refs'));
  copy.append(element('h1', 'intro-title', board.repository || 'No active release'));
  copy.append(element(
    'p',
    'intro-copy',
    board.valid
      ? 'Baton is how the work is handed off. One approved plan starts the relay; each verified leg carries committed work forward.'
      : 'At least one release cannot be trusted. Its diagnostics are shown without partial progress claims.'
  ));
  intro.append(copy);
  intro.append(statePill(board.valid ? 'valid' : 'invalid'));
  fragment.append(intro);

  if (Array.isArray(board.diagnostics) && board.diagnostics.length > 0) {
    const diagnostics = element('section', 'diagnostics global-diagnostics');
    diagnostics.append(element('h2', 'section-title', 'Board diagnostics'));
    board.diagnostics.forEach(function (item) { diagnostics.append(diagnosticCard(item)); });
    fragment.append(diagnostics);
  }

  if (!Array.isArray(board.releases) || board.releases.length === 0) {
    const empty = element('section', 'empty');
    empty.append(element('p', 'eyebrow', 'Nothing on the track'));
    empty.append(element('h2', 'section-title', 'No local release refs'));
    empty.append(element('p', 'empty-copy', 'Create an approved refs/heads/release-wt/* release to populate this board.'));
    fragment.append(empty);
  } else {
    board.releases.forEach(function (release) { fragment.append(releaseSection(release)); });
  }

  if (Array.isArray(board.next_operations) && board.next_operations.length > 0) {
    const queue = element('section', 'queue');
    const heading = element('div', 'queue-heading');
    heading.append(element('p', 'eyebrow', 'Ready now'));
    heading.append(element('h2', 'section-title', 'Next handoffs'));
    queue.append(heading);
    const operations = element('div', 'operation-grid');
    board.next_operations.forEach(function (operation) { operations.append(operationCard(operation)); });
    queue.append(operations);
    fragment.append(queue);
  }
  boardRoot.replaceChildren(fragment);
  scheduleGraphDraw();
}

function markFresh() {
  freshness.className = 'freshness freshness--fresh';
  freshness.textContent = 'Committed state · current';
}

function markStale() {
  freshness.className = 'freshness freshness--stale';
  freshness.textContent = lastBoard
    ? 'Refresh failed · showing last committed view'
    : 'Board unavailable · retrying';
}

async function refresh() {
  try {
    const response = await fetch('/api/board', {
      cache: 'no-store',
      credentials: 'same-origin'
    });
    if (!response.ok) throw new Error('board refresh failed');
    const board = await response.json();
    if (!board || board.schema_version !== 'baton.board/v1') {
      throw new Error('unexpected board contract');
    }
    if (
      !Array.isArray(board.releases)
      || board.releases.some(function (release) {
        return release.valid !== false && projectGraph(release) === null;
      })
    ) {
      throw new Error('unexpected graph contract');
    }
    lastBoard = board;
    render(board);
    markFresh();
  } catch (error) {
    markStale();
  }
}

if (typeof window.addEventListener === 'function') {
  window.addEventListener('resize', scheduleGraphDraw);
}
refresh();
window.setInterval(refresh, 15000);
`;

const CSS = `/* The relay board: a timing sheet, not a dashboard. */
* {
  box-sizing: border-box;
}

body {
  margin: 0;
  min-width: 320px;
}

.skip-link {
  position: fixed;
  top: 0.75rem;
  left: 0.75rem;
  z-index: 10;
  padding: 0.65rem 0.9rem;
  background: #17201e;
  color: #f8faf6;
  transform: translateY(-180%);
}

.skip-link:focus {
  transform: translateY(0);
}

.masthead,
.brand,
.release-heading,
.queue-heading,
.operation-lead,
.diagnostic-heading {
  display: flex;
}

.masthead {
  position: sticky;
  top: 0;
  z-index: 10;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.brand {
  align-items: center;
}

.eyebrow,
.wordmark,
.release-name,
.section-title,
.relay-title,
.track-id,
.source,
.relation,
.outcome,
.operation-target,
.diagnostic-code,
.diagnostic-scope,
.diagnostic-message,
.blocker-code,
.blocker-summary,
.empty-copy {
  margin: 0;
}

.eyebrow {
  font-family: ui-monospace, "Cascadia Code", "SFMono-Regular", Consolas, monospace;
  font-weight: 800;
  text-transform: uppercase;
}

.board {
  margin: 0 auto;
  outline: none;
}

.release-heading,
.queue-heading {
  justify-content: space-between;
  gap: 1rem;
}

.release-facts,
.track-facts,
.leg-facts {
  display: grid;
  margin: 0;
}

.fact {
  min-width: 0;
}

.fact-label {
  margin-bottom: 0.3rem;
  font-weight: 750;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.fact-value {
  margin: 0;
  overflow-wrap: anywhere;
}

.mono {
  font-family: ui-monospace, "Cascadia Code", "SFMono-Regular", Consolas, monospace;
}

.lifecycle {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.35rem;
}

.outcome {
  color: #59645f;
}

.source,
.relation,
.operation-target,
.diagnostic-scope {
  overflow-wrap: anywhere;
}

.operation-grid {
  display: grid;
  margin-top: 1rem;
}

.operation-lead,
.diagnostic-heading {
  align-items: center;
  justify-content: space-between;
  gap: 0.7rem;
}

.operation-target {
  margin-top: 0.5rem;
}

.blocker,
.diagnostic {
  margin-top: 0.8rem;
}

.blocker-summary,
.diagnostic-message,
.empty-copy {
  margin-top: 0.4rem;
  line-height: 1.5;
}

.diagnostic-heading {
  justify-content: flex-start;
}

.diagnostic-mark {
  display: grid;
  width: 22px;
  height: 22px;
  place-items: center;
  color: #f8faf6;
  font-weight: 900;
}

.diagnostic-scope {
  color: #59645f;
}

.global-diagnostics {
  margin-bottom: 1.5rem;
}

.footer {
  padding: 1rem clamp(1rem, 4vw, 4rem) 2rem;
  color: #59645f;
  font-size: 0.75rem;
  text-align: center;
}

:root {
  --ink: #17201e;
  --ink-soft: #59645f;
  --ground: #dfe5e1;
  --sheet: #f8faf6;
  --sheet-deep: #eef2ed;
  --lane: #2f687a;
  --lane-soft: #c8d9df;
  --baton: #df5038;
  --baton-dark: #8f3024;
  --pass: #27705b;
  --pass-soft: #d9ebe3;
  --caution: #8a5610;
  --caution-soft: #f2e6ce;
  --fault: #a53d47;
  --fault-soft: #f3dee0;
  --hairline: rgba(23, 32, 30, 0.2);
  --hardline: rgba(23, 32, 30, 0.58);
  --timing-shadow: #bcc6c0;
  font-family: Aptos, "Segoe UI", system-ui, sans-serif;
}

html {
  background: var(--ground);
  color: var(--ink);
}

body {
  background: var(--ground);
}

.masthead {
  min-height: 78px;
  padding: 0.85rem clamp(1rem, 4vw, 4rem);
  border-bottom: 4px solid var(--ink);
  background: rgba(248, 250, 246, 0.96);
}

.brand {
  gap: 1rem;
}

.brand-mark {
  width: 46px;
  height: 8px;
  border-radius: 999px;
  background: var(--baton);
  box-shadow: inset -11px 0 0 var(--baton-dark);
  transform: rotate(-11deg);
}

.brand-mark::after {
  top: -3px;
  left: 5px;
  width: 2px;
  height: 14px;
  border-radius: 0;
  background: var(--sheet);
  opacity: 0.72;
}

.wordmark {
  font-family: "Arial Narrow", "Aptos Display", Aptos, sans-serif;
  font-size: 1.6rem;
  font-weight: 850;
  letter-spacing: -0.035em;
  text-transform: uppercase;
}

.eyebrow {
  color: var(--ink-soft);
  font-size: 0.64rem;
  letter-spacing: 0.145em;
}

.freshness {
  position: relative;
  padding: 0.42rem 0 0.42rem 1.15rem;
  border-bottom: 1px solid var(--hardline);
  color: var(--ink-soft);
}

.freshness::before {
  position: absolute;
  top: 50%;
  left: 0;
  width: 0.55rem;
  height: 0.55rem;
  background: var(--ink-soft);
  content: "";
  transform: translateY(-50%) rotate(45deg);
}

.freshness--fresh {
  border-color: var(--pass);
  color: var(--pass);
}

.freshness--fresh::before {
  background: var(--pass);
}

.freshness--stale {
  border-color: var(--fault);
  color: var(--fault);
}

.freshness--stale::before {
  background: var(--fault);
}

.board {
  width: min(1640px, 100%);
  padding: clamp(1.25rem, 4vw, 4rem);
}

.intro {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: end;
  margin-bottom: clamp(2rem, 6vw, 5rem);
  padding: clamp(2rem, 5vw, 5.5rem) 0 clamp(1.5rem, 3vw, 2.5rem);
  border-bottom: 1px solid var(--hardline);
}

.intro .eyebrow {
  color: var(--baton-dark);
}

.intro-title {
  max-width: 19ch;
  overflow-wrap: anywhere;
  font-family: "Arial Narrow", "Aptos Display", Aptos, sans-serif;
  font-size: clamp(2.6rem, 7vw, 7rem);
  font-stretch: condensed;
  font-weight: 850;
  line-height: 0.84;
  letter-spacing: -0.07em;
  text-transform: uppercase;
}

.intro-copy {
  max-width: 66ch;
  overflow-wrap: anywhere;
  margin-top: 1.5rem;
  color: var(--ink-soft);
  font-size: clamp(0.95rem, 1.5vw, 1.12rem);
  line-height: 1.65;
}

.state {
  position: relative;
  display: inline-flex;
  padding: 0 0 0 0.9rem;
  color: var(--lane);
  font-size: 0.64rem;
  letter-spacing: 0.08em;
}

.state::before {
  position: absolute;
  top: 50%;
  left: 0;
  width: 0.5rem;
  height: 0.5rem;
  background: currentColor;
  content: "";
  transform: translateY(-50%) rotate(45deg);
}

.state--complete,
.state--composed,
.state--passed,
.state--retained,
.state--approved,
.state--not_required,
.state--valid {
  color: var(--pass);
}

.state--ready,
.state--merge_ready,
.state--assembly_ready,
.state--revision_required,
.state--stale {
  color: var(--caution);
}

.state--invalid,
.state--blocked {
  color: var(--fault);
}

.release {
  margin: 0 0 clamp(3rem, 7vw, 7rem);
  padding: 1.25rem 0 0;
  border-top: 7px solid var(--ink);
}

.release-heading {
  align-items: flex-end;
}

.release-name,
.section-title,
.relay-title {
  margin: 0;
  font-family: "Arial Narrow", "Aptos Display", Aptos, sans-serif;
  font-weight: 850;
  letter-spacing: -0.045em;
}

.release-name {
  font-size: clamp(2rem, 4vw, 3.7rem);
}

.record-summary {
  width: fit-content;
  padding: 0.42rem 0;
  border-bottom: 1px solid var(--hardline);
  color: var(--ink-soft);
  cursor: pointer;
  font-family: ui-monospace, "Cascadia Code", "SFMono-Regular", Consolas, monospace;
  font-size: 0.68rem;
  font-weight: 750;
  letter-spacing: 0.04em;
  list-style-position: outside;
}

.record-summary:hover {
  color: var(--ink);
}

.release-record {
  margin-top: 0.75rem;
}

.release-facts,
.track-facts,
.leg-facts {
  gap: 0;
  border-top: 1px solid var(--hairline);
  border-left: 1px solid var(--hairline);
}

.release-facts {
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-top: 0.8rem;
}

.fact {
  padding: 0.7rem;
  border-right: 1px solid var(--hairline);
  border-bottom: 1px solid var(--hairline);
  background: var(--sheet);
}

.fact-label {
  color: var(--ink-soft);
  font-size: 0.62rem;
}

.fact-value {
  color: var(--ink);
  font-size: 0.72rem;
}

.relay-map {
  position: relative;
  margin-top: clamp(1.5rem, 3vw, 2.5rem);
  padding: clamp(1rem, 2vw, 1.6rem);
  border: 1px solid var(--hardline);
  background: var(--sheet);
  box-shadow: 10px 10px 0 var(--timing-shadow);
}

.relay-map::after {
  position: absolute;
  top: -1px;
  right: -1px;
  width: 2.25rem;
  height: 2.25rem;
  border-bottom: 1px solid var(--hardline);
  border-left: 1px solid var(--hardline);
  background: var(--ground);
  content: "";
  clip-path: polygon(100% 0, 100% 100%, 0 0);
}

.relay-heading {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 1.5rem;
  padding-right: 2.4rem;
}

.relay-title {
  font-size: clamp(1.8rem, 3vw, 2.7rem);
}

.relay-caption {
  max-width: 72ch;
  margin: 0.65rem 0 1.3rem;
  color: var(--ink-soft);
  font-size: 0.82rem;
  line-height: 1.5;
}

.graph-unavailable {
  margin: 1rem 0 0;
  padding: 0.85rem;
  border-left: 4px solid var(--fault);
  background: var(--fault-soft);
  color: var(--fault);
}

.graph-legend {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.55rem 1rem;
  color: var(--ink-soft);
  font-family: ui-monospace, "Cascadia Code", "SFMono-Regular", Consolas, monospace;
  font-size: 0.62rem;
  text-transform: uppercase;
}

.legend-item {
  display: inline-flex;
  align-items: center;
  gap: 0.38rem;
}

.legend-line {
  display: inline-block;
  width: 1.9rem;
  height: 0;
  border-top: 2px solid var(--ink);
}

.legend-item--depends .legend-line {
  border-color: var(--lane);
}

.legend-item--consumes .legend-line {
  border-color: var(--baton);
  border-top-style: dashed;
}

.graph-viewport {
  position: relative;
  max-width: 100%;
  overflow: auto;
  border-top: 1px solid var(--hardline);
  border-bottom: 1px solid var(--hardline);
  background: var(--sheet);
  scrollbar-color: var(--hardline) var(--sheet-deep);
}

.graph-viewport:focus-visible {
  outline-offset: 5px;
}

.graph-canvas {
  position: relative;
  isolation: isolate;
  display: grid;
  grid-template-columns: 8.5rem max-content 11rem;
  align-items: stretch;
  min-width: max-content;
  padding: 1.25rem 1.4rem;
}

.graph-links {
  position: absolute;
  z-index: 1;
  inset: 0;
  overflow: visible;
  pointer-events: none;
  animation: relay-lines-in 420ms ease-out both;
}

.graph-link {
  fill: none;
  stroke: var(--ink);
  stroke-linecap: square;
  stroke-linejoin: round;
  stroke-width: 1.8;
  vector-effect: non-scaling-stroke;
}

.graph-link--branch,
.graph-link--converge {
  stroke: var(--hardline);
  stroke-width: 1.4;
}

.graph-link--handoff,
.graph-link--finish {
  stroke: var(--ink);
  stroke-width: 2.4;
}

.graph-link--depends {
  stroke: var(--lane);
  stroke-width: 2;
}

.graph-link--consumes {
  stroke: var(--baton);
  stroke-dasharray: 7 5;
  stroke-width: 2;
}

.graph-arrow {
  fill: var(--ink);
}

@supports (fill: context-stroke) {
  .graph-arrow {
    fill: context-stroke;
  }
}

@keyframes relay-lines-in {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

.start-block,
.track-field,
.cadence-stack {
  position: relative;
  z-index: 2;
}

.start-block {
  align-self: center;
  display: grid;
  justify-items: center;
  min-width: 7.5rem;
  padding: 1rem 0.7rem;
  text-align: center;
}

.start-signal {
  margin-bottom: 1rem;
  padding: 0.28rem 0.5rem;
  border: 1px solid var(--ink);
  background: var(--ink);
  color: var(--sheet);
  font-family: ui-monospace, "Cascadia Code", "SFMono-Regular", Consolas, monospace;
  font-size: 0.58rem;
  font-weight: 850;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.baton-mark {
  position: relative;
  display: block;
  width: 4rem;
  height: 0.62rem;
  margin-bottom: 0.9rem;
  border-radius: 999px;
  background: var(--baton);
  box-shadow: inset -0.9rem 0 0 var(--baton-dark);
  transform: rotate(-7deg);
}

.baton-mark::after {
  position: absolute;
  top: -0.18rem;
  left: 0.65rem;
  width: 2px;
  height: 0.98rem;
  background: var(--sheet);
  content: "";
  opacity: 0.75;
}

.start-title {
  font-family: "Arial Narrow", "Aptos Display", Aptos, sans-serif;
  font-size: 2.5rem;
  line-height: 1;
  letter-spacing: -0.07em;
}

.track-field {
  width: max-content;
  min-width: 46rem;
}

.track-lane {
  position: relative;
  display: grid;
  grid-template-columns: 7.5rem minmax(38rem, max-content);
  align-items: center;
  min-height: 10.5rem;
  border-top: 1px solid var(--hairline);
}

.track-lane:last-child {
  border-bottom: 1px solid var(--hairline);
}

.track-lane::before {
  position: absolute;
  z-index: 0;
  top: 50%;
  right: 0;
  left: 7.5rem;
  height: 2px;
  background: var(--lane-soft);
  content: "";
}

.track-stem {
  position: relative;
  z-index: 3;
  align-self: stretch;
  display: flex;
  flex-direction: column;
  justify-content: center;
  min-width: 0;
  padding: 0.7rem 0.75rem;
  border-right: 2px solid var(--lane);
  background: var(--sheet);
}

.track-id {
  margin: 0;
  font-family: "Arial Narrow", "Aptos Display", Aptos, sans-serif;
  font-size: 1.7rem;
  font-weight: 850;
  letter-spacing: -0.04em;
}

.track-authority,
.track-dependency,
.track-waiting {
  margin: 0.42rem 0 0;
  color: var(--ink-soft);
  font-size: 0.62rem;
  line-height: 1.35;
}

.track-dependency {
  color: var(--lane);
}

.track-waiting {
  color: var(--fault);
  font-weight: 750;
}

.work-list {
  position: relative;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 2.6rem;
  width: max-content;
  min-width: 38rem;
  margin: 0;
  padding: 1.45rem 2.5rem;
  list-style: none;
}

.work-node {
  position: relative;
  flex: 0 0 13.25rem;
  min-width: 0;
  list-style: none;
}

.work-node::before {
  position: absolute;
  z-index: -1;
  top: -0.45rem;
  bottom: -0.45rem;
  left: -0.7rem;
  width: 0.85rem;
  border: 1px solid rgba(223, 80, 56, 0.58);
  background: repeating-linear-gradient(
    135deg,
    rgba(223, 80, 56, 0.18) 0 4px,
    transparent 4px 8px
  );
  content: "";
}

.work-leg {
  position: relative;
  min-width: 0;
  border-top: 3px solid var(--ink);
  border-bottom: 1px solid var(--ink);
  background: var(--sheet);
  box-shadow: 4px 4px 0 var(--timing-shadow);
}

.work-leg[open] {
  width: 17.5rem;
}

.work-summary {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.55rem;
  min-width: 0;
  padding: 0.7rem 0.75rem 0.75rem;
  cursor: pointer;
  list-style: none;
}

.work-summary::-webkit-details-marker,
.cadence-summary::-webkit-details-marker {
  display: none;
}

.work-summary::after,
.cadence-summary::after {
  position: absolute;
  right: 0.55rem;
  bottom: 0.35rem;
  color: var(--ink-soft);
  content: "+";
  font-family: ui-monospace, "Cascadia Code", "SFMono-Regular", Consolas, monospace;
  font-size: 0.72rem;
}

.work-leg[open] > .work-summary::after,
.final-exchange[open] > .cadence-summary::after {
  content: "−";
}

.work-identity {
  display: grid;
  min-width: 0;
}

.leg-number {
  color: var(--baton-dark);
  font-family: ui-monospace, "Cascadia Code", "SFMono-Regular", Consolas, monospace;
  font-size: 0.57rem;
  font-weight: 850;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.work-id {
  margin-top: 0.25rem;
  overflow-wrap: anywhere;
  font-size: 0.82rem;
  font-weight: 850;
}

.leg-route,
.work-feeds {
  grid-column: 1 / -1;
}

.leg-route {
  font-size: 0.73rem;
}

.work-feeds {
  color: var(--lane);
  font-size: 0.59rem;
  line-height: 1.4;
  overflow-wrap: anywhere;
}

.lifecycle-stage {
  font-weight: 800;
}

.lifecycle-role {
  color: var(--lane);
  font-weight: 800;
}

.leg-facts {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin: 0 0.7rem;
}

.work-leg > .source,
.work-leg > .relation,
.work-leg > .blocker,
.work-leg > .next-inline {
  margin-right: 0.7rem;
  margin-left: 0.7rem;
}

.source {
  color: var(--ink-soft);
  font-size: 0.62rem;
  line-height: 1.45;
}

.source-mode {
  color: var(--ink);
}

.relation {
  margin-top: 0.55rem;
  margin-bottom: 0;
  color: var(--ink-soft);
  font-size: 0.65rem;
  line-height: 1.4;
}

.relation-label {
  color: var(--ink-soft);
  font-size: 0.58rem;
  font-weight: 850;
  letter-spacing: 0.07em;
  text-transform: uppercase;
}

.relation--depends .relation-id {
  color: var(--lane);
}

.relation--consumes .relation-id {
  color: var(--baton-dark);
}

.next-inline {
  align-items: baseline;
  margin-bottom: 0.7rem;
  padding: 0.55rem 0;
  border-top: 1px solid var(--hairline);
  border-bottom: 1px solid var(--hairline);
}

.next-label {
  color: var(--caution);
}

.track-record {
  position: absolute;
  z-index: 5;
  bottom: 0.3rem;
  left: 0.75rem;
}

.track-record .record-summary {
  max-width: 6rem;
  padding: 0.18rem 0;
  font-size: 0.56rem;
}

.track-record[open] {
  width: min(31rem, calc(100% - 1.5rem));
  padding: 0.75rem;
  border: 1px solid var(--hardline);
  background: var(--sheet);
  box-shadow: 5px 5px 0 var(--timing-shadow);
}

.track-record[open] .record-summary {
  max-width: none;
}

.track-facts {
  grid-template-columns: 1fr;
  margin-top: 0.6rem;
}

.cadence-stack {
  align-self: center;
  display: grid;
  gap: 1.25rem;
  justify-items: stretch;
  min-width: 9.5rem;
  padding-left: 1.4rem;
}

.cadence-node {
  position: relative;
  min-width: 0;
  background: var(--sheet);
}

.final-exchange {
  border-top: 3px solid var(--baton);
  border-bottom: 1px solid var(--ink);
  box-shadow: 4px 4px 0 var(--timing-shadow);
}

.final-exchange::before {
  position: absolute;
  top: -0.5rem;
  bottom: -0.5rem;
  left: -0.7rem;
  width: 0.8rem;
  border: 1px solid rgba(223, 80, 56, 0.65);
  background: repeating-linear-gradient(
    135deg,
    rgba(223, 80, 56, 0.2) 0 4px,
    transparent 4px 8px
  );
  content: "";
}

.cadence-summary {
  position: relative;
  display: grid;
  gap: 0.45rem;
  padding: 0.75rem 0.7rem 1rem;
  cursor: pointer;
  list-style: none;
}

.cadence-label {
  display: grid;
  gap: 0.2rem;
}

.cadence-title {
  margin: 0;
  font-family: "Arial Narrow", "Aptos Display", Aptos, sans-serif;
  font-size: 1.25rem;
  font-weight: 850;
  letter-spacing: -0.035em;
}

.final-exchange > .lifecycle,
.final-exchange > .outcome,
.final-exchange > .source,
.final-exchange > .blocker,
.final-exchange > .next-inline {
  margin-right: 0.7rem;
  margin-left: 0.7rem;
}

.final-exchange > .lifecycle {
  font-size: 0.72rem;
}

.final-exchange > .outcome {
  margin-top: 0.5rem;
  font-size: 0.62rem;
  text-align: left;
}

.finish-node {
  min-height: 7rem;
  padding: 0.75rem 1.05rem 0.8rem 0.7rem;
  border-top: 1px solid var(--ink);
  border-bottom: 3px solid var(--ink);
}

.finish-stripe {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 0.62rem;
  background:
    conic-gradient(
      var(--ink) 0 25%,
      var(--sheet) 0 50%,
      var(--ink) 0 75%,
      var(--sheet) 0
    ) 0 0 / 0.62rem 0.62rem;
}

.finish-copy {
  margin: 0.6rem 0 0;
  color: var(--ink-soft);
  font-size: 0.65rem;
  line-height: 1.45;
}

.diagnostics,
.queue,
.empty,
.loading {
  margin-top: 1.5rem;
  padding: 1.1rem;
  border: 1px solid var(--hardline);
  background: var(--sheet);
}

.diagnostic {
  margin-top: 0.75rem;
  padding: 0.75rem;
  border-left: 4px solid var(--fault);
  background: var(--fault-soft);
}

.diagnostic-mark {
  border-radius: 0;
  background: var(--fault);
}

.queue {
  border-top: 6px solid var(--caution);
  box-shadow: 8px 8px 0 var(--timing-shadow);
}

.operation-grid {
  grid-template-columns: repeat(auto-fit, minmax(230px, 1fr));
  gap: 1px;
  border-top: 1px solid var(--hairline);
  border-left: 1px solid var(--hairline);
}

.operation {
  padding: 0.85rem;
  border-right: 1px solid var(--hairline);
  border-bottom: 1px solid var(--hairline);
  background: var(--sheet);
}

.operation-name,
.operation-scope {
  color: var(--caution);
}

.operation-scope {
  padding-left: 0.5rem;
  border-left: 2px solid var(--caution);
}

.footer {
  border-top: 1px solid var(--hardline);
}

:focus-visible {
  outline: 3px solid var(--baton);
  outline-offset: 3px;
}

@media (max-width: 900px) {
  .release-facts {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 700px) {
  .masthead {
    align-items: center;
  }

  .brand .eyebrow {
    display: none;
  }

  .freshness {
    max-width: 52%;
    font-size: 0.62rem;
    text-align: left;
  }

  .intro,
  .release-heading,
  .relay-heading,
  .queue-heading {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
  }

  .intro {
    padding-top: 3.5rem;
  }

  .intro-title {
    font-size: clamp(2.7rem, 15vw, 5rem);
  }

  .release-facts {
    grid-template-columns: 1fr;
  }

  .relay-map {
    margin-right: 5px;
    padding: 0.9rem;
    box-shadow: 5px 5px 0 var(--timing-shadow);
  }

  .relay-heading {
    padding-right: 2rem;
  }

  .graph-legend {
    justify-content: flex-start;
  }

  .graph-viewport {
    overflow: visible;
  }

  .graph-canvas {
    display: block;
    min-width: 0;
    padding: 1rem 0;
  }

  .graph-links {
    display: none;
  }

  .start-block {
    justify-items: start;
    grid-template-columns: auto auto 1fr;
    gap: 0.55rem;
    padding: 0.8rem 0 1.2rem;
    text-align: left;
  }

  .start-signal,
  .baton-mark {
    margin: 0;
  }

  .start-signal {
    grid-row: 1;
    grid-column: 1;
  }

  .baton-mark {
    grid-row: 1;
    grid-column: 2;
  }

  .start-block .eyebrow {
    grid-row: 2;
    grid-column: 2;
    margin: 0;
  }

  .start-title {
    grid-row: 1;
    grid-column: 3;
    align-self: center;
    font-size: 2rem;
  }

  .track-field {
    width: auto;
    min-width: 0;
  }

  .track-lane {
    display: block;
    min-height: 0;
    padding: 1rem 0 1.35rem 2.8rem;
    border-top: 1px solid var(--hairline);
  }

  .track-lane::before {
    top: 0;
    right: auto;
    bottom: 0;
    left: 1.25rem;
    width: 2px;
    height: auto;
    background: var(--lane);
  }

  .track-stem {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 0.35rem;
    padding: 0 0 0.8rem;
    border: 0;
  }

  .track-authority,
  .track-dependency,
  .track-waiting {
    grid-column: 1 / -1;
  }

  .work-list {
    display: grid;
    gap: 1.15rem;
    width: auto;
    min-width: 0;
    padding: 0;
  }

  .work-node {
    width: auto;
  }

  .work-node::before {
    top: 50%;
    right: 100%;
    bottom: auto;
    left: -1.55rem;
    width: 1.55rem;
    height: 0.65rem;
    border: 0;
    border-top: 2px solid var(--lane);
    background: none;
  }

  .work-leg[open] {
    width: auto;
  }

  .track-record {
    position: relative;
    bottom: auto;
    left: auto;
    margin-top: 0.8rem;
  }

  .track-record[open] {
    width: 100%;
  }

  .cadence-stack {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 1rem;
    margin: 1.25rem 0 0 2.8rem;
    padding: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .graph-links {
    animation: none;
  }
}
`;

const SECURITY_HEADERS = Object.freeze({
  'Cache-Control': 'no-store',
  'Content-Security-Policy': "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; object-src 'none'",
  'Cross-Origin-Resource-Policy': 'same-origin',
  'Permissions-Policy': 'camera=(), microphone=(), geolocation=()',
  'Referrer-Policy': 'no-referrer',
  'X-Content-Type-Options': 'nosniff',
});

const STATIC_ROUTES = Object.freeze({
  '/': Object.freeze({
    status: 200,
    type: 'text/html; charset=utf-8',
    body: Buffer.from(HTML),
  }),
  '/app.js': Object.freeze({
    status: 200,
    type: 'text/javascript; charset=utf-8',
    body: Buffer.from(APP_JS),
  }),
  '/style.css': Object.freeze({
    status: 200,
    type: 'text/css; charset=utf-8',
    body: Buffer.from(CSS),
  }),
});

function validateHost(host) {
  if (host !== '127.0.0.1' && host !== '::1') {
    throw new TypeError('board host must be exactly 127.0.0.1 or ::1');
  }
  return host;
}

function validatePort(port) {
  if (!Number.isSafeInteger(port) || port < 0 || port > 65_535) {
    throw new TypeError('board port must be an integer from 0 to 65535');
  }
  return port;
}

function exactRequestHost(host, port) {
  return host === '::1' ? `[::1]:${port}` : `${host}:${port}`;
}

function send(response, status, type, body, extraHeaders = {}) {
  const bytes = Buffer.isBuffer(body) ? body : Buffer.from(body);
  response.writeHead(status, {
    ...SECURITY_HEADERS,
    ...extraHeaders,
    'Content-Length': bytes.byteLength,
    'Content-Type': type,
  });
  response.end(bytes);
}

function errorBody(status, code) {
  return Buffer.from(`${JSON.stringify({ status, code })}\n`);
}

export function createBoardServer({
  repo = process.cwd(),
  host = '127.0.0.1',
  project = projectBoard,
} = {}) {
  validateHost(host);
  if (typeof project !== 'function') throw new TypeError('board projector must be a function');
  const server = createServer((request, response) => {
    if (request.method !== 'GET') {
      send(
        response,
        405,
        'application/json; charset=utf-8',
        errorBody(405, 'METHOD_NOT_ALLOWED'),
        { Allow: 'GET' },
      );
      return;
    }
    const address = server.address();
    const port = typeof address === 'object' && address ? address.port : null;
    if (port === null || request.headers.host !== exactRequestHost(host, port)) {
      send(
        response,
        421,
        'application/json; charset=utf-8',
        errorBody(421, 'MISDIRECTED_REQUEST'),
      );
      return;
    }
    const route = request.url;
    if (
      typeof route !== 'string'
      || route.includes('?')
      || route.includes('#')
      || route.includes('%')
    ) {
      send(response, 404, 'application/json; charset=utf-8', errorBody(404, 'NOT_FOUND'));
      return;
    }
    if (route === '/favicon.ico') {
      response.writeHead(204, SECURITY_HEADERS);
      response.end();
      return;
    }
    if (route === '/api/board') {
      try {
        send(
          response,
          200,
          'application/json; charset=utf-8',
          boardBytes(project(repo)),
        );
      } catch {
        send(
          response,
          503,
          'application/json; charset=utf-8',
          errorBody(503, 'BOARD_UNAVAILABLE'),
        );
      }
      return;
    }
    const staticRoute = STATIC_ROUTES[route];
    if (!staticRoute) {
      send(response, 404, 'application/json; charset=utf-8', errorBody(404, 'NOT_FOUND'));
      return;
    }
    send(response, staticRoute.status, staticRoute.type, staticRoute.body);
  });
  server.requestTimeout = 10_000;
  server.headersTimeout = 5_000;
  server.keepAliveTimeout = 5_000;
  return server;
}

export function startBoardServer({
  repo = process.cwd(),
  host = '127.0.0.1',
  port = 4177,
  project = projectBoard,
} = {}) {
  validateHost(host);
  validatePort(port);
  const server = createBoardServer({ repo, host, project });
  return new Promise((resolve, reject) => {
    const onError = (error) => {
      server.off('listening', onListening);
      reject(error);
    };
    const onListening = () => {
      server.off('error', onError);
      const address = server.address();
      const actualPort = address.port;
      resolve(Object.freeze({
        server,
        host,
        port: actualPort,
        url: `http://${exactRequestHost(host, actualPort)}`,
        close() {
          return new Promise((closeResolve, closeReject) => {
            server.close((error) => (error ? closeReject(error) : closeResolve()));
          });
        },
      }));
    };
    server.once('error', onError);
    server.once('listening', onListening);
    server.listen(port, host);
  });
}

function usage() {
  return [
    'Usage: node reference/board/web.mjs [--host 127.0.0.1|::1] [--port 0..65535] [repository]',
    '',
  ].join('\n');
}

async function main(argv) {
  let host = '127.0.0.1';
  let port = 4177;
  let repo = process.cwd();
  let sawRepo = false;
  for (let index = 0; index < argv.length; index += 1) {
    const value = argv[index];
    if (value === '--host' && index + 1 < argv.length) {
      host = argv[++index];
    } else if (value === '--port' && index + 1 < argv.length) {
      port = Number(argv[++index]);
    } else if (!value.startsWith('-') && !sawRepo) {
      repo = value;
      sawRepo = true;
    } else {
      process.stderr.write(usage());
      process.exitCode = 64;
      return;
    }
  }
  try {
    const running = await startBoardServer({ repo, host, port });
    process.stdout.write(`${running.url}\n`);
  } catch (error) {
    process.stderr.write(`${error?.message ?? 'board server failed'}\n`);
    process.exitCode = error instanceof TypeError ? 64 : 2;
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  await main(process.argv.slice(2));
}
