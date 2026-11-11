"use strict";
const $ = (id) => document.getElementById(id);
let me = null;
let timer = null;

// Every call goes to our own server; the browser adds the session cookie by itself.
async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = res.status === 204 ? null : await res.json().catch(() => null);
  if (res.status === 401 && path !== "/api/login") { showLogin(); }
  if (!res.ok) { throw new Error((data && data.error) || "request failed (" + res.status + ")"); }
  return data;
}

function showLogin() {
  me = null;
  clearInterval(timer);
  $("app").hidden = true; $("who").hidden = true; $("login").hidden = false;
}

function showApp(user) {
  me = user;
  $("login").hidden = true; $("app").hidden = false; $("who").hidden = false;
  $("who-name").textContent = user.username + " (" + user.role + ")";
  // Show only what this role may do. The server enforces it too.
  $("receive").hidden = !(user.role === "receiver" || user.role === "admin");
  $("pay").hidden = !(user.role === "payer" || user.role === "admin");
  $("admin").hidden = user.role !== "admin";
  refresh();
  clearInterval(timer);
  timer = setInterval(refresh, 2000); // polling: ask the server every two seconds
}

function cell(text, cls) {
  const td = document.createElement("td");
  td.textContent = text; // textContent, never innerHTML: data cannot become markup
  if (cls) td.className = cls;
  return td;
}
function pill(status) {
  const td = document.createElement("td");
  const s = document.createElement("span");
  s.className = "pill " + status; s.textContent = status;
  td.appendChild(s);
  return td;
}
function fill(tbody, rows, build, cols) {
  tbody.replaceChildren();
  if (!rows.length) {
    const tr = document.createElement("tr");
    const td = cell("Nothing yet", "empty"); td.colSpan = cols;
    tr.appendChild(td); tbody.appendChild(tr);
    return;
  }
  for (const r of rows) { const tr = document.createElement("tr"); build(tr, r); tbody.appendChild(tr); }
}
const time = (iso) => new Date(iso).toLocaleTimeString();

async function refresh() {
  if (!me) return;
  try {
    const [payments, invoices] = await Promise.all([api("GET", "/api/payments"), api("GET", "/api/invoices")]);
    fill($("payments"), payments, (tr, p) => {
      tr.append(cell(p.id), cell(p.amount_sat), pill(p.status), cell(p.payment_hash, "mono"),
        cell(p.preimage || p.failure_reason || "", "mono"));
    }, 5);
    fill($("invoices"), invoices, (tr, v) => {
      tr.append(cell(v.amount_sat), pill(v.status), cell(v.payment_hash, "mono"),
        cell(v.paid_at ? "paid " + time(v.paid_at) : "expires " + time(v.expires_at)));
    }, 4);
    if (me.role === "admin") {
      const s = await api("GET", "/api/admin/payouts");
      $("pause-state").textContent = s.payouts_paused ? "PAUSED" : "running";
      $("pause-toggle").textContent = s.payouts_paused ? "Resume payouts" : "Pause payouts";
      $("pause-toggle").dataset.paused = s.payouts_paused ? "1" : "";
    }
  } catch (e) { /* a failed poll is retried in two seconds */ }
}

$("login-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  $("login-error").textContent = "";
  try {
    const user = await api("POST", "/api/login", { username: $("username").value, password: $("password").value });
    $("password").value = "";
    showApp(user);
  } catch (e) { $("login-error").textContent = e.message; }
});

$("logout").addEventListener("click", async () => { await api("POST", "/api/logout").catch(() => {}); showLogin(); });

$("invoice-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  $("invoice-error").textContent = "";
  try {
    const inv = await api("POST", "/api/invoices", { amount_sat: Number($("amount").value), memo: $("memo").value });
    $("invoice-text").value = inv.invoice;
    $("new-invoice").hidden = false;
    refresh();
  } catch (e) { $("invoice-error").textContent = e.message; }
});

$("copy").addEventListener("click", () => navigator.clipboard.writeText($("invoice-text").value));

$("pay-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  $("pay-error").textContent = ""; $("pay-note").textContent = "";
  try {
    const p = await api("POST", "/api/payments", { invoice: $("pay-invoice").value.trim() });
    $("pay-note").textContent = "Accepted as payment #" + p.id + ". Watch its status below.";
    $("pay-invoice").value = "";
    refresh();
  } catch (e) { $("pay-error").textContent = e.message; }
});

$("pause-toggle").addEventListener("click", async () => {
  await api("POST", "/api/admin/payouts", { paused: !$("pause-toggle").dataset.paused }).catch(() => {});
  refresh();
});

// On page load: are we already signed in?
api("GET", "/api/me").then(showApp).catch(showLogin);
