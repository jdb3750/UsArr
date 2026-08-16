/* UsArr static mockup behaviour.
   Vanilla JS, no dependencies, no animation library.
   Scope, deliberately narrow: theme toggle, density control, sidebar toggle,
   section/tab switching, the dialog, the states switcher, roving tabindex,
   and the acknowledged-write demonstration on the requests screen.
   Nothing here animates anything on a render path. */
(function () {
  'use strict';

  var root = document.documentElement;

  /* ---- theme ---------------------------------------------------------- */
  function readStored(key) {
    try { return window.localStorage.getItem(key); } catch (e) { return null; }
  }
  function store(key, value) {
    try { window.localStorage.setItem(key, value); } catch (e) { /* private mode */ }
  }

  var storedTheme = readStored('usarr.theme');
  if (storedTheme === 'light' || storedTheme === 'dark') {
    root.setAttribute('data-theme', storedTheme);
  }

  function currentTheme() {
    var explicit = root.getAttribute('data-theme');
    if (explicit) return explicit;
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }

  function paintThemeButton(btn) {
    var dark = currentTheme() === 'dark';
    btn.setAttribute('aria-label', dark ? 'Switch to the light theme' : 'Switch to the dark theme');
    btn.querySelector('use').setAttribute('href', dark ? '#i-sun' : '#i-moon');
  }

  var themeBtn = document.querySelector('[data-act="theme"]');
  if (themeBtn) {
    paintThemeButton(themeBtn);
    themeBtn.addEventListener('click', function () {
      var next = currentTheme() === 'dark' ? 'light' : 'dark';
      root.setAttribute('data-theme', next);
      store('usarr.theme', next);
      paintThemeButton(themeBtn);
    });
  }

  /* ---- density -------------------------------------------------------- */
  var storedDensity = readStored('usarr.density');
  root.setAttribute('data-density', storedDensity || 'compact');

  var densitySel = document.querySelector('[data-act="density"]');
  if (densitySel) {
    densitySel.value = root.getAttribute('data-density');
    densitySel.addEventListener('change', function () {
      root.setAttribute('data-density', densitySel.value);
      store('usarr.density', densitySel.value);
    });
  }

  /* ---- sidebar -------------------------------------------------------- */
  var app = document.querySelector('.app');
  var sideBtn = document.querySelector('[data-act="sidebar"]');
  /* On a phone the sidebar starts closed. Shipping it open is the Sonarr
     #7757 / Prowlarr #2431 failure: the nav covers the thing you came for. */
  if (app && window.matchMedia('(max-width: 900px)').matches) {
    app.setAttribute('data-sidebar', 'collapsed');
    if (sideBtn) {
      sideBtn.setAttribute('aria-expanded', 'false');
      sideBtn.setAttribute('aria-label', 'Show the sidebar');
    }
  }
  if (app && sideBtn) {
    sideBtn.addEventListener('click', function () {
      var collapsed = app.getAttribute('data-sidebar') === 'collapsed';
      app.setAttribute('data-sidebar', collapsed ? 'open' : 'collapsed');
      sideBtn.setAttribute('aria-expanded', collapsed ? 'true' : 'false');
      sideBtn.setAttribute('aria-label', collapsed ? 'Hide the sidebar' : 'Show the sidebar');
    });
  }

  /* ---- states switcher (a mockup affordance, labelled as one) ---------- */
  function applyState(scope, state) {
    scope.setAttribute('data-state', state);
    var blocks = scope.querySelectorAll('[data-when]');
    for (var i = 0; i < blocks.length; i++) {
      var wants = blocks[i].getAttribute('data-when').split(/\s+/);
      blocks[i].hidden = wants.indexOf(state) === -1;
    }
  }

  var stateSelects = document.querySelectorAll('[data-act="state"]');
  for (var s = 0; s < stateSelects.length; s++) {
    (function (sel) {
      var scope = document.getElementById(sel.getAttribute('data-scope'));
      if (!scope) return;
      applyState(scope, sel.value);
      sel.addEventListener('change', function () { applyState(scope, sel.value); });
    })(stateSelects[s]);
  }

  /* ---- generic aria-pressed / aria-current group switching ------------ */
  document.addEventListener('click', function (ev) {
    var seg = ev.target.closest('[data-group]');
    if (!seg) return;
    var group = seg.getAttribute('data-group');
    var peers = document.querySelectorAll('[data-group="' + group + '"]');
    for (var i = 0; i < peers.length; i++) {
      var on = peers[i] === seg;
      if (peers[i].hasAttribute('aria-pressed')) peers[i].setAttribute('aria-pressed', on ? 'true' : 'false');
      if (peers[i].hasAttribute('aria-current')) peers[i].setAttribute('aria-current', on ? 'true' : 'false');
    }
    var panelName = seg.getAttribute('data-panel');
    if (panelName) {
      var panels = document.querySelectorAll('[data-panel-for="' + group + '"]');
      for (var p = 0; p < panels.length; p++) {
        panels[p].hidden = panels[p].getAttribute('data-panel-name') !== panelName;
      }
    }
  });

  /* ---- expandable table rows ------------------------------------------ */
  document.addEventListener('click', function (ev) {
    var trigger = ev.target.closest('[data-act="expand"]');
    if (!trigger) return;
    var target = document.getElementById(trigger.getAttribute('aria-controls'));
    if (!target) return;
    var open = trigger.getAttribute('aria-expanded') === 'true';
    trigger.setAttribute('aria-expanded', open ? 'false' : 'true');
    target.hidden = open;
    trigger.querySelector('use').setAttribute('href', open ? '#i-chevron-right' : '#i-chevron-down');
  });

  /* ---- show advanced --------------------------------------------------- */
  var advBtn = document.querySelector('[data-act="advanced"]');
  if (advBtn) {
    advBtn.addEventListener('click', function () {
      var box = document.getElementById(advBtn.getAttribute('aria-controls'));
      var on = box.getAttribute('data-advanced') === 'on';
      box.setAttribute('data-advanced', on ? 'off' : 'on');
      advBtn.setAttribute('aria-expanded', on ? 'false' : 'true');
      advBtn.textContent = on ? 'Show advanced' : 'Hide advanced';
    });
  }

  /* ---- dirty form: Save reads "No changes" until something changes ----- */
  var form = document.querySelector('[data-act="dirtyform"]');
  if (form) {
    var save = form.querySelector('[data-act="save"]');
    form.addEventListener('input', function () {
      save.disabled = false;
      save.textContent = 'Save';
    });
    form.addEventListener('change', function () {
      save.disabled = false;
      save.textContent = 'Save';
    });
    form.addEventListener('submit', function (ev) {
      ev.preventDefault();
      var summary = document.getElementById('service-errors');
      if (summary) {
        summary.hidden = false;
        summary.focus();
      }
    });
  }

  /* ---- API key must be re-entered when the host changes --------------- */
  var hostField = document.querySelector('[data-act="host"]');
  if (hostField) {
    hostField.addEventListener('input', function () {
      var note = document.getElementById('apikey-reentry');
      var key = document.getElementById('f-apikey');
      if (!note || !key) return;
      note.hidden = false;
      key.value = '';
      key.placeholder = 'Re-enter the API key';
    });
  }

  /* ---- dialog ---------------------------------------------------------- */
  document.addEventListener('click', function (ev) {
    var open = ev.target.closest('[data-act="dialog-open"]');
    if (open) {
      var dlg = document.getElementById(open.getAttribute('aria-controls'));
      if (dlg && dlg.showModal) dlg.showModal();
      return;
    }
    var close = ev.target.closest('[data-act="dialog-close"]');
    if (close) {
      var owner = close.closest('dialog');
      if (owner) owner.close();
    }
  });

  /* ---- roving tabindex: a table is one tab stop, arrows move within --- */
  var grids = document.querySelectorAll('[data-roving]');
  for (var g = 0; g < grids.length; g++) {
    (function (grid) {
      var sel = grid.getAttribute('data-roving');
      /* Reading offsetParent forces a synchronous layout, and this runs once
         per candidate on every arrow key: at 10,000 rows that is 10,000 forced
         reflows per keypress. closest() answers the same question from the DOM
         tree without touching layout. */
      function items() {
        return Array.prototype.filter.call(grid.querySelectorAll(sel), function (el) {
          return !el.hidden && el.closest('[hidden]') === null;
        });
      }
      var initial = Array.prototype.slice.call(grid.querySelectorAll(sel));
      for (var i = 0; i < initial.length; i++) initial[i].tabIndex = i === 0 ? 0 : -1;

      grid.addEventListener('keydown', function (ev) {
        var horizontal = grid.getAttribute('data-roving-axis') === 'horizontal';
        var nextKey = horizontal ? 'ArrowRight' : 'ArrowDown';
        var prevKey = horizontal ? 'ArrowLeft' : 'ArrowUp';
        if (['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight', 'Home', 'End'].indexOf(ev.key) === -1) return;
        var list = items();
        var current = ev.target.closest(sel);
        var idx = list.indexOf(current);
        if (idx === -1) return;
        var next = idx;
        if (ev.key === nextKey) next = Math.min(idx + 1, list.length - 1);
        else if (ev.key === prevKey) next = Math.max(idx - 1, 0);
        else if (ev.key === 'Home') next = 0;
        else if (ev.key === 'End') next = list.length - 1;
        else return;
        ev.preventDefault();
        list[idx].tabIndex = -1;
        list[next].tabIndex = 0;
        list[next].focus();
      });
    })(grids[g]);
  }

  /* ---- acknowledged, not optimistic ------------------------------------
     A grab does not flip the row to "grabbed". It writes a pending row on the
     durable queue and the chip says pending. A failure is a status change on
     that same visible row plus a toast carrying the upstream text verbatim.
     Nothing ever silently reverts. */
  function toast(title, verbatim, onRetry) {
    var host = document.querySelector('.toasts');
    if (!host) return;
    var el = document.createElement('div');
    el.className = 'toast';
    el.setAttribute('role', 'alert');
    el.innerHTML =
      '<div class="toast__title"><svg class="i" aria-hidden="true"><use href="#i-x-circle"/></svg><span data-slot="title"></span></div>' +
      '<div class="toast__body"><div class="verbatim" data-slot="verbatim"></div></div>' +
      '<div class="toast__foot"><button type="button" class="btn btn--sm" data-toast="retry">Retry</button>' +
      '<button type="button" class="btn btn--sm" data-toast="dismiss">Dismiss</button></div>';
    /* Upstream error text is a remote string. It is set as text, never parsed
       as HTML: "render the upstream error verbatim" must not mean innerHTML. */
    el.querySelector('[data-slot="title"]').textContent = title;
    el.querySelector('[data-slot="verbatim"]').textContent = verbatim;
    el.addEventListener('click', function (ev) {
      var act = ev.target.closest('[data-toast]');
      if (!act) return;
      if (act.getAttribute('data-toast') === 'retry' && onRetry) onRetry();
      el.remove();
    });
    host.appendChild(el);
  }

  function setChip(row, kind, text) {
    var cell = row.querySelector('[data-slot="grabstate"]');
    if (!cell) return;
    var chip = document.createElement('span');
    chip.className = 'chip chip--' + kind;
    chip.textContent = text;
    cell.replaceChildren(chip);
  }

  document.addEventListener('click', function (ev) {
    var btn = ev.target.closest('[data-act="grab"]');
    if (!btn) return;
    var row = btn.closest('tr');
    setChip(row, 'pending', 'pending');
    btn.disabled = true;

    /* This row is scripted to fail, so the failure path is demonstrable. */
    if (row.getAttribute('data-demo') !== 'fail') return;
    window.setTimeout(function () {
      setChip(row, 'failed', 'failed: rejected');
      toast(
        'Grab rejected by Prowlarr',
        'POST /api/v1/search 400: {"message":"Release not found in cache. The 30 minute grab window has expired.","description":"IndexerId 7, guid a41c9f2e"}',
        function () {
          setChip(row, 'pending', 'pending');
          btn.disabled = true;
        }
      );
    }, 700);
  });

  /* ---- bulk grab ------------------------------------------------------- */
  var bulk = document.querySelector('[data-act="grab-bulk"]');
  if (bulk) {
    bulk.addEventListener('click', function () {
      var boxes = document.querySelectorAll('[data-act="rowselect"]:checked');
      for (var i = 0; i < boxes.length; i++) {
        var row = boxes[i].closest('tr');
        setChip(row, 'pending', 'pending');
        boxes[i].checked = false;
        row.setAttribute('aria-selected', 'false');
      }
      bulk.disabled = true;
    });
  }

  document.addEventListener('change', function (ev) {
    var box = ev.target.closest('[data-act="rowselect"]');
    if (!box) return;
    box.closest('tr').setAttribute('aria-selected', box.checked ? 'true' : 'false');
    var any = document.querySelectorAll('[data-act="rowselect"]:checked').length;
    var b = document.querySelector('[data-act="grab-bulk"]');
    if (b) b.disabled = any === 0;
  });

  /* ---- "/" focuses the search box, as the top bar hint says ------------ */
  document.addEventListener('keydown', function (ev) {
    if (ev.key !== '/' || ev.metaKey || ev.ctrlKey || ev.altKey) return;
    var tag = document.activeElement && document.activeElement.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
    var box = document.getElementById('topsearch');
    if (!box) return;
    ev.preventDefault();
    box.focus();
    box.select();
  });

  /* ---- prototype.html only: hash routing between the four screens ------ */
  var pages = document.querySelectorAll('[data-page]');
  if (pages.length > 1) {
    var route = function () {
      var name = (window.location.hash || '#home').slice(1);
      var found = false;
      for (var i = 0; i < pages.length; i++) {
        var on = pages[i].getAttribute('data-page') === name;
        pages[i].hidden = !on;
        if (on) found = true;
      }
      if (!found) { pages[0].hidden = false; name = pages[0].getAttribute('data-page'); }
      var links = document.querySelectorAll('.nav__link[data-primary]');
      for (var l = 0; l < links.length; l++) {
        if (links[l].getAttribute('data-route') === name) links[l].setAttribute('aria-current', 'page');
        else links[l].removeAttribute('aria-current');
      }
      window.scrollTo(0, 0);
    };
    window.addEventListener('hashchange', route);
    route();
  }
})();
