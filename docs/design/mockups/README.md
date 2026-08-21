# UsArr screen mockups

A static, clickable HTML prototype of the five screens this design has to carry:
**home, services, libraries, search, requests** — drawn over **two installs**, switchable.

**This is a mockup, not the application.** Every page says so itself, permanently, in the top
bar: `mockup — Static design mockup of UsArr. Every value on these screens is invented.` That
notice is not dismissible, it is in
the shared chrome rather than on one page, and it therefore survives into `prototype.html`, which is
the file built for publishing and the one most likely to reach a reader who never sees this README.
`DESIGN-DIRECTION.md` §13 forbids fabricated data in a shipped surface and makes a labelled mockup
the one exception; the in-page notice is that label.

What is built is read off the tree, not off this file — `web/src/routes` for screens, `internal/`
for backend surfaces, `ARCHITECTURE.md` §16 for which milestone owns what. The claim this README
makes is narrower and does not go stale: **these five pages are not the application.**
Nothing here is wired to a database, an *Arr, a media server, or an HTTP API. Every title, size,
narrator, issue number, seeder count, timestamp and error string on these pages is fabricated
sample data chosen to look like the real thing so the layout can be judged, and none of it should
be read as evidence that anything is implemented. There are no benchmark numbers anywhere,
deliberately: this prototype cannot make a claim about speed.

## Two installs, one switcher, and why the label carries a milestone

**The switcher is the first control in the top bar, inside the mockup notice.** It has two
positions, and every screen, count, caption and service row answers to it.

| | **Full stack** — the default | **v0.1** |
|---|---|---|
| Services | Sonarr, Sonarr Anime, Radarr, Radarr 4K *(named, never connected)*, Prowlarr, Navidrome, Audiobookshelf, **Kavita** | **Kavita**, Prowlarr |
| Service rows | 8 | 2 |
| Libraries | 8 | 4 |
| Scope chip | `All libraries (8)` | `All libraries (4)` |
| Media types in the sidebar | 6 | 2 |
| Home Block A | 6 rows, all counted | 6 rows: 2 counted, 4 with no catalogue source |
| Home Block B | 4 items | 2 items |
| Home Block C | 26 rows across 6 types | 7 rows across 2 types |
| Search `dune` | 31 results in 6 groups | 6 results in 2 groups |

**Which media types the v0.1 install catalogues is a property of that install, not a fact about the
milestone.** 🚩 **STRUCK 2026-08-20 by [ADR-0052](../../DECISIONS.md#adr-0052):** this read *"**The
two types v0.1 catalogues are ebooks and comics.** Kavita is its one catalogue source
([ADR-0041](../../DECISIONS.md#adr-0041)) and Kavita's adapter emits exactly two `work.kind` values,
so the four types with no source behind them are films, TV, music and audiobooks"*. **The count is
deliberately not corrected to a different count**, because naming which types have a source is a
category error rather than a stale fact: `cmd/usarr/import.go` imports a catalogue from `bookorbit`
**and** from `kavita`, so the answer is a property of the install. Audiobooks in the sourceless list
was additionally false on this tree — `bookorbit.MediaKindOf` classifies `m4b`, `mp3`, `m4a`, `opus`,
`ogg` and `flac` as `MediaKindAudiobook`. A type with no source still names the service that will
populate it and when — films and TV at v0.2 ([ADR-0045](../../DECISIONS.md#adr-0045)), the rest in
ARCHITECTURE §16.1's sequence after v0.1. ⚠️ **The comparison table above describes what this mockup
DRAWS**, and stands under the existing *"the re-draw is owed"* deferral; this paragraph was a claim
about the milestone and did not. Its four libraries are Ebooks, Comics, Manga and the orphaned Ongoing comics.

**The full stack is the default, and that is a deliberate choice about what is being judged.** Six
populated media types is the case the layout has to survive — the unified recently-added table, the
six-group search, the 40-row budget divided six ways, the sidebar row budget. A design reviewed only
over two types has not been reviewed.

**Which is exactly why the label has to carry a milestone.** A reader who lands on six populated
types and is told only that the data is invented has been told the *numbers* are made up and left to
assume the *stack* is what v0.1 ships. That is `CLAUDE.md`'s "no invented status" rule broken by
omission rather than by assertion, and it is the failure this switcher would otherwise introduce. So
the option labels are:

```
Full stack: a later milestone
v0.1: BookOrbit, Prowlarr
```

The separator is a colon and not an em dash, and that is not a style preference. Both labels are
short strings, §13 bans U+2014 in a string under fifteen words, and `<option>` text has no layout
box until the menu opens — so the rendered-text sweep in `check.mjs` §1b walked straight past them
for as long as they existed. They were caught the moment that sweep's corpus was widened (below),
and rewriting them was the right fix rather than exempting them: nothing about the milestone
labelling needs an em dash to survive.

**The two labels are asymmetric on purpose, and each half is the strongest true statement available
to it.** ARCHITECTURE §16 is authoritative for milestones, and it now fixes v0.1's own services:
[ADR-0041](../../DECISIONS.md#adr-0041) makes **Kavita** the one catalogue source v0.1 ships, and
[ADR-0045](../../DECISIONS.md#adr-0045) fixes Sonarr and Radarr at **v0.2**. Two services, both
pinned, so the v0.1 label names them outright — a vaguer label there would hide a fact the roadmap
has already settled. What §16 does *not* pin is everything else: §16.1 sequences Navidrome,
Audiobookshelf and Komga one at a time after v0.1 **without fixing which release each lands in**,
and the full stack is every one of them at once. No version names that state. It is later than v0.2
and there is no release the sequence guarantees it by, so `Full stack: a later milestone` is the
strongest thing that is true today and stays true whatever §16 fixes next.

⚠️ **This passage used to argue from the opposite premise** — *"per ADR-0035's amendment, v0.1 ships
no catalogue source at all"*, with the full stack read as the state after all three sources land.
ADR-0041 falsified that sentence and the drawing followed it. The **full stack's** label survives
unchanged because its argument never depended on the half that failed; the **v0.1** label changed,
because that half is exactly what ADR-0041 settled.

Three more things follow from the same rule, and `check.mjs` §8b asserts all of them:

- **The switcher lives *inside* the mockup notice**, not beside it in the product chrome. It is not
  a UsArr control and there is no setting it corresponds to; drawing it in the top bar proper would
  fabricate a product affordance. Inside the label it reads as part of the label.
- **The notice itself changes with the selection** — *"Drawn over an install with every catalogue
  source connected, which is later than v0.1"* against *"Drawn over the two services v0.1 connects"*
  — so the labelled-mockup exception in `DESIGN-DIRECTION.md` §9.6 stays true of whichever install
  is on screen, not of one of them.
- **The page loads in `full` and the switcher is not persisted.** A control that remembered `v0.1`
  from a previous visit would show a reviewer a different screen from the one being discussed.

### The v0.1 numbers are derived, not invented a second time

⚠️ **Every figure below is arithmetically derived from the Kavita-era install, and none of them has
been re-keyed for [ADR-0052](../../DECISIONS.md#adr-0052): the switcher now names BookOrbit, the
arithmetic under it is still Kavita's, and it will be re-keyed once the BookOrbit adapter's shape
settles.**

Every v0.1 figure is the full stack's own figure with the absent services' contribution removed.
That is what makes the two installs reconcile against each other rather than merely each being
internally plausible, and it is what a reviewer checks first. Almost every row is therefore present
on both installs or absent from v0.1 entirely; **Ebooks is the single row that differs by arithmetic
rather than by presence**, and it is the one to check first because it is the only place the rule
has to do any work:

- **Ebooks 2,266 → 424.** One UsArr library over two containers: Audiobookshelf's 1,842 and Kavita's
  `Books` · 424. v0.1 keeps Kavita's half and nothing else, so the sidebar row, the Libraries row,
  the edit screen's toolbar and Home Block A all read **424** there and **2,266** on the full stack.
- **Comics 553** = Kavita `Comics` 512 + Kavita `Manga` 0 + the orphaned `Ongoing comics` 41, and it
  is **the same number on both installs**, because all three of those containers are Kavita's or
  were. Comics is the one media type v0.1 loses nothing from.
- **Comics issues 7,891**, of which Kavita's own *Contributes* panel on Services reports **7,204**.
  The other 687 belong to `Ongoing comics`, whose Kavita instance was removed. **That 687 is not
  drawn as a figure on any screen** and is named here rather than in the drawing: an orphaned
  library reports what UsArr still holds of it, 41 works, and an issue count for a source that is
  gone would be a claim nothing can refresh.
- **Audiobookshelf 2,260 items** = Audiobooks 418 + Ebooks 1,842, one upstream library split by
  edition format into two UsArr libraries. Full stack only.
- **TV 275** = Sonarr 214 + Sonarr Anime 61. Full stack only — and the two-instance library it
  produces is a full-stack demonstration now, not a v0.1 one.
- **Home Block B falls 4 → 2**, because the Radarr outage and the Sonarr Anime clock skew belong to
  services v0.1 does not have. The Prowlarr 401 and the Kavita identifier warning survive, which
  then makes Block B equal Services' own System-status count on either install — `1 error, 3
  warnings` against `1 error, 1 warning` — as it should, since the two surfaces are one set of facts
  drawn twice.
- **Home Block C falls 26 → 7**, across two types rather than six. It is the only block whose v0.1
  row count is not stated on the screen itself; it is counted from the table.
- **Search falls 31 → 6, and this is the one figure naive subtraction gets wrong.** The full stack's
  six groups are Ebooks 14, Audiobooks 9, Movies 3, Comics 2, Music 2, TV 1. Deleting the four
  groups whose services are absent removes 15 — but the Ebooks group does not survive intact either,
  because it is 14 over *both* of that library's sources and **4** over Kavita's half alone, which
  takes ten more. 31 − 15 − 10 = **6**, drawn as Ebooks 4 + Comics 2, and the pagehead, the chips,
  the scoped state's `4 results … 2 more in the three the scope excludes` and the posters grid all
  follow it.
- **The linked-work rendering has no v0.1 instance, and that is a real loss rather than an
  omission.** `Dune` is one work across the 1965 novel, its M4B and the 2021 film, and §17.4 rule 4
  renders it once in the group of its best-scoring medium — Ebooks on the full stack, with the
  Movies group carrying `1 more film is on a linked row in the Ebooks group` at it. All three
  editions live in Audiobookshelf and Radarr, so on v0.1 the work is not moved to another group; it
  is simply not there. The four ebooks v0.1 does draw are Kavita's, and none of them is linked to
  anything.
- **The per-group cap moves with the group count**, and the truncation demonstration is the full
  stack's. §17.4's budget is `clamp(floor(40/g), 3, 10)`, so six groups get six rows each — which
  truncates Ebooks and Audiobooks and draws `Show all 14 ebooks matching dune` and `Show all 9
  audiobooks matching dune` — and two groups get ten, which both of v0.1's groups are under at 4 and
  2. There is no `Show all` row anywhere on the v0.1 install.
- **What moves the other way is hoisting.** The full stack's Ebooks group carries Instance as a
  column, because the group holds two of them; v0.1's carries `all from Kavita · all matched by
  title` in the group header instead, because it holds one. Same §17.4 rule, opposite outcome, and
  the two installs are what make it visible.

### What each install is *for*

They are not the same drawing twice, and the difference is not only how much is on screen. **The two
installs demonstrate opposite identity tiers.** A free Kavita returns null `aniListId`, `malId` and
`comicVineId`, so **weak identity is what a v0.1 owner sees every day**, and the strong-identity
sources — Sonarr and Radarr, carrying TVDB and TMDB ids — went to v0.2 with ADR-0045. That is the
consequence a swap of nouns cannot express, and it is why each install reaches states the other
cannot:

- **Only the full stack** can show six populated types on one screen; a group set large enough to
  truncate; an Audiobookshelf library split by edition format into two UsArr libraries; a library
  over two instances of one service with one of them degraded; a metadata-authority radio that is a
  real choice rather than a cell reading *"the only source"*; and a **mixed** identity panel — 1,703
  works matched on an external id against 563 with no identifier, in one library.
- **Only the v0.1 install** can show a media type that exists with no catalogue source behind it,
  and it shows four at once with a service and a milestone against each; an identity panel where
  **nothing** carries an identifier, 0 against 424, which §17.3 marks as v0.1's **ordinary**
  rendering rather than an edge state; a health table that is nothing but a media server and an
  indexer manager; a search whose groups both fit inside the cap; and a drilldown that stops at two
  levels, `Comics › Dune: House Atreides`, because a series and its issues is all the depth the
  medium has.
- **Both installs, now**: a source reporting zero items while healthy (Kavita `Manga`, 0 series) and
  an orphaned library whose service was removed (`Ongoing comics`, 41 works). Both used to be
  full-stack exclusives and both moved when v0.1's source became the one that owns them. They are
  still worth drawing twice, because the table around them is a different size on each.

**Two of the old claims died rather than changed hands, and are recorded here so the next reader
does not go looking for them.** *"`matched by title`, which §17.3 marks as unreachable in v0.1,
because Radarr and Sonarr carry TMDB and TVDB ids"* was the full stack's exclusive; §17.3 now says
the opposite in as many words, so the badge is not a state one install reaches and the other
cannot: it is the ordinary rendering on v0.1 and an exception on the full stack. And *"a scope chip
at the two-library minimum"* was v0.1's, and rested on v0.1 having exactly two libraries. It has
four, so that minimum is drawn on neither install and nothing here claims it.

### The mechanism, for anyone editing these files

`data-inst="full"` or `data-inst="v01"` on an element; `paintInstall()` in `usarr.js` sets `hidden`
on everything that does not match. An element with no `data-inst` renders in both.

**The one invariant: `data-when` and `data-inst` never appear on the same element.** `applyState`
owns `hidden` on the first and `paintInstall` owns it on the second, and an element carrying both has
two writers and one attribute — whose symptom is not a crash but a block quietly reappearing in a
state it does not belong to, which a reader of a mockup cannot tell from a design decision. They
compose by **nesting** instead, which costs one wrapper. `check.mjs` §1c asserts it, and also asserts
that every `data-inst` value names an install the switcher actually offers, so a typo is dead markup
that fails rather than dead markup nobody sees.

Where a whole table differs, there are **two tables**, not one table with rows switched off: a table
whose row set changes needs its own `aria-rowindex` run and its own `aria-rowcount`, and rows hidden
out of the middle of one leave gaps in both. Derived copies carry a `-v01` id suffix so the expander
a row opens is its own.

## Kavita, not Komga

[ADR-0035](../../DECISIONS.md#adr-0035) replaced Komga with Kavita as the comics-and-books catalogue
source, because Kavita is the install the owner actually runs. The mockups drew Komga; they now draw
Kavita, and the **reason** copy changed with the name rather than only the label:

- *"Komga's API exposes no external identifier of any kind"* → **"`aniListId`, `malId` and
  `comicVineId` are null on every series: those fields are a Kavita+ feature and this instance has
  not supplied them."** Per ADR-0035 §1 this is the **ordinary** case on a free Kavita, not an edge
  case, so it is drawn as a plain row on the health table and a plain badge on a search row. It must
  never read as a defect in UsArr and never as nagware: the copy says what is missing and why, and
  stops.
- The delta-strategy note no longer says *"whether Komga accepts `sort=lastModified,desc` could not
  be verified"*. Verified against Kavita `main` (ADR-0035 §2): `SortField.LastModifiedDate` exists but
  **`SeriesDto` carries no such property**, so Kavita can sort by a field it does not return and there
  is no watermark to resume a page walk from. The probe named on the row is the real one —
  `POST /api/Series/all-v2` ordered by `LastChapterAdded`.
- **Komga is now the annex row**, marked `after Kavita`, and it rests on Komga's own sourced
  behaviour: no library type, no comic/manga distinction beyond `ReadingDirection`, and no external
  identifier of any kind — which is why free Kavita loses nothing against it. Only paid Kavita beats
  either.

⚠️ The withdrawn `LibraryType 3 (Image)` claim is gone from here too. Re-checked against Kavita
`main`, `LibraryType.cs` declares exactly `Manga=0`, `Comic=1`, `Book=2` and no `Image` member. It
had been removed from `ARCHITECTURE.md` and `DESIGN-DIRECTION.md` in an earlier pass and missed here.


## Files

| File | What it is |
| --- | --- |
| `usarr.css` | The stylesheet. Layer 1 tokens, layer 2 components. Hand-written, no framework, no build step. |
| `usarr.js` | The behaviour. Vanilla, no dependencies. Theme, density, sidebar, the library scope chip, tabs, dialog, roving tabindex, the single-key-shortcut switch, the `dominant_color` contrast rule, the states switcher, and the acknowledged-write demonstration. |
| `index.html` | Home: three fixed blocks over six media types. |
| `services.html` | Service setup and health, plus the settings anatomy and the system health list. |
| `libraries.html` | Library settings: the list, the auto-proposal flow, and one library edited in full. |
| `search.html` | Search across six media types. |
| `requests.html` | Requests: the Prowlarr free-text search-and-grab path. |
| `fonts.css` | The two `@font-face` rules and nothing else: IBM Plex Sans and IBM Plex Mono, Latin subsets, self-hosted as base64 woff2. Kept out of `usarr.css` so that file stays a stylesheet a person can read. |
| `fonts/` | The three woff2 files verbatim, plus the SIL Open Font License they ship under. `fonts.css` is generated from them. |
| `prototype.html` | All five screens plus the CSS, the fonts and the JS inlined into one file, switched by `#hash` links. Generated from the files above; do not edit it by hand. |
| `build_prototype.py` | The generator for `prototype.html`. |

**The tests moved up one directory.** `selftest.mjs` used to sit here and carried five assertions
over the rendered DOM. It is gone, folded whole into **[`../check.mjs`](../check.mjs)**, which is now
the single entry point for every design rule this project enforces — the §13 ban list, token drift
between `tokens.css` and `usarr.css`, contrast on the worst of all five grounds in both themes, the
overflow sweep, row heights against all three density bands, availability names, one tab stop per
list, and the containment assertion. Run it from the repo root:

```
PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers node docs/design/check.mjs
```

**Every rendered sweep now runs over both installs**, which roughly doubles the screen×state corpus,
and **every floor in the file was restated against the doubled figure** rather than left at a number
the new reality satisfies without trying. A floor that cannot fail is the silent pass those floors
exist to convert into a failure.

### How Playwright is found, and the two ways it fails

The ladder asks for **both `playwright` and `playwright-core`**, at every location it probes.
`web/package.json` pins `playwright-core`, not `playwright`; a ladder that only ever asked for the
bare name walked straight past the pin, so the pin was **decorative** and the check silently ran on
whatever the machine happened to have installed globally — the exact opposite of what pinning is
for. `playwright-core` is the right dependency for this file: it exports the same `chromium` and
omits the test runner and the browser downloader nothing here uses.

The two failure modes have nothing in common but the word *playwright*, and one message covering
both sent people to the expensive fix for the cheap problem. They are now two branches:

| | Module missing | Browser absent or mismatched |
|---|---|---|
| Detected | `await import()` inside try/catch — a **static** `import` is hoisted and throws `ERR_MODULE_NOT_FOUND` before any handler in the file exists, so the user gets a raw stack | thrown from `chromium.launch()`, matched on `/Executable doesn't exist at/` |
| Says | this is the module, nothing was launched | this is the browser, the import succeeded |
| Advises | `pnpm -C web install` | **check the module version against the cache first** |

The second message names a **version mismatch** as the likely cause *before* it mentions any
download, and that ordering is the whole point. Each Playwright release pins one browser revision,
so a cache filled by a different release satisfies none of it — the cache can be full of Chromium
and still be the wrong Chromium. `npx playwright install` does make the symptom go away, by fetching
a *second* browser build alongside the one already there, and in doing so buries the skew that
caused it so the next upgrade fails identically. It is mentioned last, with that caveat attached.

Both branches were exercised deliberately rather than reasoned about — the module moved aside for
the first, `PLAYWRIGHT_BROWSERS_PATH` pointed at a cache holding a deliberately wrong revision for
the second — and each prints its own message and exits non-zero.

**Check 0 asserts that `chromium.launch()` resolves the `chromium_headless_shell` build**, not the
full browser. `chromium.executablePath()` cannot answer this: it reports
`chromium-<rev>/chrome-linux/chrome` no matter what, while the process that actually starts is
`chromium_headless_shell-<rev>/chrome-linux/headless_shell`. The only honest way to know is to watch
the spawn, so the check patches the `child_process` module object around the launch and reads back
the binary. The distinction is not cosmetic — the shell has no window, no compositor surface and no
GPU path, and it is the renderer every measurement in the file assumes.

### What the copy check can and cannot see

`check.mjs` §1b enforces §13's copy rules — banned words, `!`, and U+2014 in a string under fifteen
words. It read **rendered chrome text**, which meant it only ever saw strings that lay out as a
block, and every user-visible string that is not laid out at all was outside it. Outside it meant
those rules had **never once been enforced** on:

| Source | Why the walk could not see it | Floor |
|---|---|---|
| `document.title` | not in the DOM at all; it is the tab, the bookmark and the history entry | 1 |
| `aria-label` | for an icon-only control it *is* the whole user-visible string | 240 |
| `title` | the tooltip a mouse user gets | 60 |
| `placeholder` | visible until the moment the field is typed into | 130 |
| `<option>` text | an option has no layout box until the menu opens, so `display` is never blockish | 1150 |

Each source carries **its own floor**, because one combined floor would be satisfied by `aria-label`
alone while `document.title` silently contributed nothing — which is precisely how the title was
free to drift. The structural exclusions are the same ones the rendered walk uses: an attribute
inside a `<td>` is data (a release name, a timestamp), and `.statebar` is the mockup's own
scaffolding. `title`'s floor is low **by design** for that reason — most `title=` in these screens
sits in a cell and is excluded as data.

**Widening it caught three things immediately, all of them in text nobody had been able to lint:**

1. `prototype.html`'s `<title>` read *"UsArr v0.1 screen mockup — static, invented data, nothing
   implemented"* — an em dash in a nine-word string, **and** a v0.1 claim over a page whose default
   view is the full stack, which §16 sequences *after* v0.1. It had been the browser tab, the
   bookmark and the history entry for every reader of the published file. It is now
   *"UsArr screen mockups: static, invented data, nothing implemented"*; the milestone belongs to
   the install switcher, which can state it truthfully per install, and a fixed title cannot.
2. and 3. Both install-switcher `<option>` labels carried an em dash in a short string, and are now
   colon-separated. Rewriting beat exempting: nothing about milestone labelling needs an em dash.

## The typeface actually ships now

`DESIGN-DIRECTION.md` §4.2 stakes its whole case against the default UI face on IBM Plex, and until
this revision the prototype shipped **zero `@font-face` rules**. A canvas width probe settles what
that meant: `"IBM Plex Sans"` measured identically to a deliberately bogus family, and the full
`--font-ui` stack resolved to DejaVu Sans on this host. Every screenshot in every review round to
date was taken in a face roughly 24% wider than Plex, with rounder bowls and a looser default fit —
so nobody had ever seen this design in the type it was designed for, and the one typographic
decision it leans on hardest was the one thing the build did not deliver.

Both families are now self-hosted from `fonts.css`: IBM Plex Sans v23 (the variable font, weight
axis 100–700) and IBM Plex Mono v20 at 400 and 600, Latin subsets, 76 KB of woff2 in total,
`font-display: block` (set to match §4.1's call for the shipped product; the faces are inlined, so
it is unobservable here), with the system stack still in the same `font-family` declaration so a
blocked load degrades to the stack the design was reviewed against. They are embedded as base64
data URIs rather than referenced by URL for two reasons: `prototype.html` is built as a genuinely
single file, so a relative `url()` would break the moment it is opened anywhere else; and §13 bans
any reference to the hosts a Google Fonts `<link>` resolves to, which in self-hosted software leaks
every viewer's IP to a third party. `../check.mjs` asserts that both faces resolve, so this cannot
silently regress.

IBM Plex is SIL OFL 1.1 and the licence travels with the binaries in `fonts/OFL.txt`. One glyph is
outside the Latin subset and falls back to the system stack: the `→` in the Search screen's "Search
indexers →" action.

## How to open it

Open `index.html` in a browser directly from disk. No server, no build, no network. The sidebar
links are real `<a href>`s to the sibling files, so the five screens genuinely click through, and
they middle-click into new tabs the way links should.

`prototype.html` is the same thing as a single self-contained file for publishing as a web page.
Its sidebar links are `#home`, `#services`, `#libraries`, `#search` and `#requests`.

To regenerate `prototype.html` after editing any file the generator reads — the five screens, and
the assets it inlines with them — re-run the small generator script
[`build_prototype.py`](./build_prototype.py), which is committed next to its output. It inlines
`fonts.css`, `usarr.css` and `usarr.js`, splices the five `<main>` blocks between their
`<!-- PAGE:name START/END -->` markers, and rewrites the five filenames into hash routes. A
generated file whose generator lives outside the repository cannot be regenerated by anyone else,
which is how a generated file quietly becomes hand-maintained.

**The link rewriter now carries the query.** It used to drop it, so the six media-type entries in
the sidebar all collapsed onto a bare `#search`. That became untenable the moment Search grew a
per-row **Search indexers →** action, whose whole job is to hand Requests a query and a Newznab
category: dropping the query would have silently discarded the thing the action exists to pass.
`build_prototype.py` now rewrites `requests.html?q=…&type=movie&cat=2000` into
`#requests?q=…&type=movie&cat=2000`, and the router in `usarr.js` splits the hash on `?` so the
route still resolves and the parameters stay visible in the address bar. A cross-page `#fragment`
still cannot survive hash routing and is still dropped. There
is still **no per-type library grid screen anywhere in this mockup**: that screen is named in
`ARCHITECTURE.md` §16 and is not one of the five drawn here. The nearest thing drawn is the search
screen's *type-scoped* state, which is where the level breadcrumb is demonstrated.

## The two axes, and why libraries are not in the sidebar

This is the structural decision the six-type version turns on, so it is stated first.

- **Media type is the navigation axis.** It is a closed enum — movies, TV, music, audiobooks,
  ebooks, comics — bounded at six by construction, and a type with no content is not rendered at
  all. Six sidebar entries, and they cannot grow.
- **A library is scope, not a place.** It is a multi-select chip above the nav, labelled in words
  (`All libraries (8)`, `6 of 8 libraries`, `None (0 of 8)`), reflected in the URL as `?lib=…`,
  and **absent entirely when the user has fewer than two libraries**. This is Navidrome's
  `LibrarySelector`, including its `return null` at 0 or 1 library.

Libraries are never sidebar entries. Jellyfin's `libraryMenu.js` maps every user view into the
drawer with no cap, no pin and no overflow, and that is an unbounded sidebar in shipping code;
Calibre-Web reached seventeen sidebar toggles on *one* library. A scope filter removes the growth
rather than managing it, and it keeps cross-library search and browsing working — which a
single-select switcher gives up, as Audiobookshelf's own documentation records.

The scope chip is a real control here: open it (click, or press `l`), untick libraries, and the
label and the `?lib=` string both change. Its checkboxes are native, so **Space toggles and Tab
traverses** for free — but that is the whole of what native buys. Native checkboxes are *not*
arrow-navigable (only radios within a group are), and `Escape`-to-close is popover behaviour rather
than checkbox behaviour, so arrow roving and `Esc` are the two things this popover has to add. An
earlier draft of this file claimed all four came free; it was wrong, and an implementer following
it would have shipped a list where the arrow keys do nothing.

The chip's label is an `aria-live="polite"` region. Its stated job is that a control which hides
content can never be silent about what it hid, and without the live region it was silent to exactly
the users who cannot see the label change. The popover also closes on `focusout`, so tabbing past
the last checkbox no longer leaves a floating layer open over the nav. And `?lib=` is built from the
stored `library.slug` on each checkbox, never from the rendered label: the name is user-editable and
the URL is durable state, so slugifying the label would silently change a permalink on a rename.

`l` and `/` are single-character shortcuts, which WCAG 2.2 SC 2.1.4 (Level A) allows only if they
can be turned off, remapped, or are active only on focus. "It also has a visible mouse equivalent"
is not one of the three. **Settings → UI carries the off switch**, it persists, and the handlers
honour it.

**Sidebar row budget.** Scope chip 1, Home 1, six types 6, Search and Requests 2, Services,
Libraries and Settings 3, System with Status and Backup 3 — sixteen rows, which fits a 900px
viewport without scrolling the nav. Getting to sixteen cost one thing: the `General / Tags / UI`
sub-entries under Settings, which D-15 flagged as four nav entries §8.1 does not list and left for
Joe, are gone. The settings anatomy they pointed at is still on the Services screen. **That is a
judgement call made under the row budget, not a decision.**

## Home is three fixed blocks, not six strips

**This is the specified home, not a departure from it.** [ADR-0028](../../DECISIONS.md#adr-0028)
**amends** `ARCHITECTURE.md` §17.2, replacing the earlier "one section per media type present …
each showing a horizontal strip of recently added items" with three fixed blocks whose height is
O(1) in the number of media types. §17.2 as amended is what this screen draws, and it is the
authority for it. The argument ADR-0028 records:

At 1440×900, six poster strips put roughly sixteen items above the fold, against the ≥25 floor
`DESIGN-DIRECTION.md` §5.3 sets for a library screen, and put two thirds of the library below the
fold on the screen whose whole job is "what do I have?". Home's height would be O(n) in the number
of media types. So:

- **Block A — Your library.** One dense row per present media type, at most six. Answers "what do I
  have?" completely and *gains* from more types. **The unit is in the cell**, because it is not the
  same word for every type: `1,204 films`, `612 artists`, `553 series`. A bare column headed `Items`
  reads as though `Movies 1,204` and `Music 612` are comparable numbers when one counts films and
  the other counts artists over 4,118 albums and 51,204 tracks, which is the failure ADR-0031 names
  for artist-level numbers. The sidebar counts carry the same unit for a screen reader, which reads
  the number without the type name beside it.
- **On a phone, Block B goes first.** Below 1100px the two blocks stack, and Block A costs about
  105px per media type in the stacked view, which put "Prowlarr rejected the API key" **914px down
  an 844px viewport**. Block B is absent when empty, so it costs nothing when nothing is wrong.
  Measured after: Block B starts at **217px** and Block A at 812px, and Block A's stacked rows are
  two lines rather than four, because the `Type` label was redundant — its value *is* the row's
  identity.
- **Block B — Needs attention.** Wanted-but-absent, failed grabs, a degraded instance, a work
  needing re-identification. **It does not render at all when empty** — the `Populated, nothing
  needs attention` state in the switcher shows exactly that, with no green "all good" panel where
  it was. This is the block neither Jellyfin nor Plex can have, because neither knows what is
  missing.
- **Block C — Recently added.** One unified table across every type with a Type column. A sixth
  type adds rows to a list you are already reading rather than a sixth region to scan. In posters
  mode it becomes **one wrapping grid**, not six carousels — which is what
  jellyfin/jellyfin#16615 asked for and was closed as not planned.

Blocks A and B sit side by side above 1100px. Two short tables each taking a full 1232px column
wasted about 140px of vertical space, which is five rows of Block C.

**Nothing in this mockup scrolls sideways to reveal content, on any screen.** The horizontal poster
strip is gone entirely.

## Measured: items above the fold

Chromium, 1440×900, default compact density, light and dark identical:

| Screen | Any part visible | At least half visible | Fully visible |
| --- | --- | --- | --- |
| **Home** | **26** — 6 type rows, 4 attention rows, 16 recently-added rows | 25 | 25 |
| Home, posters mode | **34** — 24 cards **plus** Block A's 6 rows and Block B's 4, which stay tables | 30 | 30 |
| Search | **17** rows across 3 of its 6 groups | 17 | 17 |
| Requests | **10** release rows | 10 | 10 |
| Services | **8** service rows — the whole table | 8 | 8 |
| Libraries | **8** library rows — the whole table | 8 | 8 |

Services and Libraries now fit their entire table above the fold, which they did not before: their
rows ran 80–165px against a 28px compact standard, because prose was living in table cells. See
*Row heights* below.

Home clears the ≥25 rule. Posters mode clears it comfortably, and an earlier version of this table
undercounted it as "20 cards" by forgetting that Blocks A and B stay tables in that mode. Its
half-visible figure is 30 rather than 34, because the fold falls through the last row of cards; an
earlier version of this table copied 34 into both columns without re-measuring the second one.

**Search reaches 17 and does not clear 25.** It was 13 before this revision, and the recovery is
real rather than cosmetic: a column whose value is identical in every row of its group has been
hoisted into the group header (`Ebooks 14 · all in Ebooks · all from Audiobookshelf`), which is the
same finding as D-05 in a new place, and it took every row in the two groups above the fold from
multi-line to single-line. Column widths are now declared per list instead of derived by auto table
layout, which was re-deriving six different widths for the same column across six groups.

**Why 25 is not reachable on this screen, with the arithmetic.** §17.4 caps each group at a 40-row
budget divided by the number of groups with hits, so on six groups this query renders **22 rows in
total** — 7 ebook, 7 audiobook, 3 movie, 2 comic, 2 music, 1 TV, counting the two `Show all …`
rows. Twenty-five is above the ceiling before a single pixel of layout is considered. On top of
that, six groups cost six headings and six column-header rows: 6 × (24px heading + 6px + 25px
header row + 8px group padding) = **378px of the 734px available below the page chrome**, which is
thirteen compact rows' worth of space that carries no rows. Reaching 25 would mean raising §17.4's
per-group cap and dropping either the per-group headings or the per-group column headers — a change
to §17.4, not a change a mockup may make on its own. Recorded, not rounded up, and not fixed by
compressing type below the legibility floor.

## Measured: row heights

Chromium, 1440×900, compact density, over every state of every screen **and both installs**. The
band is the design's own: a compact row is 28px, and the ceiling is 48px of content, which is two
lines of body text plus the row padding.

| Screen | rows measured | min | median | max | one install | before the row-height work |
| --- | --- | --- | --- | --- | --- | --- |
| Home | 205 | 28 | 28 | 48 | 140 | 28–46 |
| **Services** | 65 | **45** | **48** | **48** | 32 | **103–119** |
| **Libraries** | 29 | **28** | **45** | **48** | 18 | **80–160** |
| Search | 76 | 28 | 28 | 78 | 59 | 28–78 |
| Requests | 170 | 28 | 47 | 79 | 85 | 33–68 |

`../check.mjs` holds Services and Libraries to 49px (48 of content plus the 1px row rule) and the
other three to a looser ceiling: their tallest rows are a track listing and a release row carrying
a two-line post-grab sentence, which is content doing work rather than prose that escaped into a
cell. The Services **annex** is excluded by name — it is explicitly labelled as documentation for
services neither install has, and its rows are essays by design.

**Both new row shapes hit the ceiling and were cut back rather than exempted**, which is the band
doing its job. Home Block A's four sourceless rows on the v0.1 install ran to 80px — three lines in a
half-width column — until the cause line was compressed to `Navidrome · after v0.1 · Add`, which says
no less than the sentence it replaced. The Libraries edit screen's two catalogue-source rows ran
to 61px, because the metadata-authority radio had grown a per-row explanation of what the radio does;
the explanation is a property of the control, not of the row, so it is stated once under the table.
That two-source screen was the **v0.1** install's when this was measured and is the **full stack's**
since ADR-0041 exchanged the two installs' identity tiers; the rows, the radio and the ceiling are
unchanged, and only which install draws them moved.

What actually changed, on both screens: every explanation moved into the row expander that already
existed and was barely used; the `Problem` column carries one line plus **Show detail**, which
opens the same expander the chevron does; the sync channels collapsed from three lines to the most
recent one with the other two behind the disclosure; the status badge left the *Catalogue sources*
cell, where it had been landing in four different places in four consecutive rows depending on
where the sentence happened to wrap, and stayed in the *State* column, where it is aligned; and the
`/movies` slug under each library name is gone — it was `library.slug` rendered in the face
reserved for machine data, with an invented leading slash, on the screen whose banner says UsArr
never reads a filesystem and never types a path.

`--row-lines` — the per-list placeholder `content-visibility: auto` uses before a row has ever been
laid out — is measured from the rendered rows rather than guessed, and was re-measured after this
pass. A stale value is visible: it is the height a row occupies while it is scrolled away.

## Search across six types

Four rules the two-type version did not need:

1. **A group with zero hits does not render.** No header, no "0 results in Music".
2. **Group order is by each group's best-scoring hit, not a fixed type order**, and it is computed
   once per query and then frozen. On `dune` the ebooks group is first, because the single
   best-scoring hit in the whole query is the 1965 novel.
3. **The per-group cap is a 40-row budget divided by the number of groups with hits** —
   `clamp(floor(40/g), 3, 10)`, so six here — and every truncated group's last row carries the
   **real total** (`Show all 14 ebooks matching dune`). Silent truncation makes people believe they
   have seen everything.
4. **A cross-media linked work appears exactly once**, in the group of its best-scoring medium,
   with per-medium availability beside it. `Dune` is one row, not four. Cross-media linking itself
   is v0.3; the rendering rule is drawn so the six-type screen can be reviewed against it.

5. **A column that is constant within its group is stated once in the group header, not repeated
   down the column.** §17.4's rule was "the library name renders only when the user has ≥2
   libraries"; the missing half is "*and* only when the group contains more than one distinct
   value". Measured on this result set, `Library` was constant in five of six groups and `Instance`
   in five of six. The comics group keeps both columns, because it genuinely spans two libraries and
   two provenances — the rule is per group, not per screen.

A row of type filter chips shows only the types with hits, each with its count, each a real
`<a href>` that sets `&type=`.

The `Narrowed by the library scope` state shows the scope actually narrowing: 31 results in six
groups become 2 results in one, the toolbar says `2 of 8 libraries` and names them, and the URL
carries `?lib=comics,comics-ongoing` — which is what the mockup actually emits, from the stored
slugs.

## Type-appropriate content without per-type screens

The whole component set is `ItemTable` + `ItemGrid`, plus `SectionHeader`, `LevelBar` and
`ScopeChip` — ⚠️ **`ScopeChip` is specified and not built**; the note under
`../DESIGN-DIRECTION.md` §9.7's table records what blocks it and where the measurement lives.
Column set, default sort and level count are **configuration keyed by media type**,
not new components — which is why an audiobook row can carry a narrator and a duration while a
comic row carries a publisher and a gap list, in the same table.

The one genuine per-type divergence is **hierarchy depth**: a film is one level, a comic series
two, an album three. That is a breadcrumb in the toolbar over an unchanged container, which is what
Komga's `LibraryNavigation` does. It is drawn on the search screen's `One type, three levels deep`
state: `Music › Boards of Canada › Geogaddi` over the same table every other group uses.

## The Libraries screen

Services answers *"is the pipe up, and how do I fix it?"*. Libraries answers *"what is in it, what
is it called, and where do requests go?"*. They cross-link both ways and **no credential field ever
appears on the Libraries screen**.

A library is a user-owned, named, single-kind binding to containers the services already computed —
a whole instance, a root folder, an upstream library id, or an *Arr tag — with a format filter, one
declared request destination, and a narrow correction layer. **UsArr never reads a filesystem and
there is no free-text path field anywhere on the screen**: every container was named by the service
that owns it.

Drawn in three states:

- **The list.** Eight libraries, and **four of the six media types have no request destination at
  all**, because §16 gives v0.1 *no command sinks*. That is not a gap in the drawing; it is what
  v0.1 is, and free-text indexer search through Prowlarr is the only request path it ships. Two
  libraries, **Audiobooks and Ebooks, are one Audiobookshelf library split by edition format** —
  something Audiobookshelf cannot express itself, because its `mediaType` is only `book` or
  `podcast` and the ebook/audiobook difference is a per-item field. That is the clearest case of
  UsArr's organisation being better than the service's own, and it is why the concept earns its
  place.
- **The auto-proposal.** On the full stack, connecting Audiobookshelf and Kavita proposes four libraries behind one
  Accept, all pre-selected and editable in place. Kavita's second upstream library answers with **0
  series** and is proposed anyway, because "the source reports nothing" is not the same fact as
  "we have not read the source yet" and conflating them is how a user concludes the import is
  broken. Renaming it to `Comics` is what makes it **join** the existing library as a second source
  rather than become a parallel one — getting that default wrong splits a series into two entries
  and destroys the per-instance availability badge. One thing is **declined with a reason** rather
  than dropped: an Audiobookshelf `mediaType=podcast` library.
- **One library edited.** Name (with the upstream's own name shown, greyed and not editable), kind
  as a **single-choice select that is editable**, format filter, display order, visibility, default
  sort, the catalogue-source table with per-source health and a metadata-authority radio, the
  request destination as a separate single-valued thing (`None`, with the reason), diagnostics, and
  a danger zone. ⚠️ Plex is *reported* not to allow the kind to change, but the only source for that
  is a community feature request and it has not been verified against Plex's own documentation, so
  it is no longer used as the argument anywhere — in this file or in the mockup. The argument stands
  on its own mechanism: nothing here was inferred from a path or a filename, so nothing breaks when
  the kind changes.

**The danger zone says what a delete actually does.** It removes the library from UsArr and deletes
nothing from Audiobookshelf or from your disks — *and it discards the corrections made in that
library*, because `library_override` is keyed to `library_id`. An earlier draft reassured the user
that nothing was being deleted, which was the opposite of the truth about the one thing on that
screen the user owns.

**The Corrections panel carries a `v0.3` chip.** §16.0 caps the correction UI at v0.3 and it is the
one thing §16.0 claims as payment for the six-type amendment; drawing it as though it ships would
remove the only visible evidence that anything was capped.

**The correction surface is four verbs and five fields, and the bound is the design**: `exclude`,
`include`, `relink`, `field`, where `field` may override only title, sort title, year, cover and
creator. It is a small, listed, revertible control, not a metadata editor. A correction lives in
UsArr, is keyed to UsArr's identity, and is never cleared by a sync or a tombstone expiry —
LazyLibrarian re-adding books you marked ignored, because a rescan returned them with one piece of
metadata different, is what happens when corrections are keyed to the upstream's id.

An **orphaned library** — Ongoing comics, whose only source was a Kavita instance that was removed —
is retained with its reason, still browsable, still in the scope chip, with Delete offered and
never taken automatically. A library carries a name you chose, corrections and tags; deleting owned
data to tidy up replicated data is the wrong trade.

## The honest states this version adds

Each of these exists because the research found a real failure the ecosystem produces, and each is
reachable from a states switcher:

| State | Where | Why it is drawn |
| --- | --- | --- |
| A comics series with a **gap list**, not a fraction | Search, comics group | Issue totals are declarations, and the ComicInfo spec concedes "the `Count` could be different on each book in a series". `11 · #7 missing` is computed from numbers UsArr already holds and is always true. Home's type summary says `7,891 issues · 34 with gaps` for the same reason. |
| A work with **no work-level identity** | Search, ebooks and comics groups | Kavita returns null identifier fields without Kavita+, so a series carries no identifier and says so; an ebook added from a file that supplied none is in the same position. |
| **All sources down, fully browsable** | Libraries, Movies row | Radarr is Movies' only source and its breaker is open. Every one of the 1,204 rows still renders, sorts and searches, because nothing on a render path talks to Radarr. §17.8 calls this the replica principle's demonstration; it had never been drawn. |
| **Sources healthy, zero items** | Libraries, Manga row | Kavita answers and reports 0 series. §17.8 contrasts this explicitly with "not synced yet", and the copy says which one it is: the last page walk finished at 13:31 and returned nothing. |
| **One source degraded** | Libraries, TV row | Two Sonarrs feed one library and the anime instance's clock is 212 seconds ahead, so delta polling can miss changes inside the skew window. One row, both instances. |
| **No request destination at all** | Libraries, four rows | v0.1 has no command sinks. Music, Audiobooks, Ebooks and Comics each say `none` with the reason, and the Comics reason is scoped and true rather than an overclaimed impossibility: Mylar3 and Kapowarr exist and can accept a comic request, but neither imports a Prowlarr grab. |
| **Classical is not solvable** | Search, type-scoped state | Neither Navidrome nor any *Arr models a composition, so nine recordings of the Goldberg Variations are nine unrelated albums and a composer renders as an artist. UsArr will not invent a work tier by matching titles, and it says so on the screen rather than looking quietly wrong to the person who owns that library. |
| **An orphaned item's provenance** | Search, comics group | A row in Ongoing comics, whose only source was removed, names `Kavita "garage" (removed)` rather than asserting a live link to a service that is not there. |
| **A grab with nothing to import it** | Requests, audiobook state | Readarr was archived on 2025-06-27 and nothing on this install accepts an audiobook, so the row reads `grabbed 14:07 · sent to qBittorrent` and **stops**. No progress bar, no percentage, no "importing" step, because UsArr would have nothing to measure. |
| **Every indexer failed** | Requests | Prowlarr answers a search where every indexer failed with HTTP 200 and an empty array — byte-identical to a genuine no-results response. UsArr correlates against `/api/v1/indexerstatus`, so this is a **different screen** from "nothing matched", and both are in the switcher. |
| **The grab window closed** | Requests | Prowlarr serves a grab from a 30-minute in-process cache that a restart wipes, so a results page left open outlives it. The rows say so and the control becomes `Search again` rather than a button that throws when pressed. |

### States and surfaces this round adds

| State / surface | Where | Why it is drawn |
| --- | --- | --- |
| **The library scope excludes everything** | Home and Search, and reachable by ordinary use | Unticking every library is a designed state whose only explanation was a 13px grey string in the sidebar corner — invisible below 900px, where the sidebar collapses, so a user who set a scope on a laptop and opened the same URL on a phone saw an application with no content and no explanation of any kind. It is a named state now, and emptying the chip *actually lands on it*: `paintScope()` drives the switcher, and restoring the scope comes back. |
| **The scope, stated on every scoped surface** | Home, Search, Libraries | Search already said "29 results in the six libraries the scope excludes" and was the only screen that did, which made its absence everywhere else worse rather than better. The same line, painted from the chip, now appears wherever the scope is not "everything" — and only then, so a default scope costs no chrome. Below 900px the chip is also hoisted into the top bar. |
| **The connection test's result** | Services, `Adding a service` | Specified nowhere: not idle, not running, and above all not failure, which is the most likely outcome of the first thing every user does. A service that fails its test is never saved, so it never gets a row, so the verbatim-error contract that covers configured services had nowhere to put the answer. Both outcomes are drawn, with the verbatim body and the three usual causes — a missing URL base first, because that is what a reverse-proxied or Tailscale install hits. |
| **Recent grabs** | Requests | Nothing recorded that a grab had happened. The confirmation lived in the Title cell of a search-result row and search results are transient, so UsArr's only write path produced a 64 GiB download with no UsArr-side record and no way to answer "did that one work?" — which sends the user back to Prowlarr's own UI to grab, the outcome this project exists to prevent. Six rows on ARCHITECTURE §17.5's three grab-result states (`sent`, `sent · a problem was reported after handoff`, `not sent`) rather than the write queue's internal five. `done` is a word this vocabulary cannot say, because UsArr stops observing at handoff, and the middle state sits beside `sent` rather than beside failure and offers no Retry: Prowlarr can accept a release and then fail a later step, so an error can arrive over a grab that already landed. |
| **What happens after a grab, by media type** | Requests, `nosink` | The four sink-less types got a complete, honest sentence and the two with an *Arr got "UsArr does not import downloads." and nothing else — which a Radarr owner reads as "UsArr doesn't, but obviously Radarr does." It will not: the grab went UsArr → Prowlarr → download client, and Radarr never requested that release. All six sentences are drawn, and the live movie table is wired to the movie one. |
| **Search → Requests** | Search, Movies and Comics groups | 31 result rows, zero interactive descendants: the product told you, correctly and prominently, that you do not have a film, and the only route to the one request mechanism v0.1 ships was to notice the cross, navigate to Requests and retype the title. Every row whose availability is not complete carries **Search indexers →**, linking to `#requests?q=…&type=…&cat=…` with the Newznab category preselected from the row's media type. |
| **Add library** | Libraries | A live-looking primary button with no drawn flow and no specification anywhere — and it is the recovery path when the auto-proposal got it wrong, which is the most likely reason to visit that screen. Four fields plus one source picker, reusing two components that already existed. |
| **The save model on a library** | Libraries, `edit` | Fourteen controls, no Save, no Cancel, no dirty indicator — while the add-service form 400px away had an explicit test and a disabled Save reading "No changes". One of those fourteen re-derives membership for 1,842 items from a bare `<select>` that changes value on a single arrow keypress, so that consequence is now stated the moment it is chosen rather than after the fact. |
| **A proposal rename that merges** | Libraries, `propose` | Renaming a proposal to match another makes it *join* rather than create, which §17.8 says quietly destroys the project's most visible power-user feature if it goes wrong — and it produced two identically named rows, both ticked, with the banner and both buttons still counting four. The merged row now says so, and the count follows. |
| **The keyboard sheet** | every screen, `?` | The single-key off switch satisfies SC 2.1.4 only if it can be found, and it lived five clicks deep in Settings. `?` is unconditional precisely because the sheet is where the switch is discovered. |

**A security constraint that shows up as a design rule**: Prowlarr's `downloadUrl` and `magnetUrl`
embed its API key, which is full-admin, so neither field is ever sent to the browser. The
external-link icon on a release row opens the **indexer's own page** for that release and is
labelled that way; Grab posts an opaque release id to UsArr, and UsArr talks to Prowlarr.

## Newznab categories, with the numbers

The requests screen shows the real category numbers next to their names, because two of them are
the ones integrations get wrong:

- **`3030 Audio/Audiobook` sits under `3000 Audio`, not under `7000 Books`.** A routing rule for
  `(book, audiobook)` that sends `7020` finds nothing.
- **There is no manga category anywhere in the standard tree.** Nyaa maps its literature categories
  to `7000 Books`, not to `7030 Books/Comics`, so a comics search filtered on `7030` returns zero
  manga.

Prowlarr's search types are Basic, Movie, TV, Music and Book. There is no comic type, which is why
comics go through Basic on the Books tree, and the screen says so rather than inventing one.

## Indexer flags: an open vocabulary, and what an empty one means

The `Indexer flags` column on Requests renders **whatever string Prowlarr sends, as an opaque tag**.
There is no allowlist and there must not be one.

`IndexerFlag` is a **class, not an enum**. Prowlarr's own base type declares seven —
`internal`, `exclusive`, `freeleech`, `neutralleech`, `halfleech`, `scene`, `doubleupload`
(`src/NzbDrone.Core/Indexers/IndexerFlag.cs`) — and `PassThePopcornFlag : IndexerFlag` subclasses it
to add `golden` and `approved`
(`src/NzbDrone.Core/Indexers/Definitions/PassThePopcorn/PassThePopcorn.cs`, lines 85–88).

🚩 **The two groups are different kinds of thing and must never be flattened into one list.** The
seven are the **common set** any indexer can emit. `golden` and `approved` are **one indexer's
private additions**, and they are in this document as *examples of an unbounded category*, not as
members eight and nine of a fixed one. A flat list of nine invites the original error one layer
along: a reader re-checks, counts nine, and treats *that* as the closed set. The count is a
snapshot; the openness is the fact.

**Re-check it in one command**, in a Prowlarr checkout on `develop`:

```bash
grep -rn "static IndexerFlag" src/          # 9 hits as of 2026-08-16
```

⚠️ **Grep for `static IndexerFlag`, never for `new IndexerFlag(`.** The latter returns **zero
matches in a file containing seven of them**, because `IndexerFlag.cs` constructs with C#
target-typed `new(...)` and the type name never appears at the declaration sites. It is the obvious
probe to reach for, empty output reads as *"the set is empty"*, and it is exactly how the closed-set
reading arrived here (`REVIEW-LOG.md` FI-15). `docs/reference/tags.md` and `ARCHITECTURE.md` §8.5
carry the same command and the same warning; this is their third copy on purpose, because this file
is where the column is drawn.

A seven-name allowlist would therefore drop `golden` today and every later indexer's flags forever
— and drop them **invisibly**, because the row would simply show fewer chips than the indexer sent
with nothing indicating anything had been discarded. So the sample data draws `golden` on the
PassThePopcorn row: an open-vocabulary path nobody exercises is not a designed one.

**Two names get one step of visual emphasis, and not because they matter more.** `freeleech` and
`halfleech` are the only two that are *derived* rather than sent as strings, and the only two that
change what a download costs your ratio. `TorznabRssParser.GetFlags` computes them:

```csharp
if (downloadFactor == 0.5) flags.Add(IndexerFlag.HalfLeech);
if (downloadFactor == 0.0) flags.Add(IndexerFlag.FreeLeech);
```

The emphasis is weight and fill, never a hue — a colour would have to clear the contrast sweep on
all five grounds in both themes to buy a distinction the design does not need, and the obvious hue
is a green sitting two columns from the torrent protocol dot's green.

**An absent flag means unknown. It never means "not freeleech".** Those comparisons are exact
equality against `0.0` and `0.5` on a double defaulting to `1`, so a 25% or a 75% promotion produces
no flag at all. And `GetFlags` runs only inside `if (torrentInfo != null)`; `NewznabRssParser` never
touches an `IndexerFlag`, so every usenet result yields an empty array whatever the indexer is
offering. The column therefore reads **`not reported`** on a usenet row and **`none reported`** on a
torrent row with an empty list, and **`None`** on neither, because `None` is read as the
ratio-safe claim it is not.

**What is no longer drawn: `repack` and `proper`.** Both were in this column and neither is ever
emitted from this field — they are release-title qualifiers the \*Arrs parse out of the *name*. The
titles below still carry `REPACK` and `PROPER` exactly where they belong, which is the honest
contrast. Drawing them as indexer flags invented a status our own artefact then taught to a reader,
and `docs/reference/tags.md`'s `flag:` line is the source they came from — see the routing note at
the end of this file.

## The list primitive is a grid, not a table layout

This is the one structural change in the components, and it is a correctness fix rather than a
preference.

[ADR-0029](../../DECISIONS.md#adr-0029) makes `content-visibility: auto` the default list renderer.
**It has no effect on a `<tr>`.** CSS Containment Level 2 says size containment "has no effect … if
its principal box is an internal table box", and layout and paint containment have no effect on
"an internal table box other than table-cell"; `content-visibility` is defined entirely in terms of
those three. `<tbody>` is an internal table box too, so chunking does not rescue it, and `<td>` can
take containment but collapses the cell. Measured in Chromium at 5,000 rows, a `<tr>` list had
**identical document height with and without it**.

So the rows are `display: grid` and every element carries the ARIA a native table gave for free:
`role="table"` / `rowgroup` / `row` / `columnheader` / `cell`, plus `aria-colcount` on each list and
`aria-colindex` on every header and cell. The `.tbl--stack` fork at ≤760px already did half of this,
so it was built on rather than replaced. What that buys, all measured:

| | before | after |
| --- | --- | --- |
| `content-visibility` applied at 5,000 rows | inert — document height identical with the property on and off (`<tr>` control, re-measured: 140,018px both ways) | **applied** — 500,940px contained against 480,549px with `content-visibility: visible` forced on the same markup |
| scrollbar drift after a full scroll (P-04's own <2% bar) | not measurable, nothing was contained | **0.08%** on the Libraries row mix and **−0.27%** on the Recently-added mix, both at 5,000 rows; the document settles on the real height once every row has been rendered once |
| `contain-intrinsic-size` | `auto var(--row-h)`, and `--row-h` was inert on a table row | `calc(2 * var(--row-py) + var(--row-lines) * var(--lh-base))` — the padding term tracks density (27.08 / 31.08 / 35.08px on the Recently-added list across compact, standard and relaxed), and `--row-lines` is measured **per list**: all 21 lists in these five screens now sit within **±0.31%** of their real row height. It sizes the content box, so the row's 1px bottom border is the difference between the placeholder and the border-box height, and the measurement has to subtract it |
| `min-height: var(--row-h)` | inert: forcing `--row-h: 100px` left the row at 28.0px | acts, because a grid container takes `min-height` |
| column widths | `table-layout: auto`, re-measuring every cell in every row | declared once per list in `--cols` |
| sticky column headers at 761–1099px | never pinned: `overflow-x: auto` made the wrapper a scroll container | **pinned** at the toolbar edge — see the note below the table |
| accessibility tree at 390px | `{table:1, row:7, cell:49}` — **no columnheaders at all** | `{table:1, rowgroup:2, row:9, columnheader:7, cell:56}` — read from Chromium's full AX tree over CDP, not from Playwright's filtered snapshot, which returns nothing for these nodes. The table, rowgroup, row and columnheader counts are the same at 1440px and at 390px; the cell count is 35 at rest and 56 after a full scroll, because `content-visibility: auto` skips the contents of an off-screen row until it is rendered, scrolled to, or focused. Rows and columns stay identifiable throughout — it is only the cell *text* that materialises on demand |

✅ **The derived expression agrees with the frontend thread's independently measured row heights, and
that was checked rather than assumed.** `pnpm bench:list` measured **28 / 32 / 36 px** for a one-line
row and **45 / 49 / 53 px** for a rich one across compact / standard / relaxed
(`../DESIGN-DIRECTION.md` §7.4). Re-measured here at 1440×900, these mockups render exactly those
heights, with the computed placeholder at `auto 27.98 / 31.98 / 35.98px` on the one-line
Recently-added list and `auto 45.08 / 49.08 / 53.08px` on the two-line attention list — within 0.3%
of the border-box height each stands in for, since the property sizes the content box. **No mockup
change was required and `prototype.html` did not need regenerating.** ⚠️ Note what these files
*cannot* show: §7.4's stale-remembered-size rule needs row nodes to be **reused** across a density
change, and static HTML has no reuse semantics, so `../check.mjs` is the wrong place to assert it —
`pnpm bench:list` owns that one.

The sticky box is the `thead`, not the header row: with `thead { display: block }` a sticky row's
containing block is the thead, which is one row tall, so it could not travel.

**The wrapper is `overflow-x: clip`, and the reasoning went round twice, so both halves are here.**

The defect that started it: measured at 1280, 1440, 1680 and 1920, the two fix-it buttons attached
to the degraded and the down rows on Services — **Run full sync** and **Update API key**, the only
two controls on that screen that repair a broken service — had their right border sheared off flush
with the page edge, with `document.scrollWidth` equal to the viewport at every width, so there was
not even a scrollbar to reach them with. A screen that shows what is broken and then clips the fix
is not working.

**What fixed it is the action track, not the overflow value.** Every action track is
`minmax(max-content, auto)` rather than a constant, so the column is as wide as its widest control
and the overflow never happens. That change stands.

`overflow-x: auto` was added beside it as a degrade-don't-amputate fallback, and it has been
**reverted**, because measurement says it cost a working feature to insure against a case that
never occurs:

- **It killed the sticky header outright.** `auto` computes to `auto` on *both* axes, making the
  wrapper a scroll container; the `thead` then pins to the wrapper's scrollport, and the wrapper
  never scrolls vertically. Probed at 1440×900 on Requests at scroll offsets 0 / 100 / 300 / 600,
  the `thead`'s top tracked the *table's* top to the pixel every time — `position: sticky` was
  inert on every table on every screen. With `clip` the same probe holds the header at y=40, the
  toolbar edge, while the table scrolls 182px underneath it.
- **The fallback was never reached.** `scrollWidth − clientWidth` is 0 on every rendered
  `.tablewrap` at 1440, and no descendant of any `.tablewrap` exceeds its wrapper at any of the
  five widths in any of the 35 states.
- **It was corrupting the measurements.** With the wrapper as a scroll container, `content-
  visibility: auto` rows reported the `contain-intrinsic-size` placeholder instead of their real
  height — the Corrections table on Libraries reported a uniform 46px row whose own cell measured
  60px. Under `clip` the same rows report 45 / 45 / **61** / 45, self-consistently. The 61px row
  was a real row-height violation that the scroll container had been hiding, and it is fixed
  (a `cell-sub` on the Re-linked row was three lines of prose where its siblings are two).

**The guard is now exact rather than incidental.** `../check.mjs` asserts two scopes: nothing past
`innerWidth`, *and* nothing past its own `.tablewrap`. The second is not implied by the first — a
wrapper can be narrower than the viewport, as it is in Home's two-column grid — and it is what
`overflow-x: auto` was standing in for. It fails the build rather than quietly growing a scrollbar.

`top` is `var(--toolbar-h)`, paired with `clip`: the wrapper is no longer a scrollport, so the
header pins to the viewport, whose top 40px the sticky `.topbar` already occupies. While the
wrapper *was* `overflow-x: auto` this had to be `0` instead — a `top: 40px` sticky whose static
position is the top of its own scrollport is pushed forty pixels *down*, and it painted over the
first row of every table. Both symptoms are the same fact: `top` is measured against whatever
scrollport the nearest scroll container provides, so the two lines move together or not at all.

**Two grids deliberately do not use the roving model**: the library proposals and the catalogue
sources. A form laid out in a grid is a form, and its natural tab order is the right one. Imposing
a roving model on it is what made the Kind `<select>` keyboard-inoperable. That is now a *declared*
opt-out — `data-roving-optout="…"` carrying the reason — because from outside, a form grid with no
roving model is indistinguishable from the omission that left the three Requests tables with 28 tab
stops inside 10 rows and no row reachable by Tab at all. `../check.mjs` accepts an opt-out and
fails an omission.

`aria-rowcount` and `aria-rowindex` are present now, and `aria-colindex` is the attribute this
markup did not need: every column is in the DOM, so its implicit index was already right. The rows
are the ones that are windowed — `content-visibility: auto` skips off-screen rows and "Load more"
holds a prefix of the set by construction — so a screen-reader user was being told "row 3 of 26" on
a list whose true length is 1,204. Lists that render their complete set carry the DOM count; the
truncated search groups carry the real total from their own "Show all N" line; and the fan-out
release list carries `-1`, which is ARIA's own value for "windowed, total not known" and the honest
answer for a result set that is still de-duplicating.

The ≤760px stacked label is a real element now, `<span class="stacklabel" aria-hidden="true">`,
rather than `td[data-label]::before { content: attr(data-label) }`. Generated content lands inside
the cell's accessible name, so in table-navigation mode NVDA and JAWS announced the column header
and then the cell including the same word again — "Items. Items 214", on every cell of every row.
The CSS hook is `data-col`, a slug rather than the visible string, so the column name is no longer
duplicated into an attribute a translation pipeline would not pick up.

## The keyboard model, and what was wrong with it

| | before | after |
| --- | --- | --- |
| `ArrowDown` in the Kind `<select>` | value unchanged, **focus jumped to another row** (WCAG 2.1.1, Level A) | the select changes value and keeps focus |
| `Home` with a caret in a text input | **focus lost** to a `<tr>` | caret to column 0 |
| tab stops inside the Libraries list | **8** — the row plus six Edit links plus a Delete button | **1**, the row; `←`/`→` reach the row's own actions and `Esc` returns to the row |
| cost of one arrow key at 25,000 rows | 55.1ms — a full `querySelectorAll` plus an ancestor walk per row | **0.31ms** — one sibling step |
| holding `ArrowDown` for a second at 25,000 rows | 2,255ms of main thread | **9.3ms** |
| one checkbox toggle at 25,000 rows | 32.2ms, a full-document `:checked` query, O(n²) over a range | **0.017ms** — an integer counter |
| selecting a 100-row range at 25,000 rows | ~3.2 seconds | **1.7ms** |
| a row appended by "Load more" | either a ninth tab stop or no tabindex at all, so the arrow key looked dead | idempotent re-adoption via a `MutationObserver` |
| `l` / `/` shortcuts | no off switch — SC 2.1.4, Level A | off switch in Settings → UI, persisted, honoured |
| the three Requests tables | **0 rows reachable by Tab, 28 individual stops inside 10 rows** — the only lists in the product without the roving model, on the only v0.1 screen with a stateful outbound action | one tab stop, `←`/`→` into the row's controls, `Esc` back out |
| `Esc` from a control inside a row | swallowed by the form-control bail-out, so on Requests — where every row's first control is a checkbox — there was no arrow key and no `Esc` that got you back out, only Tab | handled *before* the bail-out |
| `→` from a row-select checkbox | dead: a checkbox is an `<input>`, so the bail-out fired and you could not reach Grab | steps on to Grab, then to the indexer link. A checkbox owns no arrow keys in any browser; a `<select>`, a text input and a radio do, and those still bail |
| `/` with focus on a row checkbox | silently dead — the same over-broad guard | works |
| `l` with focus on a button | opened the scope popover from the other side of the screen — the precise bug §11 names as its motivating example | suppressed |
| the Grab control | an icon button named only by `aria-label`: no visible text, **no `title`**, so a mouse user got no tooltip either, sitting 8px from a visually similar icon that opens a browser tab. `Tab Tab Enter` fired a 64 GiB grab and you could not tell which of the two you were on | a text button reading **Grab**, beside a bordered icon control whose name and `title` both say "Open the … page for this release, in a new tab" |
| focus after a grab | dropped to `<body>` — the handler set `disabled`, and a disabled element cannot hold focus, so a keyboard user who grabbed the third release was thrown to the top of the page | stays on the button, which becomes `aria-disabled` |
| a list revealed by the states switcher or a route change | **0 tab stops** — the assignment ran once at init, when the list was hidden and had no visible row to give it to | re-run on every reveal |
| the `?` shortcut sheet | not implemented, so the off switch above had no discovery route — a compliance mechanism nobody can find | a `<dialog>` listing every key, carrying the off switch, and `?` stays exempt from it |
| skip link | none anywhere: 21 tab stops of chrome before the first row, identically on all five screens | first tab stop on every screen, pointing at whichever `<main>` is showing |
| focus after a route change | stranded on the nav link; nothing announced | moves to the new `<main>`, and the page name goes to a polite live region |
| the phone drawer | `position: fixed` over the content with no focus trap and no `Escape`; Tab past the last nav item landed on content the user could not see | `role="dialog" aria-modal`, `inert` on `<main>`, `Escape` closes and returns focus |

The density and theme controls are still O(all loaded rows) — that is inherent to setting an
attribute on `<html>` that every row reads a custom property from — but declared columns cut them
substantially: the density switch is **45ms at 1,000 rows** (was 153ms, so it is now inside Tier 0's
100ms hard fail at that size), 161ms at 5,000 (was 1,199ms) and 1,209ms at 25,000 (was 6,508ms).
Scrolling costs 0.0–0.1ms at every size. **The ceiling on rows in the DOM is set by the density
control, not by scrolling**, and that is the number ADR-0029's benchmark should be scoped to.

## The design system, briefly

**Colour.** The chrome is achromatic. There is no brand accent hue. The neutral ramp is warm grey,
never pure white and never pure black. Selection is a filled neutral step; focus is a 2px
max-contrast ring on `:focus-visible`; links are underlined rather than coloured. The only
saturated colour in the whole interface is status semantics (ok green, warning amber, error red,
and neutral grey for not-configured, which is a legitimate status and not a failure). Every status
is an icon, a word and a colour together, so nothing depends on colour alone and no colour-impaired
mode is needed later.

**Protocol chips carry no colour, and that is a reversal.** They used to take their *hue* from the
ecosystem's own values — `#00853d` for torrent and `#17b1d9` for usenet, from Sonarr's
`Styles/Themes/dark.js`, which a self-hoster reads without a legend. Those two literals are tuned
for a dark ground and fail 4.5:1 on a light one (`#00853d` measures 3.50:1 on a selected row,
`#17b1d9` measures 2.40:1 on the page ground), so the hue was kept and the lightness retuned per
theme: `#0a6b34` / `#0f6479` light, `#4fb377` / `#4ec3e0` dark.

The retune fixed the contrast and left the real problem. The torrent value is **the same green as
`--st-ok`** — computed ΔE76 **4.59** in light and **3.09** in dark, indistinguishable at an 8px
swatch and *worse* in the theme that otherwise works better. A column of green dots on Requests,
read by someone who has just come from a Home screen where green means "nothing missing", says
torrents are healthy and usenet is something else. Retuning lightness cannot fix a hue collision,
and `DESIGN-DIRECTION.md` §1.1 bans the obvious escape (indigo, violet, purple). So the fill went
and the words `torrent` and `usenet` carry the distinction, which they already did in the same cell.
`--proto-torrent` and `--proto-usenet` are gone from this file and from `docs/design/tokens.css`,
which owns them; `DESIGN-DIRECTION.md` §3.3 records the reasoning and keeps the upstream hexes as
the ecosystem reference they came from. Flagged to Joe as a change of *visible character*, not a
defect fix (`REVIEW-LOG.md` V-10).

⚠️ **And then the swatch itself went, which is the second half and was a separate mistake.** Taking
the fill out left an 8px `--border-hi` outline that is *identical on both protocols* — eighteen
copies of one mark down a column, distinguishing nothing beside a word that already says it. That is
`DESIGN-DIRECTION.md` §1.4's own test (*if a label sits next to it and the label alone would be
understood, delete the decoration*) failing on the **leftover** rather than on the fill, which the
withdrawal note had applied and then stopped one element short of. `.proto__dot` and its eighteen
spans in `requests.html` are deleted; `.proto` keeps only `white-space: nowrap`, and the
`.proto--torrent` / `.proto--usenet` modifiers stay as the filter hook. Restoring the ecosystem's
colour cue now means restoring a **dot and** a hue, not a fill — so it is no longer "reversible in
one rule", and §3.3 says that rather than repeating the promise.

**Measured contrast**, computed against the shipped token values with a WCAG relative-luminance
check in both themes. **Every figure below is the worst of the five grounds a foreground can land
on** — page, surface, hovered row, selected row and input interior — not the friendliest one. The
token values are unchanged from the reviewed set; the six-type work added no new colour.

Floors held on every ground: primary text ≥ 12:1, muted metadata ≥ 5.5:1, placeholder ≥ 4.5:1,
control borders and focus ring ≥ 3.2:1, status text ≥ 4.5:1. The protocol swatch is not in the table
because it is not in the interface: it has no colour and no longer any box, so there is no ratio to
measure. The `torrent` / `usenet` words are ordinary primary text, the top row.

| Foreground | Light, worst ground | Dark, worst ground |
| --- | --- | --- |
| Primary text | 13.58:1 | 12.47:1 |
| Muted metadata | 5.93:1 | 6.25:1 |
| Placeholder / disabled label | 4.64:1 | 4.78:1 |
| Control border | 3.42:1 | 3.23:1 |
| Focus ring | 13.58:1 | 12.47:1 |
| Status ok | 5.11:1 | 6.15:1 |
| Status warning | 4.95:1 | 6.63:1 |
| Status error | 5.14:1 | 5.77:1 |
| Status not configured | 5.93:1 | 6.25:1 |

Two roles here are mockup-local because `tokens.css` does not define them yet: `--inset`, the
interior of an input, and a `--hover` fill distinct from `--surface`. Both are now written down in
`tokens.css` §4 as an open question with their values and their reason, so the canonical file
records that the mockup did not invent colour silently; the decision itself belongs to the pass
that writes `web/src/app.css`, not to a mockup.

Both themes ship via `prefers-color-scheme` and a `data-theme` override, and the top-bar toggle
wins in both directions. The choice persists in `localStorage`.

**Type.** IBM Plex Sans for the interface, IBM Plex Mono for machine data only: paths, indexer
names, byte sizes, quality strings, release names, category numbers, error text, log lines. Both
are referenced through a `font-family` stack that falls back to the system UI stack. **Both faces
are loaded, self-hosted, and no Google Fonts link exists** — see *The typeface actually ships now*
above — because a font CDN is a privacy leak in self-hosted software. If the load is blocked the
system stack renders instead and the design still holds: it does not depend on the specific face,
only on the metrics band it sits in. **Six type steps exist, 11/12/13/14/16/20, and 20px is the
largest type anywhere.** There is no seventh: the 24px empty-state step and its exemption are both
deleted. Below 16px, hierarchy comes from weight and neutral step rather than size. Every
columnar number carries `font-variant-numeric: tabular-nums`; prose does not. Copy is sentence case
throughout.

**Space and geometry.** 4px base unit, scale 2/4/6/8/12/16/24/32/48. Rows use `min-height`, never
`height`, so a user forcing WCAG 1.4.12 text spacing does not clip anything. Radius is 0 on rows
and tables, 2px on inputs and buttons, 4px on the scope popover, the dialog and toasts, and never
more. Shadows exist only on those three floating layers, and they are neutral. There is no
`backdrop-filter` anywhere. Tables and rows are the default container; a card appears only where
cover art is the item's primary content, which is the posters view mode. Padding differs by role:
intra-row, inter-row, inter-group (search result groups) and inter-region. Layout is full-width,
not a centred column.

**Motion.** Zero on the critical path: navigation, list render, filter, sort, tab switch, scope
toggle and row expansion have no transition at all. Hover and focus are 80ms, the dialog and toasts
150ms, and the ceiling is 200ms. Hover changes colour, never geometry. A `prefers-reduced-motion`
block collapses every duration to 1ms by redefining the tokens the transitions read, so there is no
override to maintain.

**Loading.** There are no skeleton screens. Poster placeholders are a `dominant_color` fill with
the title and year set over it, inside an `aspect-ratio` box so nothing shifts — 2:3 for a film,
series, book or comic cover and 1:1 for an album, chosen from the item's own art rather than from
its media type, which is why one card component serves all six. The only remote-wait indicator is
the indexer fan-out counter on the requests screen, and it is scoped to that one strip with real
counts rather than a fake bar.

**Icons.** Thirteen inline SVG glyphs in a sprite, hand-drawn from the Tabler shapes,
`currentColor`, consistent stroke width. They appear in exactly two places: constrained row and
toolbar actions, and status glyphs. The two media-type icons the two-type version carried are gone:
at six types an icon per type is six new glyphs earning nothing that the word does not, and the
type is data rather than chrome.

**Keyboard.** `:focus-visible` rings everywhere, and **nothing in the stylesheet ever removes an
outline**. §13 now carries a narrow exception for the `:focus { outline: none }` + `:focus-visible`
replacement pattern; this mockup does not use it and does not need it, because every engine that
ships `:focus-visible` already restricts its own default ring to `:focus-visible`, so suppressing
the ring on plain `:focus` buys nothing except the risk of losing it. The banned string does not
appear anywhere in `usarr.css`, `usarr.js` or the five pages — comments included, because the rule
is enforced as a literal grep. The only occurrence anywhere in this directory is the quotation of
the §13 rule two sentences above, in this prose file, which is not a surface the grep is aimed at.
Measured on a focused control: `outline: 2px solid`, offset `1px`, in `#1c1a17` on light and
`#efece5` on dark — 13.58:1 and 12.47:1 against the worst ground either can land on.

Each data list and poster grid is one tab stop with a roving `tabindex`; up and down move rows, `←`/`→`
reach the row's own actions, `Esc` returns to the row, and Home/End are only intercepted when the
row itself has focus. `/` focuses the top-bar search box and `l` opens the scope chip; both have a
visible, mouse-reachable equivalent **and** an off switch in Settings → UI, because a visible
equivalent is not one of the three things SC 2.1.4 accepts.

**Responsive.** One layout, degraded. Below 900px the sidebar **overlays** the content behind the
toggle rather than taking a column from it, and it starts collapsed. Measured at 390×844: sidebar
hidden, content column the full 390px, no horizontal document scroll on any of the five screens, in
both themes. Below 760px the wide lists become stacked lists with their column names as inline
labels, using the same markup and the same data — and the header row is *visually hidden* rather
than `display: none`, so the `columnheader` nodes stay in the accessibility tree. Below 560px the
top bar takes a second line so the permanent mockup notice is never clipped, and every sticky offset
follows automatically because they all read `--toolbar-h`. There is no separate mobile design.

## Constraints inherited from ARCHITECTURE.md §17

These are not fresh decisions taken here. They are §17.1 and §17.7 applied, and where the brief for
this mockup differed, §17 won:

- **Home is three fixed blocks and carries no horizontal strip in any view mode** (§17.2 as amended
  by [ADR-0028](../../DECISIONS.md#adr-0028)). The posters view renders Block C as one wrapping
  grid across all types, not one strip per type.
- **Native controls** (§17.1). Every select is a native `<select>`, including the multiselects on
  the requests screen, the format filter on Libraries and the density control. The scope popover is
  native checkboxes. There is no bespoke dropdown.
- **Real links** (§17.1). Navigation is `<a href>`, middle-clickable, not a click handler on a div.
- **The UI says what Prowlarr search actually is** (§8.5). `SearchResource` carries `query`, `type`,
  `indexerIds`, `categories`, `limit` and `offset` — there is **no `author` field and no `title`
  field**, even against an indexer that advertises `book-search: [q, title, author]`. So the Search
  type select does not buy structured search, and a line under the form says so where the type is
  chosen rather than only in an empty state a user reaches after failing.
- **The grab confirmation is the literal string §8.5 fixes.** `Sent to <download client>. UsArr does
  not import downloads.`, naming the watched folder where one is known. Grabbing a row on the
  requests screen produces it.
- **No animation on any list, grid or navigation transition** (§17.1).
- **Compact is the default density**, not the middle option (§17.1). The control offers compact,
  standard and relaxed, and starts on compact.
- **No skeleton shimmer** (§17.1); the placeholder is a `dominant_color` block with the title in it.
  `dominant_color` is the only colour in this design that is **data** — §4.4.1 averages it from the
  cover art — so no token system constrains it, and a mid-luminance fill puts both near-black and
  near-white around 3.5:1. The measured worst shipped pair was **3.57:1** for the 12px semibold
  title and **3.12:1** for the year, against SC 1.4.3's 4.5:1 (12px semibold is not large text:
  large is ≥18.66px bold or ≥24px). The rule now runs in `usarr.js` where the image pipeline would
  run it: pick the foreground with the better ratio against the computed fill, and if it still
  misses the floor, **move the fill** until it clears, because the fill is decoration and the title
  is content. The `opacity: .85` on the year is gone — opacity composites, so the resulting ratio is
  not computable from the two tokens; both the title and the year now render at `opacity: 1`.
  Measured across all 30 shipped swatch elements (24 distinct fills) in both themes, six of which
  sit in the 0.15–0.35 relative-luminance band where neither pole is comfortable: **worst is 4.57:1
  for both the title and the year, and `#78736c` is the one case where neither foreground clears the
  floor on the raw fill and the fill itself is moved** — to `rgb(116, 111, 104)`. The other five
  mid-luminance fills land at 4.77, 5.23, 5.24, 5.53 and 5.70:1 unmoved. The result is identical in
  both themes, which is the point: the rule reads the fill, not the theme.
- **A degraded backend gets a small non-modal banner and the catalogue never greys out** (§17.7).
  The stale states on home and search keep every row live and interactive behind the banner.
- **Every screen works in a phone browser through responsive layout, not a separate mobile
  design** (§17.1).
- **Availability renders exactly as §6.3 defines it**: a tick when `have == total && total > 0`, a
  cross when `have == 0`, and the fraction otherwise. Where no instance of a tier exists at all, the
  state is "not configured", which is not a cross.
- **A command failure is an inline chip plus a message carrying the upstream error verbatim, with
  Retry and Dismiss, and never a silent revert** (§17.7).

## The states switcher

Every screen carries a small control marked `mockup` that switches it between the states the design
has to cover. **That control is not part of the product.** It exists because the non-happy paths are
the thing worth reviewing, and a mockup that only draws the happy path is not showing you the hard
part. There are **37 distinct states across the five screens**: home 7, services 8, libraries 4,
search 8, requests 10 — counted from the `option` values of each screen's own state `<select>`,
which is what `../check.mjs` enumerates, so the number and the tests cannot disagree. (Two earlier
versions of this line were wrong in different ways — one said 28 over a breakdown adding to 31, the
next said 32 over a breakdown that had stopped matching the markup — which is the wrong error to
have in a document whose value rests on precise counting.)

**The state axis and the install axis are orthogonal, and the check sweeps their product**: 5 screens
× 37 states × 2 installs = **74 combinations**, at each of five viewport widths. Every state is
reachable on both installs, drawn over that install's own data — there is no state that exists on one
and is silently missing on the other, which would have made the second sweep a coverage claim rather
than coverage.

The last Services state is an **annex**, labelled as one: Lidarr, LazyLibrarian and Komga are later
than **either** install draws, so they are not on the health table of either — and the annex does not
change with the switcher, because "not in this install" is true of it twice. The behaviours they
demonstrate are worth keeping — Lidarr's `status: deleted` on an MBID 404, LazyLibrarian's probed
capabilities and untagged `master` version, Komga's absent library type and absent external
identifiers — so they live in the annex with their standing on every row.

On the requests screen the grab button is live. Grabbing any row writes a `pending` chip. Grabbing
the 26-day-old HDBits release is wired to fail, so the failure path — a status change on the same
visible row plus a message carrying the upstream text verbatim — is reachable without editing
anything.

## The empty states stopped being centred heroes

They were the one region of this artefact that a reader could point at and call machine-made, and
they were on all five screens. Measured before this pass: a 24px centred heading — **larger than
the page's own H1** — over a centred 3–4 line measure over a centred button pair, inside a dashed
box, byte-identical across seven states, in a product where everything else is left-aligned and
dense. `DESIGN-DIRECTION.md` §9.6 asks for "one sentence naming why, plus the one or two buttons
that fix it. No illustration. No centred marketing block", and cites Sonarr's `NoSeries.tsx`, which
renders one sentence and two buttons in the content flow, left-aligned, in body type, with no
container. §1.2 then granted the 24px exemption that let the opposite exist.

The `--fs-empty` token is deleted rather than retuned, so 20px is the largest type anywhere in the
product again. An empty state is now a heading at `--fs-lg`, one sentence at `--fs-base`, and the
buttons — left-aligned at `--sp-5`, the same cell padding every table uses, under a hairline the
width of where the table would be. It looks like the top of a table with no rows, because that is
what it is. Every title is an `<h2>`: they were bold text, so on the one screen with nothing on it
a heading-list navigator found exactly one heading, the page title, and the user had to read
linearly to discover why the screen was blank.

The copy was trimmed to one sentence in each case. The first-run state kept the five service names,
because naming what UsArr can read a library from is the fact that state exists to deliver; what it
lost is the base-URL-and-API-key detail and the sentence about the mandatory test, which belong to
the dialog the button opens and were the empty state doing the dialog's job.

## Judgement calls made here, so a reviewer does not have to find them

1. **The 2160p column is gone from the row and stated once in the group header** — the exact
   recommendation D-05 made and D3 item 3 left for Joe. On six types the redundancy was going to be
   six columns of an identical string, not one. The "not configured" rendering rule is still
   demonstrated, once, where it carries information.
2. **The Settings sub-tree was cut** to keep the sidebar inside its row budget (D-15, D3 item 7),
   and **`Status` and `Backup` have now gone the same way**. Keeping them as System sub-rows while
   the Settings sub-tree was cut to stay inside that budget was not one rule applied twice; they
   are reachable from System, which is what a sub-page is. The sidebar is seven fixed entries
   against a stated budget of eight.
3. **Libraries is a top-level sidebar entry next to Services**, not a Settings sub-page, following
   "a second first-class settings screen alongside Services, not a sub-page of it". Its route in
   the real application is `/settings/libraries`; the mockup file is `libraries.html`. The in-page
   **Settings** nav on Services no longer lists Services and Libraries as sections of itself: it
   used to say, on one screen, both that Services was a top-level destination and that it was a
   child of Settings, with two elements carrying `aria-current` at once.
3b. **The kind vocabulary is the product's, not the schema's.** The `Kind` select offered
   `book · comic · movie · series · artist` — five values that appear nowhere else in the
   interface, with Music's answer being `artist`, a level in a hierarchy rather than a kind of
   thing. It reads `Movies · TV · Music · Books · Comics` now, with the schema value still the
   value, and one help line saying Books covers ebooks and audiobooks with the format filter
   deciding which. The Corrections column headed `Verb` — the database's discriminator, and `field`
   is not a verb — reads `Correction`, past tense.
3c. **A library name never sits beside a media-type name without the noun.** "all in Ebooks" reads
   as a tautology when the `<h2>` above it also says Ebooks, and it teaches, wrongly, that media
   type and library are one axis — which is precisely the conflation ADR-0027 exists to prevent. It
   is "all in the **Ebooks** library" now, everywhere the two meet. On Services the relationship is
   spelled in full — "catalogue source, request destination" — on the screen where a user first
   meets both terms, rather than the bare "source and destination" that dropped both nouns.
4. **The em-dash lint rule** (§13, no U+2014 in a string under 15 words) is respected everywhere
   except the two degraded-instance banners, which are the wording §17.7 fixes verbatim and which
   the rule already carries an exception for. Data separators that would otherwise use an em dash —
   `Album · Artist`, `Book · Author` — use the middle dot the rest of these tables use.

## What is deliberately not shown

| Not shown | Ships in |
| --- | --- |
| The per-type library grid screen ("Load more" + `content-visibility`, keyset pagination — ADR-0029) | v0.1, not drawn here |
| Item detail (§17.6) and the first-run wizard as its own blocking screen (§17.7) | v0.1, not drawn here; the wizard's four fields and its mandatory test appear in the Add service dialog, and both outcomes of that test are drawn as a Services state |
| The "Not in your library" unowned search section, and one search box over owned and unowned | v0.2 |
| The *Arr-backed Add flow and its `pending → approved → routed → available` states | v0.2 |
| The request model, routing rules, approval workflow, quotas, per-season TV | v0.2 |
| Cross-media links as a working feature (the *rendering rule* is drawn; the linking is not) | v0.3 |
| The corrections list as a shipping feature (drawn on Libraries so the surface can be reviewed) | v0.3 |
| The OpenSubsonic surface, stable northbound IDs, `getMusicFolders` over libraries | v0.4 |
| OPDS, multi-user, roles, per-user library access, the full tag system, saved filters | v1.0 |
| Pinned libraries in the sidebar (opt-in, default none, capped) and the `More…` overflow | seam kept, feature not drawn |
| `g`-prefixed go-to-screen keybindings | proposed in research, not implemented here |
| A `permission-denied` state on a search, library or item surface | specified from day one (`DESIGN-DIRECTION.md` §10) and **not drawn**: v0.1 has one account, and the §14 rule-6 behaviour is that a library the user cannot see renders as *absent*, which is indistinguishable from an empty scope in a static mockup. The Services `denied` state is a sudo re-auth state, which is a different thing and is not a substitute. |
| An `importing` and a `needs re-identification` per-library state (§17.8) | v0.1, not drawn here — the four §17.8 states that are drawn are *all sources down*, *sources healthy zero items*, *one source degraded* and *orphaned* |
| Lidarr, LazyLibrarian and Komga as configured services | v1.0, **v0.3** and after Kavita — drawn in the Services **annex** state, labelled with their milestone, and configured on neither install. LazyLibrarian is v0.3, not v1.0: ARCHITECTURE §16 ships it as the first Tier 1 manifest, request sink only. **Kavita is no longer in this row**: ADR-0035 made it the comics-and-books catalogue source and [ADR-0041](../../DECISIONS.md#adr-0041) put it **in v0.1**, so it is a configured service on **both** installs and the third annex row beside Lidarr and LazyLibrarian is **Komga**. Komga's milestone is stated as *after Kavita*, never as a version — §16.1 sequences it third without fixing which release it lands in, and naming one here would invent exactly the status this table exists to avoid |

Also absent, and permanently: any in-app player, any transcoding path, any FFmpeg dependency.

Because v0.1 is single-user, the approval queue and all user-management surfaces are hidden
entirely rather than shown disabled.

## Routed elsewhere: two indexer-flag items that have since LANDED, and one standing note

These are outside `docs/design/`, so they were stated here with exact text rather than edited.
**Items 1 and 2 are now closed on `main` and are kept only as a pointer to the landed wording** —
an earlier revision of this section asked for work that was already done, because it was written
against a pre-merge branch. Do not go and fix either of them.

**1. `docs/reference/tags.md`'s `flag:` line — CLOSED, and it was already correct before this
section first claimed otherwise.** The invented `proper | repack | nuked` had been removed on `main`
by the time this file reported it; we were reading a branch that predated the fix, and the routed
"suggested replacement" printed here was **also wrong in its own way** — it rendered the vocabulary
as one flat list of nine with `golden` and `approved` sitting as peers of the seven common statics.
The code thread was right to decline it. **The landed line keeps the two groups apart**, and this is
the shape to match rather than paraphrase:

```
flag:            internal | exclusive | freeleech | neutralleech | halfleech | scene
                 | doubleupload                       ← Prowlarr IndexerFlag, TORRENTS ONLY
                 (+ indexer-specific: PassThePopcorn adds golden | approved)
```

It travels with the re-check command — `grep -rn "static IndexerFlag" src/`, **not**
`grep "new IndexerFlag("`, which matches nothing in a file containing seven — and with the
torrents-only caveat: `ReleaseResourceMapper.ToResource` does `model as TorrentInfo ?? new
TorrentInfo()`, so a usenet release always takes the empty fallback and an absent `flag:freeleech`
never means the download counts fully toward ratio.

**2. `docs/ARCHITECTURE.md` §8.5's `flag:` derivation — CLOSED, landed as `4171b35`** ("docs: say in
§8 that the indexer-flag vocabulary is open"). §8.5 now states that `flag:` comes from `indexerFlags`
as **an open vocabulary**, that `IndexerFlag` is an ordinary subclassable class rather than an enum,
that the seven statics are the common set while `PassThePopcornFlag : IndexerFlag` contributes
`golden` and `approved` into the same array, and that the tag layer **matches the names it knows and
passes the rest through** rather than filtering against a list. It carries the same re-check command
and the same `new IndexerFlag(` trap. Nothing further is routed for §8.

**3. `web/package.json` pins `playwright-core@1.56.1`, and `check.mjs` now actually resolves it** —
the ladder asks for both `playwright` and `playwright-core` at every location. No change is needed
in `web/`; this is recorded only so that whoever owns that file knows the pin is now load-bearing
rather than decorative, and that **changing the pinned version without refilling
`PLAYWRIGHT_BROWSERS_PATH` is now a failure `check.mjs` diagnoses by name.**
