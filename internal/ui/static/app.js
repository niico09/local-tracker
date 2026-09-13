// Local Tracker — progressive enhancement. Served from /static/, so the
// templates never need inline script or style (the server CSP forbids both).
// Without JavaScript the app still works; these helpers only smooth it over.
(function () {
  "use strict";

  var THEME_KEY = "lt-theme";

  // The theme is applied before anything else so the document comes up in the
  // right skin. Everything stays CSP-safe: no inline script, no inline style.
  (function initTheme() {
    var mode = "light";
    try {
      if (localStorage.getItem(THEME_KEY) === "dark") mode = "dark";
    } catch (e) {}
    setTheme(mode, false);
  })();

  function setTheme(mode, persist) {
    document.documentElement.setAttribute("data-theme", mode);
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute("content", mode === "dark" ? "#1c1a17" : "#f2efeb");
    if (persist) {
      try {
        localStorage.setItem(THEME_KEY, mode);
      } catch (e) {}
    }
  }

  function wireThemeToggle() {
    var btn = document.getElementById("theme-toggle");
    if (!btn) return;
    btn.addEventListener("click", function () {
      var next = document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
      setTheme(next, true);
    });
  }

  document.addEventListener("DOMContentLoaded", function () {
    paintMeters();
    markActiveNav();
    wireConfirmForms();
    buildCatalogToolbar();
    wireThemeToggle();
  });

  // Meter fills travel as data-pct because inline style attributes are blocked
  // by the server's Content-Security-Policy.
  function paintMeters() {
    var fills = document.querySelectorAll(".meter i[data-pct]");
    for (var i = 0; i < fills.length; i++) {
      var pct = Number(fills[i].getAttribute("data-pct"));
      if (isNaN(pct)) continue;
      pct = Math.max(0, Math.min(100, pct));
      fills[i].style.width = pct + "%";
    }
  }

  function markActiveNav() {
    var path = location.pathname;
    var links = document.querySelectorAll(".nav a[data-nav]");
    for (var i = 0; i < links.length; i++) {
      var target = links[i].getAttribute("data-nav");
      var active = target === "/" ? path === "/" : (path === target || path.indexOf(target + "/") === 0);
      if (active) links[i].classList.add("is-active");
    }
  }

  // Destructive forms carry data-confirm; the message travels in the attribute.
  function wireConfirmForms() {
    document.addEventListener("submit", function (e) {
      var form = e.target;
      if (!form || !form.getAttribute) return;
      var msg = form.getAttribute("data-confirm");
      if (msg && !window.confirm(msg)) e.preventDefault();
    });
  }

  // The catalog toolbar is only built when the list page renders #lt-toolbar,
  // so every control that appears actually filters something.
  function buildCatalogToolbar() {
    var grid = document.getElementById("lt-covers");
    var bar = document.getElementById("lt-toolbar");
    if (!grid || !bar) return;

    var cards = Array.prototype.slice.call(grid.querySelectorAll(".item-card"));
    var serverOrder = cards.slice(); // repository order (title, A-Z)
    var state = { kind: "all", q: "", sort: "az" };

    var kinds = [
      { v: "all", t: "Todo" },
      { v: "series", t: "Series" },
      { v: "movie", t: "Películas" },
      { v: "book", t: "Libros" },
      { v: "course", t: "Cursos" }
    ];

    var seg = document.createElement("div");
    seg.className = "seg";
    seg.setAttribute("role", "group");
    seg.setAttribute("aria-label", "Filtrar por tipo");
    var buttons = {};
    kinds.forEach(function (k) {
      var b = document.createElement("button");
      b.type = "button";
      b.textContent = k.t;
      b.setAttribute("aria-pressed", String(k.v === state.kind));
      b.addEventListener("click", function () {
        state.kind = k.v;
        Object.keys(buttons).forEach(function (key) {
          buttons[key].setAttribute("aria-pressed", String(key === state.kind));
        });
        apply();
      });
      buttons[k.v] = b;
      seg.appendChild(b);
    });

    var search = document.createElement("input");
    search.type = "search";
    search.className = "input input-search";
    search.placeholder = "Buscar en el catálogo…";
    search.setAttribute("aria-label", "Buscar en el catálogo");
    search.addEventListener("input", function () {
      state.q = search.value.trim().toLowerCase();
      apply();
    });

    var sort = document.createElement("select");
    sort.className = "input";
    sort.setAttribute("aria-label", "Ordenar");
    [["az", "A–Z"], ["recent", "Recientes"]].forEach(function (o) {
      var opt = document.createElement("option");
      opt.value = o[0];
      opt.textContent = o[1];
      sort.appendChild(opt);
    });
    sort.addEventListener("change", function () {
      state.sort = sort.value;
      resort();
      apply();
    });

    var count = document.createElement("span");
    count.className = "count-note";

    var left = document.createElement("div");
    left.className = "toolbar-group";
    left.appendChild(seg);
    var right = document.createElement("div");
    right.className = "toolbar-group";
    right.appendChild(search);
    right.appendChild(sort);
    right.appendChild(count);
    bar.appendChild(left);
    bar.appendChild(right);

    var noRes = document.createElement("div");
    noRes.className = "empty";
    noRes.hidden = true;
    var nh = document.createElement("h3");
    nh.textContent = "Sin resultados";
    var np = document.createElement("p");
    np.textContent = "Ningún título coincide con el filtro.";
    var nb = document.createElement("button");
    nb.type = "button";
    nb.className = "btn btn-secondary";
    nb.textContent = "Limpiar filtros";
    nb.addEventListener("click", function () {
      state.kind = "all";
      state.q = "";
      search.value = "";
      Object.keys(buttons).forEach(function (key) {
        buttons[key].setAttribute("aria-pressed", String(key === "all"));
      });
      apply();
    });
    noRes.appendChild(nh);
    noRes.appendChild(np);
    noRes.appendChild(nb);
    grid.parentNode.insertBefore(noRes, grid.nextSibling);

    function resort() {
      var arr;
      if (state.sort === "recent") {
        arr = cards.slice().sort(function (a, b) {
          return Number(b.getAttribute("data-created")) - Number(a.getAttribute("data-created"));
        });
      } else {
        arr = serverOrder.slice();
      }
      arr.forEach(function (card) { grid.appendChild(card); });
    }

    function apply() {
      var visible = 0;
      cards.forEach(function (card) {
        var okKind = state.kind === "all" || card.getAttribute("data-kind") === state.kind;
        var title = card.getAttribute("data-title") || "";
        var okQ = !state.q || title.toLowerCase().indexOf(state.q) !== -1;
        var show = okKind && okQ;
        card.hidden = !show;
        if (show) visible++;
      });
      count.textContent = visible + " de " + cards.length;
      noRes.hidden = visible !== 0;
    }

    bar.hidden = false;
    apply();
  }
})();
