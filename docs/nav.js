/* gordma tutorial — shared navigation.
 * Single source of truth for the sidebar tree and prev/next footer links.
 * Each page sets <body data-page="ID">; this script renders the sidebar into
 * #sidebar-nav and the footer pager into #page-nav. */

const TOC = [
  {
    part: "开始",
    pages: [
      { id: "index",    title: "概览",   file: "index.html" },
      { id: "glossary", title: "术语表", file: "glossary.html" },
    ],
  },
  {
    part: "第一部分 · RDMA 网络概念",
    pages: [
      { id: "p1-what",      title: "什么是 RDMA",        file: "p1-what.html" },
      { id: "p1-verbs",     title: "Verbs 对象模型",      file: "p1-verbs.html" },
      { id: "p1-qp",        title: "QP 与状态机",         file: "p1-qp.html" },
      { id: "p1-transport", title: "传输类型 RC / UD",    file: "p1-transport.html" },
      { id: "p1-roce",      title: "RoCE 与 GID",         file: "p1-roce.html" },
      { id: "p1-flow",      title: "一次传输的全流程",     file: "p1-flow.html" },
    ],
  },
  {
    part: "第二部分 · 认识 gordma",
    pages: [
      { id: "p2-overview",  title: "总览与定位",          file: "p2-overview.html" },
      { id: "p2-layers",    title: "两层 API",            file: "p2-layers.html" },
      { id: "p2-lowlevel",  title: "底层 verbs 封装",     file: "p2-lowlevel.html" },
      { id: "p2-rdmanet",   title: "高层 rdmanet",        file: "p2-rdmanet.html" },
      { id: "p2-rawconn",   title: "RawConn 高速读写",     file: "p2-rawconn.html" },
      { id: "p2-connect",   title: "两种连接方式",         file: "p2-connect.html" },
      { id: "p2-perf",      title: "性能选项",            file: "p2-perf.html" },
      { id: "p2-stub",      title: "跨平台 stub",         file: "p2-stub.html" },
    ],
  },
  {
    part: "第三部分 · 工具和示例",
    pages: [
      { id: "p3-cmd",          title: "cmd 性能工具",       file: "p3-cmd.html",          section: "工具" },
      { id: "p3-rdmanet-bw",   title: "go_rdmanet_bw 详解", file: "p3-rdmanet-bw.html",   section: "工具" },
      { id: "p3-flags",        title: "通用命令行参数",     file: "p3-flags.html",        section: "工具" },
      { id: "p3-run-cmd",      title: "运行性能测试",       file: "p3-run-cmd.html",      section: "工具" },
      { id: "p3-examples-run", title: "运行示例",          file: "p3-examples-run.html", section: "示例" },
      { id: "p3-core",         title: "核心功能示例",       file: "p3-core.html",         section: "示例" },
      { id: "p3-scenario",     title: "场景示例",          file: "p3-scenario.html",     section: "示例" },
    ],
  },
];

// Flatten into an ordered list for prev/next.
const FLAT = TOC.flatMap((g) => g.pages);

function currentId() {
  return document.body.getAttribute("data-page") || "index";
}

function buildSidebar() {
  const cur = currentId();
  const nav = document.getElementById("sidebar-nav");
  if (!nav) return;
  let html = "";
  for (const group of TOC) {
    // A group is "open" when it holds the page currently being viewed.
    const isOpen = group.pages.some((p) => p.id === cur);
    const hasSub = group.pages.length > 1;
    const openCls = isOpen ? " open" : "";
    html += `<div class="nav-group${openCls}">`;
    // The part label doubles as a link to its first page. A caret marks
    // collapsible parts; only the open part reveals its sub-items.
    const first = group.pages[0];
    const caret = hasSub ? `<span class="caret">›</span>` : "";
    const activeTop = isOpen && !hasSub ? " active" : "";
    html += `<a class="nav-part${activeTop}" href="${first.file}">${caret}<span>${group.part}</span></a>`;
    if (hasSub) {
      html += `<div class="sub">`;
      let lastSection = null;
      for (const p of group.pages) {
        // Optional second-level grouping within a part (e.g. 工具 / 示例).
        if (p.section && p.section !== lastSection) {
          html += `<div class="sub-section">${p.section}</div>`;
          lastSection = p.section;
        }
        const indent = p.section ? " sub-item" : "";
        const active = p.id === cur ? ` class="active${indent}"` : (indent ? ` class="${indent.trim()}"` : "");
        html += `<a href="${p.file}"${active}>${p.title}</a>`;
      }
      html += `</div>`;
    }
    html += `</div>`;
  }
  nav.innerHTML = html;
}

function buildPager() {
  const cur = currentId();
  const idx = FLAT.findIndex((p) => p.id === cur);
  const pager = document.getElementById("page-nav");
  if (!pager || idx < 0) return;
  const prev = idx > 0 ? FLAT[idx - 1] : null;
  const next = idx < FLAT.length - 1 ? FLAT[idx + 1] : null;
  let html = "";
  html += prev
    ? `<a href="${prev.file}"><div class="dir">← 上一页</div><div class="ttl">${prev.title}</div></a>`
    : `<span></span>`;
  html += next
    ? `<a class="next" href="${next.file}"><div class="dir">下一页 →</div><div class="ttl">${next.title}</div></a>`
    : `<span></span>`;
  pager.innerHTML = html;
}

document.addEventListener("DOMContentLoaded", () => {
  buildSidebar();
  buildPager();
  loadLucideIcons();
});

// Render the Lucide icons used in tiles (<i data-lucide="...">). The UMD
// bundle is vendored locally (lucide.min.js) so no network access is needed.
function loadLucideIcons() {
  const render = () => {
    if (window.lucide && typeof window.lucide.createIcons === "function") {
      window.lucide.createIcons();
    }
  };
  if (window.lucide) {
    render();
    return;
  }
  const s = document.createElement("script");
  s.src = "lucide.min.js";
  s.onload = render;
  document.head.appendChild(s);
}
