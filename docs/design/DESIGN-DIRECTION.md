# UsArr — Design direction

**Status:** design document, pre-alpha. **Nothing here is implemented.** There is no `web/`
directory, no component, no line of CSS in the repository. This document and
[`tokens.css`](./tokens.css) are written *before* the first UI commit, in the same spirit as
[`ARCHITECTURE.md`](../ARCHITECTURE.md) §17 — which owns what the screens *are*, and which this
document is downstream of.

**Last revised:** 2026-08-16.
**Constraints this obeys:** [`ARCHITECTURE.md`](../ARCHITECTURE.md) §17 (screens and UI
philosophy), §16 (scope), §13 (performance budget); [`CLAUDE.md`](../../CLAUDE.md) (the four
principles); [`DECISIONS.md`](../DECISIONS.md) ADR-0003 (SvelteKit `adapter-static` + `embed.FS`)
and ADR-0024 (the styling and typography stack, which this document is the rationale for).
**Deferred UI surfaces and the seams they need:** [`FUTURE.md`](../FUTURE.md).

Where §17 and this document disagree, **§17 wins** and this document is the bug. §17's open
navigation question is the one place this document is permitted to decide something §17 did not —
see §8.1, and the honest note about how that decision differs from the two options §17 named.

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
| Giant centred hero heading, 64px+ | febbhav; Krebs | Largest type in chrome is a 20px page title. 24px exists only in empty states |
| All-caps tracked eyebrow labels | Krebs; febbhav; kiwibreaksme | Sentence case, no `text-transform: uppercase` on labels |
| No real hierarchy — HN **Ardon**: *"All the text sizes except for the main heading are within 15% size of each other"* | HN 47864393 | Six sizes, hard stop; below `lg` hierarchy is weight + ramp step, deliberately (§4) |
| Monospace as decoration | febbhav; HN **toraway** on the *"console-ish font Claude seems to love"* | Mono is **semantic**: machine data only (§4). This converts the tell into a feature |
| Title Case Everything | febbhav | Sentence case, enforced by lint |

### 1.3 Layout and structure

| Tell | Documented by | UsArr's rule |
|---|---|---|
| Centred max-width column with vast vertical padding — marketing rhythm on an app screen | Krebs; febbhav | Full-viewport-width app shell with a sidebar. `max-width: 1200px; margin: auto` is a document layout |
| Everything-is-a-rounded-card, `rounded-2xl`; the untouched shadcn `rounded-2xl shadow-lg p-6` | febbhav; vibecodekit | Radius 0 in the flat plane, 4px on overlays, ceiling 6 (§6). The classes do not exist (§ADR-0024) |
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

**UI copy states facts about system state.** "Sonarr unreachable — connection refused at
10.0.0.4:8989" beats "We couldn't connect. Let's get you back on track!" No exclamation marks, no
first-person plural: the application is a tool, not a company. Error copy names **the failing
component, the observed symptom, and the next action** — which is CLAUDE.md principle 3 expressed
as microcopy, and conveniently the strongest anti-slop signal available, because generated copy is
systematically vague about failure.

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
| **ARCHITECTURE §17.1 / §4.4.1** | **No skeleton shimmer.** The image placeholder is a `dominant_color` block with the title in it: informative, not decorative |
| **ARCHITECTURE §2.3 / §5.5 / §17.7** | Degraded ≠ blocked. A small **non-modal** banner. **The catalogue never greys out and never shows a spinner** |
| **ARCHITECTURE §13** | Client-side prefix filter p50 < 5 ms, p99 < 16 ms — one frame. The UI's own budget, not the server's |
| **ARCHITECTURE §16** | **v0.1 is Movies and TV only** (Sonarr + Radarr + Prowlarr). Music, books, comics and audiobooks are v1.0. Requests in v0.1 is the **Prowlarr Search-and-Grab path only** |

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

**There is no brand accent hue in UsArr. None.** Not a blue, not a green, not a "not-purple". This
is the single biggest structural defence in this document, and it is worth stating why rather than
just asserting it:

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
| `warning` | degraded, stale, partial, clock skew | `#8a5300` (6.02:1) | `#e0a33a` (8.15:1) |
| `error` | down, failed, rejected, needs re-identification | `#b3251c` (6.24:1) | `#f0837a` (7.09:1) |
| `not configured` | no service of this kind exists | `#5a534a` (7.20:1) | `#b0a89b` (7.68:1) |

**Grey is a legitimate status and it is load-bearing.** "Not configured" is a real, common,
non-broken condition in a pluggable app, and painting it a colour would make the two conditions that
*are* broken harder to find.

> **Every status is encoded as icon + text + colour together. Never colour alone.**

That is why UsArr needs no colour-impaired mode. Sonarr had to ship one —
`EnableColorImpairedMode`, help text *"Altered style to allow color-impaired users to better
distinguish color coded information"*, default `false`
([ConfigService.cs](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/Configuration/ConfigService.cs))
— precisely because those apps encode a lot of meaning in colour alone. Building the redundancy in
from day one removes the need for a setting later.

### 3.3 Protocol chips

Torrent and usenet are colour-coded consistently across Sonarr, Radarr and Prowlarr:
`torrentColor: '#00853d'` and `usenetColor: '#17b1d9'`
([Sonarr Styles/Themes/dark.js](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/Styles/Themes/dark.js)).
A self-hoster reads that green/cyan split without a legend, so UsArr reuses it — **for protocol and
nothing else**.

Honest caveat: those two literals are tuned for a dark ground and fail 4.5:1 on a light one
(`#00853d` measures 3.72:1 against the light selected-row fill; `#17b1d9` measures 1.97:1 against
the light page ground). The **hue is kept and the lightness is retuned per theme** —
`#0a6b34` / `#0f6479` in light, `#4fb377` / `#4ec3e0` in dark. Recognition survives; the contrast
floor is met. Also note the vocabulary decision: the column is labelled **"Protocol"**, matching
Prowlarr, not **"Source"**, which is what Sonarr's Interactive Search calls the same field
([Prowlarr releaseActions.js](https://github.com/Prowlarr/Prowlarr/blob/develop/frontend/src/Store/Actions/releaseActions.js);
[Sonarr InteractiveSearch.tsx](https://github.com/Sonarr/Sonarr/blob/develop/frontend/src/InteractiveSearch/InteractiveSearch.tsx)).
Prowlarr is the surface v0.1 mirrors.

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
(<https://developer.mozilla.org/en-US/docs/Web/CSS/font-family>). But San Francisco, Segoe UI and
Roboto have **different x-heights and different advance widths**, so the layout is not reproducible
across platforms: a title that fits a 32 px row on macOS may wrap on Windows. MDN also warns
explicitly that `system-ui` "may cause the displayed typeface to be undesirable for some users —
for example, the default Windows CJK font may render Latin scripts poorly." For a design whose
whole premise is fixed row heights and a dense grid, metric drift is not cosmetic.

**IBM Plex is a real, boring, well-hinted technical family.** Designed as a corporate UI/text
family, large x-height, true tabular figures, and a mono companion that pairs with it. It is not on
anybody's tell list, it is not fashionable, and it will not be fashionable next year either —
which is the point.

**The cost, stated honestly.** Two subset WOFF2 faces at two weights each (400/600 sans, 400 mono)
is realistically **~120–180 KB** total added to the binary and to first paint. **INFERENCE — this
is an estimate from typical Latin-subset WOFF2 sizes, not a measured figure; it must be measured
before the fonts are committed, and if it lands materially above ~200 KB, drop the mono weight
count before dropping the family.** Mitigations: the font is served from the same box over a LAN,
it is content-hashed and immutable, and it is precompressed at build time (`statigz`, see
ADR-0024). Set `font-display: block` with a short block period, or `optional` — on a LAN the font
always wins the race and `swap` risks a visible reflow for no benefit.

**Never a Google Fonts `<link>`.** It is both a documented tell and a third-party request in
software whose entire premise is that it runs on your own hardware.

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

### 4.3 The scale — six steps, hard stop

| Token | Size / line-height | Weight | Use |
|---|---|---|---|
| `xs` | 11 / 16 | 500 | counts, chip labels, table sub-labels |
| `sm` | 12 / 16 | 400 | secondary metadata inside a row |
| `base` | 13 / 18 | 400 | **default: list rows, table cells** |
| `md` | 14 / 20 | 400 | form inputs, dialog body, primary row title |
| `lg` | 16 / 24 | 600 | section headings |
| `xl` | 20 / 28 | 600 | page title — **the largest type in the chrome** |
| *(empty states only)* | 24 / 32 | 600 | nothing else may use it |

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

**Sentence case everywhere.** No Title Case, no all-caps labels, no letter-spaced eyebrows. Buttons
are verbs: "Add", "Retry", "Test connection", "Search indexers".

---

## 5. Space, density and geometry

### 5.1 The unit

**4 px base; scale `2 4 6 8 12 16 24 32 48`.** The 2 and 6 steps exist so a 28 px row is possible;
above 16 the scale goes in 8s. Every padding, margin, gap, offset and icon box in the app is one of
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

- **Only `opacity` and `transform` may be transitioned.** Never `height`, `width`, `top`, `left`,
  `background-position` or `box-shadow`.
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

Four tiers, keyed to **where the data lives**, not to how long anyone guesses it will take. And
one headline rule: **there are no skeleton screens anywhere in UsArr.**

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

**This is not in tension with §17.1's "a skeleton is a `dominant_color` block with the title in
it".** That is an *image placeholder*: a reserved box carrying the item's real identity, present
because §4.4.1 makes `dominant_color` available before ThumbHash. It is content, not a shimmer, and
it never pulses.

### 7.2 The four tiers

**Tier 0 — the data is in local SQLite. Nearly every read.**
Show **nothing at all**. No skeleton, no spinner, no fade-in. Render the content. Target **< 50 ms
click-to-paint; hard fail at 100 ms.** A local read over 100 ms is a **query-plan bug**, and it
belongs in the `EXPLAIN QUERY PLAN` assertions in CI (§13 of ARCHITECTURE), not behind a spinner.

**Tier 1 — 100 ms to ~1 s, and the wait is unavoidable.** First SPA boot; a cold cover-art fetch.
No indicator. **Progressive content**: render the shell, the nav, the headers and every field
already held, and let the missing pieces resolve in place. For images, reserve the box with
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
movies". **Never a fake bar.** Partial results are usable as they arrive — §8.4 already requires
per-indexer streaming over SSE, and §17.7 already requires home sections to populate live as import
phase A commits.

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

The research position is: **"Load more" plus `content-visibility: auto` with
`contain-intrinsic-size`** as the default, reaching for virtualization only above ~1,000 rows.

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

> ⚠️ **This contradicts the repository, and it is not resolved here.**
> [`ARCHITECTURE.md`](../ARCHITECTURE.md) §4.5 says **"Virtualize everything over ~200 rows"**, and
> §16 lists "Library grid, **virtualized**, keyset pagination" as a v0.1 line item. §4.5 is
> authoritative and this document does not overrule it. The conflict is recorded as **open
> question OQ-1 in §15**, for Joe to settle — either by amending §4.5's threshold, or by accepting
> the Ctrl+F loss deliberately and saying so in §4.5 rather than by omission.
> Note the two positions are closer than they look: keyset windows of ~100 rows with ±2 pages
> prefetched (§4.5) means the mounted set is small either way; what differs is whether the
> unmounted rows are absent from the DOM (virtualized) or present-but-unpainted
> (`content-visibility`).

---

## 8. Navigation and page anatomy

### 8.1 The navigation model — resolving §17.2's open choice

§17.2 deliberately left the v0.1 navigation model open between two options: *"sections on the home
page (the default assumed above), or per-type tabs in a top navbar with the home page as an
overview. Both are standard; pick one on first contact with real data and do not relitigate."*

**The decision: a persistent, collapsible left sidebar. Content nouns first; `Settings` and
`System` always the last two entries, in that order; sub-items indented inline under the expanded
parent, with no flyouts. Home remains sectioned by media type.**

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

**Honest note on how this differs from §17.2.** A sidebar is neither of the two options §17.2
named. It is closest to the second — per-type entries giving one-click access to each media type —
relocated from a top navbar to a sidebar, while keeping the first option's sectioned home page.
The reasons for the relocation: the sidebar is what three of UsArr's five v0.1-adjacent
neighbours already use, it is where the ecosystem puts status badges (§8.2), it collapses cleanly
on a phone, and a top navbar of media types does not scale to the eventual
Home · Library · Search · Requests · Calendar · Stats · Services · Settings set (§12). **If Joe
reads §17.2 as excluding a third option, §17.2 wins and this reverts to top-bar tabs** — recorded
as **OQ-2** in §15.

**The v0.1 sidebar, and nothing more.** Because v0.1 is Sonarr + Radarr + Prowlarr only, the
content nouns are **Movies** and **TV**. There is no Music, Books, Comics or Audiobooks entry,
because there is no provider for them until v1.0. §17.2's hard rule applies to the sidebar as it
does to home sections: **a type the user does not have is not shown at all.**

```
Home
Movies
TV
Search
Requests          (v0.1: the Prowlarr Search-and-Grab path — §17.5)
Services       ●2
Settings
System
```

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

### 9.2 The poster grid — the one place a card is correct

A card is justified when the item's primary content is an image, which is true here and nowhere
else. Rules:

- Reserve the box with `aspect-ratio` so nothing shifts. The empty card is **title and year over a
  `dominant_color` fill** — never a grey box, and never a shimmer (§4.4.1).
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

UsArr's wizard is **three fields — kind, base URL, API key — plus a live connection test that must
pass before Save is enabled** (§17.7, §11.3). Credentials render as `••••••1a2b` and are never
returned by the API. Changing `base_url`'s scheme/host/port **invalidates the stored credential**,
so the masked display is not theatre.

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

Toasts carry the **verbatim upstream error** with **Retry** and **Dismiss** (§7.6, ADR-0012a), and
they are never the *only* place an outcome appears — the affected row shows it too, because a toast
can be missed and a row cannot.

### 9.5 Status, chips and banners

- A status is **icon + text + colour**, in that order of importance. Removing the colour must leave
  it fully legible.
- System tags render as chips you can filter by but not delete (`is_system`, ADR-0015). Tag chips
  are neutral; `tag.color` is the only colour field in the data model and is user-controlled, not
  chrome.
- **Degraded instance → a small non-modal banner** naming it: "Radarr 4K is unreachable — showing
  cached data from 14:02", linking to Services. **The catalogue does not grey out** and shows no
  spinner. Writes are accepted with the label "queued — Radarr 4K is unreachable".
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

No illustration. No centred marketing block. Never fabricated data — not in an empty state, not in
a screenshot, not in documentation.

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
| **partial** | what arrived, what did not, and that more may come | "4 of 9 indexers responded"; "250/300" |
| **stale** | the data is real but old, with the timestamp | `"stale": true` from §2.3. Not greyed out |
| **error** | the **verbatim upstream text**, plus Retry / Dismiss | §17.3's "Problem" column is verbatim by requirement, rendered in mono |
| **unconfigured** | the service does not exist; what adding it would give you; a link to Add | The Search-and-Grab first-run copy (§8.5) is the reference tone |
| **permission-denied** | that it is denied, without leaking existence | v0.1 has one account, but the state exists from day one — and §14 rule 6: never construct a UI that hides items the API would still return |

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
| SC 2.4.7 Focus Visible | AA | a visible indicator | never removed to buy density; `:focus-visible`, so mouse users do not see rings |
| [SC 2.4.11 Focus Not Obscured](https://www.w3.org/WAI/WCAG22/quickref/) | AA | focused component not entirely hidden | sticky headers are the usual violator — test it |
| [SC 2.4.13 Focus Appearance](https://www.w3.org/WAI/WCAG22/Understanding/focus-appearance.html) | AAA | ≥2 px perimeter, 3:1 between focused and unfocused | met: 2 px ring at 16.50:1 / 15.33:1 |

**The one that bites dense UIs**, and the reason it is called out separately: the muted grey used
for secondary metadata **is text**, so it must clear 4.5:1. There is no 3:1 shortcut for it. UsArr
targets 5.5:1 to leave slack for theme tuning. Row dividers, by contrast, are decorative separators
and are genuinely unbound — but a border that is the *only* cue a control exists **is** bound by
SC 1.4.11, which is why `tokens.css` separates `--border` from `--border-strong`.

**Keyboard model, beyond the success criteria** (this is a power tool):

- **A list or grid is one tab stop; arrow keys move within it** — roving tabindex, per the ARIA APG
  (<https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/#kbd_roving_tabindex>). Ten
  thousand tab stops is not a keyboard model. This is the Motif traversal model, which is the
  ancestor of the pattern (OSF/Motif Style Guide rev 1.2, 1993).
- `/` focuses search; `Esc` clears or closes; `j`/`k` and arrows move; `Enter` opens; `?` opens a
  shortcut sheet. Sonarr already ships a keyboard-shortcuts modal in the header actions menu, so
  this is a convention, not an invention.
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
- **More media types.** Music, Books, Comics and Audiobooks arrive in v1.0. §17.2's rule — a type
  the user does not have is not shown at all — means adding them is a data change, not a layout
  change, provided nothing hard-codes two sections.
- **The cross-media review inbox** (FUTURE §5) is *"a whole second surface"* — a confirm / reject /
  not-sure queue over the 0.55–0.85 confidence band with `evidence` rendered so a human can judge.
  It is explicitly deferred and explicitly not a v1 screen. The design seam is that
  `work_relation` already carries `confidence` and `evidence`.
- **Multi-user** (v1.0) unlocks user management, roles and per-user grants. §17.1 and ADR-0019 make
  this a UI unlock, not a migration — which only holds if v0.1 never hard-codes "there is one
  user" into a layout.
- **The density control is itself a seam.** Three settings exist from day one; adding a fourth is
  three lines in `tokens.css`.
- **The token file is the styling seam.** Every decision here lives as a CSS custom property in one
  file. Tailwind consumes it, and scoped Svelte `<style>` blocks read the same variables. Swapping
  the utility engine later touches the token file and the class attributes, **not the component
  logic** (ADR-0024).

---

## 13. The lint checklist

`[grep]` = enforceable as a CI grep, ESLint or Stylelint rule. `[review]` = human judgement at PR
time. This is the reviewable form of §1, and it is the artefact that makes "don't look
AI-generated" an actual gate rather than a vibe.

**Colour**
- `[grep]` No `indigo|violet|purple|fuchsia` class or equivalent hex/oklch anywhere in the app.
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
- `[grep]` No font-size token above 24px in the application bundle.
- `[grep]` No `text-align: center` outside empty-state and dialog components.
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
- `[grep]` No `outline: none` anywhere.
- `[review]` Every dropdown of options is a native `<select>`; checkboxes and radios are native.
- `[review]` Everything that navigates is an `<a href>` with a real URL. No `<div>` with a click
  handler, no `onclick` navigation.

**Copy**
- `[grep]` Banned words across all UI strings: seamlessly, effortlessly, powerful, simply, unlock,
  empower, elevate, streamline, supercharge, robust, leverage, intuitive, blazing, world-class,
  comprehensive, "AI-powered".
- `[grep]` No `—` (U+2014) in any string under 15 words.
- `[grep]` No `!` in UI strings.
- `[grep]` Buttons and labels are sentence case (proper nouns like Sonarr, Prowlarr, Navidrome
  excepted).
- `[review]` No first-person plural.
- `[review]` Every error string names the component, the observed symptom and the next action.
- `[review]` No fabricated data anywhere — including screenshots, docs and empty states.

**States**
- `[review]` Every component demonstrates the §10 state set. A component with only a happy path
  does not merge.
- `[review]` Every screen depending on an optional service degrades with a named reason and a link
  to the fix.
- `[review]` Contrast re-measured in both themes when any token changes.

**The two catch-alls** (§1.6), applied to every PR that touches a screen.

---

## 14. Open questions for Joe

| # | Question | Why it needs you |
|---|---|---|
| **OQ-1** | **Virtualization threshold.** ARCHITECTURE §4.5 says "virtualize everything over ~200 rows"; §7.4 above argues for "Load more" + `content-visibility: auto` below ~1,000 rows, because virtualization breaks Ctrl+F in a **library browser**. Amend §4.5, or accept the Ctrl+F loss explicitly? | §4.5 is authoritative and this document does not overrule it. It is a real functional trade, not a style preference |
| **OQ-2** | **Navigation.** §17.2 named two options (home sections / top-navbar type tabs); §8.1 picks a **left sidebar** carrying both. Does that count as resolving §17.2, or does §17.2 exclude a third option? | §17.2 says "pick one and do not relitigate", so this needs settling once |
| **OQ-3** | **Font budget.** IBM Plex Sans + Mono, Latin-subset, ~120–180 KB estimated (**unmeasured**). Ship it, or fall back to the zero-webfont system stack and accept cross-OS metric drift? | It is the only decision here that costs bytes on first paint, which §17.1 calls the honest lever |
| **OQ-4** | **13 px base type.** That is the Linear/dense register. If it reads small to you, move base to 14 and shift the whole scale up one step — **do not add a seventh step** | Personal legibility; you are the only user in v0.1 |
| **OQ-5** | **Radius 0 or 2 px** on inputs and buttons. Both are within the budget; 0 is more committed | Taste, and it should be decided once and applied without exception |
| **OQ-6** | **Theme default.** Auto (Sonarr's default) or Dark (Navidrome's)? | Navidrome is the stated reference point, but Auto is the *Arr convention and UsArr sits next to three of them |
| **OQ-7** | **Tailwind's native engine.** ADR-0024 records `@tailwindcss/oxide`'s fetching postinstall as the strongest argument against, with Open Props as the pre-agreed fallback. Is that trigger condition acceptable to you as written? | It touches `make check`'s "exactly one network call" property, which is your rule |

---

## 15. Sources

Repository documents: [`CLAUDE.md`](../../CLAUDE.md) · [`ARCHITECTURE.md`](../ARCHITECTURE.md)
§4.4, §4.5, §13, §16, §17 · [`DECISIONS.md`](../DECISIONS.md) ADR-0003, ADR-0004, ADR-0012a,
ADR-0015, ADR-0019, ADR-0020, ADR-0024 · [`FUTURE.md`](../FUTURE.md) §5, §8, §9 ·
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
