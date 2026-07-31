"use strict";

const SCHEMA = "sworn.cockpit/v2";
const API = "/api/v2";
const SNAPSHOT_POLL_MILLIS = 5_000;
const MAX_ATTENTION_ANSWER_BYTES = 16 * 1024;
const RUN_PRESENTATIONS = {
  new: {
    status: "Ready to start",
    doing: "Sworn has recorded the run and has not started delivery yet.",
    next: "Start when the plan and AI connections are ready.",
    needs: "Only if you want to start the run.",
  },
  running: {
    status: "Sworn is working",
    doing: "Sworn is carrying the next recorded Baton handoff.",
    next: "Sworn will continue with the next ready handoff.",
    needs: "No, unless Sworn asks a question.",
  },
  awaiting_approval: {
    status: "Waiting for approval",
    doing: "The proposed plan is ready, but delivery has not been approved.",
    next: "Approve the plan through the configured approval source.",
    needs: "Yes — approve or decline the plan.",
  },
  pausing: {
    status: "Pausing safely",
    doing: "Current work is reaching a safe stopping point.",
    next: "The run will pause when in-flight work is settled.",
    needs: "No.",
  },
  paused: {
    status: "Paused",
    doing: "Sworn has stopped starting new work.",
    next: "Resume when you want delivery to continue.",
    needs: "Only when you are ready to resume.",
  },
  cancelling: {
    status: "Cancelling safely",
    doing: "Sworn is stopping the run at a safe boundary.",
    next: "The run will be cancelled when in-flight work is settled.",
    needs: "No.",
  },
  cancelled: {
    status: "Cancelled",
    doing: "This run will not start any more delivery work.",
    next: "No delivery work remains for this run.",
    needs: "No.",
  },
  parked: {
    status: "Stopped and needs your attention",
    doing: "One work item needs an answer or has failed repeatedly.",
    next: "Open the latest board to answer the question or review the retry action.",
    needs: "Yes — review the stopped work.",
  },
  takeover_required: {
    status: "Resume required",
    doing: "The previous Sworn process stopped and no process owns the run.",
    next: "Take over the run so Sworn can recheck it and continue.",
    needs: "Yes — take over the run.",
  },
  uncertain: {
    status: "Needs confirmation",
    doing: "Sworn cannot confirm whether the last external action finished.",
    next: "Recover the run before repeating that action.",
    needs: "Yes — confirm or recover the last action.",
  },
  complete: {
    status: "Complete",
    doing: "Baton records show that the checked release was merged.",
    next: "No delivery work remains.",
    needs: "No.",
  },
};
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
  attentionPanel: document.querySelector("#attention-panel"),
  attentions: document.querySelector("#attention-list"),
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
  outbox: document.querySelector("#outbox-list"),
  outboxWindow: document.querySelector("#outbox-window"),
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

function presentSnapshot(snapshot) {
  let presentation = RUN_PRESENTATIONS[snapshot.run.state] ?? {
    status: "Status unavailable",
    doing: "Sworn does not have a plain-language explanation for this recorded state.",
    next: "Review the technical state before continuing.",
    needs: "Yes — review the run details.",
  };
  if (snapshot.diagnostics.some(
    (diagnostic) => diagnostic.code === "BATON_UNAVAILABLE",
  )) {
    presentation = {
      status: "Needs confirmation",
      doing: "Sworn could not confirm the current Baton handoff records.",
      next: "Restore repository access and refresh before continuing.",
      needs: "Yes — controls are disabled until the facts can be confirmed.",
    };
  } else if (snapshot.runtime.attentions.some(
    (attention) => attention.state === "open",
  )) {
    presentation = {
      status: "Waiting for your answer",
      doing: "One part of the work needs a judgment Sworn cannot make safely.",
      next: "Answer the saved question; unrelated work may continue.",
      needs: "Yes — answer the question shown below.",
    };
  } else if (snapshot.run.state === "parked" &&
    snapshot.actions.some((action) => action.kind === "retry")) {
    presentation = {
      status: "Stopped after repeated failures",
      doing: "Sworn stopped this work instead of retrying without a safe reason.",
      next: "Review the failed item, then retry it from the current board.",
      needs: "Yes — review the failure before retrying.",
    };
  }
  return presentation;
}

function validSnapshot(value) {
  return value && value.schema_version === SCHEMA &&
    value.run && value.run.id === state.runID &&
    value.graph && Array.isArray(value.graph.nodes) &&
    Array.isArray(value.graph.edges) &&
    value.handoff && Array.isArray(value.handoff.nodes) &&
    Array.isArray(value.handoff.responsibilities) &&
    validGraphHandoff(value.graph, value.handoff) &&
    value.runtime && Array.isArray(value.runtime.effects) &&
    Array.isArray(value.runtime.attempts) &&
    Array.isArray(value.runtime.attentions) &&
    value.runtime.attentions.every(validAttention) &&
    typeof value.runtime.attentions_truncated === "boolean" &&
    Array.isArray(value.runtime.notifications) &&
    Array.isArray(value.evidence) &&
    Array.isArray(value.actions) &&
    Array.isArray(value.diagnostics) &&
    Number.isSafeInteger(value.through_offset) &&
    value.through_offset >= 0;
}

function validAttention(value) {
  return value && /^sha256:[0-9a-f]{64}$/.test(value.id) &&
    /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(value.lane_id) &&
    ["open", "answered", "resolved", "cancelled"].includes(value.state) &&
    Number.isSafeInteger(value.generation) &&
    value.generation >= 1 && value.generation <= 3 &&
    typeof value.question === "string" && value.question !== "" &&
    (value.answer === undefined || typeof value.answer === "string");
}

function validGraphHandoff(graph, handoff) {
  if (typeof handoff.ready !== "boolean") {
    return false;
  }
  const nodeIDs = new Set();
  const batonNodes = [];
  const batonResponsibilities = [];
  const seenResponsibilities = new Set();
  for (const node of graph.nodes) {
    if (!node || typeof node.id !== "string" || node.id === "" ||
      nodeIDs.has(node.id) ||
      typeof node.has_baton !== "boolean" ||
      (node.runtime_state !== undefined &&
        !["parked"].includes(node.runtime_state))) {
      return false;
    }
    nodeIDs.add(node.id);
    if (node.has_baton) {
      if (typeof node.next_responsibility !== "string" ||
        node.next_responsibility === "" ||
        node.next_responsibility === "none") {
        return false;
      }
      batonNodes.push(node.id);
      if (!seenResponsibilities.has(node.next_responsibility)) {
        batonResponsibilities.push(node.next_responsibility);
        seenResponsibilities.add(node.next_responsibility);
      }
    }
  }
  if (handoff.nodes.some((nodeID) =>
    typeof nodeID !== "string" || !nodeIDs.has(nodeID)) ||
    new Set(handoff.nodes).size !== handoff.nodes.length ||
    batonNodes.length !== handoff.nodes.length ||
    batonNodes.some((nodeID, index) => nodeID !== handoff.nodes[index]) ||
    handoff.responsibilities.some((responsibility) =>
      typeof responsibility !== "string") ||
    new Set(handoff.responsibilities).size !==
      handoff.responsibilities.length ||
    batonResponsibilities.length !== handoff.responsibilities.length ||
    batonResponsibilities.some((responsibility, index) =>
      responsibility !== handoff.responsibilities[index])) {
    return false;
  }
  return handoff.ready === (handoff.nodes.length > 0);
}

async function refresh(reason, reconnectEvents = true) {
  if (!state.runID || state.refreshing) {
    return;
  }
  state.refreshing = true;
  try {
    const response = await fetch(`${API}/runs/${state.runID}/snapshot`, {
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
    if (reconnectEvents) {
      connectEvents(snapshot.through_offset);
    }
  } catch {
    setConnection(
      state.snapshot ? "stale" : "offline",
      state.snapshot ? "Reconnecting" : "Cannot connect",
    );
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
    `${API}/runs/${state.runID}/events?after=${after}&limit=128`,
    { withCredentials: true },
  );
  state.source = source;
  source.addEventListener("open", () => {
    if (state.snapshot && state.connection !== "live") {
      void refresh("The delivery board reconnected and checked the latest saved state.");
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
      void refresh("The delivery board refreshed after missing an update.");
      return;
    }
    if (eventOffset <= state.snapshot.through_offset) {
      return;
    }
    void refresh("The delivery board checked the latest saved run facts.");
  });
  source.addEventListener("unavailable", () => {
    setConnection("stale", "Updates paused");
    renderFailure();
  });
  source.onerror = () => {
    if (state.snapshot) {
      setConnection("stale", "Reconnecting");
    } else {
      setConnection("offline", "Cannot connect");
    }
    renderFailure();
  };
}

function render() {
  const snapshot = state.snapshot;
  const presentation = presentSnapshot(snapshot);
  hideStatusNotice();
  elements.releaseTitle.textContent = snapshot.run.release;
  elements.releaseFacts.replaceChildren(
    fact("Status", presentation.status),
    fact("What's happening", presentation.doing),
    fact("Next", presentation.next),
    fact("Needs you", presentation.needs),
  );
  elements.offset.textContent = `Checked update ${snapshot.through_offset}`;
  renderHandoff(snapshot.handoff);
  renderDiagnosticStatus();
  renderAttentions(snapshot.runtime.attentions, snapshot.actions);
  renderEvidence(snapshot.evidence);
  renderOutbox(
    snapshot.runtime.notifications,
    snapshot.runtime.notifications_truncated,
  );
  renderActions(elements.actions, snapshot.actions);

  if (snapshot.graph.nodes.length === 0) {
    state.selectedID = "";
    state.selectedButton = null;
    if (elements.sheet.open) {
      elements.sheet.close();
    }
    elements.viewport.hidden = true;
    showEmpty(
      "No work recorded yet",
      "This run does not have a delivery plan to show yet.",
      "Sworn will show work here only after it has been recorded.",
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
  renderGraph(snapshot.graph, snapshot.handoff);
  renderDetail();
  if (selectionDisappeared) {
    requestAnimationFrame(() => {
      elements.boardTitle.focus();
      elements.announcer.textContent =
        "The selected item is no longer present. Focus returned to the delivery board.";
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
      `Live updates paused. Showing the last confirmed view from ${formatDateTime(state.confirmedAt)}. ` +
      "Controls are disabled until Sworn can check the run again.",
    );
    return;
  }
  elements.viewport.hidden = true;
  elements.releaseTitle.textContent = state.runID || "Waiting for a run";
  elements.releaseFacts.replaceChildren();
  elements.handoff.hidden = true;
  elements.evidence.replaceChildren();
  elements.outbox.replaceChildren();
  elements.outboxWindow.textContent = "";
  elements.actions.replaceChildren();
  elements.attentionPanel.hidden = true;
  elements.attentions.replaceChildren();
  if (state.runID) {
    showEmpty(
      "Run details unavailable",
      "Sworn cannot show this run yet.",
      "The local service did not provide a current saved view that the board could safely display.",
    );
  } else {
    showEmpty(
      "No run selected",
      "Open a run to see its work.",
      "Use /runs/<run-id>. Sworn only shows work it has recorded.",
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
      "Sworn could not confirm the current Baton handoff records. Controls are disabled until the facts can be checked.",
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
  elements.handoffCount.textContent =
    count === 1 ? "1 handoff ready" : `${count} handoffs ready`;
  const roles = handoff.responsibilities.map(humanize).join(" + ");
  elements.handoffCopy.textContent =
    `for ${roles} at ${handoff.nodes.join(" + ")}.`;
}

function renderGraph(graph, handoff) {
  const handoffNodes = new Set(handoff.nodes);
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
    items.push(endpointItem(release, handoffNodes));
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
    const trackNode = nodeButton(track, handoffNodes);
    trackNode.classList.add("track-node");
    rail.append(trackNode);
    slices.forEach((node) => rail.append(nodeButton(node, handoffNodes)));
    item.append(label, rail);
    items.push(item);
  });
  const assembly = graph.nodes.find((node) => node.kind === "assembly");
  if (assembly) {
    items.push(endpointItem(assembly, handoffNodes));
  }
  elements.topology.replaceChildren(...items);
  requestAnimationFrame(() => drawEdges(graph.edges));
}

function endpointItem(node, handoffNodes) {
  const item = document.createElement("li");
  item.className = "endpoint";
  item.append(nodeButton(node, handoffNodes));
  return item;
}

function nodeButton(node, handoffNodes) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "node";
  button.dataset.nodeId = node.id;
  button.dataset.kind = node.kind;
  button.dataset.state = node.state || "unknown";
  button.dataset.runtimeState = node.runtime_state || "none";
  button.dataset.outcome = node.outcome || "none";
  button.dataset.hasBaton = String(node.has_baton);
  button.dataset.handoff = String(handoffNodes.has(node.id));
  button.setAttribute("aria-pressed", String(node.id === state.selectedID));
  const label = document.createElement("span");
  label.className = "node-label";
  label.textContent = node.label;
  const meta = document.createElement("span");
  meta.className = "node-meta";
  const parts = [humanize(node.state)];
  if (node.runtime_state) {
    parts.push(`runtime ${humanize(node.runtime_state)}`);
  }
  if (node.next_responsibility && node.next_responsibility !== "none") {
    parts.push(`next ${humanize(node.next_responsibility)}`);
  }
  meta.textContent = parts.join(" · ");
  button.append(label, meta);
  if (handoffNodes.has(node.id)) {
    const joint = document.createElement("span");
    joint.className = "node-handoff";
    const baton = document.createElement("span");
    baton.textContent = "Baton";
    const sworn = document.createElement("span");
    sworn.textContent = "Sworn";
    joint.append(baton, sworn);
    button.append(joint);
  }
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
    elements.detailTitle.textContent = "Select an item";
    elements.detail.replaceChildren(
      textBlock("No recorded work item is available for detail."),
    );
    elements.sheetTitle.textContent = "Item";
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
    ["Status", humanize(node.state)],
    ["Sworn status", reported(humanize(node.runtime_state))],
    ["Step", reported(humanize(node.stage))],
    ["Result", reported(humanize(node.outcome))],
    ["Next owner", reported(humanize(node.next_responsibility))],
    ["Handoff recorded", node.has_baton ? "Yes" : "No"],
    [
      "Ready for next owner",
      snapshot.handoff.nodes.includes(node.id) ? "Ready" : "Not ready",
    ],
    ["Try", node.attempt ? String(node.attempt) : "Not recorded"],
    ["Technical ID", node.id],
  ].forEach(([label, value]) => details.append(fact(label, value)));
  elements.detail.replaceChildren(details.cloneNode(true));
  elements.sheetContent.replaceChildren(details);
  renderActions(elements.sheetActions, snapshot.actions);
}

function renderActions(container, actions) {
  const buttons = actions
    .filter((action) => action.kind !== "answer_attention")
    .map((action) => {
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

function admittedAnswerAction(attention, actions) {
  if (attention.state !== "open") {
    return undefined;
  }
  return actions.find((action) =>
    action.kind === "answer_attention" &&
    action.attention_id === attention.id &&
    action.expected_generation === attention.generation
  );
}

function renderAttentions(attentions, actions) {
  if (attentions.length === 0) {
    elements.attentionPanel.hidden = true;
    elements.attentions.replaceChildren();
    return;
  }
  elements.attentionPanel.hidden = false;
  const items = attentions.map((attention) => {
    const item = document.createElement("li");
    const identity = document.createElement("p");
    identity.className = "eyebrow";
    identity.textContent =
      `Work ${attention.lane_id} · ${humanize(attention.state)}`;
    const question = document.createElement("p");
    question.className = "attention-question";
    question.textContent = attention.question;
    item.append(identity, question);
    const action = admittedAnswerAction(attention, actions);
    if (action) {
      const form = document.createElement("form");
      form.className = "attention-form";
      const label = document.createElement("label");
      label.textContent = "Your answer";
      const input = document.createElement("textarea");
      input.name = "answer";
      input.maxLength = 16_384;
      input.required = true;
      input.rows = 3;
      const button = document.createElement("button");
      button.type = "submit";
      button.className = "control";
      button.textContent = "Answer and continue";
      button.disabled = !controlsAllowed();
      form.addEventListener("submit", (event) => {
        event.preventDefault();
        void submitAttention(attention, input, button);
      });
      label.append(input);
      form.append(label, button);
      item.append(form);
    } else if (attention.answer) {
      const answer = document.createElement("p");
      answer.className = "quiet";
      answer.textContent = `Answer: ${attention.answer}`;
      item.append(answer);
    }
    return item;
  });
  elements.attentions.replaceChildren(...items);
}

async function submitAttention(attention, input, button) {
  const answer = input.value.trim();
  const action = state.snapshot &&
    admittedAnswerAction(attention, state.snapshot.actions);
  if (!action || !controlsAllowed() || answer === "") {
    return;
  }
  input.setCustomValidity("");
  if (new TextEncoder().encode(answer).byteLength >
    MAX_ATTENTION_ANSWER_BYTES) {
    input.setCustomValidity("Keep the answer under 16 KiB of UTF-8 text.");
    input.reportValidity();
    return;
  }
  button.disabled = true;
  try {
    const response = await fetch(
      `${API}/runs/${state.runID}/attentions/${attention.id}/answer`,
      {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
        },
        credentials: "same-origin",
        body: JSON.stringify({
          run_id: state.runID,
          attention_id: attention.id,
          expected_generation: action.expected_generation,
          answer,
        }),
      },
    );
    if (!response.ok) {
      throw new Error("answer rejected");
    }
    await refresh("Answer recorded; the waiting work can continue again.");
    requestAnimationFrame(() => elements.boardTitle.focus());
  } catch {
    setConnection("stale", "Refresh required");
    elements.announcer.textContent =
      "The answer was not accepted. Refresh before trying again.";
  }
}

async function submitAction(action, button) {
  if (!state.snapshot || state.connection !== "live") {
    return;
  }
  button.disabled = true;
  const redelivery = action.kind === "redeliver";
  const body = redelivery ? {
    run_id: state.runID,
    destination_id: action.destination_id,
    message_id: action.message_id,
  } : {
    run_id: state.runID,
    command_id: commandID(),
    kind: action.kind,
    expected_generation: action.expected_generation,
  };
  if (!redelivery && action.kind === "retry") {
    body.work_id = action.work_id;
    body.expected_epoch = action.expected_epoch;
  }
  try {
    const path = redelivery
      ? `${API}/runs/${state.runID}/notifications/redeliver`
      : `${API}/runs/${state.runID}/commands`;
    const response = await fetch(path, {
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
    kind.textContent = "No saved activity yet.";
    const copy = document.createElement("span");
    copy.className = "quiet";
    copy.textContent = "Sworn has not recorded any activity for this run yet.";
    item.append(kind, copy);
    elements.evidence.replaceChildren(item);
    return;
  }
  const items = events.map((event) => {
    const item = document.createElement("li");
    const offset = document.createElement("span");
    offset.className = "eyebrow";
    offset.textContent = `Saved update ${event.offset}`;
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

function renderOutbox(notifications, truncated) {
  elements.outboxWindow.textContent = truncated
    ? "Showing the latest delivery records."
    : `${notifications.length} recorded`;
  if (notifications.length === 0) {
    const item = document.createElement("li");
    const kind = document.createElement("strong");
    kind.textContent = "No notification deliveries recorded.";
    const copy = document.createElement("span");
    copy.className = "quiet";
    copy.textContent = "Webhook delivery attempts will appear here.";
    item.append(kind, copy);
    elements.outbox.replaceChildren(item);
    return;
  }
  const items = notifications.map((notification) => {
    const item = document.createElement("li");
    item.className = `notification-${notification.state}`;
    const destination = document.createElement("span");
    destination.className = "eyebrow";
    destination.textContent =
      `${notification.destination_id} · Sequence ${notification.sequence}`;
    const stateCopy = document.createElement("strong");
    stateCopy.textContent =
      `${humanize(notification.state)} · Try ${notification.attempts}`;
    const message = document.createElement("span");
    message.className = "node-meta";
    message.textContent = notification.message_id;
    const error = document.createElement("span");
    error.className = "quiet";
    error.textContent = reported(notification.last_error_code);
    item.append(destination, stateCopy, message, error);
    return item;
  });
  elements.outbox.replaceChildren(...items);
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
    path.dataset.edgeId = edge.id;
    path.dataset.from = edge.from;
    path.dataset.to = edge.to;
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
    ? "Not recorded"
    : String(value);
}

function short(value) {
  if (!value) {
    return "Not recorded";
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
  if (action.kind === "redeliver") {
    return `Send ${short(action.message_id)} again`;
  }
  if (action.kind === "retry") {
    return "Retry this work";
  }
  if (action.kind === "pause") {
    return "Pause safely";
  }
  if (action.kind === "cancel") {
    return "Cancel safely";
  }
  if (action.kind === "takeover") {
    return "Take over";
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
    ? "Time unavailable"
    : new Intl.DateTimeFormat(undefined, {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    }).format(date);
}

function formatDateTime(value) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? "an unknown time"
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
  window.setInterval(
    () => void refresh("", false),
    SNAPSHOT_POLL_MILLIS,
  );
} else {
  setConnection("offline", "No run selected");
  renderFailure();
}
