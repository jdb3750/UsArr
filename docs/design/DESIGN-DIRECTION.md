# UsArr — Design direction

**Where implementation status lives — not here, deliberately.** This document says what the design
*is*. [`ARCHITECTURE.md`](../ARCHITECTURE.md) §16 is authoritative for what ships in which
milestone, and the tree is authoritative for what exists right now; a status line restated here
would have nothing keeping it in step with either. So read nothing below as a claim that a value is
or is not yet built — check §16, or the code, before describing it either way. This document and
[`tokens.css`](./tokens.css) remain upstream of the UI, in the same spirit as
[`ARCHITECTURE.md`](../ARCHITECTURE.md) §17 — which owns what the screens *are*, and which this
document is downstream of.

**Last revised:** 2026-08-16.
**Constraints this obeys:** [`ARCHITECTURE.md`](../ARCHITECTURE.md) §17 (screens and UI
philosophy), §16 (scope), §13 (performance budget); [`CLAUDE.md`](../../CLAUDE.md) (the four
principles); [`DECISIONS.md`](../DECISIONS.md) ADR-0003 (SvelteKit `adapter-static` + `embed.FS`)
and ADR-0025 (the styling and typography stack, which this document is the rationale for).
**Deferred UI surfaces and the seams they need:** [`FUTURE.md`](../FUTURE.md) — where a heading
reading *Declined* means closed rather than deferred.

Where §17 and this document disagree, **§17 wins** and this document is the bug.

> **Four things the owner settled on 2026-08-16, and they are decided rather than proposed.**
>
> 1. **Colourless chrome is confirmed.** There is no brand accent hue (§3). The one recorded cost —
>    that with no accent, two table screens have little left to differ by at thumbnail scale (D-24) —
>    is accepted, not reopened.
> 2. **IBM Plex is confirmed** as the family (§4), and since 2026-08-16 it is also **shipped and
>    proven to render** — self-hosted in `mockups/fonts.css`, asserted by `check.mjs` (§4.1, §13).
>    What is *not* settled is the **subset**; OQ-3 is narrowed to that and nothing else.
> 3. **The left sidebar is confirmed** over top-bar type tabs. **OQ-2 is closed** (§8.1), and the
>    move from two media types to six is new evidence in the same direction rather than a
>    re-litigation — §17.2's top-navbar option was drafted when "per-type" meant two.
> 4. **Virtualization was delegated to us and is now settled** as "Load more" plus
>    `content-visibility`, with virtualization as an escalation above a **benchmarked** threshold.
>    **OQ-1 is closed** (§7.4, ADR-0029), and §4.5 and §16 are amended to match.
>
> **Two things this revision adds that §17 now owns:** the six-media-type expansion and user-defined
> libraries. ARCHITECTURE §17.2 has been amended for both (ADR-0027, ADR-0028); §§8.1, 8.4, 8.5, 8.6
> and 9.7 below are this document's downstream detail, not a competing decision.

Citation convention follows the rest of `docs/`: every empirical claim carries a primary-source
URL, and reasoning that is not itself cited is marked **INFERENCE**.

---

## 0. What this document is for, and the honest limit on its evidence

The owner's brief was, roughly, that he would "almost take a black background with comic sans
before something that looked like a normal AI style." Read literally that is a preference for
**committed and wrong** over **uncommitted and average**, and that reading is the load-bearing
idea here: every tell catalogued in §1 is an artefact of *no decision having been made*, rendered
competently. The defence is not novelty. It is commitment, applied without exception.

**The honest limit, stated first because it constrains how hard any of this may be argued.**
There is **no controlled user study** establishing that end users detect or penalise
AI-generated UI. The most-cited article asserting otherwise
(<https://dev.to/olehvolos/users-can-tell-when-your-ui-was-ai-generated-and-they-dont-like-it-33kn>)
cites no study, no data and no measurement — it is observational, and its claim must not be
repeated as evidence. The one quantitative source in the whole discourse is Adrian Krebs' scan of
~1,590 Show HN submissions with 16 deterministic CSS/DOM detectors and manual QA estimating 5–10%
false positives — 22% of sites hit 4+ patterns, 32% hit 2–3, 46% hit 0–1
(<https://www.adriankrebs.ch/blog/design-slop/>;
HN discussion <https://news.ycombinator.com/item?id=47864393>). That measures **prevalence**, not
**perception**.

So: **this is informed practitioner taste plus an argument about the audience**, not a research
finding. The bet is that UsArr's users *are* the practitioner class doing the complaining —
self-hosters who read GitHub, run five services and have opinions about Proxmox's frontend. That
bet is reasonable. It is still a bet, and it should not be defended as though it were data.

What *is* well-evidenced, and does most of the actual work here, is the latency and density
literature in §5 and §7. Those numbers are real, and they happen to point the same way.

---

## 1. The anti-goal, as a checklist

"Don't look AI-generated" is not reviewable. The list below is. Each row names the pattern, who
documented it, and the rule UsArr adopts instead. The grep-enforceable subset is collected as a
lint checklist in §13.

### 1.1 Colour

| Tell | Documented by | UsArr's rule |
|---|---|---|
| Indigo/violet/purple as the default accent — `#4F46E5`, `#5E6AD2`, Tailwind `indigo-500` `oklch(58.5% 0.233 277.117)` | Krebs; [febbhav/signs-of-ai-design](https://github.com/febbhav/signs-of-ai-design); [alanwest](https://dev.to/alanwest/why-every-ai-built-website-looks-the-same-blame-tailwinds-indigo-500-3h2p); [tailwindcss.com/docs/colors](https://tailwindcss.com/docs/colors) | **There is no brand accent hue at all** (§3) |
| Purple→blue gradients, `bg-clip-text` gradient headings | febbhav; Krebs; [accessible-gradient guide](https://instantgradient.com/blog/accessible_gradient_guide) | Zero gradients in chrome. Any gradient on screen is inside cover art |
| Coloured glows / coloured `box-shadow` — the literal signature `0 0 20px rgba(139,92,246,0.5)` | febbhav | No coloured shadow. One neutral shadow token, overlays only (§6) |
| Rainbow status lists — a coloured badge on every row including healthy ones | [kiwibreaksme](https://dev.to/kiwibreaksme/i-catalogued-every-tell-that-makes-a-ui-look-ai-generated-my-own-tool-kept-failing-the-test-n52) — *"Color is supposed to mean look here; paint every row and nothing means anything"* | **Grey is a status.** A healthy row is neutral (§3) |
| Pure `#fff` / `#000` grounds with untinted neutrals | [vibecodekit](https://vibecodekit.dev/ai-slop-design) | Neither literal appears anywhere. Warm-neutral ramp, near-white and near-black |
| Low-contrast body text in dark themes — a *scored detector* in Krebs' methodology | Krebs; HN **vunderba** | Both themes audited independently, measured ratios recorded in `tokens.css` |
| The 2026 escape route is already a tell: cream/beige grounds, emerald `#10B981` as "the not-purple accent" | febbhav ("signs no longer working"); HN 48269907 | Off that band too. Warm neutral is a *ramp*, not a cream accent |

### 1.2 Typography

| Tell | Documented by | UsArr's rule |
|---|---|---|
| Inter everywhere, one weight, flat hierarchy | Krebs (scored detector); [mania.design](https://www.mania.design/blog/spot-the-slop-a-ui-designers-guide-to-fixing-ai-defaults/) calls changing the font *"the single highest-leverage move against slop"* | IBM Plex Sans (§4) |
| The rotating cast of not-Inter defaults — Geist, Space Grotesk, Instrument Serif, Poppins | Krebs; developersdigest; febbhav | All five named in the lint ban list |
| Serif-italic accent word in a sans headline | Krebs; [Hallmark](https://raw.githubusercontent.com/Nutlope/hallmark/main/skills/hallmark/SKILL.md) gate 38a: *"Headers always roman (never italic)"* | No italic headings. Emphasis is weight or the neutral ramp |
| Giant centred hero heading, 64px+ | febbhav; Krebs | **The largest type anywhere in the application is the 20px page title. There is no exemption.** The earlier form of this rule ended *"24px exists only in empty states"*, and that exemption is **withdrawn** — see §9.6 |
| All-caps tracked eyebrow labels | Krebs; febbhav; kiwibreaksme | Sentence case, no `text-transform: uppercase` on labels |
| No real hierarchy — HN **Ardon**: *"All the text sizes except for the main heading are within 15% size of each other"* | HN 47864393 | Six sizes, hard stop; below `lg` hierarchy is weight + ramp step, deliberately (§4) |
| Monospace as decoration | febbhav; HN **toraway** on the *"console-ish font Claude seems to love"* | Mono is **semantic**: machine data only (§4). This converts the tell into a feature |
| Title Case Everything | febbhav | Sentence case, enforced by lint |

### 1.3 Layout and structure

| Tell | Documented by | UsArr's rule |
|---|---|---|
| Centred max-width column with vast vertical padding — marketing rhythm on an app screen | Krebs; febbhav | Full-viewport-width app shell with a sidebar. `max-width: 1200px; margin: auto` is a document layout |
| Everything-is-a-rounded-card, `rounded-2xl`; the untouched shadcn `rounded-2xl shadow-lg p-6` | febbhav; vibecodekit | Radius 0 in the flat plane, 4px on overlays, ceiling 6 (§6). The classes do not exist (§ADR-0025) |
| Coloured 3–4px left/top border strip on cards | HN **michaelcampbell**: *"colored left borders are almost as reliable a sign of AI-generated design as em-dashes"* | No coloured strips. Status is icon + text + colour, inline |
| Three-column icon-top feature grid; badge pill above the H1; stat banner; numbered `01/02/03` rows; bento grid; fake terminal chrome | febbhav; Krebs; [vibecheck](https://www.vibecheck.fail/) ("hero-stat-FAQ scaffolding"); Hallmark gate 47 | **The application has no marketing surfaces at all** (§1.5). This one rule deletes the whole row |
| Glassmorphism / `backdrop-blur` by reflex — *"had a moment in 2022 and has been the LLM default ever since"* | [developersdigest](https://www.developersdigest.tech/blog/ai-design-slop-and-how-to-spot-it); Krebs | Zero `backdrop-filter`. It also costs GPU on every scroll frame |
| Cards inside cards | febbhav | No bordered surface inside another bordered surface |
| Uniform spacing and radius everywhere — *"16px radius and 24px padding"* across a whole screen; exclusive `gap-4 / p-6 / my-8` | mania.design; febbhav | **Padding differs by role** — intra-row / inter-row / inter-region (§6). Uniform padding is itself the tell |

### 1.4 Iconography, motion, copy

- **Icons.** "The Lucide five" — Sparkles, Zap, Shield, Check, BarChart3, ArrowRight — are named as
  a set (febbhav), and the ✨ sparkle is the universal AI badge. **UsArr has no AI features, so ✨
  has no possible justification.** No emoji anywhere in the chrome: Apple App Review has
  *rejected apps* for emoji-as-iconography
  (<https://9to5mac.com/2018/02/02/apple-rejecting-apps-with-emoji/>), and emoji render
  per-platform and read poorly to screen readers, so this is a correctness rule as well as a taste
  one. No icon-in-a-pastel-rounded-chip. Every icon must do a job no word can do in the space
  available: if a text label sits next to it and the label alone would be understood, delete the
  icon.
- **Motion.** Identical `opacity: 0, y: 20` fade-in-up on every element; scroll-triggered reveals;
  hover-lift on cards; bounce/elastic easing; animated count-up numbers; a 31 KB animation library
  to fade in an image (febbhav; mania.design; HN **amanzi**;
  <https://dev.to/whoffagents/framer-motion-animations-that-dont-kill-performance-patterns-and-pitfalls-5cki>).
  UsArr's answer is §7, and it is stricter than the anti-goal requires because the architecture
  demands it.
- **Copy.** Weightless headlines; the buzzword layer (*streamline, supercharge, empower, seamless,
  effortless, unlock, elevate, leverage, robust*); the negation pivot (*"It's not just X. It's
  Y."*); tricolons; bolded-label-plus-colon bullets; Title Case; fabricated data. On em dashes the
  discourse is genuinely contested — NPR, Rolling Stone and FIU have all covered the tell
  (<https://www.npr.org/2025/11/10/nx-s1-5596088/inside-the-unofficial-movement-to-save-the-em-dash-from-a-i>),
  and there is a credible rebuttal
  (<https://www.thinklikeapublisher.com/the-em-dash-is-not-a-red-flag-its-a-beat-that-a-comma-cant-land/>).
  **UsArr's position (INFERENCE): the em dash is fine in prose and banned in UI microcopy**, on the
  grounds that a sentence long enough to need one is already too long for a button, a tooltip, a
  toast or an empty state.

**UI copy states facts about system state.** "Sonarr unreachable: connection refused at
10.0.0.4:8989" beats "We couldn't connect. Let's get you back on track!" No exclamation marks, no
first-person plural: the application is a tool, not a company. Error copy names **the failing
component, the observed symptom, and the next action** — which is CLAUDE.md principle 3 expressed
as microcopy, and conveniently the strongest anti-slop signal available, because generated copy is
systematically vague about failure.

**The head-and-detail shape is the copy to imitate; the beat that joins its two halves is a colon,
not the em dash the bullet above bans.** A statement and its **reason** take the colon — *"Sonarr
unreachable: connection refused at 10.0.0.4:8989"*. A statement and an **instruction** take a full
stop and two sentences — *"Sonarr unreachable. Check the host and port on Services."* — because an
instruction is a second thing to do, not a subordinate clause. ⚠️ **This example carried an em dash
until 2026-08-17**, and it is the sentence `check.mjs` cited as the construction §13 endorsed while
§13 banned its punctuation. That is why the punctuation is named here rather than left to be
inferred from the ban: an author who reads the endorsement and never reaches §13's Copy block must
still land on the right glyph. (The mannerism the Copy bullet above rejects is the
bolded-label-plus-colon *bullet*, a document tic; a colon inside a sentence is not that.)

### 1.5 The tell that most directly threatens UsArr: marketing grammar inside an application

Asked to "design a dashboard", agents apply landing-page patterns to application interfaces
(gendesigns.ai; vibecheck's *"hero-stat-FAQ scaffolding"*). The replacements are specific:

| The imported pattern | Where it will try to appear | The application-native replacement |
|---|---|---|
| Hero section | Home | Home opens **directly on content** — the first row of the first media-type section is at the top of the viewport, with nothing above it but the toolbar |
| Badge pill above title | Home, Services | Delete. Status belongs in the row |
| Three-card feature grid | Services | Services is a **table**: one row per configured service (§17.3's exact column set) |
| Stat banner with animated counters | Home | Counts are static text in the section header: "Movies · 1,204" |
| Onboarding "get started" CTA | First run | First run is the §17.7 wizard: three fields, a live connection test, and the *reason* the service is wanted |
| Testimonials / logo wall / pricing / FAQ | — | Never. Not applicable to self-hosted software |
| Empty state as a marketing moment | Every screen when a service is absent | Principle 3 verbatim: name what is missing, say why, link to the fix. No illustration, no "Let's get started!" |

**Project rule (INFERENCE, and worth stating):** the README and any future project website are
allowed to be a landing page. **The application is not.** Keeping those two design systems
separate is the cleanest structural defence against everything in this table.

### 1.6 The two catch-all reviewer questions

1. **Screenshot a screen, blur it to thumbnail size. Can you still tell which UsArr screen it is?**
   If Home, Search, Services and Requests all blur to "a grid of rounded rectangles", the hierarchy
   is doing nothing and the layout has failed regardless of what the tokens say.
2. **Does anything on this screen exist to persuade rather than to inform?** Delete it.

### 1.7 Shelf life — read this before treating §1.1–§1.4 as permanent

**Any anti-goal defined as a list of banned aesthetics expires.** febbhav explicitly maintains a
"signs no longer working" section: purple is already migrating to cream, emerald and italic serifs;
kiwibreaksme documents "generation-two escape routes" — ghost `01/02/03` numerals, uppercase
overlines, beige paper — that are *already* clichés. Worse, **"anti-AI crafting" is itself a named
2026 trend**
(<https://www.creativebloq.com/design/graphic-design/texture-warmth-and-tactile-rebellion-the-big-graphic-design-trends-for-2026>),
which means deliberate texture, imperfection and analogue warmth will be the next recognisable
cliché — and over-correcting into it is a real failure mode.

**The durable rules are the structural ones**, and they are the ones to weight when this document
is next revised:

1. **Density** — the screen shows what the user came for, not what a template wanted.
2. **No marketing grammar in an app.**
3. **States, not the happy path** (§10).
4. **Colour is status** (§3).
5. **A token file that is genuinely ours** ([`tokens.css`](./tokens.css)).

The ban lists in §1.1–§1.4 are subordinate to those five, and should be re-checked against the
sources rather than trusted, whenever this document is revisited.

---

## 2. What this inherits from the project

These are not new. They are restated because every decision below is downstream of one of them.

| From | Constraint |
|---|---|
| **CLAUDE.md principle 1 / ADR-0004** — replica, not proxy | Essentially every read is local SQLite. That is what makes §5's Tier 0 ("show nothing at all") the *dominant* case rather than an aspiration |
| **CLAUDE.md principle 2 / ADR-0005, 0006, 0018** | No player, no transcoding. "Play" for video is a deep link into the backend's own client **and the button says so** |
| **CLAUDE.md principle 3** — degrade honestly | §10 is this principle expressed as a component contract. A missing service is never an empty screen and never a spinner |
| **CLAUDE.md principle 4 / ADR-0019** | Single-user in v0.1; the UI *hides* what has not shipped, and hiding is never the authorization boundary (§14 rule 6) |
| **ARCHITECTURE §17.1** | Utilitarian over stylish. Density and speed over animation. Standard patterns over novel ones. No visual flair that costs render time. **Navidrome is the bar** |
| **ARCHITECTURE §17.1** | **Native controls**: native `<select>`, native checkboxes and radios, native focus rings, real form submits. No bespoke dropdown that breaks keyboard navigation |
| **ARCHITECTURE §17.1** | **Real links**: anything that navigates is an `<a href>` with a real URL that middle-clicks and cmd-clicks into a new tab. Never a `<div>` with a click handler |
| **ARCHITECTURE §17.1** | **No animation on any list, grid or navigation transition** |
| **ARCHITECTURE §17.1** | **Density is a feature**, and compact is what loads |
| **ARCHITECTURE §17.1** | **Every screen usable on a phone browser** — responsive layout, not a separate mobile design |
| **ARCHITECTURE §17.1 / §4.4.1** | **No skeleton shimmer.** The image placeholder is a `dominant_color` block — the cover's own average colour, reserved at the right aspect before the image arrives. Informative, not decorative, and it never pulses. **The title sits below it, not in it** (§9.2) |
| **ARCHITECTURE §2.3 / §5.5 / §17.7** | Degraded ≠ blocked. A small **non-modal** banner. **The catalogue never greys out and never shows a spinner** |
| **ARCHITECTURE §13** | Client-side prefix filter p50 < 5 ms, p99 < 16 ms — one frame. The UI's own budget, not the server's |
| **ARCHITECTURE §16** (amended by ADR-0032, then by ADR-0035, then re-sequenced by ADR-0041 and ADR-0045, then re-sourced by ADR-0052) | **v0.1 connects two services: BookOrbit and Prowlarr** ([ADR-0052](../DECISIONS.md#adr-0052), which replaced Kavita in that slot on the owner's decision to sunset it). The six media types stay in the model and the navigation, and **which of them an install catalogues is derived from the service kinds that install has connected — this document enumerates no split** (§8.4). 🚩 **STRUCK 2026-08-20.** This passage read: *"but **v0.1 has no catalogue source for movies, TV, music or audiobooks** — §16.1 gives v0.1's one catalogue source the same three media types Kavita had, books, comics and manga, so ebooks and comics are the two types v0.1 catalogues and the other four wait. ⚠️ **That pair used to be read off the adapter**, and it no longer can be: 'its adapter emits exactly two `work.kind` values, `comic` and `book`' is measured on `mapLibraryType` (`internal/libsync/kavita.go`), and **BookOrbit's adapter is not written yet**, so the pair now rests on §16.1's media types rather than on code."* **Both halves are dead, and the shape of the mistake matters more than either.** BookOrbit's adapter **is** written (`internal/libsync/bookorbit.go`, `bookorbitfiles.go`), and `bookOrbitEditionFormat` returns `edition.format` = `'audiobook'` for `bookorbit.MediaKindAudiobook` — *"one of m4b, mp3, m4a, opus, ogg, flac"* — which `internal/store`'s `mediaTypeOf` renders as **Audiobooks**; so §16.1's *"the media types are unchanged — books, comics and manga"* is falsified by the same code, upstream of this row. ⚠️ **The count is deliberately not corrected to a different count**, because naming which types have a source is a category error rather than a stale fact: `cmd/usarr/import.go` imports a catalogue from `bookorbit` **and** from `kavita`, so the answer is a property of the install. §8.4 carries the rule that replaces it. **Sonarr and Radarr are v0.2** ([ADR-0045](../DECISIONS.md#adr-0045)); **Navidrome, Audiobookshelf and Komga** sequence after v0.1 one at a time, in §16.1's order, which does not fix which release each lands in. ⚠️ This row used to read *"v0.1 connects three services: Sonarr, Radarr and Prowlarr"* with the \*Arr library sync proving the replica thesis first; ADR-0041 moved the sync core onto Kavita because the owner runs Kavita and runs neither \*Arr, and **re-sequenced rather than cut** — both \*Arrs still arrive. Of the comics pair, this row then read *"**Kavita ships and Komga follows it**"*: ADR-0032 cut Kavita and **ADR-0035 reversed that**, because it was the install the owner actually ran and it covered books, comics and manga in one source. **ADR-0052 ended that placement.** Kavita left v0.1 with it and is **not** added back to §16.1's post-v0.1 sequence, on [ADR-0042](../DECISIONS.md#adr-0042)'s refusal-to-invent-a-milestone precedent — so Komga is that sequence's third entry with nothing of Kavita's ahead of it. ⚠️ **Sunset is not deleted:** `internal/kavita` and `internal/libsync/kavita.go` stay in the tree and stay green; what stops is investment. ARCHITECTURE §16 is authoritative for which milestone each lands in. The **command sinks are all out of v0.1** — including the minimal \*Arr write path, which [ADR-0042](../DECISIONS.md#adr-0042) re-sequenced out with the \*Arrs — and they do not all land together: **LazyLibrarian is v0.3** (the first Tier 1 manifest, request sink only), while **Lidarr, Mylar3 and Kapowarr are v1.0**. Requests in v0.1 is the **Prowlarr Search-and-Grab path only — for all six types**, which is what keeps a type the install has no catalogue source for navigable (🚩 **STRUCK 2026-08-20:** this read *"the four sourceless types"*; the number is per-install, §8.4) |
| **ARCHITECTURE §6.5 / ADR-0026** | **User-defined libraries exist and are configured separately from services.** They are a *scope*, never a navigation axis (§8.1) |

One more, from the ecosystem rather than from the repo: in this software family **stability of
layout outranks currency of visual style**. Proxmox runs ExtJS 7 with a Sencha Touch mobile UI and
is loved; Navidrome runs Material-UI **v4**, two majors behind, and has almost no open UI
complaints; Jellyfin's multi-year rewrite left the app internally inconsistent for years
(jellyfin-web [#442](https://github.com/jellyfin/jellyfin-web/issues/442) open since 2019 alongside
2026 v12 inconsistency reports [#8101](https://github.com/jellyfin/jellyfin-web/issues/8101));
Portainer ships Angular 1.8 *and* React 17 with Bootstrap 3 *and* Tailwind 3 in one manifest.
**INFERENCE: apps here are forgiven for looking ten years old and are not forgiven for moving
things.** Freeze the nav and the page skeleton in v0.1 and do not reshuffle them.

---

## 3. Colour

> **Chrome is achromatic. Chroma is reserved for status and cover art.**

**There is no brand accent hue in UsArr. None.** Not a blue, not a green, not a "not-purple".
**Confirmed by the owner on 2026-08-16.** This is the single biggest structural defence in this
document, and it is worth stating why rather than just asserting it:

1. **The AI default is a violet or blue brand accent, and this app has none.** The tell cannot be
   present if the category is absent.
2. **A media hub already gets its colour from the content.** Posters and cover art supply every
   saturated pixel on a browse screen. Every hue in the chrome competes with the artwork, which is
   the actual data. Navidrome — the stated reference point — does exactly this.
   *(INFERENCE: no source states this as an anti-slop rule; it is a data-ink argument, per Tufte's
   erasing principles, applied to chrome.)*
3. **It makes the Services screen's colours mean something**, because they are the only colours on
   the page. This is the direct answer to the rainbow-status-list tell.

### 3.1 What the chrome is made of

A nine-step warm-neutral ramp (hue ~35–45, chroma deliberately low), inverted by role rather than
by value between themes, with every ratio measured and recorded in
[`tokens.css`](./tokens.css). **No `#fff` and no `#000` anywhere** — near-white `#faf9f7` and
near-black `#1c1a17` in light; `#171614` and `#efece5` in dark.

- **Selection is a filled neutral step** (`--n-2`), not a coloured highlight.
- **Hover is a fill change only** (`--n-1`) — never a geometry change (§7).
- **Focus is a 2 px maximum-contrast ring** — `--n-8` against the ground, 16.50:1 in light and
  15.33:1 in dark, far above SC 2.4.13's 3:1 requirement between focused and unfocused states
  (<https://www.w3.org/WAI/WCAG22/Understanding/focus-appearance.html>). It is applied with
  `outline` + `outline-offset`, which is the native mechanism §17.1 asks for — **not** a
  `box-shadow` imitation, and **never** `outline: none`.
- **Links are underlined, not coloured.** With no accent hue there is no "link blue" available, and
  underline is the more robust affordance anyway. Links are still real `<a href>` elements.
- **Two elevation cues exist:** a 1 px border (`--n-3` decorative divider, `--n-4` control
  boundary) and a background step. That is the whole vocabulary. One neutral shadow token exists
  and is legal only on popover, dropdown, dialog and toast.

### 3.2 Status — the only saturated colour besides cover art

Four states, aligned to what the ecosystem has already trained self-hosters on:

| State | Meaning | Light | Dark |
|---|---|---|---|
| `ok` | healthy, synced, file present | `#1c6b3a` (6.21:1) | `#5cba7d` (7.56:1) |
| `warning` | degraded, stale, partial, clock skew | `#a44c00` (5.53:1) | `#fb9349` (8.07:1) |
| `error` | down, failed, rejected, needs re-identification | `#b3251c` (6.24:1) | `#f0837a` (7.09:1) |
| `not configured` | no service of this kind exists | `#5a534a` (7.20:1) | `#b0a89b` (7.68:1) |

The ratio in each cell is against the **page ground**, which is the best of the five. What the
values are actually held to is the **worst** of the five grounds a status can land on — page,
surface, hover, selected, inset — and `docs/design/check.mjs` asserts that, not this table:

| | page | surface | hover | selected | inset | worst |
|---|---|---|---|---|---|---|
| light `ok` `#1c6b3a` | 6.21 | 5.74 | 5.43 | 5.11 | 6.38 | **5.11** |
| light `warning` `#a44c00` | 5.53 | 5.11 | 4.84 | 4.55 | 5.67 | **4.55** |
| light `error` `#b3251c` | 6.24 | 5.77 | 5.46 | 5.14 | 6.41 | **5.14** |
| light `not configured` `#5a534a` | 7.20 | 6.66 | 6.30 | 5.93 | 7.40 | **5.93** |
| dark `ok` `#5cba7d` | 7.56 | 7.04 | 6.48 | 6.15 | 8.00 | **6.15** |
| dark `warning` `#fb9349` | 8.07 | 7.52 | 6.92 | 6.57 | 8.55 | **6.57** |
| dark `error` `#f0837a` | 7.09 | 6.61 | 6.08 | 5.77 | 7.51 | **5.77** |
| dark `not configured` `#b0a89b` | 7.68 | 7.16 | 6.59 | 6.25 | 8.13 | **6.25** |

**Grey is a legitimate status and it is load-bearing.** "Not configured" is a real, common,
non-broken condition in a pluggable app, and painting it a colour would make the two conditions that
*are* broken harder to find.

> **Every status is encoded as icon + text + colour together. Never colour alone.**

✅ **The warning is orange in both themes, and the word carries it — SW-05, closed by the owner on
2026-08-16 in his own words: the light value was "kind of eh… going more orange would be better for
warnings".**

`--status-warn` was `#8a5300` light / `#e0a33a` dark. Two things were wrong with that pair. The
light value was chosen by pushing an amber down until it cleared 4.5:1 as **text**, and what came
out the other end was a **brown**: CIE Lab hue **70.4°**, most of the way from orange (≈60°) to
yellow (≈90°), at chroma 52.4. And the dark value was Lab hue **77.8°** — a *gold*. The two themes
were not the same colour at two lightnesses; they were a brown and a gold that happened to share a
token name.

**What ships: `#a44c00` light, `#fb9349` dark.**

- **It is genuinely orange, and that is measured rather than asserted.** `#a44c00` is Lab hue
  **58.0°** at chroma **62.1**; `#fb9349` is Lab hue **58.6°** at chroma **64.0**. CSS `orange`
  `#ff7f00` is Lab hue **59.6°**. Both themes now sit within 1.6° of it, so they are the same hue
  at two lightnesses — which is what "give dark the same hue family" has to mean if it means
  anything.
- **The word clears 4.5:1 on the worst of all five grounds**, not just on the page: **4.55:1**
  against `--selected #e7e3dd`, 4.84 against hover, 5.11 against surface, 5.53 against the page,
  5.67 against the inset. Dark's worst is **6.57:1**.
- **Dark holds everything constant except hue.** Chroma 64.0 against the old 61.9, worst-ground
  contrast 6.57 against the old 6.63. The only thing that moved is the 19° of hue that made it a
  gold.
- **Warning still does not read as error.** ΔE76 from `--status-error` is **25.2** in light and
  **32.1** in dark. That is the test §3.3 failed the protocol chips on, where the collision was
  ΔE76 **4.59**.

🚩 **The word/glyph split this section grants is NOT taken, and that is the point of reporting it.**
The rule stands and is worth keeping: **the status *word* is text, bound by SC 1.4.3 at 4.5:1; the
status *glyph* is a non-text graphic, bound by SC 1.4.11 at 3:1, so it MAY take a separate, more
saturated token provided it clears this document's 3.2:1 non-text target on every ground.** But
`#a44c00` is convincingly orange *and* readable as text, so exercising the split would buy a second
token, a second row in every contrast table and a two-colour status label, in exchange for chroma the
word already has. **One value, both roles.** The split is available if a future ground or a future
hue makes it binding; nothing exercises it today. A warning row may still take a low-alpha tint of
the same hue.

⚠️ **Values that do not work, recorded so they are not retried.** The candidate this section
previously carried, `#a9700a`, was never a *text* value: it is **3.98:1** on the page ground but
**3.28:1** on `--selected`, so it fails 4.5:1 on the ground that actually binds and could only ever
have been a glyph token. `#c98a00` is **2.80:1** and `#b8860b` **3.09:1** on the page ground alone.
And `#b04300` — Lab hue 52.0°, chroma 68.4, more chromatic than what shipped and still clearing
4.50:1 — is rejected on the *other* axis: ΔE76 **18.5** from `--status-error`, close enough that a
column of warnings beside a column of errors starts to blur. Chroma is not the only thing being
maximised.

📌 **One part of the finding that raised this stays rebutted on measurement.** V-06 described
`#8a5300` as sitting *inside* the warm neutral ramp, with "the colour doing none" of the work.
Computed, ΔE76 from `--fg-muted #5a534a` was **46.3** in light against **54.2** for the dark pair —
a gap ~15% narrower, not a collapse. The *direction* was right, which is why this section exists at
all and why the value moved in the end; the *magnitude* was not supported. What settled it was not
either instrument but the owner looking at it, which is the test both of them were standing in for.
`#a44c00` measures ΔE76 **56.7** from `--fg-muted`, so the gap the finding complained about is now
*wider* in light than the dark pair's was.

That is why UsArr needs no colour-impaired mode. Sonarr had to ship one —
`EnableColorImpairedMode`, help text *"Altered style to allow color-impaired users to better
distinguish color coded information"*, default `false`
([ConfigService.cs](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/Configuration/ConfigService.cs))
— precisely because those apps encode a lot of meaning in colour alone. Building the redundancy in
from day one removes the need for a setting later.

### 3.3 Protocol chips — **there is no chip; the word carries the protocol**

Torrent and usenet are colour-coded consistently across Sonarr, Radarr and Prowlarr:
`torrentColor: '#00853d'` and `usenetColor: '#17b1d9'`
([Sonarr Styles/Themes/dark.js](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/Styles/Themes/dark.js)),
a self-hoster reads that green/cyan split without a legend, and **UsArr reused it and should not
have.**

🚩 **Reversed on measurement: `--status-ok` and the torrent hue are the same green, so protocol
reads as status.** Computed ΔE76 between `--status-ok #1c6b3a` and `--protocol-torrent #0a6b34` is
**4.59** in light and **3.09** in dark — indistinguishable at a 6 px dot, and *worse* in the theme
that otherwise works better. §3 stakes the entire no-brand-accent decision on the claim that
reserving chroma makes **status** legible; a column of green dots on the Requests screen, seen by a
user who has just come from a Home screen where green means "nothing missing", says *torrents are
fine and usenet is something else*. Importing the ecosystem's literals was a good instinct that
collided with the one ramp this design cannot afford to blur.

**Three ways out, and the third is taken.** Retuning the torrent lightness does not help — the
collision is hue, not luminance. Moving the hue away from green is foreclosed by §1.1, which bans
indigo, violet, purple and fuchsia outright, and every other direction lands on the usenet teal or
on the error red. **So the protocol chip loses its colour: the words `torrent` and `usenet` carry
the distinction — which they already do, in the same cell.** That is §1.4's own rule applied to a
fill instead of an icon (*if a label sits next to it and the label alone would be understood, delete
the decoration*), it is "cut before you add" removing two tokens rather than adding a fourth hue,
and it restores §3's argument by leaving chroma to status alone. `Protocol` is still a first-class
filterable field (§16's `source:` tag); it is the *ink* that goes.

🚩 **And then the box went too, because the first pass stopped one element short of its own rule.**
Removing the fill left an 8px `--border-hi` outline — *identical on torrent and on usenet*, repeated
down every row of the column, next to a word that already says which one it is. Apply §1.4's test to
the leftover and it fails exactly as the fill did: delete the swatch and no reader loses anything.
So the swatch is deleted, not merely drained. `.proto__dot` and its spans are gone from the mockup;
`.proto--torrent` / `.proto--usenet` stay on the markup as the filter hook.

📌 **Flagged for Joe as a change of visible character** rather than a defect fix — the tokens
`--protocol-torrent` / `--protocol-usenet` are deleted from `tokens.css`, and restoring them plus a
torrent hue outside the status band is the alternative if he wants the ecosystem's colour cue kept.
⚠️ **That reversal is no longer one line.** It was, while the drained swatch was still on the page;
now it is a rule *and* eighteen spans of markup, because there is no box left to put a hue in. The
cost is small and it is stated rather than left as a stale promise.

Also note the vocabulary decision: the column is labelled **"Protocol"**, matching
Prowlarr, not **"Source"**, which is what Sonarr's Interactive Search calls the same field
([Prowlarr releaseActions.js](https://github.com/Prowlarr/Prowlarr/blob/develop/frontend/src/Store/Actions/releaseActions.js);
[Sonarr InteractiveSearch.tsx](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/InteractiveSearch/InteractiveSearch.tsx)).
Prowlarr is the surface v0.1 mirrors.

### 3.3a Selection and the primary action — the one accent worth adding is a weight, not a hue

**Keep the no-brand-accent rule; it is correct and load-bearing.** But it is currently being asked
to justify three separate things that are not one decision, and two of them are underpowered.
Measured on the light theme: `--selected #e7e3dd` against `--hover #eceae5` against `--surface
#f2f0ec` — **three fills within six units of each other**, so selection is a whisper; and
`.btn--primary` is `--selected` fill plus 600 weight, which is that same six-unit difference and one
weight step, for the one button on the screen.

**The answer is achromatic and costs nothing: give `--selected` and `.btn--primary` a 1 px
`--border-strong` border rather than relying on fill alone.** It fixes selection and the primary
action together, it adds no hue, and it does not reopen §3's decision. That is the *only* accent
this document adopts.

**And an anchor styled as a button carries no underline.** `.btn` never reset `text-decoration`, so
the global `a { text-decoration: underline }` won on nine anchor-buttons — which meant that on every
empty state the primary action was a bold **underlined** label in a box beside a plain label in a
box, so the primary/secondary distinction was being carried by **underline**, which is the link
affordance used everywhere else in the product. Same ink, two meanings, on one screen.
`.btn { text-decoration: none }`.

### 3.4 Theme setting

Light / Dark / Auto, defaulting to Auto, which is Sonarr's shape
(`GetValue("Theme", "auto", …)`,
[ConfigFileProvider.cs](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/Configuration/ConfigFileProvider.cs);
documented at <https://wiki.servarr.com/sonarr/settings>). Navidrome, by contrast, ships dark by
default (`viper.SetDefault("defaulttheme", "Dark")`). **Both themes are audited independently or
neither ships** — a `dark:` prefix sprinkled per element is a named code-level tell and is not how
the token file works.

---

## 4. Typography

**IBM Plex Sans for the UI, IBM Plex Mono for machine data**, both SIL OFL 1.1
(<https://www.ibm.com/plex/>), self-hosted, subset to Latin, served from `embed.FS`, with the
system stack as the fallback **in the same `font-family` declaration**.

### 4.1 Why not Inter, and why not the system stack alone

**Inter is now the single most-named typographic tell of AI-generated UI.** Krebs made "Inter for
centred hero headlines" a scored detector; mania.design calls changing the font *"the single
highest-leverage move against slop"*. And its trendy substitutes are equally a tell — Geist (the v0
signature), Space Grotesk, Instrument Serif and Poppins are named as a rotating set by Krebs,
developersdigest and febbhav alike. Note the fair counter-argument, which is real: the objection to
Inter is about **identity, not legibility** — the same commentary that calls it overused also calls
it *"fantastic for a dense SaaS dashboard"* (<https://yeptype.com/article/inter-alternatives>). It
would work. It is simply the one choice guaranteed to read as defaulted.

**`system-ui` alone is the tempting alternative and it has a specific, disqualifying cost for this
app.** It is free — zero bytes, zero FOUT, zero layout shift, correct hinting for the OS's own
rasteriser, correct fallback for every script the OS supports
(<https://developer.mozilla.org/en-US/docs/Web/CSS/font-family>). **INFERENCE, and it must be marked as one because it is the claim that decides OQ-3:** San
Francisco, Segoe UI and Roboto have different x-heights and different advance widths, so the layout
is not reproducible across platforms — a title that fits a 32 px row on macOS may wrap on Windows.
No source is cited for that, because none was found that measures it; it is a typographic
commonplace, not a finding, and the magnitude of the drift for *this* design has never been
measured. MDN does warn explicitly that `system-ui` "may cause the displayed typeface to be undesirable for some users —
for example, the default Windows CJK font may render Latin scripts poorly." For a design whose
whole premise is fixed row heights and a dense grid, metric drift is not cosmetic.

**IBM Plex is a real, boring, well-hinted technical family.** Designed as a corporate UI/text
family, large x-height, true tabular figures, and a mono companion that pairs with it. It is not on
anybody's tell list, it is not fashionable, and it will not be fashionable next year either —
which is the point.

**The cost, now measured rather than estimated.** The three faces (IBM Plex Sans 400 and 600, IBM
Plex Mono 400), taken as the WOFF2 subsets Google Fonts actually serves and measured by
`Content-Length` on 2026-08-16:

| Subset | Sans 400 | Sans 600 | Mono 400 | Total |
|---|---|---|---|---|
| `latin` only | 44.6 KB | 44.6 KB | 14.4 KB | **103.6 KB** |
| `latin` + `latin-ext` | 74.8 KB | 74.8 KB | 27.4 KB | **177.2 KB** |

So the earlier ~120–180 KB estimate was right for `latin` + `latin-ext` and **pessimistic by about
40% for `latin` alone**, which is what "subset to Latin" in ADR-0025 actually means. At 103.6 KB
over a LAN this is a single-digit-millisecond cost, and WOFF2 is already Brotli-compressed
internally so the build-time precompression adds nothing here. The number that would have failed
the ~200 KB trigger is the one that includes `latin-ext`; a self-hoster with an accented library
will want it, so **the family is decided and the subset is not** (OQ-3) — and six media types sharpen
it, because a manga, classical or translated-fiction library is full of accented and transliterated
titles where a two-type video library was not. If it must be cut, drop
`latin-ext` before dropping a weight, and drop a weight before dropping the family.
Mitigations: the font is served from the same box over a LAN,
it is content-hashed and immutable, and it is precompressed at build time (`statigz`, see
ADR-0025). Set `font-display: block` with a short block period, or `optional` — on a LAN the font
always wins the race and `swap` risks a visible reflow for no benefit.

**Never a Google Fonts `<link>`.** It is both a documented tell and a third-party request in
software whose entire premise is that it runs on your own hardware.

> ✅ **The typeface decision has now been seen — validated 2026-08-16.** The history below is kept
> rather than deleted, because it is what tells a reader how much weight any review dated before that
> day can carry.
>
> **Until 2026-08-16 the prototype shipped zero `@font-face` rules**, so every screenshot, every
> review and every judgement made about this design up to then was rendered in the *fallback* stack
> — measured as **DejaVu Sans and Liberation Mono** on the review host, via a canvas advance-width
> probe (`"IBM Plex Sans"` measured **identical** to a deliberately bogus family name, while
> `document.fonts` reported it "available", which is a false positive worth knowing about). DejaVu is
> roughly **24% wider** than Plex with a taller x-height and a looser default fit, so:
>
> - **Density findings taken before that date are conservative**, not optimistic. Real Plex fits more
>   per column and would reduce wrapping, never increase it — so nothing measured then has to be
>   re-taken to stay safe, and anything re-taken now should only improve.
> - **Hierarchy findings are unaffected** — size ratio, weight and colour are face-independent.
> - **The anti-Inter argument's payload had never actually been delivered.** §1.2 and §4.1 stake the
>   strongest anti-slop claim in this document on the face — mania.design's *"the single
>   highest-leverage move against slop"* — and a build without the font gives a Linux self-hoster
>   DejaVu, a Windows one Segoe UI and a macOS one SF. On the system stack, §1.2's typography row was
>   satisfied by a fallback rather than by a decision. It is now satisfied by the decision.
>
> **What validated it — which is exactly what this block demanded, and nothing less.**
> [`design/mockups/fonts.css`](./mockups/fonts.css) carries the subsetted WOFF2 faces self-hosted as
> base64 data URIs: IBM Plex Sans v23 (the variable font, weight axis 100–700, so 400/500/600 all
> come from one file) and IBM Plex Mono v20 at 400 and 600, `latin` subset, 76 KB of woff2 on disk,
> with the SIL OFL travelling alongside them in `mockups/fonts/OFL.txt`. `docs/design/check.mjs`
> asserts the **rendered** family on every run, by the advance-width comparison this block specified
> rather than by `document.fonts.check()`: `"IBM Plex Sans"` measures **459.000 px** against a bogus
> family's **401.074 px**, `"IBM Plex Mono"` measures **504.000 px**, the body's own computed
> `font-family` measures **459.000 px** — identical to Plex Sans, so the body genuinely renders in
> Plex rather than falling through the stack — and `document.fonts` reports all three faces
> `loaded`. The probe is a standing assertion, not a one-off capture, so a regression that silently
> drops the font fails the self-test instead of quietly restoring DejaVu.
>
> **The family is therefore decided *and* validated, and OQ-3 is now the subset alone.** `REVIEW-LOG`
> D-30 — which held that the mockup rebutted ADR-0025 by shipping no font at all — is resolved by
> this, and resolved by shipping the font rather than by choosing between the two statements.

**The recorded alternative** — and it is a respectable one — is the zero-webfont system stack with
`font-variant-numeric: tabular-nums` on numeric columns, which most system UI faces support. If the
measured font cost turns out to matter more than metric reproducibility, that is the fallback, and
it changes exactly two lines of `tokens.css`.

### 4.2 Mono is semantic, not decorative

Monospace appears **only** where the content is machine data: file paths, indexer names, byte
sizes, hashes, `usarr_id` values, log lines, version strings, and **verbatim upstream error text**
(§17.3's "Problem" column). Mono on prose is the tell; mono on a file path is a feature. A
monospace face at 12 px reads noticeably smaller than a sans at 12 px, so mono runs one step up
where it sits inline with sans text.

**Two things that look like machine data and are not, because the conversion of this tell into a
feature is only complete if the boundary is drawn where the meaning is.** A **taxonomy word** —
`movie`, `album`, `issue`, `episode`, `ebook` in a `Type` column — is an enum rendered as English,
and the `Library` column one cell away holds the same kind of value in sans, so mono there is
inconsistent within a single row. A **user-chosen label** — `Radarr`, `Prowlarr`, `Sonarr Anime`, the
`name` field of §9.3's wizard — is a string the user typed into a settings form and is exactly as
human as `Movies`. Both go to sans at `--text-sm` / `--fg-muted`. And **a single value never carries
two faces**: `ER S12E14 Quintessence of Dust` rendering the episode code in mono grey between two
runs of bold sans puts three typographic treatments in one cell and reads as a rendering fault.

### 4.3 The scale — six steps, hard stop

| Token | Size / line-height | Weight | Use |
|---|---|---|---|
| `xs` | 11 / 16 | 500 | counts, chip labels, table sub-labels |
| `sm` | 12 / 16 | 400 | secondary metadata inside a row |
| `base` | 13 / 18 | 400 | **default: list rows, table cells** |
| `md` | 14 / 20 | 400 | form inputs, dialog body, primary row title |
| `lg` | 16 / 24 | 600 | section headings |
| `xl` | 20 / 28 | 600 | page title — **the largest type anywhere in the application** |

**There is no seventh step.** A 24 px empty-state token existed here and in `tokens.css` as
`--text-empty`, and it has been **deleted** rather than restated: it was larger than the page's own
H1, it applied to the screen with the least information on it, and it was the mechanism by which the
composition §9.6 bans became writable. §9.6 carries the rule and the greppable form of it.

**Below `lg`, hierarchy is carried by weight and by the neutral ramp step, never by size.** This is
the highest-leverage density technique available: a size step costs vertical space
multiplicatively (a 24 px heading costs ~34 px of line box), while a weight step 400→600 and a
colour step `--fg` → `--fg-muted` cost **zero pixels** and produce comparable perceived hierarchy.
It is also the direct answer to the "no real hierarchy" tell — the fix is not *more* size
variation, it is *fewer* sizes used more decisively.

Line-heights are whole multiples of 2 px and pair 1:1 with their size, so text in adjacent columns
aligns. Butterick's readable band is 120–145% of point size
(<https://practicaltypography.com/line-spacing.html>); UI rows at short measure want the low end.
WCAG SC 1.4.12's 1.5 figure is a **robustness** requirement — the layout must survive a user
forcing 1.5 — **not** an instruction to ship it
(<https://www.w3.org/WAI/WCAG22/Understanding/text-spacing.html>). That is why every row height in
§6 is a `min-height`.

**`font-variant-numeric: tabular-nums` on every columnar number** — sizes, seeders, ages, episode
counts, durations, timestamps, IDs — and **never on prose**, where proportional figures read better
(<https://developer.mozilla.org/en-US/docs/Web/CSS/font-variant-numeric>; baseline since January
2020). *INFERENCE: no source names tabular figures as an anti-slop rule — it is a density rule. But
generated UI essentially never does it, so it doubles as a positive craft signal.*

⚠️ **The `lg` step is the one that does not carry, and the fix is not a seventh size.** Measured:
`h1` 20/600/`--fg`, `h2` 16/600/`--fg`, bold row title 13/600/`--fg` — so a section heading differs
from a row title by **3 px and nothing else**, at the same weight, in the same ink, with
`letter-spacing: normal` at every size. On Search that puts `Ebooks` at 16/600 directly above
`Dune Messiah` at 13/600 in identical colour and the heading barely registers as a level. The
argument above — that below `lg` hierarchy is weight and ramp rather than size — is right and is
also where it stops applying: `lg` *is* the boundary, and the boundary is the weak step. **Do not
add a size step.** Add a third signal at `lg` and `xl` only: a 1 px rule the full width of the
section under the `h2` (the pattern already exists on Search and reads well there), and/or
`letter-spacing: -0.006em`, which costs zero pixels and which most grotesques want above 15 px.
Note the related honest correction to §1.2's rebuttal of the "all sizes within 15%" tell: **three of
the six steps — 13 / 12 / 11 — are within 18% of each other** (ratios 1.08 and 1.09) and read as one
size, so "six sizes, hard stop" is a thinner answer to that tell than it sounds. The six steps are
kept; the claim is not oversold.

**Sentence case everywhere.** No Title Case, no all-caps labels, no letter-spaced eyebrows. Buttons
are verbs: "Add", "Retry", "Test connection", "Search indexers".

---

## 5. Space, density and geometry

### 5.1 The unit

**4 px base; scale `2 4 6 8 12 16 24 32 48`.** The 2 and 6 steps are the off-4 values §5.3's row
heights need — 6 px is the standard row's vertical padding, 2 px the hairline gap inside a chip —
and above 16 the scale goes in 8s. Every padding, margin, gap, offset and icon box in the app is one of
those nine values. The rule comes from Müller-Brockmann — every measurement derives from one unit
(*Grid Systems in Graphic Design*, 1981) — and its stated payoff is not aesthetic: information laid
out this way "is not only read more quickly and easily, but is also more easily understood and
retained." **Density is not cramming; it is alignment.** A dense grid that aligns reads faster than
a sparse one that does not.

Cross-check: Carbon's data-table row heights are 24 / 32 / 40 / 48 / 64, all divisible by 4, with
cell type at 14/400 and 16 px horizontal padding
(<https://carbondesignsystem.com/components/data-table/style/>).

### 5.2 Padding differs by role — this is the part that is easy to get wrong

Uniform spacing everywhere is *itself* the tell (mania.design names *"16px radius and 24px
padding"* across a whole screen; febbhav names exclusive `gap-4 / p-6 / my-8`). The values used
must differ by what they separate:

- **Intra-row** (icon to label, chip internals): 2–4 px.
- **Inter-row**: 0 px of gap — separation is a 1 px rule, not whitespace. Rules are the cheapest
  separator; cards are the most expensive container per unit of information, spending a border, a
  shadow, an internal padding box and an inter-card gap to do what one 1 px line does. *(INFERENCE,
  but a direct application of Tufte's data-ink ratio to UI chrome.)*
- **Inter-region** (page sections, form groups): 16–24 px.

Whitespace is the most honest separator and the most expensive. Reserve it for separating
**groups**, not **items**. Group gap ≫ item gap is what makes a dense list scannable; equal gaps
everywhere is what makes it cramped.

### 5.3 Density is a real setting, and compact is what loads

§17.1: *"Density is a feature. More items per screen beats larger cards. The grid is compact by
default, with a size control for people who want otherwise."*

| Setting | Row | Two-line | With thumbnail |
|---|---|---|---|
| **compact — the default** | 28 px | 44 px | 52 px |
| standard | 32 px | 48 px | 56 px |
| relaxed | 36 px | 52 px | 60 px |

Toolbar and sticky table header: 40 px. Sidebar item: 32 px. All of these are **`min-height`,
never `height`** — a fixed height breaks SC 1.4.12 the moment a user forces 1.5 line spacing, and
truncates the second line of a long title.

⚠️ **Those nine numbers are floors, not the heights that render, and §7.4's placeholder sizing
depends on knowing the difference.** Measured on the shipped search screen at compact density there
are **six distinct row heights — 28, 30, 45, 47, 59, 62 px, mean 42.0** — and **eighteen across the
three densities**, because a row whose columns wrap is taller than its floor. Two corollaries. The
`min-height` must sit on an element it applies to: on a `display: table-row` it is inert
(**measured: forcing the token to 100 px left the row at 28.0 px**), which is one more reason §7.4's
list primitive is a grid row. And any code that estimates total list height from these numbers is
wrong by ~33% at 25,000 rows. **§7.4 carries the measured heights that `contain-intrinsic-size`
actually uses, and they are CONTENT-box heights** — post-`440e92d`, **content box 27 / 31 / 35 px**
for a one-line row and **content box 44 / 48 / 52 px** for a rich one, whose border boxes are
28 / 32 / 36 px and 45 / 49 / 53 px respectively. They are per row *shape*, which is the distinction
this warning is about, and the box is named because `contain-intrinsic-size` takes the content box
while every height quoted from a rect is the border box — a 1 px error per row that `auto` then
silently corrects.

🚩 **THE RICH ROW HAS NO SINGLE HEIGHT, AND `45 / 49 / 53` IS TWO DIFFERENT STATISTICS AT ONCE.**
The one-line row is uniform; the rich row is **bimodal**, because rows with more chips wrap. Measured
by the frontend thread at compact over 2,000 rows, the content boxes split **44 px × 1,308 rows and
48 px × 692 rows** — so the *mode* is content box 44 / 48 / 52 and the *mean* is 45.4 / 49.4 / 53.4.
That makes **`45 / 49 / 53` simultaneously the mean content box and the modal border box**, which is
exactly why it has read all this time as one unambiguous number. ⚠️ **The live consequence is a
correction that must not be applied.** `RELEASE_ROW_INTRINSIC` in `web/src/lib/requests.ts` holds
`45 / 49 / 53` and feeds `contain-intrinsic-size`, which takes the content box — so it looks like the
`ROW_INTRINSIC` case one paragraph up (*border box, therefore subtract one, therefore 44 / 48 / 52*)
and it is not. It is **correct as the mean**, which is the statistic a whole-list placeholder wants;
"correcting" it to the mode reintroduces the same class of error in the opposite direction. `list.ts`
carries this warning at the call site; it is carried here because this section is where the figure is
quoted from. **Name the statistic as well as the box whenever the population is not uniform.**

One consequence worth stating because it is easy to violate: SC 2.5.8 requires 24 × 24 CSS px
targets, with a spacing exception for undersized targets whose 24 px circles do not intersect
(<https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html>). **A 28 px compact row
therefore carries no inline clickable icons** — compact rows are for reading. Rows with inline
actions use the relaxed step, where 28 px hit targets fit with clearance.

*INFERENCE, as a review heuristic:* at 1440×900 a library or search screen should show on the order
of **25–40 scannable items above the fold**. If it shows six, it has been designed as a landing
page.

### 5.4 Geometry

- **Radius 0** on rows, tables, toolbars, the sidebar, inputs and buttons. 2 px is permitted on
  inputs and buttons if 0 reads too hard in practice. **4 px** on popovers, dropdowns, dialogs and
  toasts. **Hard ceiling 6 px**, and nothing may reach it without an argument.
- **No `box-shadow` anywhere except popover, dropdown, dialog and toast.** One neutral token. No
  coloured shadow, no glow, no `text-shadow`, no hued `drop-shadow`.
- **No `backdrop-filter`, ever.** Glass costs GPU time on every scroll frame, which contradicts the
  owner's number-one requirement, and it is a named tell besides.
- **Rows and tables are the default container; a card is the exception**, justified only when the
  item's *primary* content is cover art. Service health, search results, release candidates,
  episode lists, the per-instance table on item detail, the audit log — all rows.
- **No bordered surface nested inside another bordered surface.**

---

## 6. Motion

**0 ms on the critical path**: route change, list render, filter apply, sort, tab switch, row
expand, search results appearing. This is §17.1 verbatim, and the architecture is the argument for
it rather than taste.

The reasoning, from the cited numbers: Nielsen's 0.1 s is *"the limit for having the user feel that
the system is reacting instantaneously… no special feedback is necessary except to display the
result"* (<https://www.nngroup.com/articles/response-times-3-important-limits/>). A local-SQLite
render is ~8 ms (§13). Attaching a 250 ms transition to it produces a 258 ms interaction — past the
instantaneous threshold and two thirds of the way to the 400 ms flow limit Doherty and Thadani
argued for (*The Economic Value of Rapid Response Time*, IBM Systems Journal, 1982;
<https://lawsofux.com/doherty-threshold/>) — **purely for decoration**. Matt Ström-Awn puts it most
sharply for tools: under 100 ms, animation in that band actively slows perception, because you have
converted a 0 ms event into a 200 ms one (<https://mattstromawn.com/writing/ui-density/>).

Where motion is still allowed, it is off the critical path and bounded:

| Case | Duration |
|---|---|
| Anything on the critical path | **0 ms** |
| Hover / focus colour change | 80 ms (0 is also fine) |
| Menu, popover, dropdown open / close | 120 ms / 90 ms |
| Dialog in / out | 150 ms / 110 ms |
| Toast in | 150 ms; auto-dismiss ≥ 5 s |
| **Hard ceiling, anything** | **200 ms** |

Exits are shorter than entrances, which is NN/g's observation
(<https://www.nngroup.com/articles/animation-duration/>: a popup "may take 300 ms to appear, but
only 200 or 250 ms to disappear"; and *"at 500 ms, animations start to feel like a real drag"*).

Hard rules:

- **Only `opacity`, `transform` and `color` / `background-color` may be transitioned**, and the
  colour pair only for the 80 ms hover/focus row above. Never `height`, `width`, `top`, `left`,
  `background-position`, `box-shadow`, `border-width`, `text-decoration-thickness` or any other
  property that moves geometry. *(The earlier form of this rule said "only `opacity` and
  `transform`", which forbade the hover-colour change the table two rows up requires and which the
  mockup stylesheet necessarily broke six times. A rule the reference implementation cannot obey is
  a rule nobody will obey.)*
- **One easing, `ease-out`.** No overshoot — no `cubic-bezier` with a control point outside 0..1 on
  Y — no spring, no bounce, no elastic.
- **No scroll-triggered reveals.** (`IntersectionObserver` for lazy *loading* is a different call
  site and is fine.)
- **No hover-lift.** Hover changes colour only. A geometry change on hover produces
  reflow-adjacent jitter in a dense list and makes a 40-row table feel unstable under the cursor.
- **Focus rings appear instantly and are never animated** (Hallmark's rule, and correct).
- **A global `prefers-reduced-motion: reduce` block collapses every duration to 1 ms** and sets
  `scroll-behavior: auto` (<https://developer.mozilla.org/en-US/docs/Web/CSS/@media/prefers-reduced-motion>).
- **No animation library in `package.json`.** Everything above is CSS transitions.
- **No View Transitions API on navigation.** By construction it inserts a rendering pause plus an
  animation between the click and the usable screen
  (<https://developer.mozilla.org/en-US/docs/Web/API/View_Transition_API>). If navigation is already
  sub-50 ms because the data is local, a view transition makes the app measurably slower while
  making it look designed. That is exactly the flair §17.1 rules out.

---

## 7. Loading and latency policy

Four tiers, keyed to **where the data lives**, not to how long anyone guesses it will take — plus
one budget that is deliberately *not* a tier, because the interaction it governs fetches nothing and
so has no data location to key on (§7.2, **Controls**). And one headline rule: **there are no
skeleton screens anywhere in UsArr.**

### 7.1 Why no skeletons

The evidence is weak and contested, and both studies are small — this is stated plainly rather than
hidden behind a preference.

- **Viget (2017)**, n=136 (39 skeleton / 39 spinner / 58 blank), identical load duration on a
  meal-listing UI (<https://www.viget.com/articles/a-bone-to-pick-with-skeleton-screens>). **The
  skeleton lost on every measure**: mean perceived wait 2.82 s vs 2.41 s spinner and 2.29 s blank;
  "loaded quickly" agreement 59% vs 74% and 66%; post-load task time 10.54 s vs 9.49 s and 9.50 s.
  Caveats worth holding: unbalanced groups, an MTurk-heavy population, one content type, and the
  actual wait duration is not stated.
- **Chung (2018)**, n=80 + n=80 (<https://uxdesign.cc/what-you-should-know-about-skeleton-screens-a820c45a571a>)
  found the opposite direction but conceded *"the superiority of skeletons over spinners is
  minor"*, and found **spinner users were faster at finding the target on first entry**. Its most
  useful conclusion is the one that reconciles the two: *"skeleton screens should not block gradual
  content loads"* — *"real content should replace skeleton objects immediately when the data is
  available."* Most shipped skeletons behave as splash screens with a minimum display time, and a
  skeleton that is artificially held is strictly worse than nothing.
- **NN/g** (<https://www.nngroup.com/articles/skeleton-screens/>): **under 1 second, show nothing**
  — *"if a page takes less than 1 second to load, skeleton screens or spinners aren't necessary, as
  they likely won't make a difference to the users' experience."* Spinners are for **a single
  module**; skeletons are for full-page loads; **beyond 10 seconds, progress bars are strongly
  recommended**. NN/g also warns against frame-display skeletons that snap to content.

The replica architecture removes the only case a skeleton would serve — a full-page load of remote
data. So the policy is not austerity; the case simply does not arise.

**This is not in tension with §17.1's `dominant_color` placeholder.** That is an *image
placeholder*: a reserved box carrying the cover's own average colour, present because §4.4.1 makes
`dominant_color` available before ThumbHash. It is the item's real data rather than a stand-in
animation — it never pulses, and what it replaces is a grey box, not the title. The title is not in
it; §9.2 puts the title and year below the tile, on the chrome's own ground.

### 7.2 The four tiers

**Tier 0 — the data is in local SQLite. Nearly every read.**
Show **nothing at all**. No skeleton, no spinner, no fade-in. Render the content. Target **< 50 ms
click-to-paint; hard fail at 100 ms.** A local read over 100 ms is a **query-plan bug**, and it
belongs in the `EXPLAIN QUERY PLAN` assertions `make check` runs (§13 of ARCHITECTURE), not behind
a spinner.

**Tier 1 — 100 ms to ~1 s, and the wait is unavoidable.** First SPA boot; a cold cover-art fetch.
No indicator. **Progressive content**: render the shell, the nav, the headers and every field
already held, and let the missing pieces resolve in place.

> ⚠️ **On a *cold* boot that instruction is not achievable as written, and it has now been raised
> twice.** Under ADR-0003's `adapter-static` with no server render, **the shell *is* the
> JavaScript**: on a first visit, a hard refresh with an empty cache, or a phone on slow Wi-Fi, the
> document is blank until the bundle downloads, parses and mounts, so there is no shell to render
> progressively into. Tier 1 quietly assumes a warm cache, and it is the one tier covering the run
> ARCHITECTURE §4.4.1 calls *"the run that forms the speed opinion"*. **Six media types made it
> worse in a measurable direction**: the sidebar now carries six typed entries with counts plus the
> scope chip, and Home is three blocks rather than one strip, so the static chrome the user could
> have been looking at went up while the blank window did not change. **The fix is unchanged and
> still cheap: inline the static shell — top bar, sidebar skeleton with the type list, page-title
> slot — as real HTML into the built fallback `index.html`, and let the SPA hydrate into it.** That
> is a build-step change before the first UI commit and an expensive retrofit afterwards. It remains
> **§D3 item 6, open for Joe** — it is a build-config decision under ADR-0025 rather than one this
> log may take. For images, reserve the box with
`aspect-ratio` plus the `dominant_color` fill so nothing shifts, and let the image paint when it
arrives.

**Tier 2 — 1 s to 10 s, genuinely remote.** A Prowlarr free-text indexer search; an \*Arr command
round trip; a health probe of a configured service. Show a **scoped inline indicator on the element
that is waiting** — a 2 px indeterminate bar under the search field, a small spinner inside the
button that was pressed, a `pending` chip on the queued row. **Never a full-page overlay**, and the
rest of the screen stays live and interactive. **Delay the indicator by 150 ms** so a fast response
never flashes it. This is the one place a deliberate delay is correct, because it prevents a
flicker rather than hiding speed.

**Tier 3 — over 10 s, or a known N-of-M.** Indexer fan-out across nine indexers; the initial full
import. **Determinate progress with real counts**: "4 of 9 indexers responded", "1,240 of 10,000
comics". **Never a fake bar.** Partial results are usable as they arrive — §8.4 already requires
per-indexer streaming over SSE, and §17.7 already requires home sections to populate live as import
phase A commits.

**Controls — a user-initiated change to presentation, which fetches nothing.** The density toggle
and the theme toggle are the current members, and the category is closed to interactions that touch
data: the moment a control triggers a read, the read is governed by its own tier above. **Target
< 100 ms; hard fail at 400 ms.** §7.2 covers these, and that sentence is here so nobody has to reach
the category by inference again.

> ⚠️ **This category exists because the four tiers above did not contain it, and three documents
> asserted for months that they did.** §7.4, ADR-0029 and `ARCHITECTURE.md` §4.5 each called the two
> toggles *"pure-local no-data interactions"* and concluded they were *"Tier 0 by §7.2's own
> definition, whose hard fail is 100 ms"* — but Tier 0 reads, verbatim, *"the data is in local
> SQLite. Nearly every read,"* and its stated diagnostic for a breach is *"a query-plan bug"*
> belonging in the `EXPLAIN QUERY PLAN` assertions. **A control that touches no data has no query
> plan to be a bug in.** The budget was extended to controls by the documents that wanted a budget
> rather than by the document that owns latency tiers, which is the defect this block repairs.
> **Tier 0 is unchanged**: 100 ms still stands for reads, exactly as written.

**The derivation, which does not depend on what the toggles currently cost.** §6 already fixes both
ends of this from primary sources, and the argument runs from them rather than from a measurement.
Nielsen's **0.1 s** is *"the limit for having the user feel that the system is reacting
instantaneously… no special feedback is necessary except to display the result"*
(<https://www.nngroup.com/articles/response-times-3-important-limits/>); **400 ms** is the flow limit
Doherty and Thadani argued for (*The Economic Value of Rapid Response Time*, IBM Systems Journal,
1982; <https://lawsofux.com/doherty-threshold/>), and §6 already treats it as this document's flow
threshold when it rejects a 250 ms decorative transition for landing *"two thirds of the way to"* it.
**A rare, deliberate, user-initiated control is exactly the case where the flow limit governs and the
instantaneous one does not.** The user asked for the change, is expecting it, and is not mid-thought
waiting on data that has not arrived — so falling short of *instantaneous* costs polish, while
crossing the *flow* limit costs the user their place. Hence: aim at the instantaneous threshold, fail
at the flow threshold.

📌 **Two guards on that reasoning, because a threshold is only worth as much as its discipline.**
First, **the 400 ms is §6's own number, not the nearest available one** — §7.1's *"under 1 second,
show nothing"* (<https://www.nngroup.com/articles/skeleton-screens/>) is a rule about *when a loading
affordance is worth showing*, not a budget for how long an interaction may take, and reading it as
the latter is the same category slip this block was written to fix. Second, **no number here was
derived from what the toggles measure today.** The cost curve lives in ADR-0029 and moves with the
markup; this budget is a claim about the user, and it would read the same if the toggles were free or
if they were ten times worse.

**The consequence, stated separately — because it is a consequence of the budget and not its
purpose.** 📏 **Measured by the frontend thread, not by this one**, and cited with its instrument and
its tree because §7.4's standing rule for this row requires exactly that. The record is
`web/scripts/measurements/2026-08-17-density-invalidation.md`, added by **`dff20fd`**; it measures
**tree `3ff8151` plus that change**, on **Chromium 141.0.7390.37 headless** via `playwright-core`
1.56.1, **Node v22.22.2**, viewport **1440×900**, against the harness rich release row. The machine
is an **x86-64 container, 4 vCPU (Intel Xeon @ 2.80 GHz), 15 GB RAM, shared host** — which that
record calls *"a reasonable proxy for a ThinkCentre under Proxmox"* and, in the same breath,
***"not a proxy for a Pi 5"***. Each figure is one density change through the real product path,
five samples, median, on the *with-invalidation* path that holds scrollbar error at 0.00%.

| Rows in the page | 100 | 120 | 160 | 200 |
|---|---|---|---|---|
| Density toggle, shipped path | **32.1 ms** | **37.4 ms** | **49.3 ms** | **75.7 ms** |

**Every page size in that range clears the 400 ms hard fail outright on that instrument.** 🔍 **And
the shipped 200-row default still clears it with ADR-0029's pessimistic 5× Pi-5 multiplier applied —
378.5 ms, 21.5 ms under the limit — which is inference, not measurement**: the multiplier is
ADR-0029's, the source record scaled nothing to a Pi and says so in as many words, and §13 forbids
quoting a Pi-derived figure as a measured one. ⚠️ **That is about 5% of margin, and it is written
here rather than rounded away**, because
a budget a real configuration clears by 21 ms is a budget doing work. A threshold chosen to make the
measurement pass would not have been chosen this tight. ℹ️ Two limits on the row above, from the
source's own §6: this runner is noisy, 100 and 120 overlap and **must not be read apart**, and the
trend — a factor of 2.4 across the range — is what it resolves. That is enough for a budget check and
is *not* enough for a page-size decision, which the source declines to make and so does this section.

🚩 **What the old framing would have implied is the strongest single piece of evidence that it was
wrong.** Tier 0's 100 ms under the same 5× multiplier is a **20 ms** desktop-equivalent budget — a
figure the measurement file names in its own §7 — and against it **every page size in the measured
range fails, 100 rows included at 32.1 ms**, by 1.6× at the smallest setting and 3.8× at the shipped
one. **A rule that fails at every available setting is not a strict rule, it is a misapplied one**,
and the four tiers never claimed this interaction in the first place.

### 7.3 Cross-cutting rules

1. **Never impose a minimum display time** on any loading affordance. That is the failure mode both
   studies point at.
2. **Never blank content you already have on refetch.** Keep the old rows on screen with a subtle
   staleness cue. Empty-then-refill is the single most common cause of an app *feeling* slow while
   being fast.
3. **"Acknowledged, not optimistic."** For remote commands, write and render a real local `pending`
   row from the write queue — instant *and* true, because "I have queued this" really did happen
   instantly and locally. A failure is then a **status change on a visible row** rather than the
   rollback of a lie. This is exactly ADR-0012a's model expressed as a UI rule, and it is why the
   chip must distinguish `verifying` ("might have landed", 15-minute TTL) from `failed_rejected`
   ("definitely did not"). True optimistic UI is reserved for **local, reversible, idempotent**
   state. Every mutation carries a client-generated idempotency key.
   *Background:* <https://tanstack.com/query/latest/docs/framework/react/guides/optimistic-updates>;
   note React's `useOptimistic` has **no automatic rollback**
   (<https://react.dev/reference/react/useOptimistic>); Linear's model is a synchronous local write
   with the sync engine owning durability
   (<https://linear.app/blog/scaling-the-linear-sync-engine>).
4. **A missing service is not a loading state.** Render the section immediately with one line
   stating what is missing and the link that fixes it. A permanently spinning indicator for an
   unconfigured service is the worst available outcome, and it is precisely what principle 3
   exists to prevent.
5. **Preserve position on back-navigation.** Baymard's testing found that failing to return users to
   their prior position in a list is *"a direct cause of site abandonments"*
   (<https://baymard.com/blog/return-same-place>); 59% of sites violate back-button expectations in
   at least one of four patterns (<https://baymard.com/blog/back-button-expectations>). Concretely:
   **filter, sort, search and page count live in the URL** (so back/forward, reload and sharing all
   work for free); **scroll offset and the focused row's KEY — not a pixel offset — live in
   `history.state`**, restored **after** the data is in the DOM. *INFERENCE, and the reason this is
   in a speed document at all: the user's cost model is intent-to-intent, not click-to-paint. A
   0 ms navigation that dumps you at the top of a 400-row list is perceived as slower than a 300 ms
   one that puts you back where you were.*
6. **Preload on intent.** Keep SvelteKit's `data-sveltekit-preload-data="hover"` default on
   `<body>` (<https://svelte.dev/docs/kit/link-options>) — SvelteKit's own framing is that it buys
   "an extra couple of hundred milliseconds — the difference between a user interface that feels
   laggy and one that feels snappy." Add `preloadData()` on `focusin` so keyboard users get the
   same head start, and `data-sveltekit-preload-code="viewport"` on the primary nav. Skip
   Speculation Rules (an MPA technology; the document never navigates after boot) and quicklink
   (viewport prefetch wastes local server work on a LAN where the network is not the bottleneck).

### 7.4 Long lists — and an open conflict with ARCHITECTURE §4.5

**Settled, 2026-08-16: the owner delegated this and it is now ADR-0029.** The default is **"Load
more" plus `content-visibility: auto` with `contain-intrinsic-size`**, and virtualization is an
**escalation above a threshold set from a benchmark that does not exist yet** — deliberately not the
"~1,000 rows" this document previously floated, because the finding against §4.5's "~200" was that it
had no measurement behind it, and answering an unmeasured number with a different unmeasured number
concedes the argument while pretending to fix it. `make bench` gains the measurement (ARCHITECTURE
§4.5, §13); the threshold is whatever it says. **That harness is `pnpm bench:list`, and it is what
supplies every measured number below — the harness is authoritative for what it measures and where
it stops, so read it rather than a summary of it here. ADR-0029's 2026-08-16 amendment carries what
it has already settled, including a measured density-toggle cost curve and a default page size of
200 rows, and ADR-0029 is authoritative for the virtualization threshold itself.**

🚩 **The list primitive is a grid, not a table, and that is a constraint rather than a preference.**
`content-visibility: auto` is defined entirely in terms of size, layout and paint containment, and
**CSS Containment Module Level 2 excludes internal table boxes from all three**
(<https://drafts.csswg.org/css-contain-2/>, fetched 2026-08-16): *"giving an element **size
containment** has no effect if any of the following are true: … if its principal box is an
**internal table box**"*, with the same exclusion for layout and paint containment *"other than
table-cell"*. A `<tr>` is `display: table-row` — an internal table box, not a table-cell — so the
declaration applies, reads back as `auto`, and does nothing. **Measured in Chromium at 5,000 rows:
document height 120,000 px with the declaration and 120,000 px without, identical, against the
140,000 px a working placeholder would give; the same test on `<div>` rows produces the expected
185,000 px.** `<tbody>` is an internal table box too, so chunking does not rescue it, and `<td>`
takes containment but collapses to 9 px cells.

**So a UsArr list row is a `display: grid` element with `role="table"` on the container,
`role="row"` on each row, `role="columnheader"` on the header cells and `role="cell"` on the rest.**
The `.tbl--stack` fork below 760 px already builds half of this, so this is convergence, not a new
component. **The obligation it creates is not optional and is a required component test:** an ARIA
grid must carry by hand every association a native `<table>` gives for free — the roles above,
header-to-cell association, `aria-rowcount` / `aria-colcount` where the rendered rows are a window
onto a larger set, and column names that survive the stacked view where the header row is not
rendered at all. And **`make bench` asserts the mechanism rather than assuming it**: set
`content-visibility: auto` on a row and assert the container's `scrollHeight` **differs** from the
uncontained case.

✅ **`contain-intrinsic-size` has measured values now, and the "no value yet" caveat that made this
section a direction rather than a rule is withdrawn.** 📏 **The numbers in this block were measured
by the frontend thread's `pnpm bench:list`. They are not this thread's measurements — we did not run
them** — and they are recorded here because §7.4 is where the containment policy lives.

📌 **STANDING RULE FOR THIS ROW, because three separate threads have now predicted its behaviour and
been wrong: anything that touches it is measured on BOTH FORKS (`two-line` and `labels`) at ALL
THREE DENSITIES, and every number is recorded WITH ITS BOX.** Not one fork and an inference about
the other; not a plausible mechanism instead of a measurement.

🚩 **EVERY FIGURE IN THIS SECTION NAMES ITS BOX, and that is not pedantry — it is the repair for a
defect this section already had.** The same three digits, **28 / 32 / 36**, were true of the shipped
component's one-line row as its **content** box *before* the `.stacksep` fix and of the mockup's
one-line row as its **border** box throughout — and the table below used to print them under a
heading that named neither while a paragraph further down called them "the rendered border-box
heights". ⚠️ **Post-`440e92d` those same digits are the component's *border* box and its content box
is 27 / 31 / 35, so the sentence stayed readable while half of it went false.** Nothing about a bare
`28 / 32 / 36` looks stale, which is exactly why a wrong one survives a reading. **A row height
quoted without its box is not a measurement.**

📎 **The box rule here, and §7.2's requirement that a figure be cited with its instrument and its
tree, are two instances of one general rule that `docs/DEVELOPMENT.md` §11 rule 5 now owns and
states once for the whole repo.** Read it there; it is deliberately not restated in this document.

✅ **POST-FIX, RE-MEASURED ON BOTH FORKS AT ALL THREE DENSITIES.** The frontend thread's `.stacksep`
margin fix is on `main` at **`440e92d`**, and the pre-fix figures this table used to carry have been
replaced by a measurement of the merged tree. Six configurations, four numbers each, one-line rows at
1440×900, `getBoundingClientRect().height` with `content-visibility` forced visible; *natural* means
the same row under a forced `min-height: 0`, so it is the height the content produces with the floor
taken away.

| One-line row, shipped component | compact | standard | relaxed | floor |
|---|---|---|---|---|
| `two-line` — **border box, as rendered** | **28 px** | **32 px** | **36 px** | binds |
| `two-line` — **content box, as rendered** | **27 px** | **31 px** | **35 px** | binds |
| `two-line` — **border box, natural** (`min-height: 0`) | 27 px | 31 px | 35 px | — |
| `two-line` — **content box, natural** (`min-height: 0`) | 26 px | 30 px | 34 px | — |
| `labels` — **border box, as rendered** | **28 px** | **32 px** | **36 px** | binds |
| `labels` — **content box, as rendered** | **27 px** | **31 px** | **35 px** | binds |
| `labels` — **border box, natural** (`min-height: 0`) | 27 px | 31 px | 35 px | — |
| `labels` — **content box, natural** (`min-height: 0`) | 26 px | 30 px | 34 px | — |

**The two forks are now identical in every box at every density, and `min-height: var(--row-h)` binds
on all six configurations** — 1 px of it, since the natural border box lands one under the floor. The
row's own padding is `0` and its border is `1 px` (padding lives on the `<td>`), so *content box =
border box − 1* throughout, and the natural content box is exactly
`2 × var(--row-pad-y) + var(--leading-base)` — **4/6/8 px doubled plus a fixed 18 px leading = 26 / 30 / 34**.

⏳ **The rich row did not move, and that is measured rather than carried over**: on both the pre-fix
and post-fix trees it is **border box 45 / 49 / 53 px, content box 44 / 48 / 52 px**, with the floor
**slack** on it at every density. ⚠️ **Those are the MODE, and the rich row is bimodal** — rows with
more chips wrap. At compact the split is 45 px × 128 and 49 px × 72 in this run's 200-row sample, and
44 px × 1,308 against 48 px × 692 content box in the frontend thread's 2,000-row one; the **mean**
content box is 45.4 / 49.4 / 53.4. So `45 / 49 / 53` is the modal *border* box here and the mean
*content* box in `RELEASE_ROW_INTRINSIC`, which is the collision §7.4 flags: two statistics wearing
the same three digits.

🚩 **Why the fix passes the rich row by is worth writing down, because the obvious answer is wrong
and was written here first.** It is *not* that some other cell is taller: measured at compact, the
rich row's tallest cells are the numeric second-line cells at **`<td>` border box 44 px** — the very
cells the margin was landing on — while the actions cell with its 32 px `<select>` is only 32 px.
The real reason is that **in a rich row the `.stacksep` has no element sibling to give the margin
to.** `.tbl td > * + *` matches elements; in the rich Size cell the separator's `·` is followed by a
bare **text node**, so the adjacent-sibling rule never fired there on either tree. In the one-line
row the same cell renders `<span class="stacksep">` followed by `<span class="trunc">`, and that
`.trunc` measured `margin-top: 2px` pre-fix and `0px` post-fix — the only cell in the row that
differed, taking its `<td>` to 28 px against 26 px for all six others. **The stray margin never
reached the rich row at all; it is not that the rich row absorbed it.**

🚩 **The pre-fix values are kept here, as a control rather than as history, because the fix is only
legible against them.** Measured on `3ae0d44^`, the same script, the same six configurations:

| One-line row, PRE-FIX tree | compact | standard | relaxed | floor |
|---|---|---|---|---|
| `two-line` — **border box, as rendered** | 29 px | 33 px | 37 px | **inert** |
| `two-line` — **content box, as rendered** | 28 px | 32 px | 36 px | **inert** |
| `labels` — **border box, as rendered** | 28 px | 32 px | 36 px | binds |
| `labels` — **content box, as rendered** | 27 px | 31 px | 35 px | binds |

**The `labels` fork is byte-identical across the fix in all four numbers**; only the `two-line` fork
moved, by 1 px, from a natural height that cleared the floor to one that sits under it. **So the fix
did not make the rows shorter so much as hand the height back to the floor that was always meant to
set it.**

⚠️ **"28 / 32 / 36" appears four times in the two tables above and means something different each
time**, which is the whole reason every figure here names its box: it is the `two-line` fork's
**content** box before the fix and its **border** box after, and the `labels` fork's **border** box
on both sides. A digit that survives a change while its meaning moves underneath is a staleness that
never looks stale. Do not update any of these numbers without also updating which row they sit in.

Scrollbar drift over a full scroll at the one-line values is **0.76 / 0.70 / 0.65%** against the 2%
budget stated below, so all three densities clear it with better than a factor of two in hand.

✅ **All six figures are confirmed on a genuinely-loaded IBM Plex, and the reason they are stable is
worth more than the confirmation.** The bench that produced them had been running without the
webfont — its Vite root declared no `publicDir`, so `app.css`'s `@font-face` URLs 404'd for the
harness's whole life. Re-measured at 2,000 rows against the real `List.svelte` and `app.css`, with
the face verified by canvas advance-width probe rather than by `document.fonts.check` alone, **every
number is byte-identical with the face served and with it blocked**: at the time, one-line **content**
box 28.0 / 32.0 / 36.0 on the pre-fix `two-line` fork, rich rows rounding to **border** box
45 / 49 / 53. Nothing is provisional.

✅ **The post-fix re-measure above is the same null result reached from the other side, which is
worth more than repeating it.** That run served the face (the harness's server answers `/fonts/`
out of `web/static`, which the bench's own Vite root cannot), and the probe confirms it drew — IBM
Plex advance 218 px against the fallback's 221.806 px on the same string, so the two conditions are
genuinely distinguishable. The digits are the ones the earlier run got with the face **blocked**.
The webfont does not move this row in either direction.

**The mechanism is `body { line-height: var(--leading-base) }` being a fixed 18 px *length* rather
than a unitless multiplier**, so glyph metrics cannot move the line box — the missing font could not
have moved these numbers even in principle. 🚩 **And the null result is trustworthy because the
guard was fired rather than assumed:** forcing `line-height: normal` on the row *does* split the two
conditions (rich rows 43 / 47 / 51 served against 39 / 43 / 47 blocked). A probe that cannot
distinguish the conditions it is testing proves nothing; this one can.

⚠️ **Scope, stated because it bounds the claim: the row shape measured is the Search-and-Grab release
columns**, the widest list v0.1 ships and the one ADR-0029 was originally measured against, in its
one-line and rich variants. **Do not generalise these figures past the shape they were measured on.**
What is no longer a caveat is the fork: both are measured, at all three densities, in both boxes.

✅ **THE `labels` FORK'S DISPUTE IS RESOLVED, AND NOBODY WAS WRONG.** Three figures were standing for
that fork — this section's **26 / 30 / 34**, the frontend thread's **border box 28 / 32 / 36**, and a
third measurement's **content box 27 / 31 / 35** — and the reason they could not be reconciled is
that they are three different quantities, only one of which ever said so. Measured, they are all
three correct, simultaneously, on both sides of the fix:

| Figure, as it was reported | What it actually measures |
|---|---|
| **26 / 30 / 34** (this section, box unnamed) | the **natural content box** — `min-height` removed. It is `2 × --row-pad-y + --leading-base` exactly, and it is *below* the floor, which is precisely the mechanism this section attributed to it |
| **27 / 31 / 35** (a separate measurement, "content box, before the fix") | the **content box as rendered**, floor live |
| **28 / 32 / 36** (the frontend thread, "border box") | the **border box as rendered**, floor live — the same row as the line above, plus its 1 px border |

**So this section's 26 / 30 / 34 was never in conflict with the frontend thread's 28 / 32 / 36; it
was 2 px below it because it was two boxes below it** — one border and one `min-height` — and the
sentence carrying it was right about the floor binding for exactly the right reason. It was a
measurement missing its label, not a wrong measurement. 🚩 **The trap worth keeping is the middle
row: 27 / 31 / 35 is the rendered content box AND the natural border box, two different quantities
with the same three digits at every density.** Naming the box alone does not disambiguate it; the
floor condition has to be named too, which is why the table above states both.

**The declaration is not written as any of those numbers**, because `contain-intrinsic-size` sizes
the **content** box — the row's 1 px bottom border is added on top of whatever it says, and on the
shipped primitive the cell padding sits *inside* the row's content box rather than outside it
(ADR-0029's correction 1). The mockups compute it instead, as
`auto calc(2 * var(--row-py) + var(--row-lines) * var(--lh-base))`: the padding term tracks the
density token rather than hard-coding three constants, and `--row-lines` is declared per list from
that list's own rendered rows. ✅ **Checked rather than assumed, because agreement is worth
confirming and not worth guessing at — and note which box each side of the agreement is in:** at
1440×900 the mockups render a one-line row at **border box 28 / 32 / 36 px** and a rich one at
**border box 45 / 49 / 53 px**. ⏳ **That sentence used to add "which is the component's
*content*-box figure for the same digits at one-line", and post-`440e92d` it is the component's
*border*-box figure at one-line too** — the mockup and the component now agree border box to border
box on both shapes, where before the fix the one-line agreement was mockup-border against
component-content. The agreement is real and it is not an identity; with the
computed
placeholder at `auto 27.98 / 31.98 / 35.98px` and `auto 45.08 / 49.08 / 53.08px` respectively, each
within 0.3% of the border-box height it stands in for. The mockups already carry the measured values;
nothing needed changing there.

🚩 **`contain-intrinsic-size: auto` remembers a size measured at the *previous* density, and that is
a live correctness bug for every list this design is about to have.** Reported by the frontend
thread's bench **as new — they could not find it documented anywhere, upstream included**, so treat
it as a finding of this project rather than as received wisdom. `auto` means "use the last real
measurement in place of the estimate", and **a keyed `{#each}` reuses row nodes** across a
re-render. So after a compact → relaxed density switch the reused rows still carry the remembered
*compact* size, every skipped row is placeheld one density stale, and **the scrollbar is 14.57%
wrong — against 0.65% when the same rows are rebuilt rather than reused.** Neither precondition is
exotic here: the density control is a first-class setting on every screen (§5.3), and a keyed
each-block over a stable row id is the correct way to write these lists — §9.1a clause 4 requires
the key. Any list that both reuses nodes and changes row height hits this, which is all of them.

**The rule, and it is required rather than advisory: when row height changes, the remembered
intrinsic size must be invalidated.** Concretely, a density change must either **rebuild the affected
row nodes** — key the each-block on `id + density`, or wrap the list body in `{#key density}`, so the
rows are new elements with nothing remembered — or **force re-measurement of every mounted row before
the next paint**. One or the other, on every list, not "where it matters": a list that reuses nodes
across a density change and does neither is wrong, and it is wrong in the one dimension this whole
section exists to protect.

**Any measurement of the density toggle reports both numbers — what the toggle costs and the
scrollbar error it leaves behind — and neither one alone settles anything.** A fast toggle that
leaves the scrollbar 14.57% wrong is not a fix, and a correct toggle nobody can afford is not one
either; optimising whichever half is easier to measure is the failure this requirement exists to
prevent. **The corollary is that rebuild is the baseline to beat:** forced re-measurement, the other
permitted option above, wins only by matching rebuild's *scrollbar-error* figure and not merely its
cost — otherwise "cheaper" reads as "better" while the correctness half quietly regresses.

⚠️ **Where that rule is enforced, and the one place it cannot be.** It cannot be asserted by
`docs/design/check.mjs`. The bug needs node **reuse** across a density change, plus
enough rows for the drift to exceed threshold; the check's target is `prototype.html`, which is
static HTML with no reuse semantics and no list at that scale, so **an assertion written there could
not reproduce the condition and would pass for ever** — a rule that can never fire, indistinguishable
from a rule that passes, which is the exact failure shape this repository caught three times in a
single day. Enforcement therefore belongs to **`pnpm bench:list`**, the only harness that mounts a
large keyed list. **Threshold: fail above 2% drift** — the same
budget `contain-intrinsic-size` is already held to rather than a second number invented for this,
and the two cases sit either side of it with room to spare (0.65–0.76% rebuilt, 14.57% stale).
**The honest sequencing is fix, then assert, then call it enforced:** a harness that cannot complete
a full run cannot host the assertion either, so whatever stops it is fixed first, the assertion
lands second, and nobody writes "the bench asserts this" until both are true — `bench:list` itself
is the record of which of those it has done. If the app target later grows large-list mounting,
moving the assertion into `check.mjs` is a small change and nobody would object — the split is
*where the condition can exist*, not a territorial line.

**And the earlier prescription — `contain-intrinsic-size: auto var(--row-h)` — was wrong three ways.**
Kept here because each one is a way to arrive at a wrong placeholder again:

1. **`--row-h` is inert on the element it describes.** `min-height` does not apply to
   `display: table-row`; measured, forcing `--row-h: 100px` leaves the row at **28.0 px**, and the
   density control works only through the padding token. So the ADR derived the placeholder from
   the one property with no effect on the real height. The grid-row primitive above fixes this as a
   side effect, since `min-height` does apply to a grid item — but the table below must then be
   read as what it is.
   ✅ **Confirmed on the shipped grid row, and the guard was fired rather than assumed — twice, and
   the first firing was itself wrong.** Forcing `--row-h: 100px` moves every one-line row to
   **border box 100 px, content box 99 px**, on both forks at all three densities, so the property is
   live where it was inert on the `<tr>`. 🚩 **But the first attempt at that guard set the override on
   `<html>` and nothing moved, which would have read as a clean null result and was not one:**
   `List.svelte` stamps `data-density` on the list container, and §5.3's density blocks match a bare
   `[data-density]` as well as `:root`, so the container **re-declares `--row-h` on itself** and an
   inline override on the document element never reaches the rows. Anything overriding a density token
   for a measurement has to do it on the list, not on the root.
   ⚠️ **And "live but *slack*" was true when it was written and is now false, which is the point of
   re-measuring rather than re-reading.** It described the `two-line` fork **before** the `.stacksep`
   fix, where the natural height cleared the floor by 1 px so `min-height: 0` changed nothing. **After
   `440e92d` the floor binds on both forks at all three densities**: natural border box 27 / 31 / 35
   against a floor of 28 / 32 / 36. So the one-line border box *is* now produced by `--row-h`, and the
   arithmetic accident has become the mechanism. **A sentence of the form "a one-line row's content
   box comes out at exactly `--row-h`" remains false as written** — post-fix the content box is
   `--row-h − 1` on both forks — and the older reading, "the row is `--row-h` tall because the floor
   says so", has gone from a coincidence to the truth about the **border** box only. Name the fork,
   name the box, name whether the floor binds, or say neither.
2. **Row heights are not the six values in §5.3's table.** Measured on the shipped search screen at
   compact density there are **six distinct heights — 28, 30, 45, 47, 59, 62 px, mean 42.0** — and
   **eighteen across the three densities**, because real rows wrap. Estimating 25,000 rows at 28 px
   understates the scroll height by ~350,000 px (33%), resolving as the user scrolls: the drift
   itself.
3. **`contain-intrinsic-size` sizes the *content* box.** Padding and border are added on top; a
   24 px row with `auto 28px` produced a **37 px** placeholder.

**What ships: `contain-intrinsic-size: auto <measured content-box height>` per row shape**, with
`auto` remembering the last real measurement **and the invalidation rule above wherever row height
can change**, and the assertion is drift rather than frame time —
`|scrollHeight after a full scroll − scrollHeight at load| / scrollHeight < 2%` at 1k / 5k / 25k
rows, both themes, all three densities. The measured heights at the top of this block are what that
`<measured content-box height>` resolves to, so §7.4 is now an implementable rule rather than a
direction.

✅ **Two confirmations from the same bench run, because the policy rests on them rather than on the
mechanism being obvious.** First, **containment is live on the grid-row primitive**: at 5,000 rows
with a deliberately wrong placeholder the contained and uncontained scroll heights differ by
**761,316 px**, while the same rows forced back to `display: table-row` differ by **exactly 0** —
the `<tr>` limitation reproduced as a control, which is what makes the positive result mean
something. Second, **the density toggle's cost is mostly buyable**: at 5,000 rows, **containment
accounts for about 88% of it and scoping the density attribute to the list container for about
25%**, taking the extremes from **911 ms to 80 ms**.

⚠️ **And the expensive operation on a long list is not scrolling.** Measured: scrolling costs
**0.1–0.3 ms** at every size, while the **density toggle** costs **153 ms at 1,000 rows, 1,199 ms at
5,000 and 6,508 ms at 25,000**, and the **theme toggle** 1,356–4,514 ms at 25,000 — because each
sets an attribute on `<html>` and invalidates every element reading a custom property. Both are
top-bar controls on every screen, both are pure-local no-data interactions, and both are therefore
governed by **§7.2's *Controls* budget — target < 100 ms, hard fail at 400 ms**. ⚠️ **This sentence
used to read *"Tier 0 by §7.2's own definition, whose hard fail is 100 ms"*, and §7.2 contained no
such definition.** Tier 0 is *"the data is in local SQLite. Nearly every read"*; a control that
fetches nothing is not a read, and the category it needed did not exist until §7.2 grew one. The
claim is replaced rather than re-cited, because a corrected pointer to the wrong tier would still be
the wrong tier. 🔍 Extrapolating the measured 0.15–0.26 ms/row to a Pi 5 at a conservative 3–5×
puts that hard fail at **400–1,200 rows in the DOM**, or 1,200–2,400 with `table-layout: fixed` and
working containment — **so the real ceiling is still set by the density control rather than by
scrolling in the tens of thousands, but it sits in the high hundreds to low thousands rather than in
the hundreds.** ℹ️ **Those are the previous 100–300 and 300–600 multiplied by four, and that is the
whole derivation**: the row ceiling is the budget divided by a per-row cost, so it is linear in the
budget, and 100 ms → 400 ms moves it by exactly 4×. Nothing was re-measured to produce them.
⚠️ **ADR-0029's
2026-08-16 amendment sharpens this from the shipped primitive** — a measured cost curve of
0.0146 ms/row + 6.4 ms fixed, a worst-case row shape at 0.214 ms/row, and a **200-row default page
size** — and that amendment, not this paragraph, is the current arithmetic. ⚠️ **The per-row
figure is a linear fit, and the linear fit is only good to a few thousand rows**, because the
**25,000-row point is superlinear**: 0.15 ms/row at 1,000 and 0.24 at 5,000, against 0.26 at 25,000.
The extrapolation above runs *downward*, into the range the fit covers, which is why it is stated at
all — **no figure extrapolated past a few thousand rows may be quoted here as though it were
measured.** Three mitigations before any redesign: set `table-layout: fixed` (never set anywhere
today, and it halves the cost); **scope the density attribute to the list container rather than
`:root` — measured at about 25%, on top of containment's ~88%**; and if it still exceeds the 400 ms
hard fail, an explicit 150 ms "applying" state is honest where a silent multi-second freeze is not.

The reasons are concrete. `content-visibility: auto` skips rendering of off-screen content but,
unlike `display: none`, "the skipped contents must still be available as normal to user-agent
features such as find-in-page, tab order navigation, etc."
(<https://developer.mozilla.org/en-US/docs/Web/CSS/content-visibility>; baseline since September
2024). **Virtualization breaks Ctrl+F**, because only rendered rows are in the DOM — the WICG
virtual-scroller explainer states the problem exactly: "accessible landmark navigation, find in
page, or intra-page anchor navigation are based solely on DOM structure, and virtualized content is
by definition not in the DOM" (<https://github.com/WICG/virtual-scroller>). For a **library
browser**, "Ctrl+F for the album" is exactly what a power user does, so that is a real functional
regression, not a theoretical one. Virtualization also breaks `aria-setsize`/`aria-posinset` unless
set manually, text selection across the list, and pixel-offset scroll restoration. And **infinite
scroll is never used**: NN/g finds it "can be downright harmful to usability — in particular, for
search results" (<https://www.nngroup.com/articles/infinite-scrolling/>), and Baymard found
**"Load more" plus lazy loading tested best**
(<https://baymard.com/blog/external-load-more-vs-pagination-vs-infinite-scrolling>). UsArr is a
retrieval tool, not a discovery feed.

> ✅ **The three-document conflict is resolved, not deferred.** `ARCHITECTURE.md` §4.5's "virtualize
> everything over ~200 rows" is **replaced**; §16's "Library grid, virtualized" line item is amended
> to match; and **ADR-0003's conclusion is untouched while one of its arguments is corrected** — a
> 10k-item grid with instant client-side filter and sort is still a rich client-state problem (the
> client-side prefix index is the proof), so the HTMX rejection stands on its own; the word
> "virtualized" in that sentence should now be read as "large". All of it is recorded in **ADR-0029**
> rather than left as an omission.
> Note the two positions were always closer than they looked: keyset windows of ~100 rows with ±2
> pages prefetched (§4.5) means the mounted set is small either way; what differs is whether the
> unmounted rows are absent from the DOM (virtualized) or present-but-unpainted
> (`content-visibility`).

---

## 8. Navigation and page anatomy

### 8.1 The navigation model — two axes, and they are never merged

**Closed, 2026-08-16 (OQ-2), and now ARCHITECTURE §17.2 + ADR-0027.** §17.2 had left the model open
between *"sections on the home page"* and *"per-type tabs in a top navbar"*. The owner confirmed a
**persistent, collapsible left sidebar**, and the expansion to six media types is new evidence in the
same direction rather than a re-litigation: §17.2's second option was drafted when "per-type" meant
two, and six types plus Home · Search · Requests · Services · Settings · System is twelve top-level
items in a horizontal strip — before Calendar and Stats (§12) — with the persistent search input left
nowhere to go but a compressed corner or an icon.

**The decision, in two halves:**

> **Media type is the navigation axis** — a closed set of six (movies, TV, music, ebooks, audiobooks,
> comics), one sidebar entry per type, **all six of them, always** (ADR-0053), bounded at six by
> construction.
>
> **A library is a *scope*, not a place** — a multi-select chip pinned above the nav, reflected in
> the URL as `?lib=`, on the routes that already exist. Libraries are unbounded in number and are
> therefore **never** sidebar entries.

Content nouns first; `Settings` and `System` always the last two entries, in that order; sub-items
indented inline under the expanded parent, **no flyouts**.

This is **verbatim the shape of the Sonarr/Radarr/Prowlarr sidebar**, and self-hosters already know
it:

- Sonarr ([PageSidebar.js](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/Components/Page/Sidebar/PageSidebar.js)),
  in order: **Series** (→ Add New, Library Import) · **Calendar** · **Activity** (→ Queue, History,
  Blocklist) · **Wanted** (→ Missing, Cutoff Unmet) · **Settings** (→ 13 sections) · **System**
  (→ Status, Tasks, Backup, Updates, Events, Log Files).
- Prowlarr ([PageSidebar.js](https://github.com/Prowlarr/Prowlarr/blob/develop/frontend/src/Components/Page/Sidebar/PageSidebar.js)):
  **Indexers** (→ Stats) · **Search** · **History** · **Settings** · **System**.
- Navidrome uses the react-admin sidebar with a library selector pinned at the top
  ([Menu.jsx](https://github.com/navidrome/navidrome/blob/master/ui/src/layout/Menu.jsx)).

**Why the sidebar rather than the top bar, recorded now that it is settled:** it is what three of
UsArr's neighbours already use, it is where the ecosystem puts status badges (§8.2), it collapses
cleanly on a phone, and it scales to the eventual Home · six types · Search · Requests · Calendar ·
Stats · Services · Settings · System set (§12) — which a horizontal strip does not.

**The sidebar, with the scope chip and the six types. All six type entries render, unconditionally
and in one fixed order, including the types this install has nothing in.** This section specified the
opposite until 2026-08-19 — type entries **data-driven** rather than markup, §17.2's hard rule
applied to the sidebar exactly as to Home, *"a type the user does not have is not shown at all"*, so
a movies-only install rendered two content nouns — and **that shape is not buildable over the wire
UsArr serves.** [`reference/http-api.md`](../reference/http-api.md) §7.1 closes the wire question:
*"There are **no facet counts** beside the chips; each is its own aggregate and its own read."*
Nothing on any read answers per-type presence, so a data-driven sidebar has no honest source, and
both ways of faking one are worse than six rows. Six aggregates on every navigation put a count on
the render path, which is what principle 1 exists to refuse. Hiding a type on a count nobody measured
is worse still, and it is the option that looks like compliance: it fails **silently**, and it
removes the one row that would have explained the absence, so a library that is really there becomes
unreachable from the nav with nothing to say why.

**So all six ship, no row carries a count, and an empty type says it is empty on its own screen —
where the words can be true and can name their reason.** That is the decision of record, taken with
the shipped shell and recorded as **ADR-0053**; it is not an unfinished state to be tidied back to
the old spec. `browseEmptyState` (`web/src/lib/librarygrid.ts`) is where the honesty went: it words
the empty grid from the services read, separating *"no library-bearing service is connected"* from
*"this type has no rows yet"* from *"the libraries this view is scoped to hold nothing"*.
A user who clicks an empty type is told which of the three is true, which is strictly more than a
hidden row tells them.

🚩 **STRUCK 2026-08-20 — the third separation was named as a state this function does not render.**
The clause above read: *"~~from **"the library scope excludes everything"** (§10's `scope-empty`)~~"*.
**The first two separations are exact and are untouched** — *"No library-bearing service is
connected"* is `recentEmptyState`'s own title (`web/src/lib/libraryscreen.ts`) and *"No {type}
catalogued yet"* is `browseEmptyState`'s. **The third is a different proposition from §10's.** What
the code separates third is *"the libraries this view is scoped to hold nothing"*: the branch is
`query.libraries.length > 0`, which titles *"Nothing in this scope"* on the all-types view and
*"No {type} in this scope"* on a typed one, over the text *"UsArr has catalogued nothing in the
libraries this view is scoped to"* — an assertion that **the named libraries are empty**. §10's
`scope-empty` asserts that **the scope names zero libraries**: *"Your library scope is set to 0 of 8
libraries, so nothing is shown."* A scope that holds nothing and a scope that holds no libraries are
not the same fact, and only the first is worded here.

⚠️ **The zero case is not merely unhandled, it is unrepresentable at this layer — which is how the
sentence came to claim it.** `readLibraryScope` drops empty segments, so an absent `?lib=` and an
empty one both arrive as `[]`, and `browseParams` deletes the parameter rather than emptying it; `[]`
therefore means *no scope*, never *a scope of none*. So `libraries.length === 0` falls to *"No {type}
catalogued yet"* on a typed view and to `recentEmptyState` on the all-types one, and neither of those
is `scope-empty`. This is §9.7's `ScopeChip` note reached from the other end — *"an absent `lib` means
everything and an empty one is a `400`, so §8.1's third grammar and §10's `scope-empty` cannot be
expressed at all"*. **The gap is real, it belongs to `scope-empty` rather than to ADR-0053, and it is
tracked** at [`ROADMAP.md`](../ROADMAP.md) §2 under *"The `?lib=` chip"*. This rider corrects a
document and closes nothing.

⚠️ **The condition under which per-type hiding returns is a single named one, and it is a read, not a
rewrite: a facet read.** The day one statement answers *which of the six types have rows, under the
current scope*, the data-driven sidebar becomes buildable and this decision should be reopened — the
seam is one predicate on `TYPE_NAV` (`web/src/routes/+layout.svelte`), which is a `MEDIA_TYPES.map`
with none today. ARCHITECTURE §13 has already priced the shape, budgeting a scope-chip toggle of
*"1 keyset page + 6 sidebar `COUNT(*)` over `library_member ⋈ work`"* at **< 15 ms p50** — so the
cost is not the obstacle; the absence of the read is. Nothing yet decides whether those counts ride
the browse response or take their own endpoint, and ADR-0053 leaves that to whoever builds it.
**Until such a read is published in `reference/http-api.md`, restoring "one sidebar entry per type
that has content" would be re-specifying a screen the wire cannot serve.**

```
┌────────────────────────────┐
│  UsArr                 [«] │   collapse toggle
├────────────────────────────┤
│ ▣ All libraries (4)      ▾ │   scope chip — RENDERS NOTHING at 0 or 1 library
├────────────────────────────┤
│   Home                     │
├────────────────────────────┤
│   Movies                   │  ┐
│   TV                       │  │  ALL SIX, always, whether or
│   Music                    │  ├  not this install has any —
│   Audiobooks               │  │  fixed order, no counts,
│   Ebooks                   │  │  exactly six by construction
│   Comics                   │  ┘  (ADR-0053)
├────────────────────────────┤
│   Kids                     │  ┐  pinned libraries — opt-in, default none,
│   Vinyl rips               │  ┘  capped, hidden group when empty
│   More…                    │     only when something is unpinned
├────────────────────────────┤
│   Search                   │
│   Requests                 │
├────────────────────────────┤
│   Services              ●2 │   severity badge, null at zero (§8.2)
│   Settings                 │
│   System                   │
└────────────────────────────┘
```

Expanded, with the scope popover open — **native checkboxes, so Space toggles and Tab traverses for
free, and the checked / indeterminate states are announced without any ARIA**. ⚠️ **Arrow-key roving
and `Esc`-to-close are the two behaviours the popover must *add*** — native
`<input type="checkbox">` elements are Tab-traversed, not arrow-navigable (only radios within a
group rove), and `Esc` is popover behaviour rather than checkbox behaviour. "No custom key handling
written at all" was wrong, and an implementer following it ships a list where the arrow keys do
nothing. Three behaviours to write: arrow roving, `Esc` with focus returned to the chip, and close
on `focusout` when focus leaves the popover. That is still the payoff of §17.1's native-controls
rule, just a smaller one than claimed:

```
│ ▣ 2 of 4 libraries       ▴ │
│  ┌──────────────────────────────────┐
│  │ Select libraries:                │
│  │   ☑ Everything                   │   master checkbox, indeterminate state
│  │   ☑ Kids                         │
│  │   ☐ Vinyl rips                   │
│  │   ☑ Comics — ongoing             │
│  │   ☐ Archive                      │
│  └──────────────────────────────────┘
```

**Row budget, with the arithmetic, because "it fits" is not a design.** Fixed entries: Home, Search,
Requests, Services, Settings, System = 6, plus Calendar and Stats later (§12) = 8. Types = 6, always
now rather than at most (ADR-0053) — the arithmetic already took six as its worst case, so the
budget is unchanged and six is simply no longer a worst case. Scope chip = 1. That is 15 before pins,
so the **budget is 16 rows** and pins are capped at
`16 − fixed − types` — the cap shrinks automatically when Calendar and Stats arrive, and the excess
goes under `More…`. At §5.3's 32 px sidebar row height, 16 rows plus 4 separators ≈ 528 px, which
fits a 900 px viewport with the collapse toggle and without scrolling the nav. **INFERENCE:** the
number is derived from this document's own row height and viewport target, cross-checked against
Kavita's published "10 items + Home", not from a study.

**The scope chip is Navidrome's `LibrarySelector`, deliberately and in detail** — four properties,
each of which is a decision:

1. **Multi-select, not single-select.** It is a filter, not a mode, so cross-library browsing and
   search survive.
2. **It states the scope in words** — and in **one grammar**, not three. `All libraries (4)` /
   `2 of 4 libraries` / `No libraries selected`. The earlier third form, `None (0 of 4)`, used the
   parenthesis for a different thing from the first and read as a fourth shape when announced in
   sequence by a live region. A switcher that hides content is only dangerous when it is silent
   about what it hid.
3. **It renders `null` at 0 or 1 library.** No control appears for a distinction the user does not
   have — the same discipline as Sonarr's `PageSidebarStatus` returning `null` at zero (§8.2).
4. **Native checkboxes in a popover — with a maximum height, an internal scroll, and a filter once
   the list is long.** 🚩 The popover shipped with `max-height: none` and `overflow-y: visible`:
   measured at ~34 px per row it is ~310 px at the nine libraries drawn, **551 px at sixteen**, and
   it reaches the bottom of a 900 px viewport at roughly **24 libraries** and of a 768 px laptop at
   **19** — past which the list is clipped or overflows with no way to scroll it. **This is the one
   control §17.2 designates as *the* answer to unbounded cardinality** — *"libraries are unbounded,
   and therefore never a nav list"* — so a control that does not itself survive them defeats its own
   justification. `max-height: min(60vh, 420px)` with `overflow-y: auto`, a filter input above the
   list once the count exceeds ~12, and `Select all` / `Select none`. All three are what Navidrome's
   `LibrarySelector` and Jellyfin's user-view picker already do.
5. 🚩 **It is reachable at every viewport whenever the scope is not "all libraries".** Below 900 px
   the sidebar is a collapsed overlay drawer and takes the chip with it, so the only statement of an
   active scope is invisible until the drawer is opened — on the device §17.1 singles out, after a
   scope set on a desktop travels to the phone in the same `?lib=` URL. **A non-default scope
   therefore hoists the chip into the top bar**, beside the search box, and the drawer is never its
   only home. This is property 2 taken seriously: a control whose entire job is to state what it hid
   may not itself be hidden. Its terminal case is `scope-empty` (§10), which empties the whole
   application.
6. **A non-default scope looks different from the default one.** As drawn, the chip is
   pixel-identical to the density `<select>` in the top bar — same border, radius, chevron and fill
   — and nothing changes when a scope is active, so the only difference between "everything is
   shown" and "six of eight libraries are hidden" is two words of 13 px grey in a corner. The status
   token system already carries a neutral "not default" step; use it. **And the Search screen's
   inline line — *"29 results in the six libraries the scope excludes"* + `Clear the scope` — is the
   pattern for every scoped surface, not just Search.** Sonarr's
   `AllSeriesAreHiddenByTheAppliedFilter`, cited in §9.6, puts the message *where the content is
   not*, which is exactly what Home and the type grids lack.

**Pinning is the honest concession.** Plex users pin because a library used hourly should be one click
away, and refusing that is dogma — so pins exist, **opt-in, default none, capped, in their own
sidebar group**, with one `More…` overflow and no second level (NN/g: designs beyond two disclosure
levels *"typically have low usability"*). A pin **sets the scope and lands on Home**; it is not a
separate view of the app. That is Plex's affordance with Plex's failure mode designed out — the pin
state is purely additive, and unpinning hides nothing, because everything is still reachable at the
default scope.

**One word is mandatory wherever a library name is rendered beside a media-type name: `library`.**
Write *"all in the **Ebooks** library"*, never *"all in Ebooks"*. The two axes are the whole model of
this section, and on the common install they collide as strings — Audiobookshelf's one container is
called *"Audiobooks & Ebooks"*, UsArr splits it into libraries named **Audiobooks** and **Ebooks**,
and those are also two of the six media-type names, so the same word appears at three levels. Without
the noun a search group header reads *"Ebooks: all in Ebooks"*, which is a tautology to a sighted
reader and, to a screen-reader user hearing *"heading level 2, Ebooks 14, all in Ebooks"*, carries no
cue that the two are different kinds of thing. **It teaches the user that media type and library are
one axis, which is precisely what ADR-0027 exists to prevent** — and it costs one word. It applies
everywhere the two meet: search group sub-headers (ARCHITECTURE §17.4 rule 5), the Services screen's
`Libraries` column, the scope popover's legend, and the Libraries detail screen's Identity panel.
The reverse case needs no special handling: where the library name and the type name differ, the
noun is still correct and still cheap.

**Where library configuration lives: Settings → Libraries** (ARCHITECTURE §17.8), **not on Services.**
Services stays exactly what §17.3 says it is — one row per configured *service*, its health, its
problem, and the one button that fixes it. Membership of a library is not a service concern, and
putting it there blurs the screen that has to stay legible when things break.

### 8.2 Status badges live on the sidebar item

Sonarr attaches a `statusComponent` to exactly two nav items — `QueueStatus` on Activity and
`HealthStatus` on System — and
[PageSidebarStatus.js](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/Components/Page/Sidebar/PageSidebarStatus.js)
renders a count label coloured by severity (errors → danger, else warnings → warning, else info)
and **renders `null` when the count is zero**.

UsArr copies this exactly: **a severity-coloured count badge on the Services sidebar item — red for
errors, amber for warnings, hidden at zero.** It is the single highest-value borrowed pattern for
principle 3, because it makes "something in the pipeline is broken" visible from every screen
without a banner competing for attention. It coexists with §17.7's non-modal banner, which is what
names the specific instance.

### 8.3 The page skeleton, and the top bar

Every screen is: **sidebar · toolbar · content**. Nothing else. The toolbar is 40 px, holds the
page title (20 px), the view toggle, the filter and Options controls, and the primary action if the
page has one. The content region is full width — not a centred max-width column.

The top bar carries **one search input with a keyboard shortcut**, matching Sonarr's
`SeriesSearchInputConnector` and its `"Focus Search Box"` shortcut string. The *Arrs distinguish
two things and never merge them, and UsArr keeps the distinction: **library search** (the
persistent box, jumping to something you already have) versus **release search** (a dedicated
screen — Prowlarr's Search page, or Sonarr's Interactive Search modal). Merging them is how a
0 ms local query ends up waiting on a 30 s indexer.

**Responsive, not a separate mobile design** (§17.1). The sidebar collapses to a toggle; the
toolbar wraps; tables degrade to a simple two-line list rather than scrolling horizontally, which
is what Navidrome does (`SongDatagrid` on desktop, `SongSimpleList` on mobile, with several columns
hard-gated to desktop regardless of preference,
[SongList.jsx](https://github.com/navidrome/navidrome/blob/master/ui/src/song/SongList.jsx)).
Mobile is where this ecosystem's UI complaints actually cluster — Sonarr
[#7757](https://github.com/Sonarr/Sonarr/issues/7757) (sidebar hides content),
[#4401](https://github.com/Sonarr/Sonarr/issues/4401) (import dialog too big),
Prowlarr [#2431](https://github.com/Prowlarr/Prowlarr/issues/2431) (add-indexer modal unusable on
mobile) — so this is the specific failure mode to design against, not a checkbox.

⚠️ **Not designed yet, and it is a genuinely separate problem:** the phone layout for six types plus
a scope chip. A 16-row sidebar and a multi-select popover in a drawer is not the desktop layout at a
narrower width, and §17.1 requires every screen to work on a phone browser. Named here rather than
assumed away.

### 8.4 Home — three fixed blocks

**ARCHITECTURE §17.2, amended by ADR-0028.** Home is **not** one recently-added strip per media type.
Its height is **O(1) in the number of media types**.

⚠️ **The wireframe below draws the *full stack* — Sonarr, Radarr, Prowlarr, Navidrome,
Audiobookshelf and Kavita — because six populated types is what this layout has to be judged on.**
**That is not the v0.1 install.** **v0.1 connects BookOrbit and Prowlarr only**
([ADR-0052](../DECISIONS.md#adr-0052), which replaced Kavita in that slot; this sentence read
*"v0.1 connects Kavita and Prowlarr only (ADR-0041)"*). The remaining
catalogue sources sequence after it, one at a time.

🚩 **STRUCK 2026-08-20 — the two sentences that stood here read:**

> ~~so on a v0.1 install movies, TV, music and audiobooks have **no catalogue source** and Block A
> renders those four rows in the per-type `unconfigured` state, naming the service that will
> populate each and the milestone it arrives in (ARCHITECTURE §17.2, and rule 13 in §13 below for
> why four stateful rows are not an empty section). **Which four types those are is unchanged** —
> BookOrbit's media types are Kavita's, books, comics and manga (§16.1)~~

⚠️ **THE DEFECT IS NOT THAT THAT LIST WENT OUT OF DATE.** Audiobooks in it is false of this tree:
`bookOrbitEditionFormat` (`internal/libsync/bookorbitfiles.go`) returns `edition.format` =
`'audiobook'` for `bookorbit.MediaKindAudiobook`, which `internal/bookorbit/catalogue.go` defines
as *"one of m4b, mp3, m4a, opus, ogg, flac"*, and `internal/store`'s `mediaTypeOf` reads exactly
that format to answer `Audiobooks` instead of `Ebooks`. **But replacing four with three would carry
the same defect forward**, because **enumerating which media types have a source is a category
error rather than a fact with a shelf life.** Whether a type is catalogued is a property of **the
install**, derived from **the service kinds that install has connected**: `cmd/usarr/import.go`
accepts a catalogue import from `bookorbit` **and** from `kavita`, so a Kavita-only install yields
no audiobooks, a BookOrbit install yields them, and an install holding both is a third answer. The
struck sentences were only ever accidentally correct, for as long as one install shape was the only
one anybody had in mind.

**So this section names no catalogued-versus-sourceless split at all, and the rule stands in its
place:** Block A's rows are **derived from what is connected**, never from a list written here or
anywhere else in this document. A type an install has a source for renders its counts; a type it
has none for renders the per-type `unconfigured` state, naming the service that will populate it
and the milestone that service arrives in (ARCHITECTURE §17.2, and §8.6 row 13 below for why
stateful rows are not an empty section). **How many rows land on each side is the derivation's
output and is not a design constant.** The shipped precedent to follow is
`$lib/librarygrid.browseEmptyState`, which answers *why is this empty* from `homeMode` —
computed by `$lib/home` from the health response, on the rule that *"a build that later accepts a
library-bearing kind changes what this returns without anything here being edited"* — rather than
from any enumeration of types. Block A owes the same derivation one grain finer: per media type
rather than per install.

⚠️ **Two things the struck text pointed at are named rather than carried.** Its *"rule 13 in §13
below"* meant **§8.6 row 13**, which is where that rule lives. And its *"BookOrbit's media types
are Kavita's, books, comics and manga"* was quoting [ADR-0052](../DECISIONS.md#adr-0052) clause 1,
restated at ARCHITECTURE §16.1 — so that phrasing is **the upstream of the residue struck here**,
and the same code falsifies it there. Both belong to other threads; this section stops repeating
the claim and amends neither.

The wireframe below is right about the *shape*, while its v0.1 counterpart in
`design/mockups/` still **draws** Kavita by name. That re-draw is owed and is not done here:
ADR-0052 lists it among the documents it deliberately did not sweep, calling it a design-asset
change rather than a prose one. Both installs are real screens the design owes; neither is the
other's placeholder.

```
┌───────────────────────────────────────────────────────────────────────────────────────┐
│ Home                                                    [ Table | Posters ]  [Filter] │  40px toolbar
├───────────────────────────────────────────────────────────────────────────────────────┤
│ Kavita is unreachable — showing cached data from 14:02             [Open Services]    │  banner, only when degraded
├───────────────────────────────────────────────────────────────────────────────────────┤
│ Your library                                                                          │  BLOCK A
│ ───────────────────────────────────────────────────────────────────────────────────── │
│ Movies          1,204     1,187 have · 17 wanted     synced 14:02          see all →  │
│ TV                214     28,904 / 31,110 episodes   synced 14:02          see all →  │
│ Music           8,930     8,930 have                 synced 13:40          see all →  │
│ Audiobooks        412     410 have · 2 wanted        synced 13:40          see all →  │
│ Ebooks          2,051     2,051 have                 synced 09:12          see all →  │
│ Comics            733     733 issues · 12 gaps       synced 09:12          see all →  │
│                                                                                       │
│ Needs attention                                            3 items                    │  BLOCK B — hidden when empty
│ ───────────────────────────────────────────────────────────────────────────────────── │
│ ✕ error    Sonarr        401 Unauthorized on /api/v3/series      [Update API key]     │
│ ▲ warning  Kavita        last successful sync 3 d ago            [Run full sync now]  │
│ ▲ warning  1 work needs re-identification (Radarr 4K)            [Re-link]            │
│                                                                                       │
│ Recently added                                          across 6 types    see all →   │  BLOCK C
│ ───────────────────────────────────────────────────────────────────────────────────── │
│ Type    Title                            Added   Detail              Have   Instance  │
│ movie   Dune: Part Two                   14:02   Bluray-1080p 14.2G   ✓     Radarr    │
│ album   Geogaddi — Boards of Canada      13:58   FLAC · 23 tracks     ✓     Navidrome │
│ comic   Saga #61                         13:55   CBZ · 24 p           ✓     Kavita    │
│ tv      ER S12E14 Quintessence of Dust   13:22   HDTV-1080p 1.9G   250/331  Sonarr    │
│ book    The Overstory — Powers           12:40   EPUB · 2.1 MB        ✓     Kavita    │
│ audio   Piranesi — Susanna Clarke        12:04   M4B · 6 h 45 m       ✓     ABS       │
│ …                                                                                     │
└───────────────────────────────────────────────────────────────────────────────────────┘
```

- **Block A is a table, not tiles**, per §5.4: a media-type summary's primary content is a **count**,
  and a card is justified only when the primary content is cover art.
- **Block B is hidden entirely when empty** — §10's state table, applied. A green "all good" panel is
  the thing it must never become.
- **Block C** carries the same small-multiple row as search (§9.1), so a sixth type adds rows rather
  than a sixth region. In the Posters view it becomes **one wrapping grid across all types**, never
  six strips.
- **No horizontal strip appears on Home in any view mode.** Strips are legitimate on item detail
  ("More from this artist", ≤5 items per NN/g's frames guidance).
- **v0.1 ships no home configuration**, with two seams pre-wired: one ordered list of present types
  per user driving **both** sidebar order and Block A order (Plex's "sidebar order is home order" —
  one stored list, two renderers), and a per-type `show_on_home` boolean defaulting true, unset in
  the UI.

The arithmetic that decided this, because it is UsArr's own test rather than a borrowed one: at
1440×900 a 154 px poster strip fits ~8 cards across a ~1200 px column; a portrait card plus its meta
line is ~260 px, plus a 24 px header and a 16 px gap ≈ 300 px per section; a 900 px viewport minus the
40 px toolbar shows **2.8 sections** and about **16 items above the fold** — under §5.3's own 25-item
floor, on the screen whose whole job is inventory.

### 8.5 Search across six types

§17.4's separation of owned from unowned survives the expansion and gains four rules. They are
ARCHITECTURE §17.4's, restated here as component behaviour:

1. **A group with zero hits does not render.** No header, no empty state, no "0 results in Music".
2. **Group order is by the group's best-scoring hit, descending — not a fixed type order — computed
   once per query and then frozen.** Late-arriving unowned results never reorder or move a group.
3. **The per-group cap comes from a total row budget:** `cap = clamp(floor(40 / groups), 3, 10)`, so
   6 groups get 6 rows each. Every truncated group's last row is `Show all 34 movies →` **carrying
   the real total**, because Baymard found silent truncation makes users believe they have seen
   everything. The 40 comes from §5.3's above-the-fold heuristic; **INFERENCE**, not a measurement.
4. **A cross-media linked work appears exactly once**, in the group of its highest-scoring medium,
   with the other media as availability chips on that row. *Annihilation* is a film, a novel, an
   audiobook and an ebook; showing it four times is the specific incoherence a hub creates that a
   single-type tool cannot.

Above the results: **a single row of type filter chips** — `All · Movies · TV · Music · Audiobooks` —
showing only types with hits, each carrying its count, each a real `<a href>` that sets `&type=`.
That is the affordance people want from "type tabs" without a navbar competing with the sidebar. It
disappears at one type, and it is **not navigation**.

Every row is one template — `[type chip] Title … secondary metadata … [availability] [library]` —
varying only in data (§9.1). The type chip is **neutral**, because the type is data, not status
(§9.5); that is the whole trick, since six types in one list are incoherent only if the layout changes
per type. The library name renders only at ≥2 libraries, matching the scope chip's own `null`
discipline — Kavita's `in {{libraryName}}` line is the cheapest thing that makes a heterogeneous list
cohere, and it costs nothing.

### 8.6 Recommended against, with the evidence

Carried across so none of it has to be rediscovered:

| # | Do not | Because |
|---|---|---|
| 1 | Put user-defined libraries in the sidebar as nav items | Jellyfin's `libraryMenu.js` maps `items.map(...)` over every user view with **no cap, pin, overflow or reorder**; Calibre-Web reached **17** `SIDEBAR_*` bits on *one* library; Kavita had to retrofit "10 items + Home, rest under More" |
| 2 | Use a **single-select** library switcher that scopes the app | Audiobookshelf's own docs: *"Most actions … apply to the currently selected library, **including browsing and searching**"*, and an author with series in two libraries shows as **two separate author entries** |
| 3 | Rely on pin state as the only way to reach a library | Plex forum, 2026-07-09: *"10+ of my users have reported that their pinned libraries have vanished … The 'More' option … is gone entirely"*, with a four-year tail of the same class |
| 4 | Ship six recently-added carousels on Home | ~1% CTR with **84%** of clicks on slide 1 on ND.edu over 28,928 tracked feature clicks ([Runyon, Jan 2013](https://erikrunyon.com/2013/01/carousel-interaction-stats/); the [five-site follow-up](https://erikrunyon.com/2013/07/carousel-interaction-stats/) found position-1 shares of 54–89% and CTR of 1.1–9.4%, and publishes no aggregate); NN/g *"5 or fewer frames"* and *"people often scroll past carousels"*; jellyfin/jellyfin#16615 asked for a wrapping grid and was **closed as not planned**; and six strips show ~16 items above the fold against §5.3's 25 floor |
| 5 | Auto-advance anything | NN/g: auto-forwarding carousels *"annoy users and reduce visibility"*. Independently banned by §6 |
| 6 | Put per-type tabs in a top navbar | Twelve top-level items before Calendar and Stats, and the persistent search input loses its home |
| 7 | Build a two-level sidebar or flyout submenus | NN/g: beyond two disclosure levels, *"users often get lost when moving between the levels"* |
| 8 | Make a command palette the way to reach a library | Zero surveyed tools ship one; Sonarr's entire global keyboard surface is five bindings. Nielsen's heuristic 7 makes accelerators a *second* path and heuristic 6 wants options visible. **The seam ships — build the `/` search input so a leading `>` could switch it into command mode, and keep routes and registries as data — the palette does not** |
| 9 | Interleave search results across types | §17.4's stability argument, plus Sushmita et al. (CIKM 2010): position is significant **only** in the blended design, so blending manufactures a ranking bias across incomparable signals |
| 10 | Let a linked work appear once per medium | §8.5 rule 4 |
| 11 | Build per-type screens | Jellyfin serves eight content types from one card builder branching on **aspect ratio, not media type**; §9.1 already forbids varying row treatment by type |
| 12 | Hide a media type behind "More" | Types are a closed enum capped at six and all fit the row budget. Overflow is for pinned libraries only |
| 13 | Show a type, section, group or control with no content | §17.2; Komga's `v-if="collectionsCount > 0"`; Navidrome's `LibrarySelector` returning `null` at ≤1 library; Sonarr's status badge returning `null` at zero. ⚠️ **The bound is *no content*, not *no items*.** Home Block A's **sourceless rows** — however many an install has, which §8.4 derives from the connected service kinds rather than fixing as a count — are **not** an exception to this rule: a row reading *"Music — no catalogue source · Navidrome · after v0.1 · Add"* (the mockup's verbatim string) carries a state, a cause and an action, which is content. What the rule bans is a region that says nothing. Dropping those rows instead would leave a Home screen from which the only inference is that UsArr does not do whichever types that install has no source for. 🚩 **STRUCK 2026-08-20:** this cell read *"Home Block A's **four** sourceless rows in v0.1 (§17.2)"*, *"Dropping those **four** rows"* and *"UsArr does not do **movies, TV, music or audiobooks**"*. The count and the enumeration are both residue from the Kavita slot — BookOrbit catalogues audiobooks, so the sourceless set is neither four nor fixed (§8.4) — and **the argument never rested on either**: it needs at least one sourceless row, not four, so nothing above it moves |

---

## 9. Component conventions

### 9.1 Rows and tables

The default container. A row is a **small multiple**: identical template everywhere, varying only
in data, so the user stops parsing layout and starts parsing data. **Corollary: do not vary row
treatment by media type.** Same slots, same positions, different values.

- **Text left-aligned** — both the cell and its header. **Numbers compared down a column
  right-aligned** with `tabular-nums`. **Header alignment matches its column's data alignment**; a
  left header over right numbers is a persistent low-grade scanning cost. Centre nothing except a
  single status glyph in a fixed-width column.
- **Row separation is a 1 px rule**, not a gap and not a card. **No zebra striping by default** —
  the evidence is weaker than the folklore: A List Apart's first study (n=244) found **no
  statistically significant accuracy improvement** and little speed improvement, with one
  significant result at 16% faster for a complex wide-table lookup
  (<https://alistapart.com/article/zebrastripingdoesithelp/>); the follow-up found stronger
  evidence, *particularly in complex tables*
  (<https://alistapart.com/article/zebrastripingmoredataforthecase/>). **INFERENCE:** striping helps
  in proportion to horizontal travel. A **row hover highlight is better than striping in an
  interactive tool anyway** — zero static ink, and it tracks the pointer.
- **Sticky header** (`position: sticky; top: 0`) on any table long enough to scroll, with an opaque
  background — never a translucent one, which costs a composite layer and looks dirty over dense
  rows — and a bottom rule so it reads as pinned. Note SC 2.4.11: a sticky header must not
  entirely obscure a focused row.
  ⚠️ **A sticky header must be tested at every breakpoint, not just the widest one**, because an
  ancestor that becomes a scroll container silently breaks it: `overflow-x: auto` with
  `overflow-y: visible` **computes to `auto` on both axes**, so the wrapper becomes a scroll
  container that never scrolls vertically, and the sticky header then sticks to *that* box and
  scrolls away with the page. Measured on the shipped mockup: **one header pinned at 1440 px, zero
  at 1000 px** — a broken band from **761 px to 1,099 px**, which is precisely the laptop/tablet
  range, on the screen carrying six different column sets. Below 761 px the stacked view hides the
  header by design, so that band is fine. The fix is either `overflow-x: clip` on the wrapper with
  per-cell wrapping, or moving the horizontal scroller inside the row so the wrapper never becomes
  a scroll container at all.
- **A column whose value is identical for every row in its group is not data, and is not rendered.**
  State the fact once — in the group header or a banner — and drop the column. Measured on the
  six-group search screen, four of six groups carried one distinct `Library` value across every row,
  costing ~120 px of a 1,232 px content column and wrapping 12 of the 13 rows above the fold. This
  is §1.6's *delete this element; is information lost?* applied per group rather than per screen.
- **Declare column widths per group rather than letting auto layout derive them.** `table-layout:
  auto` must measure every cell in every row to compute column widths — the one layout mode that is
  inherently O(all rows) and that no containment can help — and with six groups it computes six
  different layouts for one content column. Measured, declaring widths cut the density-switch cost
  from 1,199 ms to 547 ms at 5,000 rows and from 6,508 ms to 2,956 ms at 25,000.
- 🚩 **Truncation is a named policy with three tiers, and a row has a stated maximum height.** The
  prototype contained **exactly one** `text-overflow: ellipsis` in 323 KB, no `-webkit-line-clamp`,
  and three `overflow-wrap` declarations of which two were `anywhere` — so the layout's answer to
  every long string was "wrap it", and row height became a function of the worst string present.
  Measured consequence: Home rows 28–46 px and Search rows 28–78 px against Services at **103–119**
  and Libraries at **80–160 px** — 5.7× the design's own compact unit, with a median around 96.
  Two screens in one product looking like two products is not eight separate bugs; it is one missing
  decision. The decision:
  1. **Identity fields** — titles, names, series — truncate at the cell with an ellipsis and carry
     the full string in `title`.
  2. **Machine strings do not wrap.** A release name, a path, a JSON body or a log line scrolls
     inside its own cell (`white-space: nowrap; overflow-x: auto`) or truncates with the full value
     behind the row expander. **`overflow-wrap: anywhere` is banned on mono content**: it breaks at
     any character, so `x264` renders as `x26` / `4` and a JSON key splits mid-token — destroying
     the one thing the reader is scanning for, on exactly the content §4.2 identifies as machine
     data. `overflow-wrap: break-word` with `word-break: normal` is the fallback if wrapping must
     stay; it breaks at `.` and `-` and leaves tokens intact.
  3. **Explanations never appear in a cell at all.** One clause in the row, the rest behind the
     expander that these rows already have. See the next rule.
- 🚩 **No table cell contains prose, and no cell contains design rationale.** Measured: a Services
  `Problem` cell carried a **six-line essay** explaining a rendering decision on a *different
  screen*; a Libraries `State` cell carried a four-line paragraph with a scare-quoted rebuttal of a
  reviewer; a `Request destination` cell carried sixty-two words of competitive analysis in a column
  whose other rows hold the word `None`. Three different kinds of object in one column means the
  column has no defined role, and the tallest one sets the row height for everything beside it —
  which is why the left-hand cells of those rows sit top-aligned against 130 px of dead space.
  Nothing in this ecosystem does it: Sonarr's health page gives one line and a wiki link. **The test
  is §1.6's, tightened: delete this clause — does the user lose a fact they can act on?** A clause
  that argues with a reviewer, congratulates the design or explains another screen fails it.
- **A declared-column list needs an overflow policy, and "clip" is not one.** §9.1 requires declared
  widths (below), and a fixed track plus `overflow-x: clip` on the wrapper does not degrade — it
  **amputates**, with no scrollbar and no way to reach what was cut. Measured on the Services table
  at 1280 / 1440 / 1680 / 1920: the two longest action buttons — *"Run full sync now"* and
  *"Update API key"*, i.e. the ones attached to the degraded and the down rows — overhang their
  132 px track by 10–12 px and are sheared off flush with the page edge at **every desktop width**,
  while `document.scrollWidth` equals the viewport so nothing scrolls. The screen shows what is
  broken and clips the fix. **So: the action track is `minmax(max-content, auto)` and the
  explanation column absorbs the remainder; a genuine overflow degrades to a scroll, never to a
  clip.** ⚠️ This interacts with the sticky-header rule above — `overflow-x: auto` on the wrapper is
  what breaks sticky headers between 761 and 1,099 px — so the scroller goes **inside the row**,
  which satisfies both. **`docs/design/check.mjs` asserts it** — the guard is real, and the actor
  named here was not: there is no CI (`REVIEW-LOG` `NOCI-01`), and what runs the assertion is
  whoever types `make design` (`OPTIN-01`). It is cheap: no element's
  `getBoundingClientRect().right` may exceed `innerWidth` on any screen × state × width triple.
- **A figure and its unit are two slots, not one string.** `tabular-nums` on 222 cells buys nothing
  when what is right-aligned is the *word after the number*: `1,204 films` / `275 series` /
  `612 artists` right-aligned as one string starts the digits at six different x-positions. The
  figure is a right-aligned numeric span, the unit is a fixed-width left-aligned span beside it, and
  `tabular-nums` is applied at the cell rather than per-span so composite values
  (`1,187 have · 17 wanted`, `13,204 / 14,061 episodes`) inherit it — eight such cells had it
  missing, and they are precisely the composite ones.
  - **The treatment: the figure at full contrast, the unit muted, `tabular-nums` retained on the
    cell, and the unit box given a fixed width.** The figure is the value and the unit is only its
    scale, so the unit is `--fg-muted` and the figure is not. **The fixed width is the half that does
    the work**, and it is what makes right-alignment mean anything: right-aligning `4.8 GiB` over
    `820 MiB` aligns the `B`, so the digits — the one thing being compared down the column — land at
    two different x-positions, and `tabular-nums` cannot help because the misalignment is caused by
    the word. Measured on the Requests release table with rows moved onto the widest unit in the
    family, one narrower, and the narrowest: **with the fixed unit box the last digit of
    `68.4 GiB`, of `820 MiB` and of `4 B` all sits at x = 924.06 px, a spread of 0.00 px; with the
    box switched off they sit at 928.06, 926.06 and 940.06, a spread of 14.00 px.** The sample has
    to contain the widest unit or the spread is understated: a two-row `68.4 GiB` / `4 B` set
    reports 9.00 px, and it is `MiB` — the widest — that makes the real figure 14.
  - **The width belongs to the unit FAMILY, not to the value in front of you**, or the digits jump
    the first time a row crosses from GiB to MiB. Reserve the widest unit the column can ever print,
    measured in the cell font (13 px IBM Plex Sans, `1ch` = 8 px) and rounded up to the next half-ch:
    **size** `B · KiB · MiB · GiB · TiB`, widest `MiB` at 22 px → `3ch`; **duration** `h · d`,
    widest `d` at 8 px → `1ch`; **Home's item nouns** `films · series · artists · books`, widest
    `artists` at 37 px → `5ch`. ⚠️ **The duration family read `days · weeks · months · years ·
    hours`, widest `months` at 43 px → `5.5ch`, and that was a family the mockups invented.**
    `formatAge` emits `h` below a day and `d` above it and nothing else, so five of the seven
    members were never printable — the same calibration error as the size family, one column along.
    Re-measured on the real family the reserve is `1ch`, and `Age` now takes it.
    ⚠️ **The item-noun family read `films · series · books · artists · issues · episodes`, widest
    `episodes` at 52 px → `6.5ch`, and it is the same error a third time — a family assembled from
    the words the table uses SOMEWHERE rather than the words that column prints.** Home's `Items`
    holds one noun per media type and there are six types: `films`, `series`, `artists`, `books`,
    `books`, `series`. `episodes` and `issues` belong to `Have`, one column along — `13,204 /
    14,061 episodes`, `7,891 issues · 34 with gaps` — and Items counts a comic library in series,
    not in issues. Measured on the four nouns that column can print: `films` 28 px, `series` 34 px,
    `books` 35 px, `artists` 37 px, so the reserve is **`5ch`, not `6.5ch`**. ℹ️ **The family here
    is the DESIGN's, not the product's, and that is stated rather than glossed**: Home's Block A is
    `NOT DRAWN` in the app — `web/src/routes/+page.svelte` says so and gives the reason — so unlike
    `formatAge` there is no shipping function to read the nouns off. §17.2's six rows are the
    source, and the day Block A is built the family is re-derived from whatever the builder emits.
  - ⚠️ **The size family is the BINARY one, and mistaking it for the decimal one is how this
    reserve was first mis-measured.** The rule shipped at `2.5ch`, derived from
    `B · KB · MB · GB · TB` whose widest member `MB` is 19 px — the family **the mockups drew at the
    time**, not the family **the application prints**. (The mockups' size sample data has since been
    converted, so both trees now draw the binary family; the figures were converted with it, not
    relabelled — `14.2 GB` became `13.2 GiB`.) Every indexer and every \*Arr reports binary units and so
    does UsArr (`sizeParts` in `web/src/lib/format.ts`), and the binary family is wider: measured in
    the same cell font, `KiB` 19 px, `MiB` 22 px, `GiB` 20 px, `TiB` 18 px. `2.5ch` was then run as
    a control rather than assumed to fail, and the control is the interesting half — the figures
    still align at 0.00 px, but `MiB`'s ink is 22 px inside a 20 px box and **spills 2 px into the
    cell's right padding at every density**. A reserve that does not hold the widest unit in its
    family is not a reserve. **So `3ch` is this bullet's own rule applied to the real family, not a
    departure from it**: the rule is unchanged and only its input was wrong. The defect underneath —
    mockups drawing data the product cannot emit — is recorded separately in `docs/REVIEW-LOG.md` as
    **SU-05**, because until the sample data is corrected it will keep producing measurements that
    are right about the mockup and wrong about the product. **All three of SU-05's columns are
    closed** — size in `SU-07`, `Age` in `SU-08`, `Category` in `SU-09` — and the class of defect
    it names is not: `SU-10` is the same error twice more, in the item-noun family and in the two
    trees' column widths.
  - **An absent value gets no unit box, and the reserve is the only thing this rule governs.** A
    `3ch` box held open around an em dash reserves width for a unit that is not there, so the
    absent-value branch emits `—` and no `.unit` span. ⚠️ **Whether the em dash is reachable is a
    property of the wire contract, not of the markup, and the two cases look identical in the
    code.** On the release tables `size_bytes` is a plain `int64` with no `omitempty`, so the server
    always sends it and the branch is **defensive and unreachable**. On Recent grabs the field is
    `*int64` with `omitempty` and `toNotSentGrabResponse` **never assigns it at all**, so a not-sent
    row **structurally cannot carry a size** — the branch is guaranteed reachable, and that is the
    reason the Recent-grabs Size column was worth the split and the release table's was not. Keep
    the distinction in writing wherever both appear, because a defensive arm nobody can reach and a
    guaranteed one are different objects wearing the same three lines.
  - **The rule constrains the unit box. The surrounding table's own conventions govern everything
    else about an absent value.** Recent grabs wraps its em dash in `<span class="muted">` because
    every other absent value in that table already mutes — `when`, `indexer`, `protocol` — and that
    is correct: this rule's requirement is *no `.unit` box on an absent value*, and the wrapper does
    not violate it. **Stated explicitly so the question stops recurring**: every new table that
    meets this rule otherwise re-litigates its own local conventions against it, and a design rule
    that reaches past what it is about turns every local convention into a conflict.
  - 🚩 **The alternative was the unit in the column header with bare numbers in the cells, and it is
    rejected: it only works where every row of the column shares one unit, which rules out size.**
    A `Size` column holding `68.4 GiB` beside `820 MiB` beside `4.2 KiB` has no unit to put in its
    header, and the moment one exists the header is lying about some rows. The same disqualifies it
    for `Age` (`7 h` beside `31 d`) and for Home's `Items`, whose six rows count films,
    series, artists and books. A rule that fails on the three columns it was proposed for is not a
    rule. The header form remains right for a column whose unit is genuinely constant — but that
    column is covered already, by the rule four bullets above: a value identical for every row is
    not data and is not rendered.
  - ✅ **Applied to the size columns (98 cells) and to `Age` (36 cells). One column is still
    deferred — Home's `Items`, 16 cells — and the reason is a measured cost, not an unfinished
    opinion.** A reserved unit box costs column width, and that track cannot pay it. Measured over
    the render sweep — 5 widths × 3 densities × 2 installs × every state — a correctly-sized `5ch`
    reserve takes the mockups' `Items` from **48 wrapped cells to 96**. ⚠️ **The failing width is 1280 px, and
    the deferral has never named it, because `Items` is a FRACTIONAL track and the number the
    deferral quotes is a resolution rather than a width.** `minmax(0, 1.15fr)` is 107.375 px at
    1440 px and **87.80 px at 1280 px**, the narrowest desktop width the design check sweeps —
    63.80 px of content, in which `1,204 films` (67 px), `612 artists` (64 px) and `1,842 books`
    (74 px) **already wrap today with no reserve at all**, and the reserve takes the full install's
    three wrapped rows of six to six of six. At 1440 px and above the reserve fits, with 9.38 px to spare. Widening a
    fractional track on Home is its own decision, and §9.1's own overflow bullet is the reason it is
    not folded in casually. ⚠️ **The deferral's stated premise was wrong twice over and the verdict
    survives both corrections**, which is worth saying plainly because a verdict that survives is
    not thereby vindicated: the reserve was costed at **52 px for `episodes`**, a word this column
    never prints (see the family bullet above — the real reserve is `5ch`, 40 px), and the track was
    costed at its 1440 px resolution, where it does not fail. **Corrected, re-measured, and still
    deferred.**
    ✅ **`Age` is no longer deferred, and both numbers the deferral rested on are gone.** `months`
    at 43 px is a word `formatAge` cannot emit; the mockups' sample data has been converted, and the
    real family is `h` 7 px and `d` 8 px, so the reserve is `1ch`. **The cost is exactly nothing**,
    measured twice: 960 cells per arm before anything was edited, by rewriting the cells in the
    rendered page, and **1,920 cells and 1,920 `.unit--age` spans afterwards against the shipped
    class**. `1095 d` — four digits at 32 px under `tabular-nums`, plus a 3 px gap, plus an 8 px
    box — is **43 px in the mockups' 56 px content box, 13 px of slack**, identical with the box and without
    it, and **0 cells on more than one line either way**. `d`'s ink is 8 px in an 8 px box, so **0
    of 1,920 spans spill**, which is the test `2.5ch` failed for `MiB`. ⚠️ **What the box buys on this column is 1.00 px, and that is the number to quote, not
    size's 14.00 px.** Forced onto the widest and narrowest unit the family has, the figures' right
    edges sit at **416.00 px and 416.00 px with the box — spread 0.00 px — and at 416.00 px and
    417.00 px without it, spread 1.00 px**, because `h` and `d` differ by one pixel. The reserve is
    still correct: a two-member family is a family, and the box is what stops the digits moving the
    first time a row crosses from days to hours. But on `Age` the treatment's larger half is the
    **muting** — the figure is the value, `d` is only its scale — and a reader who expects size's
    payoff here will not find it. ℹ️ **A one-member family would earn no box at all**, and that test
    was run rather than assumed.
  - ⚠️ **The `Age` track is 80 px in the mockups and 68 px in the application, and neither reading is
    a miscount.** `.cols-requests-releases` in `docs/design/mockups/usarr.css` has read `60px 88px
    80px … 80px 68px 84px` since it was written; `COLUMNS` in
    `web/src/routes/requests/+page.svelte` has read `Age` 68 px since `cd94779`. **Both trees hold
    both numbers, on different columns** — 68 px is `Grabs` in the mockups and `Age` in the app,
    80 px is `Age` in the mockups and `Protocol` in the app — which is exactly how one came to be
    quoted for the other. So **every width in this section names the file it came from**, and the
    divergence itself is the finding: of the ten columns the two trees share, **nine carry
    different widths** — only `Category` (`0.9fr`) still agrees — `Category` is column 9 in the
    mockups and column 5 in the app, and the mockups carry a `Select` column the app does not have
    at all. ⚠️ **That count was eight when it was walked at 03:21 Z, and nine six minutes later.**
    `Actions` agreed at `minmax(max-content, auto)` in both trees until **`7fc932e`** gave the app a
    `198px` reserve — because ADR-0029 makes every row its own grid and a content-sized track has no
    cross-row agreement to appeal to — and that commit reached `main` at 03:27 Z, while this
    section's gates were still running. **The divergence widened during the round that documents
    it**, which is the most direct evidence this bullet could have that it describes a live drift
    rather than a one-off. **So a count of divergent columns is perishable and dates itself**, the
    same discipline this bullet imposes on widths, applied to the count. Recorded in
    `docs/REVIEW-LOG.md`
    as **SU-10**, in the same family as SU-05 — a mockup is a measuring instrument, so a mockup that
    disagrees with the product keeps generating numbers that are right about one tree and wrong
    about the other.
  - **`Category` prints UsArr's derived tags; the indexer's own Newznab path goes in `title`.** The
    same release found on two indexers comes back with two different paths, so the raw value renders
    one fact two ways depending on who answered. UsArr's tags are stable across indexers, so they are
    what the column says — `movie`, `book · audiobook` — and the raw path carries detail ours drops,
    `Movies/UHD` against `movie`, so it goes in the tooltip rather than in the bin. Same pattern as
    the poster titles, and tier 1 above is why the detail is kept at all.
  - ⚠️ **That tooltip's content is RECONSTRUCTED, not transmitted, and looking for a `raw_category`
    field will not find one.** `ReleaseResource.CategoryIDs()` flattens `Categories` to ids and drops
    `Name` at the first hop, so what reaches the browser is integers all the way down. The name is
    recovered by joining the row's `indexerId` and `categories[]` against the **indexer catalogue**,
    which ships each indexer's own tree. **Not against `categoryTree()`** — that is a deliberate
    union across indexers, *"first non-empty name wins"*, which is right for the picker and destroys
    the per-indexer divergence this tooltip exists to show. Two consequences: the catalogue is a
    probed replica, so an id newer than the last probe shows the bare number (`Category 2045`) and
    never a guessed name; and a surface that does not already load the catalogue **does not fetch it
    for a tooltip** — it shows the tag with no tooltip, which is §2's first principle and this
    section's own rule that an absent value is not decorated.
- **A composite numeric cell says what its parts are.** `41 / 9` in a `Peers` column announces as
  *"Peers, 41 slash 9"* and nothing on the screen says which number is seeders — while prose four
  hundred pixels below calls the same column "seeders". Keep the ecosystem-verbatim header (`Peers`
  is Prowlarr's word) and make the **cell** self-describing, with the expansion in a visually-hidden
  span and in `title`. The same applies to `250/331`, `43 / 60` and `4 of 9`.
- **Three words for "there is nothing here", and no others.** Nine renderings shipped — `None`,
  `No action needed`, `Never`, `not applicable`, `none`, `n/a` twice, `no file` — with Search's
  Movies group rendering *the same fact* as `no file` in one column and `n/a` in the next. Three
  concepts are in play and the vocabulary must separate them: **`—`** for *the value is genuinely
  empty and that is unremarkable*; **`Not configured`** for *this exists as a concept and you have
  not set it up* (which is already a first-class status with its own token, `--status-unset`, and
  was then not used for it); **`Not applicable`** for *this concept does not exist for this row*.
  `Never` stays — it is a real answer to "last successful sync".
- **Two duration formats, chosen by magnitude, and two never becomes four.** `M:SS` for a single
  playable item under an hour (a track); `H h MM m` for everything else. Four shipped — `8 h 56 m`,
  `0:57`, `66 min`, `5,912 h` — two of them in adjacent blocks on Home. A library-level total is
  prose rather than a column, so it reads `5,912 hours`.
- **Every user-facing timestamp carries the relative form, and past 24 hours it carries a date.**
  `14:02, 6 minutes ago`; `11:47 on 15 Aug, 1 day ago`. §17.3 already required absolute **and**
  relative and the Services screen honours it; 87 bare `HH:MM` values shipped elsewhere, including
  in a degraded banner whose whole job is the number — *"showing cached data from 11:47"* is
  identical whether the instance has been down for six minutes or twenty-two hours. Where the column
  is too narrow, the relative form wins and the absolute goes in `title`. **One date format**:
  `8 Aug 2026`, no leading zero, always with the year.
- **A results list and a detail table get different rules below 760 px.** The stacked
  label/value treatment is right for Services and Libraries, where each row *is* a record you read
  one at a time — and it is wrong for a search result list, which is scanned: five labelled lines
  per result puts three results in an 844 px viewport and turns fourteen ebooks into ~70 lines.
  **A results row stacks to two lines** — title in `--weight-semibold`, then the two most
  identifying secondary fields joined by `·` — with the rest behind the row.
- **The stacked label is a real element marked `aria-hidden`, never generated content.** The
  `td[data-label]::before` pattern puts the column name *inside the cell's accessible name*, so a
  screen reader in table-navigation mode announces the column header and then the cell and the user
  hears *"Service. Service, Sonarr…"*, *"Items. Items 214"*, on every cell of every row. It is also
  a localisation trap: `data-label` duplicates the header string into an attribute most translation
  pipelines will not pick up, so the two drift on the first translation. Ship
  `<span class="stacklabel" aria-hidden="true">Items</span>` and keep the visually-hidden header row
  (which is the right call and is why the `columnheader` nodes survive at all).
- **A cell that renders one chip per related object caps at three plus `+N more`.** The Services
  screen's `Libraries` column is the live case: one Audiobookshelf feeding fifteen libraries makes
  that cell the tallest thing on the screen.
- **An indexer-flag chip is a positive claim only. Its absence is rendered as nothing, never as a
  negative.** No "not freeleech", no greyed-out chip, no empty slot holding the column open — a
  release with no flags shows no flags. **The rule is stronger than "we might not have checked".**
  Prowlarr's JSON surface derives the flags by exact equality on a volume factor, so a **partial
  discount of 25% or 75% carries no flag at all** and is indistinguishable from a release at full
  price on the surface UsArr reads. An absent `freeleech` chip therefore does not mean *not
  freeleech*, and does not even mean *not discounted*. `reference/arr-apis.md` §7.2 owns that fact
  and its primary sources — cite it, do not fork it. Nor is the chip actionable: there is no
  per-request token instruction to attach a control to, which the same section establishes.
- **A column picker labelled "Options", next to a control labelled "Filter".** Ship many columns,
  show few by default. This is verbatim the Prowlarr vocabulary — *"you can add or remove columns
  using the **Options** button, and you can sort and filter your results by either clicking on the
  column headers or using the **Filter** button"* (<https://wiki.servarr.com/prowlarr/search>) —
  and every \*Arr column already carries a user-overridable `isVisible` default. **Do not invent a
  new word for it.**
- **View toggle** in the toolbar, persisted client-side per media type. Sonarr and Radarr define
  exactly three modes — **posters / overview / table** — and Navidrome offers grid and table.
  UsArr ships **table** and **posters** in v0.1; "overview" (a wide row with thumbnail plus
  synopsis) is the natural third and is not required to ship first.

### 9.1a Ordering under an engaged user — *instability is acceptable only while nobody is aiming at anything*

**Settled 2026-08-16, in four rounds across three threads, and closed.** The sentence in the heading
is the rule; everything below is how to apply it. It is written here rather than only on the screen
that produced it because **it governs every list in UsArr that can change while a user is looking at
it**, and because the next such list should not have to rediscover it.

The condition that decides whether a list may reorder itself is **whether a person is committed to a
target** — not whether the *application* considers itself settled. That distinction is the whole
rule. "The fan-out finished, so the order is final now" is the app's own state and it is the wrong
thing to key on: the user reaching for the third row does not care that more results are still
arriving, and the user idly reading the screen is not harmed by a row moving. **Aim, not
readiness.**

**The general form, which is not specific to any one screen:**

- 🚩 **A list must not move a destructive or outward-facing affordance under an engaged pointer.**
  *Outward-facing* means the control spends something outside UsArr that UsArr cannot take back — a
  grab handed to a download client, a delete, a write to an \*Arr, an outbound request. Ordering may
  change freely around a read-only list nobody is pointing at. It may not change under a hand
  already aimed at a button that acts on the world.
- 🚩 **Identity, not position** — for **focus, hover, selection and pending row state**. Key every
  one of them to the row's **stable id**, never to its index. A list keyed by index reassigns all
  four the moment the order changes, so the highlighted row, the focused row and the row with a
  request in flight all silently become different rows. **Every mutable list in the product inherits
  this**, not only the one below.
- **A list fed by an incremental source re-sorts live while that source is still delivering, and
  freezes its order on completion** until the user explicitly re-sorts. Explicit means the user
  operated a sort control — a column header, the toolbar's sort, or the control in the next rule.
- 🚩 **While the pointer is inside the results region OR focus is within it, the order is frozen**,
  regardless of what the source is doing. Anything that would have reordered the list instead
  surfaces as **one explicit control** carrying its own count — `3 new results · re-sort` — so an
  ordering change under an engaged user only ever happens **because the user asked for it**.
- **A late arrival is not a special case.** It is just another thing that would have reordered, it is
  counted by that same control, and it does not enter the rendered list until the user re-sorts.
  **There is no separate append-below-marked-late mechanism**, and this is a rejection rather than an
  omission: appending a straggler below a *late* marker is a second rendering rule and a second
  vocabulary for one condition, the marker is meaningless the instant the list is re-sorted, and
  appending still changes the list under an engaged user — the row count moves, the scroll extent
  moves, and a pointer resting near the foot of the list gets a new row under it. One condition, one
  control.
- **0 ms, and never animate a re-sort.** §6 already puts sort at 0 ms on the critical path; the
  addition is that a reorder must not be animated *anywhere*, because an animated reorder is
  strictly worse than an instant one for this rule — it stretches the window in which the row under
  the pointer is neither where it was nor where it is going, and a click landing mid-flight is the
  ambiguous case the rule exists to remove.
- **Sort keys live in the URL**, so a sorted view is linkable, survives reload, and is restored
  identically rather than approximately. Same discipline as §8.5's `&type=` chips.

**Why this earns a rule of its own — the reason matters more than the mechanism, because the reason
is what survives a refactor.** On the Requests screen the affordance in question is **Grab**, and a
grab is **irreversible from UsArr's side**: the release is handed to a download client that UsArr
deliberately stops observing after handoff (ARCHITECTURE §17.5, §8.5), so a mis-click cannot be
detected, cannot be reported and cannot be reversed. There is no undo to fall back on. **Where there
is no undo, prevention is the only lever available**, which is what justifies spending a rule, a
frozen order and a visible control on it. A future list that carries a comparably irreversible
action inherits the rule for the same reason; a list of purely local reads does not need it and
should not pay for it.

**Why the freeze keys on pointer-within *or* focus-within, and why either alone is insufficient.**
Identity-keyed focus already protects the keyboard user completely: focus is attached to a row, so
the row can move and take the focus with it, and Enter still fires on the row the user chose. **The
physical pointer has no such attachment.** It sits at a screen coordinate; nothing in the DOM moves
it, and re-keying every state by identity does not help it at all. If rows move under a resting
pointer, the click lands on whatever is now at that coordinate — a *different release*, grabbed
irreversibly, with no error and nothing on screen to indicate the substitution. **The two input
paths fail differently and both need the guarantee**, which is why one condition covers both and why
neither half may be dropped as redundant.

### 9.2 The poster grid — the one place a card is correct

A card is justified when the item's primary content is an image, which is true here and nowhere
else. Rules:

- Reserve the box with `aspect-ratio` so nothing shifts. The *empty* card is a `dominant_color`
  fill — never a grey box, and never a shimmer (§4.4.1).
- 🚩 **The title and year sit BELOW the tile, on the chrome's own ground, never over the art.**
  They were set in white bold at the tile's bottom-left directly on the cover, with no scrim, no
  gradient and no plate. The runtime `dominant_color` contrast machinery (§11) does real work and
  still cannot make that safe, because it constrains against a **single averaged colour** and real
  cover art is not one colour: a white title over the light half of a Blue Note sleeve, or over a
  manga cover with a white top-left corner, fails whatever the average says. The reference app
  solves it by not attempting it — Navidrome sets the album and artist names below the cover, in
  the chrome — and so does every \*Arr poster view. **This deletes a subsystem from this surface
  rather than adding a scrim to it**, which is `CLAUDE.md`'s "cut before you add" working in the
  right direction. §11's `dominant_color` rule is **retained without a call site, and that is
  stated rather than dressed up**: it still binds any surface that sets text on a computed fill, and
  after this change **no surface does** — the poster card was the only one, in the mockups and in
  `web/src/app.css` alike. So it is a rule waiting for a case, not a rule doing work, and §11's
  assertion has nothing to run over until one appears. Recording it that way is the point: a rule
  described as active over a surface that does not exist is the invented status `CLAUDE.md` bans,
  and it is also how a deleted subsystem grows back.
- 🚩 **The title is one line, ellipsised, with the full string in `title`.** Moving it below the art
  made card height a function of title length, so a row of cards ended ragged — measured, five of 42
  card renderings took two lines and stood 16 px taller than their 2:3 neighbours. `overflow:
  hidden; white-space: nowrap; text-overflow: ellipsis` restores level rows; `-webkit-line-clamp` is
  refused because the ellipsis is what makes the cut visible, which is what §9.1 tier 1 demands of an
  identity field. **The tooltip is the native `title` attribute, set unconditionally on both the
  title and the art** — measuring each card to decide whether a tooltip is needed is a layout read
  per card on the render path, which §2's first principle rules out; a redundant attribute on a short
  title is far cheaper than a forced reflow per grid paint. Cards still differ in height where the
  *aspect* differs, and that is this section's own rule about aspect coming from the item's art, not
  raggedness.
- Design to the **fixed width allowlist**: `92, 154, 200, 342, 500, 780, orig`. An arbitrary `?w=`
  is refused as a cache-poisoning DoS (§4.4).
- Availability renders per §6.3's rollup rule: `have == total && total > 0` → ✓; `have == 0` → ✗;
  otherwise the fraction ("250/300"). The **"1080p ✓ / 4K ✗"** badge falls out of the M:N link.
- The grid paints from `work` rows as import phase A commits; images fill in behind it. **The grid
  is never blocked on the image queue.**

### 9.3 Forms, and the Add-service wizard

The \*Arr settings anatomy is standardised across three apps that self-hosters already run, and
copying it is free familiarity:

1. A list of configured services, each a row; **`+` opens a type picker**, then a **modal form with
   Test next to Save**.
2. A page-level toolbar with **Show/Hide Advanced** and a **Save** control that reads **"No
   Changes"** and is disabled until dirty (verbatim strings `HideAdvanced`, `ShowAdvanced`,
   `NoChanges`, `Test` from
   [Sonarr en.json](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/Localization/Core/en.json);
   <https://wiki.servarr.com/sonarr/settings>).
3. Per-field help text.

UsArr's wizard is **four fields — kind, name, base URL, API key — plus a live connection test whose
four result states are specified in ARCHITECTURE §17.3 and which must pass before Save is enabled**
(§17.7, §11.3). The **name** was missing from the add flow while the whole Services screen is keyed
on it, so a second Radarr arrived indistinguishable from the first and the "1080p ✓ / 4K ✗" badge —
v0.1's named power-user signal — had nothing to tell the two apart. It is defaulted from the probe
and editable, so the single-instance case still types three things. **A fifth input, `URL base`, is
optional and empty by default**, and it is on the wizard rather than behind `Show advanced`
(ARCHITECTURE §17.3.1): a reverse-proxy sub-path is how a large share of this audience reaches its
\*Arrs, and it is the most likely reason a first connection test fails.

Credentials render as `••••••1a2b` and are never returned by the API. Changing `base_url`'s
scheme/host/port **invalidates the stored credential**, and that has to be visible rather than
implicit: **the key field clears, Save disables, and the form says why** (§17.3.2). Fixing a typo in
a hostname is the most common edit on that screen and it was silently repointing a full-admin
credential at a host the user had just typed. Editing `url_base` alone changes nothing about the
credential, and the form must not pretend otherwise.

Three smaller rules the same forms need:

- **A blocked commit must be reachable by keyboard.** A `disabled` Save is skipped by Tab, so a
  keyboard user never encounters it and gets no signal that a commit exists and is blocked. Use
  `aria-disabled="true"` on a focusable control and swallow the activation.
- **A read-only field must not look editable.** A `readonly` input styled with the same border and
  the same inset fill as the editable one two rows above is a lie told in CSS.
- **Below ~700 px a form is a stacked column** — label above control, every control full width,
  multi-selects at full width with a `size` that fits. The responsive work was done properly for the
  hard case (the tables) and skipped for the form, which at 390 px put labels to the *right* of
  their boxes at unrelated vertical offsets and dropped the submit button into the middle of the
  column.

Error handling follows GOV.UK, which is the best-documented accessible form-error pattern in
public (<https://design-system.service.gov.uk/components/error-message/>):

- **An error summary at the top of the form on failed submit**, receiving focus, each entry linking
  to its field.
- **Every error appears twice** — in the summary and inline immediately before the input.
- The inline message carries a **visually hidden "Error:" prefix** and is **programmatically
  associated** with its input.
- The message says **what is wrong and what to do next**, never only that something failed.

**Native controls.** Native `<select>`, native checkboxes and radios, native form submits, native
focus rings (§17.1). Headless behaviour from Bits UI is used **only** where there is no native
element — dialog, popover/menu, tooltip, tabs — and never to replace a `<select>`.

### 9.4 Dialogs and toasts

Dialogs are rare and deliberate. Fixed grammar, borrowed from the DOS-era modal discipline where
constraint rather than taste produced the clarity: a title, a fixed control order, a
default-focused **safe** option on any destructive confirmation, and focus returned to the invoking
element on close. No dialog traps the user in a mode they must exit before doing anything else
(Apple's *Modelessness*; *Macintosh Human Interface Guidelines*, 1992,
<https://dev.os9.ca/techpubs/mac/HIGuidelines/HIGuidelines-15.html>).

**An overlay that covers the content is modal, or it is not an overlay.** The sidebar below 900 px
is `position: fixed` at `z-index: 40` over the page, with `aria-expanded` on its toggle and **no
`aria-modal`, no focus trap and no `Escape` handler** — so tabbing past the last nav item lands on
content the user cannot see. Either trap focus and mark it `role="dialog" aria-modal="true"` while
open, or apply `inert` to `main` for the duration. The same rule governs any future drawer.

**A destructive confirmation always offers the safe option as a control, not as an escape.** A
prompt whose only buttons are `Open Libraries` and `Remove anyway` has no `Cancel`; navigating away
is technically an exit and is a known pattern failure. And **`Escape` on a dialog holding unsaved
credential input confirms before discarding** — reflexively hitting `Esc` to dismiss a password
manager's popup should not silently throw away a pasted 32-character API key.

Toasts carry the **verbatim upstream error** with **Retry** and **Dismiss** (§7.6, ADR-0012a), and
they are never the *only* place an outcome appears — the affected row shows it too, because a toast
can be missed and a row cannot. Four structural rules, because the markup shape decides whether the
recovery action is reachable:

- **`role="alert"` is not nested inside an `aria-live` container.** `role="alert"` carries an
  implicit `aria-live="assertive"`; inside a `polite` region the behaviour is
  implementation-defined and at best redundant. One region, one politeness.
- **No interactive control inside the alert.** The ARIA APG is explicit that an alert should not
  contain focusable elements, because assistive technology may present the region as a flat
  announcement — which makes `Retry` an announcement rather than a button. The actions go in a
  sibling `role="group"` with its own accessible name.
- **Focus moves to the recovery action when the error is the direct result of a user action**, which
  a failed grab is. Otherwise the toast sits at the end of the document behind everything the user
  has already tabbed past.
- **Success is announced too.** The live region was wired to the failure path only, so a
  screen-reader user pressing Grab heard nothing at all, and a sighted user whose row had scrolled
  away got nothing either — for the one confirmation sentence three documents quote verbatim. Route
  it through the same polite region and keep the row chip.
- **A toast names the object it is about.** Bulk-grab five rows, get two identical failure toasts,
  and nothing says which two.

### 9.5 Status, chips and banners

- A status is **icon + text + colour**, in that order of importance. Removing the colour must leave
  it fully legible — **and removing it for a screen-reader user must not leave nothing**, which is
  §11's empty-accessible-name rule and is the case this rule was already meant to cover.
- **A status indicator never sits inline in a run of prose.** Its position must be a fixed slot, not
  a function of where the sentence happened to wrap. Measured on four consecutive Libraries rows, an
  identical `✓ healthy` badge landed in four different places — line 2 flush left, line 3 flush
  left, line 3 after an orphaned word, line 2 after a figure separated from its unit — while the
  *same* badge two columns to the right sat perfectly aligned on all four. A status whose
  x-position is a function of string length is not a status indicator. **And if the aligned column
  already carries it, the inline copy is redundancy, not reinforcement: delete it.**
- **Chroma marks what is wrong, not what is fine.** On Home's `Have` column the three complete rows
  rendered in saturated green across the whole value while the three rows with 17 missing films,
  857 missing episodes and 34 comics with gaps rendered in neutral grey — so on the screen whose job
  is "what needs my attention", the eye is pulled to the good news. §1.1's own rule is already the
  answer — *"Grey is a status. A healthy row is neutral"* — and it inverts here: **a complete row is
  a muted `✓` and grey text; an incomplete row carries its gap figure in the warn role.** The
  grammar is normalised with it, because six rows carried six grammars for one fact
  (`X have · Y wanted`, `X / Y episodes`, `X albums · Y tracks`, `X have · Y h`, `X have`,
  `X issues · Y with gaps`) and no two of them can be compared.
- 🚩 **A confirmed grab is a NEUTRAL chip, and this rule caught the mockup breaking it.** The
  Requests screen painted a successful `sent` chip green (`--status-ok`) while Recent grabs rendered
  the identical stored fact neutral — two renderings of one state in two colours, on one screen.
  The rule above already decides it: a successful handoff is not something wrong, so it takes no
  chroma. **The app shipped it neutral first** (`web/src/routes/requests`, merged `db93781`) and the
  mockup now follows. ⚠️ **The withdrawal is success, not status.** `pending` keeps the warm-orange
  warn role and `failed` keeps the error red, because both mark something the user may need to act
  on — and the ambiguous *sent, outcome unknown* row moves **from green to warn** for the same
  reason, which is where the shipped screen already had it.
- System tags render as chips you can filter by but not delete (`is_system`, ADR-0015). Tag chips
  are neutral; `tag.color` is the only colour field in the data model and is user-controlled, not
  chrome.
- **Degraded instance → a small non-modal banner** naming it: "Kavita is unreachable — showing
  cached data from 14:02, 6 minutes ago", linking to Services. **The catalogue does not grey out** and shows no
  spinner. Writes are accepted with the label "queued — Radarr 4K is unreachable" — which keeps an
  \*Arr on purpose, because writes to a media backend do not exist (§17.7).
- **Needs re-identification → a blocking banner** on that instance's rows and on Services, because
  §7.4's guard 2 exists precisely to stop a silent library-destroying sweep. Loud on purpose.

### 9.6 Empty states

Every empty state is **one sentence naming why, plus the one or two buttons that fix it**, and
**filtered-empty is a different message from empty**. The \*Arrs already do exactly this, and the
strings are worth reading as a house style:

| App | Key | Text |
|---|---|---|
| Sonarr | `NoSeriesFoundImportOrAdd` | "No series found, to get started you'll want to import your existing series or add a new series." |
| Sonarr | `AllSeriesAreHiddenByTheAppliedFilter` | "All results are hidden by the applied filter" |
| Prowlarr | `NoSearchResultsFound` | "No search results found, try performing a new search below." |

([Sonarr en.json](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/Localization/Core/en.json);
[NoSeries.tsx](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/Series/NoSeries.tsx),
which renders the message plus **Import Existing Series** and **Add New Series**, and swaps to the
"hidden by filter" message when `totalItems > 0`.)

**The rule needs enforcing, not restating, so here it is in a form the ban list can grep.** §9.6 has
said "one sentence, no centred marketing block" since it was written, and the design shipped a
centred 24 px hero — larger than the page's own H1 — inside a dashed box with a three-to-four-line
centred paragraph and a centred button pair, byte-identical across seven states on five screens,
made writable by a `--fs-empty` / `--text-empty` token §1.2 had granted an exemption for. In an
artefact where every other region was tuned by hand, the one region built by applying a template is
the one a reader will point at. **The exemption is withdrawn, the token is deleted from
`tokens.css`, and the rule is now four constraints that a linter can check rather than a sentence a
reviewer has to remember:**

1. **The heading is `--text-lg` (16/600) and it is an `<h2>`.** No token above `--text-xl` exists;
   the empty-state heading is not the page title and must not out-size it. It was bold text rather
   than a heading in all eight empty states, so a screen-reader user navigating by heading on the
   **first-run screen** found one heading — "Home" — and had to read linearly to discover that
   nothing was configured. The heading hierarchy is otherwise clean and this was the one gap.
2. **Everything is left-aligned at the same content edge as the table or grid it replaces.** No
   `text-align: center` on an empty state, on its heading, on its sentence or on its buttons. The
   `[grep]` rule in §13 previously exempted empty states from the centring ban; that exemption goes
   with this one, leaving dialogs as the only exception.
3. **No container.** No dashed border, no box, no panel, no background step. The empty state is not
   a component sitting in the region; it *is* the region's content. It should look like the top of a
   table that has no rows, because that is what it is.
4. **One sentence.** Not three. If the explanation genuinely needs more — and the first-run
   `unconfigured` state is the case where it might, because it is teaching the product rather than
   reporting a state — the extra sentences are a separate paragraph below the buttons at
   `--text-base`, not a wider centred measure above them. ⚠️ **This is the one place the rule and
   the drawn screens still disagree**: the `No services configured` copy is four sentences and is
   genuinely good, and either the rule is wrong for first run or that state is. Recorded rather than
   resolved by fiat; it is the smaller of the two open copy questions and it belongs to the pass
   that rewrites the state.

The model is the one this section already cites and did not follow: Sonarr's `NoSeries.tsx` renders
its message plus two buttons **in the content flow, left-aligned, in body type, with no box**.

No illustration. No centred marketing block. **Never fabricated data in a shipped product surface**
— not in an empty state, not in a screenshot, not in documentation. **The one exception is a design
mockup, which must say on every page that its data is fabricated** — not only in a sibling README,
because the single-file published build is detached from it and is the artefact most likely to
reach an outside reader. §13 carries the same exception in its greppable form; the two must not
drift apart again, and the earlier absolute wording forbade the artefact that makes the rest of this
document reviewable.

⚠️ **The label has to name the milestone as well as the fabrication, and this is where the exception
nearly swallowed the rule.** The mockups draw two installs behind a switcher: a **full stack** with
every catalogue source connected, six media types populated, which is the default because six
populated types is what the layout must survive; and the **v0.1 install** beside it, drawn as
BookOrbit and Prowlarr per [ADR-0052](../DECISIONS.md#adr-0052), which moved v0.1's catalogue source
off the Kavita [ADR-0041](../DECISIONS.md#adr-0041) had named. **The strings below are quoted as the
assets *render* them rather than corrected in place**, and that rule is what keeps this passage
correct rather than what makes it stale: a quotation that no longer matches the asset would make
this section unreviewable against it, so the quotation moves **when** the asset moves and never
before. ✅ **The re-draw landed in `a1995f9`**, across the five screen pages, the generated
`prototype.html` and `design/mockups/README.md`; between ADR-0052 and that commit this section quoted
`v0.1: Kavita, Prowlarr` and said in the same breath why it did, which is the same rule running in
the other direction. Do not "fix" a quotation here ahead of the asset it quotes.
A reader who meets the default and is told
only *"this data is invented"* has been told the numbers are made up and left to assume the
**stack** is real for the milestone being discussed, which is `CLAUDE.md`'s "no invented status"
failure reached by omission rather than by assertion. So: **the switcher's own option labels place each stack against a milestone**, not merely
name it (`Full stack: a later milestone`, `v0.1: BookOrbit, Prowlarr` — colon-separated because
both are short strings and §13 bans U+2014 in a string under fifteen words, a rule that had never
reached `<option>` text until `check.mjs` §1b's corpus was widened to the strings that never lay out
as a block); **the permanent notice
states which install is drawn and changes with the selection**; and the switcher lives **inside the
notice**, because it is not a product control and drawing it in the product chrome would fabricate a
setting. "A later milestone" rather than a number is deliberate — ARCHITECTURE §16 sequences the
catalogue sources one at a time after v0.1 and has not fixed which release each lands in, so a
number here would invent exactly the status the rule forbids. `check.mjs` §8b enforces all four — and note what it enforces, because it is what made the
stale name a scope error rather than a gate failure: §8b asserts that each label matches
`/v0\.1|milestone/i`, not which service it names. ✅ **This section predicted that
`v0.1: BookOrbit, Prowlarr` would pass §8b unchanged whenever the re-draw landed, and `a1995f9`
landed it — §8b stayed green and was not edited with it.** Recorded rather than deleted, because a
prediction that was made and then held is the evidence that the assertion is the right shape: a
§8b that had pinned the service name would have gone red on the re-draw and had to be re-written
alongside it, which is a guard tracking the asset rather than governing it.

### 9.7 The minimum component set, and where per-type divergence is allowed

> **One amendment before the argument: "shape from the image, not the type" holds inside a
> single-type grid and costs legibility in the one container where all six types meet.** Home's
> Block C posters view is a unified grid across every type, so 2:3 film posters and 1:1 album covers
> land on the same visual row and their per-card meta lines (`album ✓`, `movie ✓`) sit at four
> different heights. **In the unified grid the card *box* is one shape and the image is fitted
> inside it**; per-type grids keep the image-derived shape unchanged. The rule below is right; it
> just needed the case where the premise (one type per grid) is false.

**Five components cover all six media types.** The evidence that this stretches further than intuition
suggests is Jellyfin's, and it is decisive: it serves movies, shows, music, books, audiobooks, photos,
live TV and home videos from **one** card builder whose shape is chosen from the *image*, not from the
media type —

```js
if (primaryImageAspectRatio >= 3)         { options.shape = 'banner'; }
else if (primaryImageAspectRatio >= 1.33) { options.shape = getBackdropShape(...); }
else if (primaryImageAspectRatio > 0.8)   { options.shape = getSquareShape(...); }
else                                      { options.shape = getPortraitShape(...); }
```

— four shapes, chosen from data, with **not one branch on media type**. Sonarr and Radarr define
exactly three view modes across their whole application; Navidrome offers two.

| # | Component | Covers | Notes |
|---|---|---|---|
| 1 | **`ItemTable`** | ebooks, tracks, albums, audiobooks, episodes, comic issues, films, search results, release candidates, services, the item-detail per-instance table | The default container (§5.4). Columns come from a **column registry keyed by type**; the user-facing control is the existing **"Options"** column picker (§9.1) |
| 2 | **`ItemGrid`** | every type's cover view | One card, `aspect ∈ {2:3, 1:1, 16:9}`, chosen from the image's real ratio. `variant="strip"` is a **modifier** adding `overflow-x`, not a second component |
| 3 | **`SectionHeader`** | Home blocks, search groups, type screens | Name · count · "see all" |
| 4 | **`LevelBar`** | TV, music, comics, multi-book series | **The one genuine per-type addition** — below |
| 5 | **`ScopeChip`** | the library selector | §8.1. ⚠️ **Specified and not built** — see the note directly below |

> ⚠️ **`ScopeChip` is specified and UNBUILT, recorded 2026-08-19. It is deferred, not cut, and it
> stays in the minimum set** — this table is what someone reads to decide what to build next, so an
> unmarked entry reads as buildable. **What blocks it is the wire, not effort**, and the
> precondition is one line: `GET /api/v1/library/recent` and `GET /api/v1/search` would both have to
> accept `lib`, and the wire would need a spelling for *"no libraries selected"* — today an absent
> `lib` means everything and an empty one is a `400`, so §8.1's third grammar and §10's `scope-empty`
> cannot be expressed at all. **The measurement is written up in [`ROADMAP.md`](../ROADMAP.md) §2,
> under *"The `?lib=` chip"*,** over the wire facts at
> [`reference/http-api.md`](../reference/http-api.md) §1.1, §6.1 and §7.3. It is pointed at rather
> than restated here, because a second copy of a measurement is the copy that goes stale.

Aspect assignment is **data, not code**: portrait 2:3 for films, series, ebooks, audiobooks and comic
series; square 1:1 for albums and artists; 16:9 for episodes. Cross-checked against §9.2's fixed width
allowlist `92, 154, 200, 342, 500, 780, orig` — those work for 2:3 and 1:1 alike, so **no
image-pipeline work is implied**.

**Hierarchy depth is the only genuine per-type divergence, and it is a breadcrumb over an unchanged
container.**

| Type | Levels |
|---|---|
| Movies | 1 — work |
| Ebooks / audiobooks | 1–2 — (series →) book |
| Comics / manga | 2 — series → issue |
| TV | 3 — series → season → episode |
| Music | 3 — artist → album → track |

A flat grid genuinely cannot express "artist → album → track" — but **the fix is a level control in
the toolbar, not a different screen**, and the container below it is `ItemTable` or `ItemGrid` at
every level. Komga ships exactly this: a row of level buttons (Series / Books / Collections / Read
lists) above an unchanged grid, **hiding the levels with zero content**. ARCHITECTURE §6.1's
three-layer core already carries the model, so the level set is **data** and `LevelBar` renders
whatever the type declares.

Two more divergences are **config, not components**: the **primary metric column** (films: quality +
size; TV: `250/331`; music: tracks + duration; audiobooks: duration + narrator; ebooks: format + size;
comics: issue count **and its gap list**, never a fraction against a moving total — ARCHITECTURE §6.1)
and the **default sort** (films by added, music by artist, ebooks by author, comics by series then
issue number). One row each in a registry.

**Not warranted, and each already has a rule against it:** per-type row heights or padding (§5.3
defines three density steps for the whole app); per-type colour or accent (§3 — chrome is achromatic,
colour is status); per-type card chrome or hover behaviour (§9.1 — do not vary row treatment by media
type); a bespoke music screen with a player or a bespoke reader (CLAUDE.md principle 2); per-type
empty states beyond the one sentence §9.6 specifies.

> **The review test, and it is the useful one:** if a proposed per-type divergence can be expressed as
> *a different column set, a different default sort, or a different number of levels*, it is config
> and it ships. **If it needs a new component, it needs an argument in `REVIEW-LOG.md`.**

---

### 9.8 Promoting a page-scoped style into a shared class

§9.7 says which components exist. This says what may go *inside* one when a page-scoped style is
promoted into it — the split that has to be made line by line, and that had no written home until
now.

> **The rule.** A promoted component class owns **what the thing looks like**. The caller owns
> **where it sits and how big it is**.

**The acceptance test is the operative half, because "appearance versus layout" is not a distinction
anybody applies consistently by feel:**

> ⚠️ **If a second caller in a different container would have to undo a declaration, that
> declaration is not appearance and does not go in the shared class.**

Run it on every line, one at a time, and let it decide. It is not a summary of a judgement made some
other way.

**The worked example, because the hard cases do not look like layout — `min-width: 0`.** It reads as
housekeeping: a zero, no visible effect, the sort of line that gets swept into a shared class
without anybody stopping on it. It is layout, and it is the example that teaches the test.

It overrides the **flex automatic minimum size**, which for an `<input>` is its `size`-derived
intrinsic width. Set to `auto`, the field refuses to shrink below that intrinsic width and pushes
the row wider than its container.

📌 **Measured by the frontend thread at `07f89b0`, in Chromium, against the real `pnpm build` output
of `web/src/routes/+page.svelte`'s search input at `--text-md` (14 px)** — not read off the
specification and not inferred:

* The input's intrinsic width is **177 px**.
* With `min-width: auto` at a **160 px viewport**, the field held **177 px** and the document's
  `scrollWidth` went **160 → 193** — the page scrolled sideways.
* It **binds below roughly a 209 px viewport and is inert above it.**

So it is a floor, not a look, and a caller that *wanted* the field to hold its intrinsic width in a
row would have to undo it. It correctly stayed page-side on **both** callers — `.homesearch__input`
in `web/src/routes/+page.svelte` and `.searchbar__input` in `web/src/routes/requests/+page.svelte`
each still declare `min-width: 0` themselves.

🚩 **This is the case worth remembering, because someone applying "appearance versus layout" by
intuition puts `min-width: 0` in the shared class every time.** A property that is invisible, that
does nothing at any viewport you happen to be looking at, and that has no colour, no size and no
type in it, is still layout if a container can be built that has to override it.

**The second test, and it is free at promotion time.**

> **A property the existing copies already disagree on is proven to belong to the caller.**

You are promoting because two or more page-scoped copies exist. **Diff them before you promote.**
Every property they already disagree on is *evidence*, not an inconsistency to reconcile — the
disagreement is the two containers reporting, in advance, that a shared value would be wrong for one
of them.

The instance: the two search bars' flex `basis` differed — **`1 1 20rem`** on the Requests toolbar,
whose row carries a label, a select and two buttons, against **`1 1 24rem`** on Home, whose row
holds the field and one button. The rows carry different sibling sets, so the numbers disagreed
**from the day they were written**. A shared basis would have been wrong for one caller on the day
it landed. Both stayed page-side.

Home's `max-width: 42rem` is the same shape without needing the diff: it is the §1.5 fix that stops
the field reading as a hero-search bar, Requests wants no cap, and it survives untouched in Home's
own `.homesearch__input`.

**A promotion must not add behaviour.**

> ⚠️ **A promotion moves what exists. Anything else is a new design decision, and it should be made
> on its own terms rather than smuggled in behind a refactor.**

The shipped `.searchfield` in `app.css` §2.3 carries **exactly seven declarations** — `min-height`,
`padding`, `background`, `color`, `border`, `border-radius`, `font-size` — which is the intersection
of the two page-scoped copies and nothing more.

§10's state set names a `::placeholder` treatment, a disabled surface, an invalid border and an icon
slot as belonging to this component, and **neither copy had any of them**; both screens render the
placeholder on the UA default today. The promotion **named that gap in the class comment as a seam
and authored none of the four.** The frontend thread's phrase for the alternative is the one to
quote: **"a restyle wearing a promotion's clothes"**. Four states nobody had shipped, arriving in a
commit whose subject is a refactor, is a change to two screens that no reviewer was asked to look
at.

ℹ️ **This is not §10 being waived.** §10 says a component ships with a demonstrated state set; the
promotion commit is not what ships that set. The seam is written down so the next author reads the
gap as a gap rather than as a rule — and `.input` above it, which already carries a `::placeholder`
colour and an `[aria-invalid]` border, is named there as the precedent to copy when they land.

**And it was deliberately not a modifier.** `.searchfield` is **not** `.input--search`. `.input` is
eight declarations: **four are identical** here — background, colour, border, radius — and **four
are differences that were already shipping**.
The floor is `--control-h` (32 px) and not `--control-h-sm` (28 px); the inset is `--space-4` and
not the `--space-2`/`--space-3` pair; the type is `--text-md` and not `--text-base` (§4.3 names
`--text-md` for form inputs); and `max-width: 100%` is absent because width is not this class's
business.

> **A modifier that disagrees with its base as often as it agrees is not a modifier.** It carries
> four lines forward and overrides the other four, and costs two classes on every caller to save
> four declarations in one place.

**The counterpart to §9.7's review test:** a shared class earns a *declaration* by passing the undo
test; a second class earns its *existence* when a modifier would spend as many lines undoing its
base as inheriting it. Both answers go in the class's own comment, next to the code, because the
next author to touch it is reading the CSS and not this section.

---

## 10. The required state set per component

**This is the strongest positive signal in the whole anti-goal corpus, and it is the one that maps
onto UsArr's own principles rather than onto taste.** "Only the happy path exists" is named
independently by mania.design ("Missing Edge States"), gendesigns.ai and kiwibreaksme ("inert hover
states"); Hallmark requires an 8-state demo for any component it emits. And CLAUDE.md principle 3
says a system must **degrade honestly when a service is absent** — which means **UsArr's
non-happy paths are the product**, not an edge case.

Every component ships with a demonstrated state set. Missing states are a review failure, not a
follow-up:

| State | What it must show | Notes |
|---|---|---|
| **default** | the data | — |
| **hover / focus / active / disabled** | per §3 and §6 | hover is colour only; focus is never removed |
| **loading** — *only where it genuinely applies* | per §7's tier | Tier 0 components have **no loading state at all**, and inventing one is the failure |
| **empty** | why it is empty + the fix | — |
| **filtered-empty** | a **different** message: the filter is responsible | Sonarr's `AllSeriesAreHiddenByTheAppliedFilter` is the model |
| **scope-empty** | that the **library scope** — not the query, not the filter — is why there is nothing, and the one control that undoes it | *"Your library scope is set to 0 of 8 libraries, so nothing is shown."* + **Show all libraries**. Required on Home, on Search and on every per-type grid. It is a **third** thing: `empty` says you own nothing, `filtered-empty` says the filter hid it, and neither is true when the scope hid it. Reachable in two clicks — unticking the chip's `Everything` box unticks all eight — and without this state the whole application renders with no content and no explanation. Worse on Search, where §17.4 rule 1 means a zero-hit group does not render at all, so the screen draws literally nothing. **And the chip that caused it must be reachable at the viewport the user is on:** below 900 px the sidebar collapses to a drawer and takes the chip with it, so whenever the scope is not "all libraries" the chip renders in the top bar instead (§8.1, ARCHITECTURE §17.2 and §17.7) |
| **partial** | what arrived, what did not, and that more may come | "4 of 9 indexers responded"; "250/300" |
| **stale** | the data is real but old, with the timestamp | `"stale": true` from §2.3. Not greyed out |
| **error** | the **verbatim upstream text**, plus Retry / Dismiss | §17.3's "Problem" column is verbatim by requirement, rendered in mono |
| **unconfigured** | the service does not exist; what adding it would give you; a link to Add | The Search-and-Grab first-run copy (§8.5) is the reference tone |
| **permission-denied** | that it is denied, without leaking existence | v0.1 has one account, so **what exists from day one is the *behaviour*, not a drawn screen**: §14 rule 6 plus §1.3's access-scope parameter mean an item the caller may not see is **absent from the response**, so the honest rendering is the ordinary `empty` or `filtered-empty` state and there is nothing distinct to draw. A visibly *denied* surface is a v1.0 screen and arrives with `user_library_access`. (Services' `denied` state is a sudo re-authentication prompt — a different thing, and it should not be read as this one.) |
| **re-authentication-required** — *Services, and every screen that writes a credential* | that the sudo window closed, that nothing was lost, and one password field that **retries the pending action** on success | The API answers `403` with `error: sudo_required` on all five service writes (ARCHITECTURE §17.3.3). It is a **prompt, not an `error`**: no verbatim block, UsArr's own words, and the raw code only on a muted `for the record` line. It must be distinguishable in the code from the other two 403s — `forbidden` (retrying cannot help) and `csrf` (reload) — so the screen branches on `error` and never on the status alone. Rendering all three the same way is the failure this row exists to catch |
| **credential-re-entry** — *Services* | that the saved key is bound to the old address and will not be sent to the new one; the cleared key field; the address it *was* stored against | `400` `credential_reentry_required` from `PATCH /api/v1/services/{id}` and `POST /api/v1/services/{id}/test` (ARCHITECTURE §17.3.2). Triggered by a change of **scheme, host or port** and by nothing else — not the path, not `url_base`, not letter-case, not an explicitly written default port. Also not an `error` state: nothing was sent upstream, so there is no verbatim text and inventing one would be a fabrication. The form pre-empts it while the user types **and** renders it from the response, because pre-empting alone turns the server's refusal into an unexplained failure |

The Services screen is the sharpest test of this. **It must be *more* informative when things are
broken than when they are fine** — a screen that is a wall of green dots when healthy and a wall of
red dots when not has failed. §17.3 already specifies the content: the actual error text verbatim,
the \*Arr's own `/health` warnings surfaced, the breaker state and next retry, measured clock skew,
and **one button per failure state that names its fix** (unreachable → Test connection; 401/403 →
Update API key under sudo mode; TLS pin changed → show both fingerprints and require explicit
accept; needs re-identification → Re-link; degraded → Run full sync now).

Sonarr's health panel is the shape to borrow: **a list of named problems, each row
`severity icon | message | fix link`**, with the wiki link at the end of the row
(<https://wiki.servarr.com/sonarr/system>, whose table of contents is literally a taxonomy of
failure modes). Not a green dot.

---

## 11. Accessibility floors

Density is never a reason to breach these.

| Requirement | Level | Floor | UsArr target |
|---|---|---|---|
| [SC 1.4.3 Contrast (Minimum)](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html) | AA | 4.5:1 normal text, 3:1 large (≥18.5 px, or ≥24 px equivalent) | **≥12:1 primary, ≥5.5:1 muted metadata** |
| [SC 1.4.11 Non-text Contrast](https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html) | AA | 3:1, **explicitly unrounded** — "2.999:1 would not meet the 3:1 threshold" | **≥3.2:1** on control boundaries and the focus ring |
| [SC 1.4.12 Text Spacing](https://www.w3.org/WAI/WCAG22/Understanding/text-spacing.html) | AA | survive line-height 1.5×, paragraph 2×, letter 0.12×, word 0.16× | `min-height` everywhere; never clip overflow on a text container |
| [SC 2.5.8 Target Size (Minimum)](https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html) | AA | 24×24 CSS px, with the spacing exception | 28 px compact rows carry no inline targets; ≥32 px for repeated actions |
| SC 2.1.1 / 2.1.2 Keyboard, No Keyboard Trap | A | all functionality keyboard-operable | plus a real keyboard model (below) |
| [SC 2.4.1 Bypass Blocks](https://www.w3.org/WAI/WCAG22/Understanding/bypass-blocks.html) | **A** | a mechanism to skip repeated blocks | **a `Skip to content` link as the first tab stop.** Landmarks are an accepted technique and screen-reader users have `main`, so this was not a strict failure — but **21 tab stops** (top bar + 15 nav rows) precede content on every screen, identically, and a keyboard-only sighted user has no landmark navigation at all. The criterion was missing from this table while 2.1.1, 2.1.2 and 2.1.4 were named |
| [SC 2.1.4 Character Key Shortcuts](https://www.w3.org/WAI/WCAG22/Understanding/character-key-shortcuts.html) | **A** | a single-character shortcut must be **turn-off-able**, **remappable**, **or active only on focus** | **a Settings toggle satisfies "turn off" for all five at once** — see below |
| SC 2.4.7 Focus Visible | AA | a visible indicator | never removed to buy density; `:focus-visible`, so mouse users do not see rings |
| [SC 2.4.11 Focus Not Obscured](https://www.w3.org/WAI/WCAG22/quickref/) | AA | focused component not entirely hidden | sticky headers are the usual violator — test it |
| [SC 2.4.13 Focus Appearance](https://www.w3.org/WAI/WCAG22/Understanding/focus-appearance.html) | AAA | ≥2 px perimeter, 3:1 between focused and unfocused | met: 2 px ring at 16.50:1 / 15.33:1 |

**The one that bites dense UIs**, and the reason it is called out separately: the muted grey used
for secondary metadata **is text**, so it must clear 4.5:1. There is no 3:1 shortcut for it. UsArr
targets 5.5:1 to leave slack for theme tuning. Row dividers, by contrast, are decorative separators
and are genuinely unbound — but a border that is the *only* cue a control exists **is** bound by
SC 1.4.11, which is why `tokens.css` separates `--border` from `--border-strong`.

**The one colour in the system that is *data*, and the rule it needs.** Every token above is a fixed
value that can be checked once. `dominant_color` is not: ARCHITECTURE §4.4.1 computes it at runtime
as one average over the 92 px poster fetch. The poster card **used to** render the title (12 px /
600) and year on top of it, nothing constrained the pair, and the shipped sample data failed —
`#16130e` on `#7d6a4f` is **3.57:1** for the title and **3.12:1** for the year, against 4.5:1 for
both. One bad hand-picked swatch would have been a nit; having no rule was the finding, because with
an average taken over arbitrary cover art, mid-luminance fills are common and *both* black and white
land near 3.5:1 on them.

**§9.2 resolved that by moving the text rather than by constraining the colour**, and the rule below
survives it as a general one. The reason the move beats the constraint is worth keeping: a solver
constrains against a **single averaged colour**, and real cover art is not one colour — a white
title over the light half of a Blue Note sleeve fails whatever the average says. The poster title
and year are now ordinary `--fg` / `--fg-muted` on a known ground, which the contrast sweep above
already covers.

> **Where a surface sets text on a computed fill, pick whichever of the two theme text tokens
> scores higher against the computed `dominant_color`. If the winner is still below 4.5:1, adjust
> `dominant_color`'s lightness — away from the text colour, in 2% steps in OKLCh, preserving hue and
> chroma — until it clears. The fill is decoration; the title is content, and content wins.**

Two supporting rules, because otherwise the ratio is not computable from what ships. **Neither the
title nor the year carries `opacity`** — compositing changes the effective ratio (by ~0.45 on the
measured pair) through a mechanism no contrast check sees, so the year gets a real colour token.
And **12 px semibold is normal text under WCAG, not large** (large is ≥18.66 px bold or ≥24 px), so
4.5:1 applies to both lines. **To be asserted over any computed-fill / foreground pair that ships in
a fixture** — in the design gate, and in a CI if one is ever added — and §13's checklist carries the
entry. **The assertion is retained deliberately, and
the reason is the shape of the rule above:** a conditional rule needs a *standing* guard, because a
guard added by whoever writes the first call site is a guard that call site had to know about
first — which is the same as having no rule. It binds the moment any surface sets text on a computed
fill, and it binds in the image pipeline where the colour is produced regardless (ARCHITECTURE
§4.4.1). **Which surfaces do that is not this section's to say** — §9.2's poster-grid entry owns the
call-site question, and the tree owns the answer. §11 fixes the rule; it does not keep an inventory.

**Two ARIA requirements the grid-row primitive creates, both stated as requirements rather than as
review items**, because a hand-built grid supplies nothing a native `<table>` supplies for free
(§7.4, ARCHITECTURE §4.5).

- 🚩 **`aria-rowcount` on the grid and `aria-rowindex` on every row — including the header row —
  wherever the rendered rows are a window onto a larger set.** That is every list in the product, for
  two reasons the design has already committed to: **"Load more" over keyset pages is the primary
  pagination interaction** (ADR-0029), so the DOM holds a prefix of the result set by construction;
  and **`content-visibility: auto` skips the contents of off-screen rows**, so a row whose contents
  are skipped is a row whose position in the whole set an assistive technology cannot compute.
  Indices are **1-based over the full set, not over the rendered window**, and `aria-rowcount="-1"`
  is the correct value when the total is genuinely unknown — ARIA defines it for exactly that.
  **Without them a screen-reader user is told "row 3 of 26" when the truth is "row 3 of 1,204"** —
  a confidently wrong number, which is the failure mode `CLAUDE.md`'s honesty rules exist to prevent,
  arriving through the accessibility tree instead of through the UI.
  Note the asymmetry with the attribute that *is* everywhere: `aria-colindex` carries no information
  here, because every column is present in the DOM and the implicit index is already right. It is
  harmless and may stay; it must not be allowed to stand in for the two attributes that carry
  something.
  ⚠️ **Unverified, and it is a tooling limit rather than a defect:** the reviewer who found this
  could not confirm `aria-colindex`/`aria-colcount` from the Chrome DevTools Protocol accessibility
  tree, because **CDP exposes neither column nor row indices as node properties for a native
  `<table>` or for an ARIA grid** — established against a purpose-built control page with both side
  by side. Absence from that tree is therefore not evidence of anything. **Verifying these attributes
  needs a real screen reader**, which is what GOV.UK's own strategy (cited below) says about
  automated checking generally.
- 🚩 **A row expander is a second `role="row"`, it consumes a row index, and both index and count
  have to account for it.** §17.3 makes the expander a requirement rather than a nicety — the breaker
  state, an *Arr's own health warnings and the verbatim upstream text all live behind it, because
  §9.1 is explicit that an explanation is not a cell value at all — so the primitive owns it and the
  contract is stated here rather than rediscovered per screen. **It cannot be a `region`**, however
  much better that would read: a `rowgroup`'s owned elements are rows and nothing else, so an
  expander that is not a row is a child its parent may not own. Four consequences, and the last two
  are the ones a hand-written implementation gets wrong:
  **(a)** it renders *only* when open, so a collapsed list costs nothing in the DOM, which is what
  keeps "Load more" cheap on a screen whose rows are otherwise one line;
  **(b)** `colspan` means nothing to a grid, so the spanning cell says so in grid terms
  (`grid-column: 1 / -1`) and repeats the fact to the accessibility tree with `aria-colspan`;
  **(c)** because an open expander is a real row, `aria-rowindex` is a **running total** rather than
  `offset + i + 2` — the moment one row opens, position arithmetic done in the template is wrong for
  every row beneath it, and a confidently wrong position arriving through the accessibility tree is
  exactly what the bullet above exists to prevent;
  **(d)** and `aria-rowcount` has to include the open expanders for the same reason, or the count
  and the indices disagree and the user is told "row 9 of 7". Bumping the index without bumping the
  count is the natural half-fix and it is worse than neither.
  **The expander carries no row identity**, which is what keeps it out of the roving model: the list
  stays one tab stop and arrowing walks services, not services and their expanders alternately. That
  is the same requirement as the keyboard model below, reached from the other side.

  📌 **Written against the shipped component, not against a sketch: `web/src/lib/List.svelte`
  implements all of the above** — `laidOut` computes the running index, `declaredTotal` adds the open
  expanders to the count, and the expander `<tr>` is the one row in the list with no `data-key`.
  **Two places the implementation is more specific than this document was**, both recorded here so
  the contract and the code do not drift:
  **`--row-lines` and `--row-ci` are the same seam in different units.** The mockups declare a
  unitless multiplier per list and compute
  `auto calc(2 * var(--row-py) + var(--row-lines) * var(--lh-base))`; the component declares
  `--row-ci` as a **measured content-box height in px**, defaulting to `ROW_INTRINSIC` per density
  and overridable per list via the `rowIntrinsic` prop. Both satisfy §7.4's "declared per list from
  that list's own rendered rows"; neither is the other's literal value, so a number must never be
  copied between them.
  **And the component writes `--cols` and `--row-ci` through `element.style.setProperty()`**, because
  the server sends `style-src 'self'` with no `'unsafe-inline'` and no `style-src-attr`, so a `style`
  attribute falls back to `style-src` and is refused. That is the same constraint `check.mjs` §1d
  enforces over the mockups, and the same reason CSSOM mutation is explicitly not banned there.

  🚩 **A CORRECTION TO THIS DOCUMENT'S OWN CLAIM, and it is the useful half.** This paragraph used to
  end *"it stays in the DOM and applies nothing"*. **That is false**, and the true statement is
  narrower and more useful: **a reported CSP violation is not evidence the declaration was dropped.**
  Three paths, measured against the header `internal/httpapi/middleware.go` actually sends, in
  Chromium **141.0.7390.37**:

  | Path | Violation reported | `style.length` | Computed | Verdict |
  | --- | --- | --- | --- | --- |
  | `el.setAttribute('style', …)` | yes | **0** | `position: static` | **genuinely blocked** |
  | `<template>.innerHTML` carrying `style=`, then cloned in | yes | **5** | `position: absolute` | **applied** |
  | `el.style.setProperty(…)` | **no** | 1 | `position: absolute` | applies — the CSSOM carve-out |

  **The first row is the control, and without it this finding reads as the directive being
  toothless.** `setAttribute` is the path a screen author actually reaches for, and it is stopped
  dead — so §1d's ban and its conclusion are unchanged. Exactly one construction path gets past, and
  it is **Svelte 5's**: the framework builds a fragment by assigning to a `<template>`'s `innerHTML`
  and cloning the content in, which is how SvelteKit's route announcer gets its hiding styles.
  💭 **Inference, not measurement:** the reason is presumably that a `<template>`'s contents belong
  to an inert document outside the page's CSP, so the attribute parses into a populated declaration
  block there and the clone carries it in already-parsed. The behaviour is verified; Blink's reason
  for it is not, and it is marked so nobody cites it as a fact about the spec.
  📌 **The practical rule: check the computed style before concluding anything was blocked.** A
  console violation tells you the browser objected, not that it won.
- **No status glyph may have an empty accessible name**, and availability is the case that matters:
  §9.5 already requires *"icon + text + colour, in that order of importance"* and that *"removing
  the colour must leave it fully legible"*. An icon-only ✓ or ✗ leaves **nothing** — a screen-reader
  user hears identical silence on the film they have and the film they do not, which defeats the
  product's central question for a whole class of user and is an SC 1.1.1 (Level A) failure. Every
  availability rendering carries the word from ARCHITECTURE §6.3's rollup — `Have`, `Missing`, or
  the existing visible fraction for a partial — as visible text or as a visually-hidden span, with
  the glyph `aria-hidden`. **`[grep]`-able as: no element carrying an availability or status class
  may compute to an empty accessible name.** This is the second time an icon-only status has slipped
  through, which is why it is a lint line in §13 and not a note here.

**Keyboard model, beyond the success criteria** (this is a power tool):

- **A list or grid is one tab stop; arrow keys move within it** — roving tabindex, per the ARIA APG
  (<https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/#kbd_roving_tabindex>). Ten
  thousand tab stops is not a keyboard model. This is the Motif traversal model, which is the
  ancestor of the pattern (OSF/Motif Style Guide rev 1.2, 1993).
  **Two rules that decide whether "one tab stop" is true in practice**, both of which a naive roving
  implementation gets wrong. **(a) The row is the *only* stop**: everything interactive inside it is
  `tabindex="-1"`, reached by arrowing plus `Enter`/`Space` or a per-row menu. Adding the row as a
  stop *on top of* the links already in it makes a seven-row table eight tab stops, not one, and
  arrowing from a focused link then jumps to the next *row* rather than the next link. **(b) The
  assignment must be idempotent and must run after every append.** ADR-0029 makes "Load more"
  appending the primary interaction, so a `tabindex` set once at init is wrong within one click: a
  cloned row inherits `tabindex="0"` and becomes an extra stop, while a row built from a template
  gets none and `focus()` on it silently does nothing — an arrow key that looks dead. Either call
  the assignment from the append handler, or use a delegated `:focus-within` model that needs no
  per-row attribute at all.
  **And an arrow key needs the *adjacent* row, not a rescan of the list.** A `querySelectorAll` plus
  an ancestor walk per keypress is O(rows) per key: measured at **1.18 ms at 1,000 rows, 9.69 ms at
  5,000 and 55.12 ms at 25,000** — 60% of a 16 ms frame at 5,000 on a *desktop*, and 2.25 s of main
  thread for one second of key repeat at 25,000. Walk `nextElementSibling` / `previousElementSibling`
  instead, and keep the full scan for `Home`/`End`, which do not repeat. The same path serves
  `j`/`k`. A row-selection counter has the same shape and the same fix: keep an integer, do not
  re-run `querySelectorAll(':checked')` over the document on every toggle (**32 ms at 25,000 rows,
  and O(n²) for a range selection on the screen that has bulk grab**).
- `/` focuses search; `Esc` clears or closes; `j`/`k` and arrows move; `Enter` opens; `?` opens a
  shortcut sheet; `l` opens the scope popover. Sonarr already ships a keyboard-shortcuts modal in
  the header actions menu, so this is a convention, not an invention.
- 🚩 **Those are five single-character shortcuts, and SC 2.1.4 is Level A.** The criterion is
  explicit: *"If a keyboard shortcut is implemented … using only letter … characters, then at least
  one of the following is true"* — **turn off**, **remap**, or **active only on focus**. "It also
  has a visible mouse equivalent" is not one of the three; that addresses discoverability, while
  2.1.4 exists for speech-input users whose dictation is typed into the page, and for anyone with a
  tremor. **Three requirements, all v0.1:**
  1. **A "Keyboard shortcuts" toggle in Settings, on by default.** One control satisfies "turn off"
     for all five, and it is the cheapest of the three routes.
  2. **The guard is `if (t.isContentEditable || t.closest('input, select, textarea,
     [contenteditable]')) return;`** — excluding only `INPUT`/`TEXTAREA`/`SELECT` by tag name leaves
     buttons, links, rows and `contenteditable` unguarded, so `l` fires with focus on "Add library"
     and `/` steals focus from a button.
  3. **`?` stays unconditionally**, because the sheet it opens is where the toggle is discovered.
- **A roving-tabindex handler must not intercept keys inside a form control.** A grid-level
  `keydown` listener that resolves `closest('[role=row]')` fires for anything focused *inside* a
  row, so `ArrowUp`/`ArrowDown`/`Home`/`End` are stolen from every `<select>` and text input in it —
  which makes a `<select>` keyboard-inoperable outright, an SC 2.1.1 Level A failure. **Bail out
  before the key switch when the event originates in a form control**, and never intercept
  `Home`/`End` unless the target is the row itself. **This is a required test**, because it is
  invisible to `svelte-check`: *for every roving grid, arrowing and `Home`/`End` inside a contained
  `input`/`select` must not move focus.*
- 🚩 **Focus follows navigation.** Activating a nav link replaces the whole main region and leaves
  `document.activeElement` on the nav link, with nothing announced — so the user must tab past every
  remaining nav row to reach the screen they just opened. **On route change, move focus to the new
  `<main>`'s `<h1>` (`tabindex="-1"`) and announce the page name in a polite live region.** §11's
  keyboard model was detailed about *lists* and silent about *route changes*.
- 🚩 **Never disable the control that was just activated.** `btn.disabled = true` inside a click
  handler drops focus to `<body>`, because a disabled element cannot hold it — measured on both the
  success and the failure path of a grab, which throws a keyboard user to the top of the document
  mid-task, and on the failure path sends them away from the very toast carrying the recovery
  action. Set `aria-disabled="true"`, change the label, keep it focusable, and swallow subsequent
  activations. If it genuinely must be disabled, move focus deliberately *first*.
- 🚩 **`Escape` is handled before the form-control bail-out, not after it.** The bail-out below is
  correct for `ArrowUp`/`ArrowDown`/`Home`/`End` and it also swallows `Escape`, which removes the
  documented escape route exactly where it is needed: on the Requests table the first control in
  every row is a checkbox, so arrowing into it leaves no arrow key and no `Escape` that gets back
  out — only Tab. Handle `Escape` first, return focus to the row, then bail out for the rest.
- ⚠️ **§11's own prescribed single-key guard is wrong in both directions, corrected here rather than
  left standing.** `el.closest('input, select, textarea, [contenteditable]')` — the exact form this
  section prescribed in the previous round — is **too broad**, because focus on a row-select
  *checkbox* suppresses `/`, so pressing `/` to reach search silently does nothing; and **too
  narrow**, because focus on a `<button>` does not suppress `l`, so the scope popover opens while
  focus stays elsewhere — **which is the precise bug this section names as its own motivating
  example** (*"so `l` fires with focus on 'Add library'"*). The corrected guard: suppress when
  `el.isContentEditable` or `el.matches('textarea, select, input:not([type=checkbox]):not([type=radio]):not([type=button])')`,
  and additionally suppress letter shortcuts that navigate when
  `el.matches('button, [role=button], a[href]')`. Measured, not reasoned.
- **`preloadData()` on `focusin`**, so keyboard users get the hover path's head start (§7.3).
- *INFERENCE:* a visible keyboard model is one of the clearest positive craft signals available,
  because generated UI essentially never ships one.

**`svelte-check` covers a11y and unused CSS and is already part of `make lint`** — per
`DEVELOPMENT.md`, "not an afterthought". Automated checking is a floor, not a ceiling: GOV.UK tests
its components against real assistive technology
(<https://design-system.service.gov.uk/accessibility/accessibility-strategy/>), and the contrast
ratios in `tokens.css` were computed rather than eyeballed for the same reason.

---

## 12. Room for the future

Per CLAUDE.md's "build the base with intentional space for what comes later" — **the seam ships,
the feature does not.** These are layout obligations, not v0.1 work.

- **The sidebar must grow.** The eventual set is Home · Library (per type) · Search · Requests ·
  **Calendar** · **Stats** · Services · Settings. Calendar (FUTURE §8, trigger: after v0.2) and
  Stats (FUTURE §9) are both named as wanted. The sidebar shape accommodates them without a
  reshuffle; a top navbar of media types would not. **Do not draw them in v0.1.**
- **More media types are here, not deferred.** All six arrive with the read-only catalogue sources in
  v0.1 (ADR-0032). §17.2's rule — a type the user does not have is not shown at all — means the
  *presence* of each is a data change rather than a layout change, **provided nothing hard-codes a
  type list anywhere**. That is now a live requirement rather than a future-proofing note.
  ⚠️ **The rule was narrowed on 2026-08-19 and this bullet kept the un-narrowed form**, so the
  seam it describes is real for two of the three places and not for the third.
  [ADR-0053](../DECISIONS.md#adr-0053) carves **the sidebar** out of *"not shown at all"* and leaves
  Block A and search groups under it: **nothing UsArr serves answers per-type presence**
  ([`reference/http-api.md`](../reference/http-api.md) §7.1 — *"no facet counts … each is its own
  aggregate and its own read"*), so the shipped shell renders **all six entries unconditionally**,
  no row carries a count, and an empty type says it is empty on its own screen (§8.1). **Per-type
  hiding is closed, not abandoned, and it reopens on one named condition — a facet read**: one
  statement answering which of the six types have rows under the current scope, published on
  `reference/http-api.md`. Until then the sidebar half of this bullet is a screen the wire cannot
  serve, and ADR-0053 is the record that says so, so a later pass does not "restore" it as though
  it had been dropped by accident. **The seam itself survives intact** — hard-coding a type list is
  still forbidden, because the facet read turns the sidebar back into a data change without a
  layout change, which is exactly what this bullet exists to protect.
- **Libraries grow without bound and the layout must not care.** The scope chip (§8.1) is the seam:
  a library is `?lib=` on an existing route, so a user with thirty libraries costs zero new page
  types and zero sidebar rows.
- **The cross-media review inbox** (FUTURE §5) is *"a whole second surface"* — a confirm / reject /
  not-sure queue over the 0.55–0.85 confidence band with `evidence` rendered so a human can judge.
  It is explicitly deferred and explicitly not a v1 screen. The design seam is that `work_relation`
  **is designed to carry** `confidence` and `evidence` — the design of record is
  [`reference/schema.md`](../reference/schema.md) §11 *Cross-media edges · **v0.3***. ⚠️ **The table
  is deferred to v0.3 and no shipped migration creates it** — `internal/db/migrations` names it only
  in 00005's and 00006's deferred lists, and `TestDeferredTablesAreAbsent`
  (`internal/db/migrate_test.go`) fails if one does. The seam is a designed shape to build to, which
  is exactly what makes the inbox cheap to add later; it is not a column pair already sitting in the
  schema waiting.
- **Multi-user** (v1.0) unlocks user management, roles and per-user grants. §17.1 and ADR-0019 make
  this a UI unlock, not a migration — which only holds if v0.1 never hard-codes "there is one
  user" into a layout.
- **The density control is itself a seam.** Three settings exist from day one; adding a fourth is
  three lines in `tokens.css`.
- **The token file is the styling seam.** Every decision here lives as a CSS custom property in one
  file. Tailwind consumes it, and scoped Svelte `<style>` blocks read the same variables. Swapping
  the utility engine later touches the token file and the class attributes, **not the component
  logic** (ADR-0025).

---

## 13. The lint checklist

`[grep]` = mechanically enforceable — a grep, an ESLint or a Stylelint rule; §13.0 names
`docs/design/check.mjs` as where those live and `make design` as what runs them. `[review]` = human judgement at PR
time. This is the reviewable form of §1, and it is the artefact that makes "don't look
AI-generated" an actual gate rather than a vibe.

### 13.0 The enforcement mechanism — `docs/design/check.mjs`

> **Every `[grep]` rule above the line is *run*, not remembered.** One file, one command, one exit
> code:
>
> ```
> PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers node docs/design/check.mjs
> ```

**SW-06, from the owner directly on 2026-08-16: he is "open to some kind of adversarial design
guideline checking".** The machinery already existed and was *scattered* — `mockups/selftest.mjs`
held five rendered-DOM assertions, and the §13 ban sweep was a pipeline of greps retyped by hand
each review round. `check.mjs` is those two consolidated; `selftest.mjs` is deleted, folded in
whole, so there is one place to run and one place to add to.

It covers, in order, printing a count for each: the **§13 ban list** over the stripped sources ·
**token drift** between `tokens.css` and the mockup's copy of it · **contrast**, worst of all five
grounds (page, surface, hover, selected, inset), both themes, against §3's floors · **overflow** at
390/1280/1440/1680/1920 across every screen in every state, at two scopes · **row heights** against
all three density bands · **availability glyph accessible names** · **one tab stop per list** ·
**containment** · **the webfont** · and the **copy bans** over rendered chrome text.

**Two rules govern the file, and they are the reason it is a program rather than a shell pipeline.**

**1. It prints what it checked, not only what failed.** A silent pass is indistinguishable from a
check whose glob was wrong and matched nothing — which is the specific way a hand-run sweep decays.
Every assertion emits its own count: *"14 font-family declarations parsed"*, *"36 screen×state
combinations"*, *"1,214 rendered chrome strings"*, *"49 list renderings checked"*. A number that
drops is a finding.

**2. False positives are excluded structurally, never by name.** The hand-run sweep kept
rediscovering the same four, and each round re-explained them in prose that was then thrown away.
They are gone now because of *where each ban is evaluated*, not because a list names them — which
means a new instance of the same shape is also excluded, without anyone adding it:

| False positive | Structural exclusion |
|---|---|
| `ArrowRight` as a `KeyboardEvent.key` | §13's rule is *no Sparkles / Zap / Shield / BarChart3 / ArrowRight **imports***. The icon bans are evaluated over **ES import specifiers** and the mockup's own icon vocabulary (`<symbol id="i-…">`) — and nowhere else. A string compared against `ev.key` is neither, so it is out of scope rather than excused. |
| `Inter` inside "Internally" and "The Zone of Interest" | The typography ban is a **font-stack** ban. It is evaluated over the comma-separated family list of `font-family` and `--font-*` declarations, matched as a **whole family name**. Prose containing the substring is not a font stack. |
| `BOOM! Studios`, against *no `!` in UI strings* | The copy bans are evaluated over rendered text **outside any `<td>`**. A cell holds *data* — sample data here, database rows in the product — and data is not the product's voice. That exemption is only safe because prose is kept out of cells, which is the **row-height** check in the same file: the two rules hold each other up. |
| A comment quoting the ban it documents | Every source-level scan runs over the file with **comments stripped**, line numbers preserved. A rule cannot fire on its own documentation. |

**And a fifth, derived from a document rather than declared.** The em-dash rule below already carries
an exception for wording that [`ARCHITECTURE.md`](../ARCHITECTURE.md) §17 fixes. That exception was
written as one hard-coded banner, so the *next* string §17 fixed would have failed the sweep and been
rewritten in the mockup instead of recognised. `check.mjs` reads §17 at run time and exempts a short
em-dash string when §17 contains the em dash's own two-words-either-side window — tight enough to
require the real phrase, loose enough to survive a substituted service name or timestamp. Two
consequences worth having: the exemption cannot go stale, and a label that **drifts** from §17's
wording loses its exemption and fails, which turns the rule into a copy-drift check between the
mockups and the section that specifies them. It caught one on its first run: the re-identification
banner's second button read `Not the same — remove and re-add` against §17.3's
`Not the same instance — remove and re-add`.

⚠️ **And the channel that exemption opened, now closed.** Because §17 was in `check.mjs` *only* as
the source of that exemption, the order of operations laundered the rule: a UI label is authored in
§17 and copied into a mockup afterwards, so writing the em dash into §17 first made every mockup
copy of it exempt **because §17 said so**. A §17-originated em dash could not be caught, by
construction. One was drafted during the §17.8 pass and removed by hand rather than by the checker.

**So §17's shipping copy — the `*"…"*` spans, 56 of them — is now a corpus of its own**, checked by
the same three rules with the §17 exemption **withheld**, because a string cannot be its own
authority. That closes the laundering channel. It is not sufficient on its own: the sentence actually
drafted runs to twenty-four words, so a corpus fix alone still let it through, which was confirmed by
planting it rather than reasoned about.

**The fifteen-word floor is therefore narrowed to the corpus it was meant for, and nowhere widened.**
The floor is easy to read backwards — `< 15` is what *fires*, so it confines the rule to short
strings rather than excusing them — and it is a **proxy for "is this a UI string"**, worth having only
where a corpus mixes microcopy with prose, as the rendered walk does. Every `*"…"*` span in §17 is a
specified UI string by construction, so there the proxy has no question left to answer, and the floor
is not applied: this section grounds it on *"a sentence long enough to need one is already too long
for a button, a tooltip, a toast or an empty state"*, which is a claim about the **element**, not
about the count. The rendered walk keeps the floor exactly as it was.

**Six §17 strings are recorded individually in `check.mjs`, each with a reason and what retires it.**
Four are the head-and-detail error form **§1.4** prescribes — not, as this paragraph read until
2026-08-17, "two paragraphs above", which pointed at the fifteen-word floor and prescribed nothing —
(`Grab failed — HTTP 502`), which no word count can distinguish from prose. ⚠️ **The form is still
prescribed; its punctuation no longer is.** §1.4 and the Copy block below now write that beat as a
colon, so these four are §17's **existing wording** rather than a licence for new copy: they predate
the ruling, §17's shipping copy is the owner's to word, and a checker does not edit the
specification it checks. ⚠️ **Two are open copy questions rather than blessings** — §17.5's
`— [Show it]`, which may be notation for two adjacent
elements rather than one string, and §17.8's twenty-two-word `add sources — never reshape it`, which
is the likelier real finding. They are carried visibly because §17 shipping copy is the owner's to
word and a checker does not edit the specification it checks. A seventh fails and asks a human, which
is the point.

⚠️ **What it is not.** It runs against `docs/design/mockups/prototype.html`, because that is the only
artefact that exists — **there is no `web/` implementation to check** (`CLAUDE.md`: the code does not
exist). Every DOM assertion is written against the rendered document rather than against the mockup's
markup idiom, so it survives the port to Svelte; the source-level scans will need their file list
widened, and nothing else. The `[review]` rules below are still human judgement and always will be.

---

**Colour**
- `[grep]` No `indigo|violet|purple|fuchsia` class or equivalent hex/oklch anywhere in the app —
  and none of `orchid`, `plum`, `magenta`, `lavender` or `thistle` either, which are the same four
  families wearing another CSS keyword and escape the word list otherwise. The last two are the
  family's pale tints, and they escape a chroma floor as well as the word list: `thistle` measures
  C 0.0439 and `lavender` C 0.0269, below any floor that does not also ban neutrals.
- `[grep]` No `bg-gradient`, `linear-gradient`, `radial-gradient` or `bg-clip-text` in app CSS.
- `[grep]` No `box-shadow` with a non-neutral colour; no `text-shadow`; no hued `drop-shadow`.
- `[grep]` No `#fff`, `#ffffff`, `#000`, `#000000`, `white` or `black` literals — use tokens.
- `[grep]` Every colour value in a component resolves to a CSS custom property. No literal colours
  outside `tokens.css`.
- `[review]` Count the distinct hues rendered on the busiest screen. More than one non-neutral hue
  that is not encoding a status → fail.
- `[review]` Every coloured element answers "what state does this encode?" No answer → make it
  neutral.

**Typography**
- `[grep]` No `Inter`, `Geist`, `Space Grotesk`, `Instrument Serif` or `Poppins` in any font stack.
- `[grep]` No `fonts.googleapis.com` or `fonts.gstatic.com` reference anywhere.
- `[grep]` No `font-style: italic` on a heading; no `text-transform: uppercase` on a label; no
  `letter-spacing` below `-0.01em` on body text.
- `[grep]` **No font-size token above `--text-xl` (20px) anywhere, and no `--text-empty` /
  `--fs-empty` token at all.** The 24px empty-state step is deleted, not capped (§9.6): a token that
  exists will be used, and this one was — on five screens, at a size larger than the page's own H1.
- `[grep]` **No `text-align: center` outside dialog components.** Empty states were the other
  exception and are no longer one — §9.6 requires them left-aligned at the content edge of the
  region they replace.
- `[grep]` **No empty state renders inside a border, a dashed border, a panel or a background
  step** — greppable as: no class matching `empty` may set `border`, `border-style: dashed`,
  `background` or `box-shadow`. §9.6's composition rule; this is the specific construction that made
  the region read as generated.
- `[review]` Count distinct font sizes in the chrome. More than six → fail.
- `[review]` Every numeric column uses `tabular-nums`; no prose does.

**Layout**
- `[grep]` No `backdrop-filter` / `backdrop-blur`.
- `[grep]` Radius tokens: at most three values, maximum 6px.
- `[grep]` No `box-shadow` on any component that is not a popover, dropdown, dialog or toast.
- `[grep]` No arbitrary-value syntax (`[13px]`, `[#abc]`) in class attributes.
- `[review]` No hero, badge-above-title, three-card feature grid, stat banner, numbered step row,
  testimonial, pricing table, FAQ accordion, logo wall or bento grid anywhere in the app.
- `[review]` No bordered surface nested inside another bordered surface.
- `[review]` Any list of >8 homogeneous items is a table or list, not a card grid — unless its
  primary content is an image.
- `[review]` At 1440×900, a library or search screen shows ≥25 scannable items above the fold.
- `[review]` Padding values differ by role (intra-row / inter-row / inter-region). Uniform padding
  across all three → fail.

**Iconography**
- `[grep]` No emoji codepoints in any `.svelte`, `.ts` or string file in the app bundle.
- `[grep]` Icon imports match an explicit allowlist; a new icon is a PR discussion.
- `[grep]` No `Sparkles`, `Zap`, `Shield`, `BarChart3` or `ArrowRight` imports.
- `[review]` Every icon either replaces a word in constrained space or encodes a repeated category.
  Decorative icon → delete.
- `[review]` No icon inside a tinted rounded chip.

**Motion**
- `[grep]` `transition` and `animation` properties limited to `opacity` and `transform`.
- `[grep]` No transition-duration above 200ms.
- `[grep]` No `cubic-bezier` with a control point outside 0..1 on Y; no spring.
- `[grep]` `IntersectionObserver` used for a scroll-triggered *reveal* → fail (lazy loading is a
  different call site; review the intent).
- `[grep]` A `prefers-reduced-motion: reduce` block exists and neutralises every transition.
- `[grep]` No animation library in `package.json`.
- `[grep]` No `startViewTransition` on a navigation path.
- `[review]` Hover changes colour only, never geometry. Focus rings are instant.

**Controls and links**
- `[grep]` **No `outline: none` (or `outline: 0`) except in the one pattern below, which is the
  correct way to write the rule and which the absolute form would fail the build on.** The
  exception, stated precisely enough to grep: a declaration removing the outline is permitted **only**
  when the *immediately following* rule in the same file targets the same selector with
  `:focus-visible` appended and sets an `outline` of at least `2px`. Greppable form — the rule fails
  unless every match of
  `/([^{}]+):focus\s*\{[^}]*outline:\s*(none|0)[^}]*\}/` is followed, with only whitespace and
  comments between, by a match of `/\1:focus-visible\s*\{[^}]*outline:\s*(?!none|0)\d*\.?\d+(px|rem|em)/`
  on the **same** captured selector. Anything else — an `outline: none` with no replacement, a
  replacement on a different selector, a replacement further down the file — fails.
  *Why the exception exists:* `:focus { outline: none }` followed immediately by
  `:focus-visible { outline: 2px solid var(--focus) }` is the standard, correct way to keep the ring
  for keyboard users and drop it for mouse users, which is exactly what §11's SC 2.4.7 row requires.
  A flat ban makes the compliant pattern unwritable.
- `[grep]` No single-character keyboard shortcut without the Settings toggle that turns all of them
  off (SC 2.1.4, §11).
- `[review]` Every dropdown of options is a native `<select>`; checkboxes and radios are native.
- `[review]` Everything that navigates is an `<a href>` with a real URL. No `<div>` with a click
  handler, no `onclick` navigation.
- `[review]` **For every roving-tabindex grid: arrowing, `Home` and `End` inside a contained
  `input`, `select` or `textarea` must not move focus.** SC 2.1.1 is Level A and `svelte-check`
  cannot see this.
- `[grep]` **A `Skip to content` link is the first tab stop on every page** (SC 2.4.1, §11).
- `[review]` **Focus moves to the new page's `<h1>` on every route change**, and no handler disables
  the control that invoked it (§11). Both are invisible to `svelte-check` and both strand focus on
  `<body>`.
- `[grep]` **Every list with `role="table"` and focusable descendants has exactly one row at
  `tabindex="0"`.** One `querySelectorAll` over the rendered DOM. The roving model was applied to
  seventeen lists and not to the three on the screen with the only stateful outbound action, where
  it left 28 tab stops inside 10 rows and no row reachable by Tab at all.
- `[review]` **An irreversible action is never an icon with no visible label, and never adjacent to
  a visually similar benign one.** The Grab control was an unlabelled download-arrow with no
  `title`, 8 px from an external-link anchor, firing a multi-gigabyte grab on `Enter` with no
  confirmation — and the glyph means "download this file to my computer" in every other application.
- `[review]` **A live region is atomic and carries the whole sentence**, and is never nested inside
  another live region, and is never *inside* the control whose accessible name it is. Measured
  failures: a `<span aria-live>` around `9 of 9` announcing a bare number without "indexers
  responded", inside a `role="status"` that is itself a region; and the scope chip's label, which
  makes toggling a checkbox both rename the focused control and fire an announcement.
- `[review]` **An accessible name describes the pattern, not the component's internal name.**
  `<nav aria-label="Level">` puts a Svelte component name into a screen reader's landmark list;
  it is a breadcrumb, so it is `aria-label="Breadcrumb"`.
- `[review]` **`aria-current="page"` marks a destination, never a filter**, and never two elements
  at once. Filter chips are `aria-pressed` buttons or `aria-current="true"` links.
- `[review]` **A group of related controls carries a role and an accessible name.** A bare `<div>`
  of seven filter links, immediately above six headings carrying the same seven strings, tells a
  screen-reader user nothing about what they are or what activating one does.
- `[review]` **No skipped heading levels and no two headings with the same text in one document.**
  Measured: H2 → H4 on the Libraries detail screen, with `Identity` appearing as both.
- `[review]` **No raw schema identifier in running copy**, outside an explicitly-labelled
  Diagnostics panel where identifiers are the content: `managed_by user` → "user-managed",
  `sort_title` → "sort title", `no work identity` → "no identifier", `breaker open` → "paused:
  7 failed attempts, retrying 14:19". The replacement obeys the Copy block below like any other UI
  string, so its beat is a colon; it carried an em dash until 2026-08-17, which is a seven-word
  violation this checklist was prescribing three bullets above the rule that bans it.
- `[review]` **One label per action across the whole product.** Shipped: four labels for "test",
  three for "retry", two for "add a service", four for a destructive action, and `Run full sync now`
  beside `Run full sync`. A verb that reads as a task description (`Confirm your password`) is a
  label for the *field*, not for the button.
- `[review]` **A control labelled as an operation performs it.** `Test Radarr` and `Run full sync`
  as `<a href="#services">` are imperative verbs that navigate to the top of another screen with
  nothing indicating which row you came for; either perform it inline (the handler already exists on
  that screen) or label it as navigation — `Radarr health →` — and deep-link with the row anchored.
- `[review]` **A derived summary line renders from the same source as the body it sits above, or it
  is not rendered.** Measured in one viewport: a Home sub-caption reading *"Last delta sync 14:02, 6
  minutes ago"* above the body *"No services configured"*; *"8 rows"* over an empty table;
  *"31 results in 6 of 6 media types"* over *"No results for duen"*. The sub-caption ships, it
  precedes the state block in both the reading order and the accessibility tree, and a number that
  is reassuring and wrong is worse than no number — `CLAUDE.md`'s "no invented status", arriving
  through page furniture.
- `[review]` **Every ARIA grid carries the roles a native `<table>` would have supplied** —
  `role="table"`/`"row"`/`"columnheader"`/`"cell"`, header association, and column names that
  survive the ≤760 px stacked view. This is the cost of §7.4's grid-row primitive and it is paid per
  component.

**Copy**
- `[grep]` Banned words across all UI strings: seamlessly, effortlessly, powerful, simply, unlock,
  empower, elevate, streamline, supercharge, robust, leverage, intuitive, blazing, world-class,
  comprehensive, "AI-powered".
- `[grep]` No `—` (U+2014) in any string under 15 words, **except where
  [`ARCHITECTURE.md`](../ARCHITECTURE.md) §17.7 fixes the wording**: its degraded-instance banner
  is quoted verbatim as *"Kavita is unreachable — showing cached data from 14:02, 6 minutes
  ago"*, which is eleven words. §17 wins over this checklist, so the rule carries the exception rather than the
  banner carrying a rewrite. **What the ban forbids is the punctuation, never the construction:**
  §1.4's head-and-detail form is prescribed, and in a UI string its beat is a **colon** for a
  statement and its reason (*"Grab failed: HTTP 502"*), or a full stop and two sentences for a
  statement and an instruction (*"Grab failed. Check the indexer on Services."*). Stated here as
  well as at §1.4 because a section that endorses a construction and bans its punctuation produces
  correct-looking violations forever, and this one did.
  **And a string whose entire content is one `—` is a glyph, not a sentence.** It is the empty-cell
  convention — *"no value here"* — used by the app (`NOTHING.empty`) and by every `<td>` in the
  mockups that has nothing to print, and it is exempt **by shape**: no sentence fits in one
  character, so nothing can launder through it, and no name, file or token appears in the exemption.
  Recorded here because the convention was tacit, and a tacit convention is what a later author
  "fixes" into a word.
- `[grep]` No `!` in UI strings.
- `[grep]` **The UI locale is en-GB** — `catalogue`, `behaviour`, `colour`, `organisation` —
  **with one exception: a string quoted verbatim from an \*Arr keeps the \*Arr's spelling**
  (`Enable Color-Impaired Mode`, `Organize`, `color impaired options`). Recorded as a decision
  rather than left as drift, because it interacts with every borrowed string and because
  `catalogue source` — one of the product's two coined terms — is the visible edge of it.
  📌 Joe's call to confirm; en-GB is the current answer and the whole corpus already follows it.
- `[grep]` **No parenthesised plural (`release(s)`) where the count is known.** The bulk button is
  disabled at zero selections, so it always knows: `Grab 3 releases` / `Grab 1 release`. Keeping
  Prowlarr's verb `Grab` is what buys the familiarity; the `(s)` buys nothing and does not
  translate.
- `[grep]` Buttons and labels are sentence case (proper nouns like Sonarr, Prowlarr, Navidrome
  excepted).
- `[review]` No first-person plural.
- `[review]` Every error string names the component, the observed symptom and the next action.
- `[review]` No fabricated data **in any shipped product surface** — no placeholder rows, no
  invented counts in an empty state, no seeded demo library, no screenshot in the README that shows
  data no user has. **A design mockup is the one exception and must be labelled as one**, because a
  mockup with no data shows nothing: `docs/design/mockups/` uses sample data throughout and says so
  in its first paragraph. *(The earlier form of this rule — "no fabricated data anywhere, including
  screenshots, docs and empty states" — forbade the mockup that exists to make the rest of this
  document reviewable.)*
- `[review]` Sample data in a mockup is still checked against reality: real episode counts, release
  names a scene group would actually produce, sizes in the right order of magnitude, indexer and
  category names that exist. Implausible sample data reads as generated even when the layout does
  not.

**States**
- `[review]` Every component demonstrates the §10 state set. A component with only a happy path
  does not merge.
- `[review]` Every screen depending on an optional service degrades with a named reason and a link
  to the fix.
- `[review]` Contrast re-measured in both themes when any token changes.
- `[grep]` **Every `dominant_color` / foreground pair in a fixture clears 4.5:1** (§11). This is the
  one colour that is data rather than a token, so it cannot be checked once. ⚠️ **No such pair ships
  today** — §9.2 moved the poster title off the fill and the constraint machinery was deleted with
  it — so this line is armed and idle. Do not record it as passing; it has nothing to check until a
  surface sets text on a computed fill again.
- `[review]` No live region missing on a determinate progress readout or on a control that changes a
  visible summary string — the scope chip's label and the indexer fan-out count are both Tier 3-ish
  readouts that a sighted user watches change and a screen-reader user is told nothing about.
- `[review]` **Every screen that can be scoped demonstrates `scope-empty`** (§10), not just `empty`
  and `filtered-empty`. Three states, three different sentences.
- `[grep]` **Every list whose rendered rows are a window onto a larger set carries `aria-rowcount`
  on the grid and `aria-rowindex` on every row, header row included, 1-based over the full set**
  (§11). Under ADR-0029 that is every list in the product. `aria-rowcount="-1"` where the total is
  genuinely unknown. ⚠️ Assert it over the rendered DOM, not over the accessibility tree: CDP
  exposes neither row nor column indices as node properties, for a native `<table>` or an ARIA grid,
  so tooling absence proves nothing here.
- `[grep]` **No status or availability glyph computes to an empty accessible name** (§11). An
  icon-only ✓/✗ makes "have" and "missing" indistinguishable to a screen reader while the UI prints
  a claim to the contrary; the word from §6.3's rollup ships as text or as a visually-hidden span.

**Copy — the two axes**
- `[review]` **A library name rendered beside a media-type name carries the noun `library`** (§8.1).
  *"all in the **Ebooks** library"*, never *"all in Ebooks"*. Without it the two axes ADR-0027
  separates read as one, and on the common install they are the same string.

**The two catch-alls** (§1.6), applied to every PR that touches a screen.

---

## 14. Open questions for Joe

| # | Question | Why it needs you |
|---|---|---|
| ~~**OQ-1**~~ | ~~Virtualization threshold.~~ **CLOSED 2026-08-16** — delegated to us and settled as "Load more" + `content-visibility`, with virtualization as an escalation above a **benchmarked** threshold. **ADR-0029**; §4.5, §16 and §7.4 amended | — |
| ~~**OQ-2**~~ | ~~Navigation.~~ **CLOSED 2026-08-16** — the owner confirmed the **left sidebar**. **ADR-0027**; §17.2 amended to the two-axis model, and §8.1 rewritten | — |
| **OQ-3** *(narrowed twice)* | **The family is settled — IBM Plex, confirmed 2026-08-16 — and as of the same day it is also *shipped and visible*:** `mockups/fonts.css` self-hosts the subsets and `docs/design/check.mjs` proves by advance-width probe that the body renders in Plex rather than in a fallback (§4.1). **What is left open is the subset, and only the subset.** `latin` alone is **103.6 KB**; `latin`+`latin-ext` is **177.2 KB** (§4.1) — 73.6 KB of first paint. Neither trips the ~200 KB trigger, so the question is whether an accented library gets `latin-ext` or falls back mid-string, and six media types make that sharper than two did, since a manga, classical-music or translated-fiction library is *full* of accented and transliterated titles. **The honest limit on both options: neither renders native CJK.** A manga library holding untransliterated Japanese titles falls through to the system stack on those rows whichever subset wins — that is an argument for a third, much more expensive option, not a tiebreaker between these two, and it should be decided knowingly rather than discovered on a real library. One loose end the family decision does not close: the argument that beat the system stack (cross-OS metric drift) is **uncited inference, unmeasured for this design** (D-32) | It is the only decision here that costs bytes on first paint, and the shipped subset is the one thing a self-hoster cannot change without a rebuild |
| **OQ-4** | **13 px base type.** That is the Linear/dense register. If it reads small to you, move base to 14 and shift the whole scale up one step — **do not add a seventh step** | Personal legibility; you are the only user in v0.1 |
| **OQ-5** | **Radius 0 or 2 px** on inputs and buttons. Both are within the budget; 0 is more committed | Taste, and it should be decided once and applied without exception |
| **OQ-6** | **Theme default.** Auto (Sonarr's default) or Dark (Navidrome's)? | Navidrome is the stated reference point, but Auto is the *Arr convention and UsArr sits next to three of them |
| **OQ-7** | **Tailwind's native engine.** ADR-0025 records `@tailwindcss/oxide`'s fetching postinstall as the strongest argument against, with Open Props as the pre-agreed fallback. Is that trigger condition acceptable to you as written? | It touches `make check`'s network-call budget. ~~It touches `make check`'s "exactly one network call" property, which is your rule~~ ⚠️ **Struck 2026-08-21: the figure is wrong, and no owner statement behind the attribution appears in the corpora swept.** **The figure.** `make check` makes **two** deliberate calls, both in the `vuln` target and both to vulnerability databases — `$(GOVULNCHECK) ./...` to `vuln.go.dev` and `$(call pnpm_if_web,audit)` to the npm registry (`Makefile`, target `vuln`; lines 1111 and 1113 read at `2e7c8f7`). [`REVIEW-LOG.md`](../REVIEW-LOG.md)'s **FI-14** measured the same two on 2026-08-16 at `ac1ab29` and corrected the `Makefile`, `DEVELOPMENT.md` §4 and §8, and `CLAUDE.md` — this cell was not among the sites it named, and neither were ADR-0025's Context and its *strongest argument against*, both corrected at their own sites. **The attribution — and this is a different defect from the others this pass corrected.** Elsewhere in ADR-0025 a real owner statement was rendered too firmly; here **no owner statement was found at all**, which is not the same failure and should not be read as one. **Method, corrected 2026-08-21 after re-measurement, and leading with the read that carries it:** the raw tool output on disk is **five dumps** — two pages of the top-level timeline and three whole-thread fetches (`…9J6UvMkpeCcQZWPM3YLjjQ2`, `…9GmAPi2ZBTaTFSRKrTCaohN`, `…97Xwu72fVNeDn9cNXaioNno`) — read end to end by two workers quoting independently, each covering its files by character range with the arithmetic stated. ⚠️ **The project's opening thread — the fetched unit and its replies, not its root message — is NOT among those five and was never on disk here** — checked by id, its 00:26:22Z, 00:35:12Z and 02:31:23Z messages appear in none of them. ⚠️ **Its root message is on disk, and the distinction is load-bearing here:** a thread carries its opening message's id, so `…9RXhqeDFjfm9b6JCFa9jyab` is both *the opening thread* (absent) and *the opening message* (present, on the first timeline page) — and it is the very message the `/adversarial agent/i` positive control below returns, so the two sentences are not in conflict. It was read separately, **at source and end to end on 2026-08-21** (`has_more: false`, 38 messages, 2026-08-16T00:09:09Z → 06:07:35Z), by a lane holding project-chat access; **that read is first-hand and cannot be re-run from this repository.** The full coverage statement is [`DEVELOPMENT.md`](../DEVELOPMENT.md)'s *An attribution to the owner carries a locator*, under *Coverage*. The token searches — `network`, `offline`, `hermetic`, `internet`, single-token and line-oriented — are corroboration only; `make check` is two words and proves nothing on its own. **No message from the owner matching those terms appears in the corpora swept.** ⚠️ **Positive control, fired, and one item withdrawn on re-measurement:** the identical method over the identical five dumps **does** surface his statements on other topics — the storage-folder libraries ask (`/storage folder/i`, thread `…9J6UvMkpeCcQZWPM3YLjjQ2`) and his adversarial-review sentence (`/adversarial agent/i`, first timeline page) — so the instrument finds his words in these dumps when they are there. ⚠️ **The custom-CSS approval quoted in ADR-0025's Amendment was listed here as a third control and is withdrawn:** re-measured 2026-08-21, `cmsg_01S5UQT5yPAMR4PFkxyLGSj94bkQrhhPJ4dPNrpWctD3DU` and its text (`/super matter/i`) appear in **none** of the five dumps — it was resolved by a lane reading it at source, not by this instrument, and citing it as a control overstated what these files contain. ⚠️ **That withdrawal is itself the negative control**, and it is kept here as one: a message known to exist and known to be his returns **nothing** from these five files, which is the honest boundary of what a zero from this instrument means. ⚠️ **Near miss, recorded so it is not rediscovered and mistaken for a source:** the closest match anywhere is **agent-authored, not his** — *"the gate is deliberately kept to one network call and no heavy dependencies"* — and the same agent partly walked it back within the hour, naming the real disqualifier as the check importing Playwright by absolute path from outside the repo. **Bound:** several fetches returned `has_more: true`, so this is a measurement of the corpora named above, **not a survey of everything the owner has written**. **The budget is a real constraint and the question still stands on it**; the figure is withdrawn, and the attribution is unsupported on what was searched. |

---

## 15. Sources

Repository documents: [`CLAUDE.md`](../../CLAUDE.md) · [`ARCHITECTURE.md`](../ARCHITECTURE.md)
§4.4, §4.5, §13, §16, §17 · [`DECISIONS.md`](../DECISIONS.md) ADR-0003, ADR-0004, ADR-0012a,
ADR-0015, ADR-0019, ADR-0020, ADR-0025 · [`FUTURE.md`](../FUTURE.md) §5, §8, §9 ·
[`DEVELOPMENT.md`](../DEVELOPMENT.md) §2, §9.

**Anti-goal.** <https://www.adriankrebs.ch/blog/design-slop/> ·
<https://news.ycombinator.com/item?id=47864393> · <https://github.com/febbhav/signs-of-ai-design> ·
<https://www.developersdigest.tech/blog/ai-design-slop-and-how-to-spot-it> ·
<https://dev.to/kiwibreaksme/i-catalogued-every-tell-that-makes-a-ui-look-ai-generated-my-own-tool-kept-failing-the-test-n52> ·
<https://www.mania.design/blog/spot-the-slop-a-ui-designers-guide-to-fixing-ai-defaults/> ·
<https://vibecodekit.dev/ai-slop-design> ·
<https://dev.to/alanwest/why-every-ai-built-website-looks-the-same-blame-tailwinds-indigo-500-3h2p> ·
<https://www.designsystemscollective.com/is-anyone-else-tired-of-every-tailwind-shadcn-app-looking-the-same-69c545e73114> ·
<https://github.com/Nutlope/hallmark> · <https://www.vibecheck.fail/> ·
<https://tailwindcss.com/docs/colors> · <https://9to5mac.com/2018/02/02/apple-rejecting-apps-with-emoji/> ·
<https://www.creativebloq.com/design/graphic-design/texture-warmth-and-tactile-rebellion-the-big-graphic-design-trends-for-2026> ·
<https://dev.to/olehvolos/users-can-tell-when-your-ui-was-ai-generated-and-they-dont-like-it-33kn>
(**weak — cites no data; do not repeat its claim as evidence**).

**Latency and density.** <https://www.nngroup.com/articles/response-times-3-important-limits/> ·
<https://www.nngroup.com/articles/skeleton-screens/> ·
<https://www.viget.com/articles/a-bone-to-pick-with-skeleton-screens> ·
<https://uxdesign.cc/what-you-should-know-about-skeleton-screens-a820c45a571a> ·
<https://www.nngroup.com/articles/animation-duration/> ·
<https://www.nngroup.com/articles/infinite-scrolling/> ·
<https://baymard.com/blog/return-same-place> · <https://baymard.com/blog/back-button-expectations> ·
<https://baymard.com/blog/external-load-more-vs-pagination-vs-infinite-scrolling> ·
<https://lawsofux.com/doherty-threshold/> · <https://web.dev/articles/rail> ·
<https://web.dev/articles/inp> · <https://blog.superhuman.com/superhuman-is-built-for-speed/> ·
<https://linear.app/blog/scaling-the-linear-sync-engine> · <https://www.inkandswitch.com/local-first/> ·
<https://mattstromawn.com/writing/ui-density/> · <https://github.com/WICG/virtual-scroller> ·
<https://developer.mozilla.org/en-US/docs/Web/CSS/content-visibility> ·
<https://alistapart.com/article/zebrastripingdoesithelp/> ·
<https://alistapart.com/article/zebrastripingmoredataforthecase/> ·
<https://carbondesignsystem.com/components/data-table/style/> ·
<https://practicaltypography.com/line-spacing.html> ·
<https://developer.mozilla.org/en-US/docs/Web/CSS/font-variant-numeric>.

**Accessibility.** <https://www.w3.org/WAI/WCAG22/quickref/> ·
<https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html> ·
<https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html> ·
<https://www.w3.org/WAI/WCAG22/Understanding/text-spacing.html> ·
<https://www.w3.org/WAI/WCAG22/Understanding/target-size-minimum.html> ·
<https://www.w3.org/WAI/WCAG22/Understanding/focus-appearance.html> ·
<https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/#kbd_roving_tabindex> ·
<https://design-system.service.gov.uk/components/error-message/> ·
<https://design-system.service.gov.uk/accessibility/accessibility-strategy/>.

**Ecosystem.** [Sonarr PageSidebar.js](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/Components/Page/Sidebar/PageSidebar.js) ·
[Sonarr PageSidebarStatus.js](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/Components/Page/Sidebar/PageSidebarStatus.js) ·
[Sonarr Styles/Themes/dark.js](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/Styles/Themes/dark.js) ·
[Sonarr en.json](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/Localization/Core/en.json) ·
[Sonarr NoSeries.tsx](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/Series/NoSeries.tsx) ·
[Sonarr ConfigService.cs](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/Configuration/ConfigService.cs) ·
[Sonarr ConfigFileProvider.cs](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/Configuration/ConfigFileProvider.cs) ·
[Prowlarr PageSidebar.js](https://github.com/Prowlarr/Prowlarr/blob/develop/frontend/src/Components/Page/Sidebar/PageSidebar.js) ·
[Prowlarr releaseActions.js](https://github.com/Prowlarr/Prowlarr/blob/develop/frontend/src/Store/Actions/releaseActions.js) ·
[Navidrome Menu.jsx](https://github.com/navidrome/navidrome/blob/master/ui/src/layout/Menu.jsx) ·
[Navidrome SongList.jsx](https://github.com/navidrome/navidrome/blob/master/ui/src/song/SongList.jsx) ·
<https://wiki.servarr.com/prowlarr/search> · <https://wiki.servarr.com/sonarr/settings> ·
<https://wiki.servarr.com/sonarr/system> · <https://www.navidrome.org/docs/faq/>.

**Typography and precedent.** <https://www.ibm.com/plex/> ·
<https://developer.mozilla.org/en-US/docs/Web/CSS/font-family> · <https://rsms.me/inter/> ·
<https://yeptype.com/article/inter-alternatives> ·
<https://dev.os9.ca/techpubs/mac/HIGuidelines/HIGuidelines-15.html> ·
<http://bitsavers.informatik.uni-stuttgart.de/pdf/openSoftwareFoundation/motif/OSF_Motif_Style_Guide_Revision_1.2_1993.pdf> ·
<https://www.edwardtufte.com/notebook/sparkline-theory-and-practice-edward-tufte/> ·
<https://svelte.dev/docs/kit/link-options>.
