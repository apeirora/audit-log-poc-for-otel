package ui

var pageHTML = []byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Audit log demo</title>
<style>
  body { font-family: "Segoe UI", system-ui, sans-serif; margin: 2rem auto; max-width: 64rem; color: #1c1917; background: #fafaf9; }
  h1 { font-size: 1.6rem; font-weight: 600; margin: 0 0 1rem; }
  .top { display: flex; gap: 1.5rem; align-items: end; flex-wrap: wrap; }
  label { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.9rem; }
  input, select { font: inherit; padding: 0.35rem 0.5rem; border: 1px solid #a8a29e; background: white; }
  input { width: 6rem; }
  select { width: 24rem; max-width: 100%; }
  #status { margin-left: auto; font-size: 1rem; }
  .rows { margin: 0.5rem 0 1rem; }
  .row { display: flex; gap: 0.75rem; align-items: end; flex-wrap: wrap; margin-bottom: 0.5rem; }
  .row button { margin: 0; }
  .groups { display: grid; grid-template-columns: 1fr 1fr; gap: 1.5rem; margin: 1.5rem 0; }
  h2 { font-size: 0.8rem; letter-spacing: 0.04em; text-transform: uppercase; color: #57534e; margin: 0 0 0.5rem; }
  button { font: inherit; margin: 0 0.5rem 0.5rem 0; padding: 0.4rem 0.7rem; background: white; border: 1px solid #44403c; cursor: pointer; }
  button:hover { background: #e7e5e4; }
  #launch { margin-top: 0.25rem; padding: 0.55rem 1rem; background: #1c1917; color: #fafaf9; border-color: #1c1917; }
  #launch:hover { background: #44403c; }
  #loss { grid-column: 1 / -1; }
  .nums { display: grid; grid-template-columns: repeat(4, 1fr); gap: 0.75rem; }
  .nums div, .hops div { background: white; border: 1px solid #e7e5e4; padding: 0.6rem 0.75rem; }
  .nums span, .hops span { display: block; font-size: 0.75rem; color: #57534e; }
  .nums strong, .hops strong { font-size: 1.4rem; font-variant-numeric: tabular-nums; }
  .hops { display: grid; grid-template-columns: repeat(4, 1fr); gap: 0.75rem; margin-top: 0.75rem; }
  #verdict { font-size: 1.1rem; min-height: 1.4rem; }
  .pass { color: #166534; }
  .fail { color: #991b1b; }
  #error { color: #991b1b; min-height: 1.2rem; }
  #missing { white-space: pre-wrap; }
  @media (max-width: 700px) {
    .groups, .nums, .hops { grid-template-columns: 1fr 1fr; }
    #status { margin-left: 0; }
  }
</style>
</head>
<body>
<h1>Audit log demo</h1>
<div class="top">
  <label>count <input id="count" type="number" min="1" value="100"></label>
  <label>threads <input id="threads" type="number" min="1" value="1"></label>
  <label>batch <input id="batch" type="number" min="0" value="0" title="Logs per request. 0 sends them in one batch, except when a per-log fault or a step needs one log at a time."></label>
  <div id="status">status: idle</div>
</div>
<h2>Every Nth problem</h2>
<div id="problems" class="rows"></div>
<button type="button" id="add-problem">Add problem</button>
<h2>After N logs</h2>
<div id="steps" class="rows"></div>
<button type="button" id="add-step">Add step</button>
<button type="button" id="launch" title="Runs the happy path with the problems and steps above.">Launch test</button>
<div class="groups">
  <section id="delivery"><h2>Delivery</h2></section>
  <section id="failover"><h2>Failover</h2></section>
  <section id="loss"><h2>Loss</h2></section>
</div>
<p id="error"></p>
<div class="nums">
  <div><span>Sent</span><strong id="sent">0</strong></div>
  <div><span>Received</span><strong id="received">0</strong></div>
  <div><span>Missing</span><strong id="missing-count">0</strong></div>
  <div><span>Duplicates</span><strong id="duplicates">0</strong></div>
</div>
<div class="hops">
  <div><span>Via main</span><strong id="main">0</strong></div>
  <div><span>Via secondary</span><strong id="secondary">0</strong></div>
  <div><span>Latency avg</span><strong id="avg">0ms</strong></div>
  <div><span>Latency p95</span><strong id="p95">0ms</strong></div>
</div>
<p id="latency-rest"></p>
<p id="verdict"></p>
<p id="expected"></p>
<h2>Missing sequences</h2>
<p id="missing">(none)</p>
<script>
const statusEl = document.querySelector("#status");
const errorEl = document.querySelector("#error");
let timer = 0;
let started = 0;

function ms(n) {
  if (n === undefined || n === null || Number.isNaN(n)) return "0ms";
  return (Math.round(n * 10) / 10) + "ms";
}

function show(data) {
  const by = data.by_collector || {};
  const lat = data.latency_ms || {};
  const missing = Array.isArray(data.missing) ? data.missing : [];
  document.querySelector("#sent").textContent = data.sent || 0;
  document.querySelector("#received").textContent = data.received || 0;
  document.querySelector("#missing-count").textContent = missing.length;
  document.querySelector("#duplicates").textContent = data.duplicates || 0;
  document.querySelector("#main").textContent = by.main || 0;
  document.querySelector("#secondary").textContent = by.secondary || 0;
  document.querySelector("#avg").textContent = ms(lat.avg);
  document.querySelector("#p95").textContent = ms(lat.p95);
  document.querySelector("#latency-rest").textContent = "min " + ms(lat.min) + "    max " + ms(lat.max);
  const shown = missing.slice(0, 50);
  let line = shown.length ? shown.join(", ") : "(none)";
  if (missing.length > 50) line += " … " + missing.length + " total";
  document.querySelector("#missing").textContent = line;
  if (data.status === "drained" && started) {
    const sec = Math.max(0, (Date.now() - started) / 1000);
    statusEl.textContent = "status: drained, " + sec.toFixed(1) + "s";
  } else if (data.status === "running") {
    statusEl.textContent = "status: running";
  }
  const verdict = document.querySelector("#verdict");
  verdict.className = "";
  if (data.pass === true) {
    verdict.textContent = "Pass. " + (data.detail || "");
    verdict.className = "pass";
  } else if (data.pass === false) {
    verdict.textContent = "Fail. " + (data.detail || data.error || "");
    verdict.className = "fail";
  } else {
    verdict.textContent = "";
  }
  document.querySelector("#expected").textContent = data.expected || "";
}

async function poll(runID) {
  const resp = await fetch("/api/stats?run_id=" + encodeURIComponent(runID));
  const data = await resp.json();
  show(data);
  if (data.pass === true || data.pass === false) {
    clearInterval(timer);
    timer = 0;
  }
}

const faultChoices = [
  ["none", "none"],
  ["malformed_json", "malformed JSON from app"],
  ["missing_attrs", "missing attributes, dropped at verify"],
  ["bad_timestamp", "bad timestamp from app"],
  ["oversize", "oversize body from app"],
  ["duplicate_id", "duplicate id, counted at sink"],
  ["connection_reset", "connection reset from app"],
  ["bad_signature", "bad signature, rejected at verify"],
  ["bad_in_batch", "bad signature in batch, rejected at verify"],
  ["stall", "stall on sink"],
  ["slow", "slow on sink"]
];
const actionChoices = [
  ["none", "none"],
  ["kill-main", "kill main collector"],
  ["stop-main", "stop main collector"],
  ["start-main", "start main collector"],
  ["kill-secondary", "kill secondary collector"],
  ["stop-secondary", "stop secondary collector"],
  ["start-secondary", "start secondary collector"],
  ["blackhole-main", "blackhole on path to main"],
  ["blackhole-secondary", "blackhole on path to secondary"],
  ["clear-faults", "clear faults on all paths"],
  ["low-bandwidth-main", "low bandwidth on path to main"],
  ["low-bandwidth-secondary", "low bandwidth on path to secondary"],
  ["low-bandwidth-sink", "low bandwidth on path to sink"],
  ["restart-sidecar", "restart sidecar"]
];

function choiceList(choices, selected) {
  return choices.map(function (item) {
    const mark = item[0] === selected ? " selected" : "";
    return "<option value=\"" + item[0] + "\"" + mark + ">" + item[1] + "</option>";
  }).join("");
}

function addRow(parent, kind, selected) {
  const row = document.createElement("div");
  row.className = "row " + kind;
  if (kind === "problem") {
    row.innerHTML = "<label>every <input class=\"every\" type=\"number\" min=\"0\" value=\"0\"></label><label>fault <select class=\"fault\">" + choiceList(faultChoices, selected) + "</select></label><button type=\"button\" class=\"remove\">remove</button>";
  } else {
    row.innerHTML = "<label>after <input class=\"after\" type=\"number\" min=\"0\" value=\"0\"></label><label>action <select class=\"action\">" + choiceList(actionChoices, selected) + "</select></label><button type=\"button\" class=\"remove\">remove</button>";
  }
  row.querySelector(".remove").addEventListener("click", function () { row.remove(); });
  parent.appendChild(row);
}

document.querySelector("#add-problem").addEventListener("click", function () {
  addRow(document.querySelector("#problems"), "problem", "none");
});
document.querySelector("#add-step").addEventListener("click", function () {
  addRow(document.querySelector("#steps"), "step", "none");
});
addRow(document.querySelector("#problems"), "problem", "none");
addRow(document.querySelector("#steps"), "step", "none");
document.querySelector("#launch").addEventListener("click", function () { run("happy"); });

async function run(id) {
  errorEl.textContent = "";
  document.querySelector("#verdict").textContent = "";
  document.querySelector("#expected").textContent = "";
  const injections = [];
  document.querySelectorAll(".problem").forEach(function (row) {
    injections.push({
      every: Number(row.querySelector(".every").value),
      fault: row.querySelector(".fault").value
    });
  });
  const steps = [];
  document.querySelectorAll(".step").forEach(function (row) {
    steps.push({
      after: Number(row.querySelector(".after").value),
      action: row.querySelector(".action").value
    });
  });
  const body = {
    count: Number(document.querySelector("#count").value),
    threads: Number(document.querySelector("#threads").value),
    batch: Number(document.querySelector("#batch").value),
    injections: injections,
    steps: steps
  };
  const resp = await fetch("/api/scenarios/" + id, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
  const data = await resp.json();
  if (!resp.ok) {
    errorEl.textContent = data.error || "request failed";
    return;
  }
  started = Date.now();
  statusEl.textContent = "status: running";
  if (timer) clearInterval(timer);
  poll(data.run_id);
  timer = setInterval(() => poll(data.run_id), 1000);
}

fetch("/api/scenarios").then(r => r.json()).then(list => {
  for (const sc of list) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = sc.label;
    button.addEventListener("click", () => run(sc.id));
    document.querySelector("#" + sc.group).appendChild(button);
  }
});
</script>
</body>
</html>
`)
