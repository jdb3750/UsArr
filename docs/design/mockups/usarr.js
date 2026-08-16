/* UsArr static mockup behaviour.
   Vanilla JS, no dependencies, no animation library.
   Scope, deliberately narrow: theme toggle, density control, sidebar toggle,
   the library scope chip, section/tab switching, the dialog, the states
   switcher, roving tabindex, the dominant_color contrast rule, and the
   acknowledged-write demonstration on the requests screen.
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

  /* ---- single-character shortcuts, and the switch that turns them off ---
     WCAG 2.2 SC 2.1.4 (Level A) requires that a shortcut made only of letter,
     punctuation, number or symbol characters can be turned off, remapped, or
     be active only on focus. "It also has a visible mouse equivalent" is not
     one of the three: the criterion exists for speech-input users, whose
     dictation is typed into the page, and for anyone with a tremor. So there
     is a switch, it is honoured here, and it is discoverable from the shortcut
     sheet the "?" key opens. */
  var shortcutsOn = readStored('usarr.shortcuts') !== 'off';
  var shortcutToggle = document.querySelector('[data-act="shortcuts"]');
  if (shortcutToggle) {
    shortcutToggle.checked = shortcutsOn;
    shortcutToggle.addEventListener('change', function () {
      shortcutsOn = shortcutToggle.checked;
      store('usarr.shortcuts', shortcutsOn ? 'on' : 'off');
    });
  }

  /* A single-character shortcut must never fire while the user is typing, and
     "the active element is an INPUT" is not the same test: a caret inside a
     contenteditable, or inside a wrapper element within one, is also typing. */
  function typing() {
    var el = document.activeElement;
    if (!el) return false;
    if (el.isContentEditable) return true;
    return !!(el.closest && el.closest('input, select, textarea, [contenteditable]'));
  }
  function shortcutBlocked(ev) {
    return !shortcutsOn || ev.metaKey || ev.ctrlKey || ev.altKey || typing();
  }

  /* ---- library scope chip ---------------------------------------------
     Navidrome's LibrarySelector: a multi-select filter, not a mode. The chip
     always states the current scope in words, so a control that hides content
     can never be silent about what it hid -- and the label is a live region,
     because otherwise it states it only to people who can see it.

     The checkboxes are native, so Space toggles and Tab traverses for free.
     Arrow-key roving and Escape-to-close are NOT free: native checkboxes are
     Tab-traversed, not arrow-navigable (only radios in a group are), and Esc
     is popover behaviour rather than checkbox behaviour. Those two are the
     behaviours this handler adds. */
  var scopeBtn = document.querySelector('[data-act="scope"]');
  if (scopeBtn) {
    var scopePop = document.getElementById(scopeBtn.getAttribute('aria-controls'));
    var scopeLabel = scopeBtn.querySelector('[data-slot="scope-label"]');
    var scopeUrl = scopePop.querySelector('[data-slot="scope-url"]');
    var libBoxes = scopePop.querySelectorAll('[data-act="scope-lib"]');
    var allBox = scopePop.querySelector('[data-act="scope-all"]');

    function paintScope() {
      var total = libBoxes.length;
      var on = 0;
      var slugs = [];
      for (var i = 0; i < libBoxes.length; i++) {
        if (!libBoxes[i].checked) continue;
        on++;
        /* The slug is stored on the library, never derived from the rendered
           label: the name is user-editable and the URL is durable state, so
           slugifying the label would change a permalink when a library is
           renamed. */
        slugs.push(libBoxes[i].getAttribute('data-slug'));
      }
      allBox.checked = on === total;
      allBox.indeterminate = on > 0 && on < total;
      if (on === 0) scopeLabel.textContent = 'None (0 of ' + total + ')';
      else if (on === total) scopeLabel.textContent = 'All libraries (' + total + ')';
      else scopeLabel.textContent = on + ' of ' + total + ' libraries';
      scopeUrl.textContent = on === total ? '?lib=all' : '?lib=' + (slugs.join(',') || 'none');
    }

    function openScope(open) {
      scopePop.hidden = !open;
      scopeBtn.setAttribute('aria-expanded', open ? 'true' : 'false');
    }

    scopeBtn.addEventListener('click', function () {
      openScope(scopePop.hidden);
    });
    scopePop.addEventListener('change', function (ev) {
      if (ev.target === allBox) {
        for (var i = 0; i < libBoxes.length; i++) libBoxes[i].checked = allBox.checked;
      }
      paintScope();
    });
    /* Tabbing past the last checkbox used to leave the popover open with focus
       on a nav link behind it. */
    document.querySelector('.scope').addEventListener('focusout', function (ev) {
      if (scopePop.hidden) return;
      if (ev.relatedTarget && ev.relatedTarget.closest('.scope')) return;
      openScope(false);
    });
    document.addEventListener('keydown', function (ev) {
      if (ev.key === 'Escape' && !scopePop.hidden) { openScope(false); scopeBtn.focus(); }
      /* "l" opens the scope, matching "/" for search. */
      if (ev.key !== 'l' || shortcutBlocked(ev)) return;
      /* Below 900px the sidebar is display:none, so focusing into the popover
         would silently do nothing and the key would read as broken rather than
         absent. Open the drawer first. */
      if (!scopeBtn.offsetParent) {
        if (!app || !sideBtn) return;
        ev.preventDefault();
        sideBtn.click();
      } else {
        ev.preventDefault();
      }
      openScope(true);
      allBox.focus();
    });
    document.addEventListener('click', function (ev) {
      if (scopePop.hidden) return;
      if (ev.target.closest('.scope')) return;
      openScope(false);
    });
    paintScope();
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

  /* ---- roving tabindex: a list is one tab stop, arrows move within -----

     data-roving is on lists of *data*. It is deliberately not on the three
     grids whose rows contain form controls -- the library proposals, the
     catalogue sources, the release results with their select checkboxes.
     A form laid out in a grid is a form, and its natural tab order is the
     correct one; imposing a roving model on it is how the Kind select on the
     Libraries screen became keyboard-inoperable.

     Three defects fixed here, all in the same block:

     1. The handler used to fire for anything focused inside a row, so
        ArrowUp/ArrowDown/Home/End were swallowed inside every text input and
        every select. WCAG 2.1.1, Level A. It now bails before the key switch
        when the event starts in a form control, and Home/End are only handled
        when the row itself is the target.

     2. items() was a full querySelectorAll plus an ancestor walk per row, per
        keypress: measured 9.7ms at 5,000 rows and 55ms at 25,000, i.e. 2.25
        seconds of main thread for one second of held ArrowDown. An arrow key
        needs the adjacent row, not the whole list, so it walks siblings and is
        O(1) amortised. The full scan survives only for Home/End, which do not
        repeat.

     3. The roving row was an *extra* tab stop on top of the links and buttons
        already inside every row -- eight stops on the Libraries table, not
        one -- and the assignment ran once at init, so any row appended by a
        "Load more" either arrived as a ninth tab stop or with no tabindex at
        all. Row-internal controls are now -1 and reachable with Left/Right,
        which is the standard grid model, and the assignment is idempotent and
        re-applied to anything appended. */
  var FOCUSABLE = 'a[href], button, input, select, textarea, [tabindex]';
  function isFormTarget(el) {
    return !!(el && el.closest && el.closest('input, select, textarea, [contenteditable]'));
  }
  function visible(el) {
    return !el.hidden && el.closest('[hidden]') === null;
  }

  var grids = document.querySelectorAll('[data-roving]');
  for (var g = 0; g < grids.length; g++) {
    (function (grid) {
      var sel = grid.getAttribute('data-roving');
      var horizontal = grid.getAttribute('data-roving-axis') === 'horizontal';
      var nextKey = horizontal ? 'ArrowRight' : 'ArrowDown';
      var prevKey = horizontal ? 'ArrowLeft' : 'ArrowUp';

      /* Idempotent, and it keeps the roved position rather than snapping back
         to row one every time something is appended. */
      function refresh() {
        var all = grid.querySelectorAll(sel);
        var chosen = null;
        var i;
        for (i = 0; i < all.length; i++) {
          if (all[i].tabIndex === 0 && visible(all[i])) { chosen = all[i]; break; }
        }
        if (!chosen) {
          for (i = 0; i < all.length; i++) {
            if (visible(all[i])) { chosen = all[i]; break; }
          }
        }
        for (i = 0; i < all.length; i++) {
          all[i].tabIndex = all[i] === chosen ? 0 : -1;
          var inner = all[i].querySelectorAll(FOCUSABLE);
          for (var k = 0; k < inner.length; k++) inner[k].tabIndex = -1;
        }
      }
      refresh();

      /* "Load more" appends rows, and ADR-0029 makes that the primary
         interaction, so the invariant has to survive an append rather than
         being established once at init. */
      if (window.MutationObserver) {
        new window.MutationObserver(function (records) {
          for (var r = 0; r < records.length; r++) {
            if (records[r].addedNodes.length) { refresh(); return; }
          }
        }).observe(grid, { childList: true, subtree: true });
      }

      function step(from, dir) {
        var el = from;
        do {
          el = el[dir === 1 ? 'nextElementSibling' : 'previousElementSibling'];
          if (el && el.matches(sel) && visible(el)) return el;
        } while (el);
        return null;
      }
      function move(from, to) {
        if (!to) return;
        from.tabIndex = -1;
        to.tabIndex = 0;
        to.focus();
      }

      grid.addEventListener('keydown', function (ev) {
        if (['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight', 'Home', 'End', 'Escape'].indexOf(ev.key) === -1) return;
        /* A caret in a text field owns its own arrow keys, Home and End. */
        if (isFormTarget(ev.target)) return;
        var current = ev.target.closest(sel);
        if (!current) return;

        /* Left/Right inside a vertical list walks the row's own controls, so
           the row is one tab stop and its actions are still reachable. */
        if (!horizontal && (ev.key === 'ArrowRight' || ev.key === 'ArrowLeft')) {
          var controls = Array.prototype.filter.call(current.querySelectorAll(FOCUSABLE), visible);
          if (!controls.length) return;
          var at = controls.indexOf(ev.target);
          var to = ev.key === 'ArrowRight' ? at + 1 : at - 1;
          ev.preventDefault();
          if (to < 0) { current.focus(); return; }
          if (to >= controls.length) return;
          controls[to].focus();
          return;
        }
        if (!horizontal && ev.key === 'Escape' && ev.target !== current) {
          ev.preventDefault();
          current.focus();
          return;
        }
        if (ev.key === 'Escape') return;

        if (ev.key === 'Home' || ev.key === 'End') {
          /* Only from the row itself: Home in a caret means column zero. */
          if (ev.target !== current) return;
          var all = Array.prototype.filter.call(grid.querySelectorAll(sel), visible);
          if (!all.length) return;
          ev.preventDefault();
          move(current, ev.key === 'Home' ? all[0] : all[all.length - 1]);
          return;
        }
        if (ev.key !== nextKey && ev.key !== prevKey) return;
        ev.preventDefault();
        move(current, step(current, ev.key === nextKey ? 1 : -1));
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

  /* ARCHITECTURE 8.5 fixes the grab confirmation literally: "Sent to <download
     client>. UsArr does not import downloads.", naming the watched folder when
     a library-bearing service is configured. It is a sentence, not a chip, so
     it renders as the row's sub-line beside the chip. */
  function setChip(row, kind, text, note) {
    var cell = row.querySelector('[data-slot="grabstate"]');
    if (!cell) return;
    var chip = document.createElement('span');
    chip.className = 'chip chip--' + kind;
    chip.textContent = text;
    cell.replaceChildren(chip);
    if (note) {
      var sub = document.createElement('div');
      sub.className = 'cell-sub';
      sub.textContent = note;
      cell.appendChild(sub);
    }
  }

  document.addEventListener('click', function (ev) {
    var btn = ev.target.closest('[data-act="grab"]');
    if (!btn) return;
    var row = btn.closest('tr');
    setChip(row, 'pending', 'pending');
    btn.disabled = true;

    /* This row is scripted to fail, so the failure path is demonstrable. */
    if (row.getAttribute('data-demo') !== 'fail') {
      var table = row.closest('table');
      var watched = (table && table.getAttribute('data-watched-default')) || '';
      window.setTimeout(function () {
        setChip(row, 'done', 'grabbed · sent to qBittorrent',
          ('UsArr does not import downloads. ' + watched).trim());
      }, 700);
      return;
    }
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

  /* ---- bulk grab -------------------------------------------------------
     The selected count is a counter, not a query. It used to be
     `document.querySelectorAll('[data-act="rowselect"]:checked').length` on
     every single change event, unscoped: 5.25ms at 5,000 rows and 32ms at
     25,000, which makes selecting a hundred-row range O(n^2) and costs 3.2
     seconds of blocked main thread on a desktop. One integer instead. */
  var selectedRows = 0;
  function paintBulk() {
    var b = document.querySelector('[data-act="grab-bulk"]');
    if (b) b.disabled = selectedRows === 0;
  }

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
      selectedRows = 0;
      paintBulk();
      bulk.disabled = true;
    });
  }

  document.addEventListener('change', function (ev) {
    var box = ev.target.closest('[data-act="rowselect"]');
    if (!box) return;
    box.closest('tr').setAttribute('aria-selected', box.checked ? 'true' : 'false');
    selectedRows += box.checked ? 1 : -1;
    if (selectedRows < 0) selectedRows = 0;
    paintBulk();
  });

  /* ---- "/" focuses the search box, as the top bar hint says ------------ */
  document.addEventListener('keydown', function (ev) {
    if (ev.key !== '/' || shortcutBlocked(ev)) return;
    var box = document.getElementById('topsearch');
    if (!box) return;
    ev.preventDefault();
    box.focus();
    box.select();
  });

  /* ---- dominant_color: the one colour in this design that is data ------
     ARCHITECTURE 4.4.1 computes dominant_color as one average over the 92px
     art fetch, so it is whatever the cover happens to be. Nothing constrains
     it, and a mid-luminance fill puts both black and white near 3.5:1 -- the
     shipped #7d6a4f swatch measured 3.57:1 for a 12px semibold title, which is
     normal text under WCAG (large is >= 18.66px bold or >= 24px), so 4.5:1
     applies. This is the rule, run where the pipeline would run it:

       pick the foreground with the better ratio, then, if it still misses the
       floor, move the *fill* until it clears. The fill is decoration and the
       title is content, so the fill is what gives way.

     Nothing here is a render-path cost in production: it is computed once per
     cover at import and stored beside dominant_color. It runs in the browser
     here only because the mockup has no import step. */
  function srgbToLin(c) {
    c = c / 255;
    return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  }
  function lum(rgb) {
    return 0.2126 * srgbToLin(rgb[0]) + 0.7152 * srgbToLin(rgb[1]) + 0.0722 * srgbToLin(rgb[2]);
  }
  function ratio(a, b) {
    var la = lum(a), lb = lum(b);
    return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
  }
  function hexToRgb(h) {
    h = h.trim().replace('#', '');
    if (h.length === 3) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2];
    return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
  }
  function rgbToHex(c) {
    return '#' + c.map(function (v) {
      var s = Math.max(0, Math.min(255, Math.round(v))).toString(16);
      return s.length === 1 ? '0' + s : s;
    }).join('');
  }
  function mix(c, towards, t) {
    return [c[0] + (towards[0] - c[0]) * t, c[1] + (towards[1] - c[1]) * t, c[2] + (towards[2] - c[2]) * t];
  }

  var DC_FLOOR = 4.5;
  function constrainDominant(hex) {
    var fill = hexToRgb(hex);
    /* The two theme text tokens, not the pure poles: the ramp is warm and
       near-dark / near-light by rule, and the ratio has to be computable from
       the tokens that actually ship. */
    var dark = hexToRgb('#16130e');
    var light = hexToRgb('#f7f5f1');
    var useDark = ratio(fill, dark) >= ratio(fill, light);
    var fg = useDark ? dark : light;
    /* Push the fill away from the text until the floor clears. 24 steps of 4%
       always terminates: at t = 1 the fill is the text colour's opposite pole. */
    var pole = useDark ? light : dark;
    for (var i = 0; i < 24 && ratio(fill, fg) < DC_FLOOR; i++) {
      fill = mix(fill, pole, 0.04);
    }
    return { dc: rgbToHex(fill), fg: rgbToHex(fg), ratio: ratio(fill, fg) };
  }

  var arts = document.querySelectorAll('[data-dc]');
  for (var a = 0; a < arts.length; a++) {
    var picked = constrainDominant(arts[a].getAttribute('data-dc'));
    arts[a].style.setProperty('--dc', picked.dc);
    arts[a].style.setProperty('--dc-fg', picked.fg);
    arts[a].setAttribute('data-dc-ratio', picked.ratio.toFixed(2));
  }

  /* ---- prototype.html only: hash routing between the five screens ------ */
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
