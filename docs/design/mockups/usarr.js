/* UsArr static mockup behaviour.
   Vanilla JS, no dependencies, no animation library.
   Scope, deliberately narrow: theme toggle, density control, sidebar toggle,
   the library scope chip, section/tab switching, the dialog, the states
   switcher, roving tabindex, the dominant_color placeholder fill, and the
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

  /* ---- the install switcher (a mockup affordance, labelled as one) ------
     These screens are drawn over two different installs, because a design
     that is only ever drawn over one cannot be judged on the thing UsArr
     claims -- that every feature degrades honestly when a service is absent.

       full  Sonarr, Radarr, Prowlarr, Navidrome, Audiobookshelf and Kavita.
             All six media types have a catalogue source. This is the DEFAULT,
             because six populated types is what the layout has to survive.
       v01   Kavita and Prowlarr, which is what this install DRAWS. It was
             drawn as every service v0.1 connects (ADR-0041); ADR-0052 has
             since made v0.1's catalogue source BOOKORBIT, on the owner's
             decision to sunset Kavita, so the name here is one milestone
             stale and the re-draw is owed. What it changes is the name and
             not the shape: ARCHITECTURE 16.1 gives BookOrbit the same three
             media types Kavita had -- books, comics and manga -- so ebooks
             and comics still have a source; movies, TV, music and audiobooks
             still do not, and each still says which service will populate it
             and when -- Radarr and Sonarr at v0.2 (ADR-0045), Navidrome and
             Audiobookshelf after v0.1.

     The two installs also demonstrate the OPPOSITE identity tiers, and that is
     the sharpest thing the switcher shows. The v0.1 install's one source as
     drawn is a free Kavita, which returns null aniListId, malId and
     comicVineId, so "matched by title" is the ordinary case there rather than
     an exception. That is MEASURED for free Kavita (ADR-0035 section 1) and
     it is why the null-identifier screens exist; it is NOT re-measured for
     BookOrbit, whose own tier ADR-0052 read off source as mixed -- ComicVine
     ships as a metadata provider, and mangabaka/anilist/myanimelist return
     zero hits repo-wide. So read this as the reason the tier is drawn at all,
     not as a claim about what v0.1 will return once its adapter exists. The
     *Arrs that carry TMDB and TVDB ids are on the full stack and arrive in
     v0.2.

     The v0.1 numbers are DERIVED from the full-stack ones by removing what
     the absent services contributed -- they are not a second invented data
     set, so the two installs reconcile against each other row by row. Ebooks
     is the row where that is arithmetic rather than presence: the library is
     bound to Audiobookshelf and Kavita on the full stack, so 2,266 books
     becomes Kavita's 424 here rather than disappearing.

     `full` is deliberately NOT persisted. The default view has to be the full
     stack on every load, and a switcher that remembers "v0.1" from a previous
     visit would show a reviewer a different screen from the one being
     discussed.

     The visibility rule is one line, and the invariant it rests on is that
     `data-inst` and `data-when` NEVER appear on the same element: `applyState`
     owns `hidden` on the first, this owns it on the second, and an element
     carrying both would have two writers and one attribute. They compose by
     nesting instead, which is what check.mjs asserts. */
  var install = 'full';
  var repaintScope = null;

  function paintInstall() {
    var els = document.querySelectorAll('[data-inst]');
    for (var i = 0; i < els.length; i++) {
      els[i].hidden = els[i].getAttribute('data-inst').split(/\s+/).indexOf(install) === -1;
    }
    root.setAttribute('data-install', install);
    /* The scope chip's library list is install-scoped, so its total, its
       label and the ?lib= string all move with the switch. */
    if (repaintScope) repaintScope();
    /* A list this just revealed needs its tab stop assigned. */
    if (typeof refreshRoving === 'function') refreshRoving();
  }

  var installSel = document.querySelector('[data-act="install"]');
  if (installSel) {
    installSel.value = install;
    installSel.addEventListener('change', function () {
      install = installSel.value;
      paintInstall();
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
  /* The switch appears twice: in Settings, where it is configured, and in the
     shortcut sheet, where it is discovered. They are one setting, so they are
     painted together. */
  var shortcutToggles = document.querySelectorAll('[data-act="shortcuts"]');
  for (var st = 0; st < shortcutToggles.length; st++) {
    shortcutToggles[st].checked = shortcutsOn;
    shortcutToggles[st].addEventListener('change', function (ev) {
      shortcutsOn = ev.target.checked;
      store('usarr.shortcuts', shortcutsOn ? 'on' : 'off');
      for (var i = 0; i < shortcutToggles.length; i++) shortcutToggles[i].checked = shortcutsOn;
    });
  }

  /* A single-character shortcut must never fire while the user is typing, and
     "the active element is an INPUT" is not the same test: a caret inside a
     contenteditable, or inside a wrapper element within one, is also typing.

     The guard used to be `input, select, textarea, [contenteditable]` flat,
     which was simultaneously too broad and too narrow. Too broad: a row-select
     checkbox is an <input> that consumes no text, so focusing one silently
     killed "/" -- the key the top bar advertises. Too narrow: a <button> is
     none of those, so "l" fired with focus on "Add library" and opened the
     scope popover from the other side of the screen, which is the precise bug
     the accessibility floor names as its motivating example. Checkboxes,
     radios and push buttons are excluded from the typing test and handled by
     the navigation test instead. */
  var TYPING_SEL = 'select, textarea, [contenteditable], ' +
    'input:not([type=checkbox]):not([type=radio]):not([type=button]):not([type=submit])';
  function typing() {
    var el = document.activeElement;
    if (!el) return false;
    if (el.isContentEditable) return true;
    return !!(el.closest && el.closest(TYPING_SEL));
  }
  /* A letter shortcut that navigates or opens a layer must not fire while the
     focus is on something the same letter could be operating. */
  function onControl() {
    var el = document.activeElement;
    return !!(el && el.matches && el.matches('button, [role=button], a[href]'));
  }
  function shortcutBlocked(ev) {
    return !shortcutsOn || ev.metaKey || ev.ctrlKey || ev.altKey || typing();
  }

  /* The states switcher is a mockup affordance, but two real behaviours below
     need to drive it: emptying the library scope has to land on the scope-empty
     state, and restoring the scope has to come back. */
  function setPageState(name) {
    var sels = document.querySelectorAll('[data-act="state"]');
    for (var i = 0; i < sels.length; i++) {
      var sel = sels[i];
      var page = document.getElementById(sel.getAttribute('data-scope'));
      if (!page || page.hidden) continue;
      var has = false;
      for (var o = 0; o < sel.options.length; o++) {
        if (sel.options[o].value === name) has = true;
      }
      if (!has || sel.value === name) continue;
      if (name !== 'scope-empty') sel.setAttribute('data-prev', sel.value);
      sel.value = name;
      sel.dispatchEvent(new Event('change'));
    }
  }
  function restorePageState() {
    var sels = document.querySelectorAll('[data-act="state"]');
    for (var i = 0; i < sels.length; i++) {
      var sel = sels[i];
      if (sel.value !== 'scope-empty') continue;
      sel.value = sel.getAttribute('data-prev') || sel.options[0].value;
      sel.dispatchEvent(new Event('change'));
    }
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

    var topScope = document.querySelector('[data-slot="topscope"]');
    var scopeLive = document.querySelector('[data-slot="scope-live"]');

    /* The library list is install-scoped: the v0.1 install has four libraries,
       not eight, because the four bound only to Sonarr, Radarr, Navidrome and
       Audiobookshelf do not exist on it.
       Every count the chip states is therefore computed over the boxes this
       install actually has, never over the markup's total -- otherwise the
       chip would read "All libraries (8)" over an install with four. */
    /* The test is [data-inst][hidden] and not a bare [hidden], because the
       popover these boxes live in is itself [hidden] whenever it is closed --
       which is nearly always. A bare test read every library as absent on load
       and put the whole mockup into the scope-empty state before a user had
       touched anything. */
    function activeBoxes() {
      var out = [];
      for (var i = 0; i < libBoxes.length; i++) {
        if (libBoxes[i].closest('[data-inst][hidden]')) continue;
        out.push(libBoxes[i]);
      }
      return out;
    }

    function paintScope() {
      var boxes = activeBoxes();
      var total = boxes.length;
      var on = 0;
      var slugs = [];
      var names = [];
      for (var i = 0; i < boxes.length; i++) {
        if (!boxes[i].checked) continue;
        on++;
        /* The slug is stored on the library, never derived from the rendered
           label: the name is user-editable and the URL is durable state, so
           slugifying the label would change a permalink when a library is
           renamed. */
        slugs.push(boxes[i].getAttribute('data-slug'));
        names.push(boxes[i].parentNode.textContent.trim());
      }
      allBox.checked = on === total;
      allBox.indeterminate = on > 0 && on < total;
      /* One grammar, three cases. There used to be three shapes for one
         control -- "All libraries (8)", "7 of 8 libraries", "None (0 of 8)" --
         with the parenthesis meaning something different in the first and the
         third, which is jarring read aloud in sequence. */
      var label;
      if (on === 0) label = 'No libraries selected';
      else if (on === total) label = 'All libraries (' + total + ')';
      else label = on + ' of ' + total + ' libraries';
      scopeLabel.textContent = label;
      scopeUrl.textContent = on === total ? '?lib=all' : '?lib=' + (slugs.join(',') || 'none');

      var scoped = on !== total;
      scopeBtn.setAttribute('data-scoped', scoped ? 'true' : 'false');
      if (topScope) {
        topScope.setAttribute('data-scoped', scoped ? 'true' : 'false');
        topScope.querySelector('[data-slot="topscope-label"]').textContent = label;
      }
      /* The live region is a sibling of the button rather than its own
         accessible name. Inside the button, toggling a checkbox changed the
         focused control's name *and* fired an announcement, so most assistive
         technology said it twice. */
      if (scopeLive) scopeLive.textContent = label + ' in scope.';

      var lines = document.querySelectorAll('[data-slot="scopeline"]');
      for (var l = 0; l < lines.length; l++) {
        lines[l].hidden = !scoped || on === 0;
        var count = lines[l].querySelector('[data-slot="scopeline-count"]');
        var rest = lines[l].querySelector('[data-slot="scopeline-rest"]');
        if (count) count.textContent = on + ' of ' + total + ' libraries';
        if (rest) {
          rest.textContent = ' in scope: ' + names.join(', ') +
            '. Everything in the other ' + (total - on) + ' is hidden.';
        }
      }

      if (on === 0) setPageState('scope-empty');
      else restorePageState();
    }

    function openScope(open) {
      scopePop.hidden = !open;
      scopeBtn.setAttribute('aria-expanded', open ? 'true' : 'false');
    }

    scopeBtn.addEventListener('click', function () {
      openScope(scopePop.hidden);
    });
    repaintScope = paintScope;

    scopePop.addEventListener('change', function (ev) {
      if (ev.target === allBox) {
        var boxes = activeBoxes();
        for (var i = 0; i < boxes.length; i++) boxes[i].checked = allBox.checked;
      }
      paintScope();
    });
    /* Delegated at the document, because "Clear the scope" also appears in the
       scope line on every scoped surface, not only inside the popover. */
    document.addEventListener('click', function (ev) {
      var act = ev.target.closest('[data-scope-act]');
      if (!act) return;
      var want = act.getAttribute('data-scope-act') === 'all';
      var boxes = activeBoxes();
      for (var i = 0; i < boxes.length; i++) boxes[i].checked = want;
      paintScope();
    });
    /* Below 900px the sidebar is display:none, so the chip in it is not a
       control the user can reach. The hoisted copy in the top bar opens the
       drawer and then the popover, which is the same journey the "l" shortcut
       already takes. */
    if (topScope) {
      topScope.addEventListener('click', function (ev) {
        ev.preventDefault();
        if (!scopeBtn.offsetParent && sideBtn) sideBtn.click();
        openScope(true);
        allBox.focus();
      });
    }
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
      if (ev.key !== 'l' || shortcutBlocked(ev) || onControl()) return;
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
    /* prototype.html swaps <main> on a hash change, so the state switcher of
       the newly shown screen has to be brought into line with the scope. */
    window.addEventListener('hashchange', function () { window.setTimeout(paintScope, 0); });
  }

  /* ---- states switcher (a mockup affordance, labelled as one) ---------- */
  function applyState(scope, state) {
    scope.setAttribute('data-state', state);
    var blocks = scope.querySelectorAll('[data-when]');
    for (var i = 0; i < blocks.length; i++) {
      var wants = blocks[i].getAttribute('data-when').split(/\s+/);
      blocks[i].hidden = wants.indexOf(state) === -1;
    }
    /* A list this just revealed needs its tab stop assigned. */
    if (typeof refreshRoving === 'function') refreshRoving();
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
      refreshRoving();
    }
  });

  /* ---- expandable table rows ------------------------------------------
     A row now has two triggers for one detail row: the chevron in the first
     cell, and "Show detail" in the Problem cell, which is where the reader
     actually is when they want it. Both are updated together, because two
     disclosures pointing at one target with independent aria-expanded is a
     state that goes wrong the first time either is used. */
  document.addEventListener('click', function (ev) {
    var trigger = ev.target.closest('[data-act="expand"]');
    if (!trigger) return;
    var id = trigger.getAttribute('aria-controls');
    var target = document.getElementById(id);
    if (!target) return;
    var open = trigger.getAttribute('aria-expanded') === 'true';
    target.hidden = open;
    var peers = document.querySelectorAll('[data-act="expand"][aria-controls="' + id + '"]');
    for (var i = 0; i < peers.length; i++) {
      peers[i].setAttribute('aria-expanded', open ? 'false' : 'true');
      var glyph = peers[i].querySelector('use');
      if (glyph) glyph.setAttribute('href', open ? '#i-chevron-right' : '#i-chevron-down');
      if (peers[i].hasAttribute('data-expand-label')) {
        peers[i].textContent = open ? 'Show detail' : 'Hide detail';
      }
    }
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
  var dialogInvoker = null;
  document.addEventListener('click', function (ev) {
    var open = ev.target.closest('[data-act="dialog-open"]');
    if (open) {
      var dlg = document.getElementById(open.getAttribute('aria-controls'));
      if (dlg && dlg.showModal) { dialogInvoker = open; dlg.showModal(); }
      return;
    }
    var close = ev.target.closest('[data-act="dialog-close"]');
    if (close) {
      var owner = close.closest('dialog');
      if (owner) owner.close();
    }
  });
  var dialogs = document.querySelectorAll('dialog');
  for (var d = 0; d < dialogs.length; d++) {
    /* Focus goes back to the control that opened it. Without this, Escape on
       the add-service dialog left document.activeElement on <body> and the
       button that opened it never got focus back. */
    dialogs[d].addEventListener('close', function () {
      if (dialogInvoker && dialogInvoker.isConnected) dialogInvoker.focus();
      dialogInvoker = null;
    });
    /* Escape on a dialog holding a pasted 32-character API key throws it away
       with no confirmation, and Escape is reflexive -- it is also how you
       dismiss a password-manager popup. One press asks; a second closes. */
    dialogs[d].addEventListener('cancel', function (ev) {
      var dlg = ev.target;
      var key = dlg.querySelector('input[type="password"]');
      var warn = dlg.querySelector('[data-slot="discard-warn"]');
      if (!key || !key.value || !warn || !warn.hidden) return;
      ev.preventDefault();
      warn.hidden = false;
    });
  }

  /* ---- roving tabindex: a list is one tab stop, arrows move within -----

     data-roving is on lists of *data*, and the release tables are now among
     them. They were excluded on the argument that a grid whose rows contain
     form controls is a form and its natural tab order is the correct one --
     which is true of the library proposals and the catalogue sources, where
     every cell is an input, and was false of the release tables, where the
     controls are one checkbox and two actions per row of otherwise pure data.
     The measured cost of the exclusion: 28 individual tab stops inside 10
     rows, no row reachable by Tab at all, so the arrow keys learned on every
     other list did nothing on the one v0.1 screen with a stateful outbound
     action. The original hazard -- arrowing inside a <select> changing its
     value -- is handled by the form-target bail-out below rather than by
     withholding the model.

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
  /* Which controls own their own arrow keys. A text input owns Left/Right and
     Home/End for its caret, a <select> owns Up/Down for its value, and a radio
     owns Up/Down for its group -- so the roving handler must not touch any of
     them, which is what SC 2.1.1 requires and what this bail-out is for.

     A checkbox owns none of them, in any browser. Including it made the
     bail-out too broad in exactly the place it hurt most: on the release
     table every row's first control is a row-select checkbox, so arrowing into
     one left no way to arrow onward to Grab and no Escape to get back out --
     only Tab. Push buttons are excluded for the same reason. */
  function isFormTarget(el) {
    return !!(el && el.closest && el.closest(
      'select, textarea, [contenteditable], ' +
      'input:not([type=checkbox]):not([type=button]):not([type=submit])'));
  }
  function visible(el) {
    return !el.hidden && el.closest('[hidden]') === null;
  }

  var grids = document.querySelectorAll('[data-roving]');
  /* A list that is hidden when the page loads had no visible row to give the
     tab stop to, and nothing re-ran the assignment when the state switcher or
     a route change revealed it -- so six lists across three screens ended up
     with zero tab stops and were unreachable by keyboard entirely. Every
     list's refresh is registered here and re-run whenever visibility can have
     changed. */
  var rovingRefreshers = [];
  function refreshRoving() {
    /* applyState runs at init before this block has been reached, so the
       hoisted var is still undefined on the first pass. */
    if (!rovingRefreshers) return;
    for (var i = 0; i < rovingRefreshers.length; i++) rovingRefreshers[i]();
  }
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
      rovingRefreshers.push(refresh);

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
        /* Escape is handled before the form-control bail-out, and that order
           is the whole point. The bail-out is correct for arrows, Home and End
           -- a caret owns those -- but it was swallowing Escape too, which
           removed the documented way out of a row's controls exactly where it
           is needed: on the release table every row's first control is a
           checkbox, so arrowing into one left no arrow key and no Escape that
           got you back out, only Tab. */
        if (ev.key === 'Escape') {
          var owner = ev.target.closest(sel);
          if (owner && ev.target !== owner) { ev.preventDefault(); owner.focus(); }
          return;
        }
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
  /* One polite region for anything that succeeded. The failure path had a live
     region and the success path had none, so a screen-reader user pressed Grab
     and heard nothing at all -- the one confirmation the architecture fixes
     literally, delivered by the least perceivable mechanism on the page. */
  function announce(text) {
    var region = document.querySelector('[data-slot="announce"]');
    if (region) region.textContent = text;
  }

  /* The alert carries the title and the verbatim text and nothing focusable:
     ARIA's own guidance is that an alert should not contain controls, because
     assistive technology may present the region as a flat announcement and the
     controls then never arrive. Retry and Dismiss sit in a sibling group, and
     focus is moved to the first of them -- which is safe here because the
     error is the direct result of something the user just pressed. */
  function toast(title, verbatim, retryLabel, onRetry) {
    var host = document.querySelector('.toasts');
    if (!host) return;
    var el = document.createElement('div');
    el.className = 'toast';
    el.innerHTML =
      '<div role="alert">' +
      '<div class="toast__title"><svg class="i" aria-hidden="true"><use href="#i-x-circle"/></svg><span data-slot="title"></span></div>' +
      '<div class="toast__body"><div class="verbatim" data-slot="verbatim"></div></div>' +
      '</div>' +
      '<div class="toast__foot" role="group" aria-label="Actions for this error">' +
      '<button type="button" class="btn btn--sm btn--primary" data-toast="retry"></button>' +
      '<button type="button" class="btn btn--sm" data-toast="dismiss">Dismiss</button></div>';
    /* Upstream error text is a remote string. It is set as text, never parsed
       as HTML: "render the upstream error verbatim" must not mean innerHTML. */
    el.querySelector('[data-slot="title"]').textContent = title;
    el.querySelector('[data-slot="verbatim"]').textContent = verbatim;
    el.querySelector('[data-toast="retry"]').textContent = retryLabel;
    el.addEventListener('click', function (ev) {
      var act = ev.target.closest('[data-toast]');
      if (!act) return;
      if (act.getAttribute('data-toast') === 'retry' && onRetry) onRetry();
      el.remove();
    });
    host.appendChild(el);
    el.querySelector('[data-toast="retry"]').focus();
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

  /* The post-grab sentence, per media type, and complete for all six.

     The four sink-less types used to get an honest whole sentence and the two
     that have an *Arr got "UsArr does not import downloads." and nothing else
     -- which a Radarr owner reads as "UsArr doesn't, but obviously Radarr
     does." It will not: the grab went from UsArr to Prowlarr to the download
     client, Radarr never requested that release and has no record of it. The
     failure is silent and cumulative, so the sentence names what will pick the
     file up and what will not, for every type, and the type is a property of
     the table the row is in.

     These strings are ALSO install-scoped, and they are the same strings the
     "Post-grab behaviour by media type" table renders, verbatim. Two reasons
     they cannot be one install-blind map:

       · it named Komga for comics, which ADR-0035 replaced with Kavita. The
         table was updated and this was not, so the same screen stated two
         different comics servers depending on whether you read a row or
         grabbed one.
       · every sentence here names a service. On the v0.1 install Sonarr,
         Radarr, Navidrome and Audiobookshelf are not connected, so "Radarr did
         not request this release" is this mockup asserting a service the
         selected install does not have -- the precise failure the two-install
         switcher exists to expose.

     `both` is not laziness, and it moved. It used to cover movie and series on
     the ground that "Radarr and Sonarr are connected on either install", which
     ADR-0041 falsified; those two fork now. What genuinely does not move is
     comic, because Kavita is connected on BOTH installs and the sentence about
     it is the same one either way. */
  var SINK_NOTE = {
    movie: {
      full: 'UsArr does not import downloads. Radarr did not request this release, so Radarr will not import it either — the file stays in your download client until you move it into /media/movies yourself.',
      v01: 'UsArr does not import downloads, and no connected service catalogues films. Radarr would import only the releases it asked for itself, and it arrives in v0.2; the file stays in your download client either way.'
    },
    series: {
      full: 'UsArr does not import downloads. Sonarr did not request this release, so Sonarr will not import it either — the file stays in your download client until you move it into /media/tv yourself.',
      v01: 'UsArr does not import downloads, and no connected service catalogues series. Sonarr would import only the releases it asked for itself, and it arrives in v0.2; the file stays in your download client either way.'
    },
    music: {
      full: 'UsArr does not import downloads, and no connected service accepts a music request. Navidrome shows what is already inside the folder it scans, so the file stays in your download client until you move it there.',
      v01: 'UsArr does not import downloads, and no connected service accepts a music request. A music server would show the file once it were inside the folder it scans; this install has none yet, so the file stays in your download client.'
    },
    audiobook: {
      full: 'UsArr does not import downloads. Audiobookshelf watches /media/books and will show it once the file is there.',
      v01: 'UsArr does not import downloads, and no connected service catalogues audiobooks. Audiobookshelf would show the file once it were inside the folder it watches; connecting it is what makes that true.'
    },
    ebook: {
      full: 'UsArr does not import downloads, and Readarr — the *Arr that used to own books — was archived on 27 Jun 2025. Audiobookshelf watches /media/books and will show it once the file is there.',
      v01: 'UsArr does not import downloads, and Readarr — the *Arr that used to own books — was archived on 27 Jun 2025. Kavita catalogues ebooks here and scans /books, so it will show the file once it is there and not before.'
    },
    comic: { both: 'UsArr does not import downloads, and no connected service accepts a comic request. Kavita shows what is already inside the folder it scans, so the file stays in your download client until you move it there.' }
  };

  function grabbedNote(row) {
    var table = row.closest('table');
    var type = (row.getAttribute('data-media') ||
      (table && table.getAttribute('data-media')) || '').trim();
    var entry = SINK_NOTE[type] || SINK_NOTE.movie;
    return entry.both || entry[install];
  }

  /* Every message about a grab names the release. Two identical toasts after a
     five-row bulk grab tell you two failed and nothing about which two. */
  function releaseName(row) {
    var el = row.querySelector('td[data-col="title"] .mono');
    return el ? el.textContent.trim() : 'this release';
  }

  document.addEventListener('click', function (ev) {
    var btn = ev.target.closest('[data-act="grab"]');
    if (!btn || btn.getAttribute('aria-disabled') === 'true') return;
    var row = btn.closest('tr');
    setChip(row, 'pending', 'pending');
    /* The button is not disabled. A disabled element cannot hold focus, so
       disabling it inside the click handler dropped focus to <body> and threw
       a keyboard user who had just grabbed the third release back to the top
       of the page, 30-odd tab stops from the fourth. aria-disabled keeps it
       focusable and the handler swallows the second press. */
    btn.setAttribute('aria-disabled', 'true');
    btn.classList.add('is-disabled');

    var name = releaseName(row);

    /* This row is scripted to fail, so the failure path is demonstrable. */
    if (row.getAttribute('data-demo') !== 'fail') {
      var note = grabbedNote(row);
      window.setTimeout(function () {
        setChip(row, 'done', 'grabbed · sent to qBittorrent', note);
        announce('Grabbed ' + name + ', sent to qBittorrent. ' + note);
      }, 700);
      return;
    }
    window.setTimeout(function () {
      /* Prowlarr serves a grab from an in-process cache keyed to the search
         that produced it. Once that entry is gone the same opaque release id
         returns the same 400 for ever, so the recovery is not Retry -- it is
         the action the expired state already models. Offering Retry here
         contradicts the screen's own answer to the identical condition. */
      setChip(row, 'failed', 'expired — search again');
      toast(
        'Grab rejected by Prowlarr: ' + name,
        'POST /api/v1/search 400: {"message":"Release not found in cache. The 30 minute grab window has expired.","description":"IndexerId 7, guid a41c9f2e"}',
        'Search again',
        function () {
          setChip(row, 'pending', 'searching again');
          announce('Running the search again for ' + name + '.');
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
    var bs = document.querySelectorAll('[data-act="grab-bulk"]');
    for (var i = 0; i < bs.length; i++) {
      bs[i].disabled = selectedRows === 0;
      /* Prowlarr's own string is "Grab release(s)", and keeping its word
         "Grab" is what buys the familiarity. The parenthesised plural buys
         nothing: the button is disabled at zero selections, so the count is
         always known when it is live, and "(s)" does not translate. */
      bs[i].textContent = selectedRows === 0 ? 'Grab releases'
        : (selectedRows === 1 ? 'Grab 1 release' : 'Grab ' + selectedRows + ' releases');
    }
  }

  var bulks = document.querySelectorAll('[data-act="grab-bulk"]');
  for (var bi = 0; bi < bulks.length; bi++) {
    bulks[bi].addEventListener('click', function () {
      var boxes = document.querySelectorAll('[data-act="rowselect"]:checked');
      var names = [];
      for (var i = 0; i < boxes.length; i++) {
        var row = boxes[i].closest('tr');
        setChip(row, 'pending', 'pending');
        names.push(releaseName(row));
        boxes[i].checked = false;
        row.setAttribute('aria-selected', 'false');
      }
      announce(names.length + ' releases queued: ' + names.join('; ') + '.');
      selectedRows = 0;
      paintBulk();
    });
  }
  paintBulk();

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
     art fetch, so it is whatever the cover happens to be. It is the image
     PLACEHOLDER: a reserved box carrying the cover's real average colour,
     present because 4.4.1 makes dominant_color available before ThumbHash. It
     is not a shimmer and it never pulses.

     What used to live here was constrainDominant(): a WCAG ratio solver that
     picked a text colour against the fill and then moved the fill until the
     pair cleared 4.5:1. It is gone, and it was not a bug fix -- it was doing
     its job correctly. It existed only to keep a title legible ON TOP OF the
     fill, and DESIGN-DIRECTION 9.2 had already ruled that the title and year
     sit BELOW the tile, on the chrome's own ground. The mockup had simply not
     caught up. With the title on a known ground the whole runtime-contrast
     problem disappears: the title and year are ordinary --fg / --fg-muted
     text, which check.mjs section 3 already asserts against both themes.

     Note that constrainDominant could not have been made safe by improving
     it. It constrained against a SINGLE AVERAGED colour, and real cover art is
     not one colour -- a white title over the light half of a Blue Note sleeve
     fails whatever the average says. Deleting the subsystem is the fix.

     setProperty rather than a style attribute: the production CSP drops inline
     styles, and CSSOM mutation is not covered by the directive. check.mjs 1d
     bans the attribute and carves this out by name. */
  var arts = document.querySelectorAll('[data-dc]');
  for (var a = 0; a < arts.length; a++) {
    arts[a].style.setProperty('--dc', arts[a].getAttribute('data-dc'));
  }

  /* ---- prototype.html only: hash routing between the five screens ------ */
  var pages = document.querySelectorAll('[data-page]');
  if (pages.length > 1) {
    var firstRoute = true;
    var route = function () {
      /* The hash carries the query as well as the route -- #requests?q=dune
         part two&type=movie is what the Search screen's per-row action links
         to -- so the route is the part before the "?". */
      var name = (window.location.hash || '#home').slice(1).split('?')[0];
      /* The skip link and the in-page anchors target element ids, not routes.
         Leave the page alone for those. */
      if (name.indexOf('pg-') === 0 || name === 'settings' || name === 'system') return;
      var found = null;
      for (var i = 0; i < pages.length; i++) {
        var on = pages[i].getAttribute('data-page') === name;
        pages[i].hidden = !on;
        if (on) found = pages[i];
      }
      if (!found) { found = pages[0]; found.hidden = false; name = found.getAttribute('data-page'); }
      var links = document.querySelectorAll('.nav__link[data-primary]');
      for (var l = 0; l < links.length; l++) {
        if (links[l].getAttribute('data-route') === name) links[l].setAttribute('aria-current', 'page');
        else links[l].removeAttribute('aria-current');
      }
      window.scrollTo(0, 0);
      refreshRoving();
      /* Focus used to stay on the nav link after the whole main region had
         been replaced, so a keyboard user who opened Services then had to tab
         past five more nav rows to reach the screen they had just opened, and
         nothing was announced. */
      if (!firstRoute) {
        found.focus();
        announce(found.querySelector('h1').textContent + '. Page loaded.');
      }
      firstRoute = false;
    };
    window.addEventListener('hashchange', route);
    route();
  }

  /* ---- the skip link points at whichever main is showing ----------------
     prototype.html holds five <main> elements and shows one, so a literal
     id="main" would be five duplicate ids. The link carries the visible one. */
  function paintSkip() {
    var skip = document.querySelector('.skip');
    if (!skip) return;
    var mains = document.querySelectorAll('.main');
    for (var i = 0; i < mains.length; i++) {
      if (mains[i].hidden) continue;
      skip.setAttribute('href', '#' + mains[i].id);
      return;
    }
  }
  paintSkip();
  window.addEventListener('hashchange', function () { window.setTimeout(paintSkip, 0); });

  /* ---- the shortcut sheet, and why "?" is unconditional -----------------
     The single-key off switch satisfies SC 2.1.4 only if it can be found, and
     it lives five clicks deep in Settings. The sheet is where it is
     discovered, which is why "?" is exempt from the switch it documents. */
  var sheet = document.getElementById('shortcut-sheet');
  if (sheet) {
    document.addEventListener('keydown', function (ev) {
      if (ev.key !== '?' || ev.metaKey || ev.ctrlKey || ev.altKey || typing()) return;
      ev.preventDefault();
      if (sheet.open) sheet.close();
      else { dialogInvoker = document.activeElement; sheet.showModal(); }
    });
  }

  /* ---- the phone drawer is modal, so it behaves like one ----------------
     It is position: fixed over the content at 40, and content the user cannot
     see was still in the tab order behind it. */
  if (app && sideBtn) {
    var mainEls = document.querySelectorAll('.main');
    var syncDrawer = function () {
      var overlay = window.matchMedia('(max-width: 900px)').matches &&
        app.getAttribute('data-sidebar') === 'open';
      var side = document.querySelector('.sidebar');
      if (side) {
        if (overlay) { side.setAttribute('role', 'dialog'); side.setAttribute('aria-modal', 'true'); }
        else { side.setAttribute('role', 'navigation'); side.removeAttribute('aria-modal'); }
      }
      for (var i = 0; i < mainEls.length; i++) {
        if (overlay) mainEls[i].setAttribute('inert', '');
        else mainEls[i].removeAttribute('inert');
      }
    };
    sideBtn.addEventListener('click', function () { window.setTimeout(syncDrawer, 0); });
    window.addEventListener('resize', syncDrawer);
    document.addEventListener('keydown', function (ev) {
      if (ev.key !== 'Escape') return;
      if (!window.matchMedia('(max-width: 900px)').matches) return;
      if (app.getAttribute('data-sidebar') !== 'open') return;
      sideBtn.click();
      sideBtn.focus();
    });
    syncDrawer();
  }

  /* ---- the library detail form declares its save model ------------------ */
  /* Both installs draw this screen, over different libraries, so there are two
     of every one of these elements in the document and exactly one of each is
     rendered. Every lookup below is therefore per-bar rather than a single
     document.querySelector, which used to bind the whole save model to
     whichever copy happened to come first in source order. */
  var editForms = document.querySelectorAll('[data-act="editform"]');
  var saveBars = document.querySelectorAll('[data-slot="savebar"]');
  if (editForms.length) {
    var dirty = {};
    var paintSave = function () {
      var n = Object.keys(dirty).length;
      for (var b = 0; b < saveBars.length; b++) {
        var bar = saveBars[b];
        bar.querySelector('[data-slot="dirty"]').textContent =
          n === 0 ? 'No unsaved changes' : (n === 1 ? '1 unsaved change' : n + ' unsaved changes');
        bar.querySelector('[data-act="save-lib"]').disabled = n === 0;
        bar.querySelector('[data-act="discard-lib"]').disabled = n === 0;
      }
    };
    var mark = function (ev) {
      var t = ev.target;
      if (!t.id) return;
      dirty[t.id] = true;
      paintSave();
      /* Changing the kind re-derives membership for every work in the library
         from what the services report. A bare <select> changes value on one
         arrow keypress with no menu open, so the consequence is stated the
         moment it is chosen rather than after Save. The warning is the one
         belonging to THIS field, not the first in the document. */
      if (t.getAttribute('data-act') === 'kind') {
        var warn = t.parentNode.querySelector('[data-slot="kind-warn"]');
        if (warn) warn.hidden = false;
      }
    };
    for (var e = 0; e < editForms.length; e++) {
      editForms[e].addEventListener('input', mark);
      editForms[e].addEventListener('change', mark);
    }
    for (var sb = 0; sb < saveBars.length; sb++) {
      saveBars[sb].addEventListener('click', function (ev) {
        if (!ev.target.closest('[data-act="discard-lib"]')) return;
        dirty = {};
        paintSave();
        var warns = document.querySelectorAll('[data-slot="kind-warn"]');
        for (var w = 0; w < warns.length; w++) warns[w].hidden = true;
        announce('Changes discarded.');
      });
    }
    paintSave();
  }

  /* ---- proposal names that collide become one library, visibly ----------
     Renaming a proposal to match another makes it join rather than create,
     which is the auto-proposal's most consequential hidden mechanic. It used
     to produce two identically named rows, both ticked, with the banner and
     both buttons still counting four. Match is case- and whitespace-
     insensitive, and that is stated on the row. */
  var propTables = document.querySelectorAll('[data-act="proposals"]');
  for (var pt = 0; pt < propTables.length; pt++) (function (propTable) {
    var names = propTable.querySelectorAll('[data-act="propname"]');
    /* The buttons, the running count and the declined tally all belong to THIS
       proposal table. Each install proposes a different set over a different
       pair of services, so a document-wide lookup would have one install's
       banner counting the other install's rows. */
    var block = propTable.closest('[data-inst]') || document;
    var declined = propTable.querySelectorAll('tbody .st--none').length;
    var repaintProposals = function () {
      var seen = {}, merged = 0, kept = 0;
      for (var i = 0; i < names.length; i++) {
        var row = names[i].closest('tr');
        var box = row.querySelector('input[type="checkbox"]');
        var key = names[i].value.trim().toLowerCase();
        var note = row.querySelector('[data-slot="mergenote"]');
        var into = seen[key];
        if (into && box && box.checked) {
          merged++;
          if (note) { note.hidden = false; note.textContent = 'Joins ' + into + ' as a second source rather than creating a second library.'; }
        } else {
          if (box && box.checked) { seen[key] = names[i].value.trim(); kept++; }
          if (note) note.hidden = true;
        }
      }
      var label = kept === 1 ? 'Accept 1 proposal' : 'Accept ' + kept + ' proposals';
      var btns = block.querySelectorAll('[data-act="accept-props"]');
      for (var b = 0; b < btns.length; b++) btns[b].textContent = label;
      var count = block.querySelector('[data-slot="propcount"]');
      if (count) {
        var tail = declined === 1
          ? ' and declines 1 thing it cannot model.'
          : ' and declines ' + declined + ' things it cannot model.';
        count.textContent = merged
          ? 'UsArr proposes ' + kept + ' libraries, joins ' + merged + ' container' +
            (merged === 1 ? '' : 's') + ' into one of them,' + tail
          : 'UsArr proposes ' + kept + ' libraries' + tail;
      }
    };
    propTable.addEventListener('input', repaintProposals);
    propTable.addEventListener('change', repaintProposals);
    repaintProposals();
  })(propTables[pt]);

  /* Last, because it repaints the scope chip and re-assigns every list's tab
     stop, and both of those have to exist before it runs. */
  paintInstall();
})();
