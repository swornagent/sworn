"use strict";

const SCHEMA = "sworn.cockpit/v1";
const state = {
  runID: runFromPath(),
  snapshot: null,
  confirmedAt: "",
  selectedID: "",
  selectedButton: null,
  source: null,
  refreshing: false,
  connection: "connecting",
};

const elements = {
  connection: document.querySelector("#connection"),
  releaseTitle: document.querySelector("#release-title"),
  releaseFacts: document.querySelector("#release-facts"),
  handoff: document.querySelector("#handoff"),
  handoffCount: document.querySelector("#handoff-count"),
  handoffCopy: document.querySelector("#handoff-copy"),
  statusNotice: document.querySelector("#status-notice"),
  offset: document.querySelector("#offset"),
  viewport: document.querySelector("#board-viewport"),
  boardTitle: document.querySelector("#board-title"),
  edges: document.querySelector("#edges"),
  topology: document.querySelector("#topology"),
  empty: document.querySelector("#empty-state"),
  emptyEyebrow: document.querySelector("#empty-eyebrow"),
  emptyTitle: document.querySelector("#empty-title"),
  emptyCopy: document.querySelector("#empty-copy"),
  detailTitle: document.querySelector("#detail-title"),
  detail: document.querySelector("#detail-content"),
  actions: document.querySelector("#actions"),
  evidence: document.querySelector("#evidence-list"),
  sheet: document.querySelector("#detail-sheet"),
  sheetTitle: document.querySelector("#sheet-title"),
  sheetContent: document.querySelector("#sheet-content"),
  sheetActions: document.querySelector("#sheet-actions"),
  closeSheet: document.querySelector("#close-sheet"),
  announcer: document.querySelector("#announcer"),
};

function runFromPath() {
  const match = window.location.pathname.match(/^\/runs\/([A-Za-z0-9][A-Za-z0-9._-]{0,127})$/);
  return match ? match[1] : "";
}

function setConnection(value, copy) {
  state.connection = value;
  elements.connection.dataset.state = value;
  elements.connection.lastElementChild.textContent = copy;
  document.querySelectorAll(".control").forEach((button) => {
    button.disabled = !controlsAllowed();
  });
}

function controlsAllowed() {
  return state.connection === "live" && !hasUnconfirmedState();
}

function hasUnconfirmedState() {
  return Boolean(state.snapshot?.diagnostics.some(
    (diagnostic) => diagnostic.code === "BATON_UNAVAILABLE",
  ));
}

function validSnapshot(value) {
  return value && value.schema_version === SCHEMA &&
    value.run && value.run.id === state.runID &&
    value.graph && Array.isArray(value.graph.nodes) &&
    Array.isArray(value.graph.edges) &&
    value.handoff && Array.isArray(value.handoff.nodes) &&
    value.runtime && Array.isArray(value.runtime.effects) &&
    Array.isArray(value.runtime.attempts) &&
    Array.isArray(value.evidence) &&
    Array.isArray(value.actions) &&
    Array.isArray(value.diagnostics) &&
    Number.isSafeInteger(value.through_offset) &&
    value.through_offset >= 0;
}

async function refresh(reason) {
  if (!state.runID || state.refreshing) {
    return;
  }
  state.refreshing = true;
  try {
    const response = await fetch(`/api/v1/runs/${state.runID}/snapshot`, {
      headers: { Accept: "application/json" },
      cache: "no-store",
      credentials: "same-origin",
    });
    if (!response.ok) {
      throw new Error("snapshot unavailable");
    }
    const snapshot = await response.json();
    if (!validSnapshot(snapshot)) {
      throw new Error("snapshot schema mismatch");
    }
    state.snapshot = snapshot;
    state.confirmedAt = new Date().toISOString();
    render();
    setConnection("live", "Live");
    if (reason) {
      elements.announcer.textContent = reason;
    }
    connectEvents(snapshot.through_offset);
  } catch {
    setConnection(state.snapshot ? "stale" : "offline", state.snapshot ? "Stale" : "Unavailable");
    renderFailure();
  } finally {
    state.refreshing = false;
  }
}

function connectEvents(after) {
  if (state.source) {
    state.source.close();
  }
  const source = new EventSource(
    `/api/v1/runs/${state.runID}/events?after=${after}&limit=128`,
    { withCredentials: true },
  );
  state.source = source;
  source.addEventListener("open", () => {
    if (state.snapshot && state.connection !== "live") {
      void refresh("Relay reconnected to a fresh snapshot.");
    }
  });
  source.addEventListener("invalidate", (event) => {
    let value;
    try {
      value = JSON.parse(event.data);
    } catch {
      value = null;
    }
    const eventOffset = Number(event.lastEventId);
    if (!value || value.schema_version !== SCHEMA ||
      !Number.isSafeInteger(value.through_offset) ||
      !Number.isSafeInteger(eventOffset) ||
      eventOffset !== value.through_offset ||
      !state.snapshot) {
      void refresh("Relay rebuilt after an event gap.");
      return;
    }
    if (eventOffset <= state.snapshot.through_offset) {
      return;
    }
    void refresh("Relay updated from durable engine facts.");
  });
  source.addEventListener("unavailable", () => {
    setConnection("stale", "Stale");
    renderFailure();
  });
  source.onerror = () => {
    if (state.snapshot) {
      setConnection("stale", "Reconnecting");
    } else {
      setConnection("offline", "Unavailable");
    }
    renderFailure();
  };
}

function render() {
  const snapshot = state.snapshot;
  hideStatusNotice();
  elements.releaseTitle.textContent = snapshot.run.release;
  elements.releaseFacts.replaceChildren(
    fact("Engine state", snapshot.run.state),
    fact("Desired", snapshot.run.desired_state),
    fact("Plan", short(snapshot.run.plan_digest)),
    fact("Target", short(snapshot.run.target_head)),
  );
  elements.offset.textContent = `Event ${snapshot.through_offset}`;
  renderHandoff(snapshot.handoff);
  renderDiagnosticStatus();
  renderEvidence(snapshot.evidence);
  renderActions(elements.actions, snapshot.actions);

  if (snapshot.graph.nodes.length === 0) {
    state.selectedID = "";
    state.selectedButton = null;
    if (elements.sheet.open) {
      elements.sheet.close();
    }
    elements.viewport.hidden = true;
    showEmpty(
      "Valid empty run",
      "No admitted delivery graph is recorded yet.",
      "This is a valid snapshot. No work has been inferred or invented.",
    );
    elements.topology.replaceChildren();
    elements.edges.replaceChildren();
    renderDetail();
    return;
  }

  const selectionDisappeared = state.selectedID !== "" &&
    !snapshot.graph.nodes.some((node) => node.id === state.selectedID);
  if (!state.selectedID || selectionDisappeared) {
    state.selectedID = snapshot.graph.nodes[0]?.id ?? "";
  }
  if (selectionDisappeared) {
    state.selectedButton = null;
    if (elements.sheet.open) {
      elements.sheet.close();
    }
  }
  elements.empty.hidden = true;
  elements.viewport.hidden = false;
  const selectedWasFocused =
    document.activeElement?.dataset.nodeId === state.selectedID;
  renderGraph(snapshot.graph);
  renderDetail();
  if (selectionDisappeared) {
    requestAnimationFrame(() => {
      elements.boardTitle.focus();
      elements.announcer.textContent =
        "The selected carrier is no longer present. Focus returned to the relay.";
    });
  } else if (selectedWasFocused) {
    requestAnimationFrame(() => {
      if (state.selectedButton?.isConnected) {
        state.selectedButton.focus();
      }
    });
  }
}

function renderFailure() {
  if (state.snapshot) {
    showStatusNotice(
      `Live updates paused. Showing the last confirmed snapshot from ${formatDateTime(state.confirmedAt)}. ` +
      "Controls are disabled until a fresh snapshot arrives.",
    );
    return;
  }
  elements.viewport.hidden = true;
  elements.releaseTitle.textContent = state.runID || "Waiting for a run";
  elements.releaseFacts.replaceChildren();
  elements.handoff.hidden = true;
  elements.evidence.replaceChildren();
  elements.actions.replaceChildren();
  if (state.runID) {
    showEmpty(
      "Snapshot unavailable",
      "Cockpit unavailable.",
      "The local service returned a snapshot this UI cannot safely display.",
    );
  } else {
    showEmpty(
      "No run selected",
      "Open a run to see its relay.",
      "Use /runs/<run-id>. Nothing is inferred before the engine records it.",
    );
  }
}

function showEmpty(eyebrow, title, copy) {
  elements.empty.hidden = false;
  elements.emptyEyebrow.textContent = eyebrow;
  elements.emptyTitle.textContent = title;
  elements.emptyCopy.textContent = copy;
}

function renderDiagnosticStatus() {
  if (hasUnconfirmedState()) {
    showStatusNotice(
      "State unavailable. Sworn could not confirm this item from durable facts. Controls are disabled.",
    );
  }
}

function showStatusNotice(copy) {
  elements.statusNotice.hidden = false;
  elements.statusNotice.textContent = copy;
}

function hideStatusNotice() {
  elements.statusNotice.hidden = true;
  elements.statusNotice.textContent = "";
}

function renderHandoff(handoff) {
  if (!handoff.ready || handoff.nodes.length === 0) {
    elements.handoff.hidden = true;
    return;
  }
  elements.handoff.hidden = false;
  const count = handoff.nodes.length;
  elements.handoffCount.textContent = count === 1 ? "1 exact exchange" : `${count} exact exchanges`;
  const roles = handoff.responsibilities.map(humanize).join(" + ");
  elements.handoffCopy.textContent = `ready for ${roles}.`;
}

function renderGraph(graph) {
  const nodes = new Map(graph.nodes.map((node) => [node.id, node]));
  const tracks = graph.nodes.filter((node) => node.kind === "track");
  const children = new Map(tracks.map((track) => [track.label, []]));
  graph.nodes.filter((node) => node.kind === "slice").forEach((node) => {
    if (children.has(node.track)) {
      children.get(node.track).push(node);
    }
  });
  const items = [];
  const release = graph.nodes.find((node) => node.kind === "release");
  if (release) {
    items.push(endpointItem(release));
  }
  tracks.forEach((track) => {
    const item = document.createElement("li");
    item.className = "track-lane";
    const label = document.createElement("p");
    label.className = "track-label";
    label.textContent = track.label;
    const rail = document.createElement("div");
    rail.className = "slice-rail";
    const slices = children.get(track.label) ?? [];
    if (slices.length === 0) {
      rail.append(nodeButton(track));
    } else {
      slices.forEach((node) => rail.append(nodeButton(node)));
    }
    item.append(label, rail);
    items.push(item);
  });
  const assembly = graph.nodes.find((node) => node.kind === "assembly");
  if (assembly) {
    items.push(endpointItem(assembly));
  }
  elements.topology.replaceChildren(...items);
  requestAnimationFrame(() => drawEdges(graph.edges, nodes));
}

function endpointItem(node) {
  const item = document.createElement("li");
  item.className = "endpoint";
  item.append(nodeButton(node));
  return item;
}

function nodeButton(node) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "node";
  button.dataset.nodeId = node.id;
  button.dataset.state = node.state || "unknown";
  button.dataset.outcome = node.outcome || "none";
  button.setAttribute("aria-pressed", String(node.id === state.selectedID));
  const label = document.createElement("span");
  label.className = "node-label";
  label.textContent = node.label;
  const meta = document.createElement("span");
  meta.className = "node-meta";
  const parts = [humanize(node.state)];
  if (node.next_responsibility && node.next_responsibility !== "none") {
    parts.push(`next ${humanize(node.next_responsibility)}`);
  }
  meta.textContent = parts.join(" · ");
  button.append(label, meta);
  button.addEventListener("click", () => selectNode(node.id, button));
  if (node.id === state.selectedID) {
    state.selectedButton = button;
  }
  return button;
}

function selectNode(nodeID, button) {
  state.selectedID = nodeID;
  state.selectedButton = button;
  document.querySelectorAll(".node").forEach((node) => {
    node.setAttribute("aria-pressed", String(node.dataset.nodeId === nodeID));
  });
  renderDetail();
  if (window.matchMedia("(max-width: 72rem)").matches &&
    typeof elements.sheet.showModal === "function") {
    elements.sheet.showModal();
    elements.closeSheet.focus();
  }
}

function renderDetail() {
  const snapshot = state.snapshot;
  if (!snapshot || !state.selectedID) {
    elements.detailTitle.textContent = "Select a carrier";
    elements.detail.replaceChildren(
      textBlock("No relay node is available for detail."),
    );
    elements.sheetTitle.textContent = "Carrier";
    elements.sheetContent.replaceChildren();
    elements.sheetActions.replaceChildren();
    return;
  }
  const node = snapshot.graph.nodes.find((item) => item.id === state.selectedID);
  if (!node) {
    return;
  }
  const title = `${humanize(node.kind)} · ${node.label}`;
  elements.detailTitle.textContent = title;
  elements.sheetTitle.textContent = title;
  const details = document.createElement("dl");
  details.className = "detail-grid";
  [
    ["State", humanize(node.state)],
    ["Stage", reported(node.stage)],
    ["Outcome", reported(node.outcome)],
    ["Next responsibility", reported(humanize(node.next_responsibility))],
    ["Attempt", node.attempt ? String(node.attempt) : "Not reported"],
    ["Node ID", node.id],
  ].forEach(([label, value]) => details.append(fact(label, value)));
  elements.detail.replaceChildren(details.cloneNode(true));
  elements.sheetContent.replaceChildren(details);
  renderActions(elements.sheetActions, snapshot.actions);
}

function renderActions(container, actions) {
  const buttons = actions.map((action) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "control";
    button.dataset.kind = action.kind;
    button.textContent = actionLabel(action);
    button.disabled = !controlsAllowed();
    button.addEventListener("click", () => void submitAction(action, button));
    return button;
  });
  container.replaceChildren(...buttons);
}

async function submitAction(action, button) {
  if (!state.snapshot || state.connection !== "live") {
    return;
  }
  button.disabled = true;
  const body = {
    run_id: state.runID,
    command_id: commandID(),
    kind: action.kind,
    expected_generation: action.expected_generation,
  };
  if (action.kind === "retry") {
    body.work_id = action.work_id;
    body.expected_epoch = action.expected_epoch;
  }
  try {
    const response = await fetch(`/api/v1/runs/${state.runID}/commands`, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      credentials: "same-origin",
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      throw new Error("command rejected");
    }
    await refresh(`${humanize(action.kind)} accepted.`);
  } catch {
    setConnection("stale", "Refresh required");
    elements.announcer.textContent = `${humanize(action.kind)} was not accepted. Refresh before trying again.`;
  }
}

function renderEvidence(events) {
  if (events.length === 0) {
    const item = document.createElement("li");
    item.className = "evidence-empty";
    const kind = document.createElement("strong");
    kind.textContent = "No durable events yet.";
    const copy = document.createElement("span");
    copy.className = "quiet";
    copy.textContent = "No durable evidence has been recorded for this snapshot.";
    item.append(kind, copy);
    elements.evidence.replaceChildren(item);
    return;
  }
  const items = events.map((event) => {
    const item = document.createElement("li");
    const offset = document.createElement("span");
    offset.className = "eyebrow";
    offset.textContent = `Event ${event.offset}`;
    const kind = document.createElement("strong");
    kind.textContent = humanize(event.kind);
    const time = document.createElement("time");
    time.dateTime = event.created_at;
    time.textContent = formatTime(event.created_at);
    item.append(offset, kind, time);
    return item;
  });
  elements.evidence.replaceChildren(...items);
}

function drawEdges(edges) {
  const viewportBox = elements.viewport.getBoundingClientRect();
  const width = elements.viewport.scrollWidth;
  const height = elements.viewport.scrollHeight;
  elements.edges.setAttribute("viewBox", `0 0 ${width} ${height}`);
  elements.edges.setAttribute("width", String(width));
  elements.edges.setAttribute("height", String(height));
  const paths = [];
  edges.forEach((edge) => {
    if (edge.kind === "contains") {
      return;
    }
    const from = elements.topology.querySelector(`[data-node-id="${CSS.escape(edge.from)}"]`);
    const to = elements.topology.querySelector(`[data-node-id="${CSS.escape(edge.to)}"]`);
    if (!from || !to) {
      return;
    }
    const left = from.getBoundingClientRect();
    const right = to.getBoundingClientRect();
    const x1 = left.right - viewportBox.left + elements.viewport.scrollLeft;
    const y1 = left.top + left.height / 2 - viewportBox.top + elements.viewport.scrollTop;
    const x2 = right.left - viewportBox.left + elements.viewport.scrollLeft;
    const y2 = right.top + right.height / 2 - viewportBox.top + elements.viewport.scrollTop;
    const bend = Math.max(24, Math.abs(x2 - x1) / 2);
    const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
    path.classList.add("edge-line");
    path.dataset.kind = edge.kind;
    path.setAttribute("d", `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`);
    paths.push(path);
  });
  elements.edges.replaceChildren(...paths);
}

function fact(label, value) {
  const wrapper = document.createElement("div");
  const term = document.createElement("dt");
  term.className = "fact-label";
  term.textContent = label;
  const detail = document.createElement("dd");
  detail.className = "fact-value";
  detail.textContent = reported(value);
  wrapper.append(term, detail);
  return wrapper;
}

function textBlock(value) {
  const paragraph = document.createElement("p");
  paragraph.className = "quiet";
  paragraph.textContent = value;
  return paragraph;
}

function reported(value) {
  return value === null || value === undefined || value === "" || value === "none"
    ? "Not reported"
    : String(value);
}

function short(value) {
  if (!value) {
    return "Not reported";
  }
  return value.length > 14 ? `${value.slice(0, 10)}…` : value;
}

function humanize(value) {
  if (!value) {
    return "";
  }
  return String(value).replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function actionLabel(action) {
  if (action.kind === "retry") {
    return `Retry epoch ${action.expected_epoch}`;
  }
  return humanize(action.kind);
}

function commandID() {
  if (crypto.randomUUID) {
    return crypto.randomUUID();
  }
  return `cockpit-${Date.now()}-${Math.floor(Math.random() * 1_000_000)}`;
}

function formatTime(value) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? "Not reported"
    : new Intl.DateTimeFormat(undefined, {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    }).format(date);
}

function formatDateTime(value) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? "an unreported time"
    : new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "medium",
    }).format(date);
}

elements.closeSheet.addEventListener("click", () => elements.sheet.close());
elements.sheet.addEventListener("click", (event) => {
  if (event.target === elements.sheet) {
    elements.sheet.close();
  }
});
elements.sheet.addEventListener("close", () => {
  if (state.selectedButton?.isConnected) {
    state.selectedButton.focus();
  }
});
window.addEventListener("resize", () => {
  if (elements.sheet.open &&
    !window.matchMedia("(max-width: 72rem)").matches) {
    elements.sheet.close();
  }
  if (state.snapshot) {
    requestAnimationFrame(() => drawEdges(state.snapshot.graph.edges));
  }
});

if (state.runID) {
  void refresh("");
} else {
  setConnection("offline", "No run selected");
  renderFailure();
}
