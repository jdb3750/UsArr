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
and ADR-0025 (the styling and typography stack, which this document is the rationale for).
**Deferred UI surfaces and the seams they need:** [`FUTURE.md`](../FUTURE.md).

Where §17 and this document disagree, **§17 wins** and this document is the bug.

> **Four things the owner settled on 2026-08-16, and they are decided rather than proposed.**
>
> 1. **Colourless chrome is confirmed.** There is no brand accent hue (§3). The one recorded cost —
>    that with no accent, two table screens have little left to differ by at thumbnail scale (D-24) —
>    is accepted, not reopened.
> 2. **IBM Plex is confirmed** as the family (§4). What is *not* settled is the **subset**; OQ-3 is
>    narrowed to that and nothing else.
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
| **ARCHITECTURE §16** (amended, ADR-0032) | **v0.1 is six media types, read-only beyond video**: Sonarr + Radarr, plus the read-only catalogue sources **Navidrome, Audiobookshelf and Komga**, plus Prowlarr. **Kavita is v0.2**, not v0.1 — ADR-0032 moved it out, and ARCHITECTURE §16 and §7.1a are authoritative. The **command sinks** (Lidarr, LazyLibrarian, Mylar3, Kapowarr) are v1.0. Requests in v0.1 is the **Prowlarr Search-and-Grab path only — for all six types** |
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
| `warning` | degraded, stale, partial, clock skew | `#8a5300` (6.02:1) | `#e0a33a` (8.15:1) |
| `error` | down, failed, rejected, needs re-identification | `#b3251c` (6.24:1) | `#f0837a` (7.09:1) |
| `not configured` | no service of this kind exists | `#5a534a` (7.20:1) | `#b0a89b` (7.68:1) |

**Grey is a legitimate status and it is load-bearing.** "Not configured" is a real, common,
non-broken condition in a pluggable app, and painting it a colour would make the two conditions that
*are* broken harder to find.

> **Every status is encoded as icon + text + colour together. Never colour alone.**

🚩 **The status *word* and the status *glyph* are bound by different success criteria, and the light
warning is where that matters.** `--status-warn: #8a5300` was chosen to clear 4.5:1 as **text** on
the light ground, and forcing an amber dark enough to be readable body text is what took the amber
out of it — the ⚠ glyph then does the signalling and the colour does much less than §3's argument
promises. **But the glyph is a non-text graphic, bound by SC 1.4.11 at 3:1, not by 1.4.3 at 4.5:1**,
so it can carry materially more chroma than the word beside it. **The rule: the word keeps the
readable value; the glyph may take a separate, more saturated token, provided it clears this
document's own 3.2:1 non-text target against both the page ground and the surface fill.** A warning
row may additionally take a low-alpha tint of the same hue.

⚠️ **The candidate value is computed, not verified, and it must go through the repo's contrast
script before it lands.** `#a9700a` measures **3.98:1** on `#faf9f7` and **3.68:1** on `#f2f0ec`,
clearing both the 3:1 floor and the 3.2:1 target, at ΔE76 ≈ 13.7 from the text value — a real gain
in chroma. Two values the finding suggested do **not** work and are recorded so they are not tried
again: `#c98a00` is **2.80:1** and `#b8860b` is **3.09:1**, so both fail this document's target and
the second only scrapes the WCAG floor.

📌 **And one part of the finding that raised this is rebutted on measurement.** It described
`#8a5300` as sitting *inside* the warm neutral ramp, reading as "slightly redder body text", with
"the colour doing none" of the work. Computed, ΔE76 from `--fg-muted #5a534a` is **46.3** in light
against **54.2** for the dark pair (`#e0a33a` from `#b0a89b`) — a gap ~15% narrower, not a collapse.
The *direction* is right and is why the rule above exists; the *magnitude* is not supported.
🔍 Both instruments are crude in opposite ways: ΔE76 is a poor model of perceived difference at small
sizes, and the finding's own instrument was a judgement on rendered pixels at thumbnail scale, which
is a legitimate test this arithmetic does not replace. Neither settles it alone, which is the reason
the value is escalated rather than simply changed.

That is why UsArr needs no colour-impaired mode. Sonarr had to ship one —
`EnableColorImpairedMode`, help text *"Altered style to allow color-impaired users to better
distinguish color coded information"*, default `false`
([ConfigService.cs](https://github.com/Sonarr/Sonarr/blob/develop/src/NzbDrone.Core/Configuration/ConfigService.cs))
— precisely because those apps encode a lot of meaning in colour alone. Building the redundancy in
from day one removes the need for a setting later.

### 3.3 Protocol chips — **the chip is neutral; the word carries the protocol**

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
on the error red. **So the protocol chip loses its colour: it is neutral, and the words `torrent`
and `usenet` carry the distinction — which they already do, in the same cell.** That is §1.4's own
rule applied to a fill instead of an icon (*if a label sits next to it and the label alone would be
understood, delete the decoration*), it is "cut before you add" removing two tokens rather than
adding a fourth hue, and it restores §3's argument by leaving chroma to status alone. `Protocol` is
still a first-class filterable field (§16's `source:` tag); it is the *ink* that goes.

📌 **Reversible in one line, and flagged for Joe as a change of visible character** rather than a
defect fix — the tokens `--protocol-torrent` / `--protocol-usenet` are deleted from `tokens.css`,
and restoring them plus a torrent hue outside the status band is the alternative if he wants the
ecosystem's colour cue kept.

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

> ⚠️ **The typeface decision has never been seen. Recorded here because it changes how much weight
> §4.1 can carry.** The prototype ships **zero `@font-face` rules**, so every screenshot, every
> review and every judgement made about this design so far has been rendered in the *fallback* stack
> — measured as **DejaVu Sans and Liberation Mono** on the review host, via a canvas advance-width
> probe (`"IBM Plex Sans"` measured **identical** to a deliberately bogus family name, while
> `document.fonts` reported it "available", which is a false positive worth knowing about). DejaVu is
> roughly **24% wider** than Plex with a taller x-height and a looser default fit, so:
>
> - **Density findings against the prototype are conservative**, not optimistic. Real Plex fits more
>   per column and would reduce wrapping, never increase it.
> - **Hierarchy findings are unaffected** — size ratio, weight and colour are face-independent.
> - **The anti-Inter argument's payload has never actually been delivered.** §1.2 and §4.1 stake the
>   strongest anti-slop claim in this document on the face — mania.design's *"the single
>   highest-leverage move against slop"* — and a build without the font gives a Linux self-hoster
>   DejaVu, a Windows one Segoe UI and a macOS one SF. On the system stack, §1.2's typography row is
>   satisfied by a fallback rather than by a decision.
>
> **What would validate it, and nothing short of this does:** the subsetted WOFF2 faces actually
> loaded — Plex Sans 400/500/600 and Plex Mono 400/600, `latin` subset, ~104 KB per §4.1's measured
> table — served locally with a probe confirming the *rendered* family (an advance-width comparison
> against a bogus family name, not `document.fonts.check()`), and the density and hierarchy screens
> re-shot on it side by side with the system-stack capture. Until that exists, **the family is
> decided and unvalidated**, and OQ-3 (the subset) is being answered on top of a choice nobody has
> looked at. If the load cannot be made to work, the honest alternative is not to keep the claim —
> it is to take the recorded fallback below and design against the system stack deliberately.

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
inconsistent within a single row. A **user-chosen label** — `Radarr`, `Komga`, `Sonarr Anime`, the
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
wrong by ~33% at 25,000 rows.

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

**Settled, 2026-08-16: the owner delegated this and it is now ADR-0029.** The default is **"Load
more" plus `content-visibility: auto` with `contain-intrinsic-size`**, and virtualization is an
**escalation above a threshold set from a benchmark that does not exist yet** — deliberately not the
"~1,000 rows" this document previously floated, because the finding against §4.5's "~200" was that it
had no measurement behind it, and answering an unmeasured number with a different unmeasured number
concedes the argument while pretending to fix it. `make bench` gains the measurement (ARCHITECTURE
§4.5, §13); the threshold is whatever it says.

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

⚠️ **`contain-intrinsic-size` still has no value, and the value this section previously prescribed
is wrong three ways.** The browser uses that number as the placeholder height for every skipped
element; when it is wrong the scrollbar drifts as content scrolls into view, which reads as
*slowness* — the precise failure this section exists to avoid. `contain-intrinsic-size: auto
var(--row-h)` fails because:

1. **`--row-h` is inert on the element it describes.** `min-height` does not apply to
   `display: table-row`; measured, forcing `--row-h: 100px` leaves the row at **28.0 px**, and the
   density control works only through the padding token. So the ADR derived the placeholder from
   the one property with no effect on the real height. The grid-row primitive above fixes this as a
   side effect, since `min-height` does apply to a grid item — but the table below must then be
   read as what it is.
2. **Row heights are not the six values in §5.3's table.** Measured on the shipped search screen at
   compact density there are **six distinct heights — 28, 30, 45, 47, 59, 62 px, mean 42.0** — and
   **eighteen across the three densities**, because real rows wrap. Estimating 25,000 rows at 28 px
   understates the scroll height by ~350,000 px (33%), resolving as the user scrolls: the drift
   itself.
3. **`contain-intrinsic-size` sizes the *content* box.** Padding and border are added on top; a
   24 px row with `auto 28px` produced a **37 px** placeholder.

**What ships: `contain-intrinsic-size: auto <measured content-box height>` per row shape**, with
`auto` remembering the last real measurement, and the assertion is drift rather than frame time —
`|scrollHeight after a full scroll − scrollHeight at load| / scrollHeight < 2%` at 1k / 5k / 25k
rows, both themes, all three densities. **Until that measurement exists, §7.4 is a direction, not an
implementable rule** — and settling OQ-1 did not close this, deliberately.

⚠️ **And the expensive operation on a long list is not scrolling.** Measured: scrolling costs
**0.1–0.3 ms** at every size, while the **density toggle** costs **153 ms at 1,000 rows, 1,199 ms at
5,000 and 6,508 ms at 25,000**, and the **theme toggle** 1,356–4,514 ms at 25,000 — because each
sets an attribute on `<html>` and invalidates every element reading a custom property. Both are
top-bar controls on every screen, both are pure-local no-data interactions, and both are therefore
**Tier 0 by §7.2's own definition, whose hard fail is 100 ms**. 🔍 Extrapolating the measured
0.15–0.26 ms/row to a Pi 5 at a conservative 3–5× puts that hard fail at **100–300 rows in the DOM**,
or 300–600 with `table-layout: fixed` and working containment — **so the real ceiling is set by the
density control, in the hundreds, not by scrolling in the tens of thousands.** Three mitigations
before any redesign: set `table-layout: fixed` (never set anywhere today, and it halves the cost);
scope the density attribute to the list container rather than `:root`; and if it still exceeds
100 ms, an explicit 150 ms "applying" state is honest where a silent multi-second freeze is not.

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
> comics), one sidebar entry per type *that has content*, bounded at six by construction.
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

**The sidebar, with the scope chip and the six types.** Type entries are **data-driven**, not
markup: §17.2's hard rule applies to the sidebar exactly as it does to Home — **a type the user does
not have is not shown at all**, so a movies-only install renders two content nouns and a
music-and-books install renders three. Nothing hard-codes a type.

```
┌────────────────────────────┐
│  UsArr                 [«] │   collapse toggle
├────────────────────────────┤
│ ▣ All libraries (4)      ▾ │   scope chip — RENDERS NOTHING at 0 or 1 library
├────────────────────────────┤
│   Home                     │
├────────────────────────────┤
│   Movies             1,204 │  ┐
│   TV                   214 │  │  present media types only,
│   Music              8,930 │  ├  fixed order, hard max 6,
│   Audiobooks           412 │  │  counts respect the scope chip
│   Ebooks             2,051 │  │
│   Comics               733 │  ┘
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
Requests, Services, Settings, System = 6, plus Calendar and Stats later (§12) = 8. Types ≤ 6. Scope
chip = 1. That is 15 before pins, so the **budget is 16 rows** and pins are capped at
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
Its height is **O(1) in the number of media types**:

```
┌───────────────────────────────────────────────────────────────────────────────────────┐
│ Home                                                    [ Table | Posters ]  [Filter] │  40px toolbar
├───────────────────────────────────────────────────────────────────────────────────────┤
│ Radarr 4K is unreachable — showing cached data from 14:02          [Open Services]    │  banner, only when degraded
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
│ ▲ warning  Komga         last successful sync 3 d ago            [Run full sync now]  │
│ ▲ warning  1 work needs re-identification (Lidarr)               [Re-link]            │
│                                                                                       │
│ Recently added                                          across 6 types    see all →   │  BLOCK C
│ ───────────────────────────────────────────────────────────────────────────────────── │
│ Type    Title                            Added   Detail              Have   Instance  │
│ movie   Dune: Part Two                   14:02   Bluray-1080p 14.2G   ✓     Radarr    │
│ album   Geogaddi — Boards of Canada      13:58   FLAC · 23 tracks     ✓     Lidarr    │
│ comic   Saga #61                         13:55   CBZ · 24 p           ✓     Komga     │
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
| 13 | Show a type, section, group or control with no content | §17.2; Komga's `v-if="collectionsCount > 0"`; Navidrome's `LibrarySelector` returning `null` at ≤1 library; Sonarr's status badge returning `null` at zero |

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
  which satisfies both. **CI asserts it**, and it is cheap: no element's
  `getBoundingClientRect().right` may exceed `innerWidth` on any screen × state × width triple.
- **A figure and its unit are two slots, not one string.** `tabular-nums` on 222 cells buys nothing
  when what is right-aligned is the *word after the number*: `1,204 films` / `275 series` /
  `612 artists` right-aligned as one string starts the digits at six different x-positions. The
  figure is a right-aligned numeric span, the unit is a fixed-width left-aligned span beside it, and
  `tabular-nums` is applied at the cell rather than per-span so composite values
  (`1,187 have · 17 wanted`, `13,204 / 14,061 episodes`) inherit it — eight such cells had it
  missing, and they are precisely the composite ones.
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
  right direction. §11's `dominant_color` rule is **narrowed, not withdrawn**: it still governs any
  text set on a computed fill, which is the row-level dominant tint, where the ground is known.
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
and editable, so the single-instance case still types three things.

Credentials render as `••••••1a2b` and are never returned by the API. Changing `base_url`'s
scheme/host/port **invalidates the stored credential**, and that has to be visible rather than
implicit: **the key field clears, Save disables, and the form says why** (§17.3). Fixing a typo in a
hostname is the most common edit on that screen and it was silently repointing a full-admin
credential at a host the user had just typed.

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
| 5 | **`ScopeChip`** | the library selector | §8.1 |

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
as one average over the 92 px poster fetch, and the poster card renders the title (12 px / 600) and
year on top of it. **Nothing constrained the pair, and the shipped sample data already fails** —
`#16130e` on `#7d6a4f` is **3.57:1** for the title and **3.12:1** for the year, against 4.5:1 for
both. One bad hand-picked swatch would be a nit; having no rule is the finding, because with an
average taken over arbitrary cover art, mid-luminance fills are common and *both* black and white
land near 3.5:1 on them.

> **Pick whichever of the two theme text tokens scores higher against the computed
> `dominant_color`. If the winner is still below 4.5:1, adjust `dominant_color`'s lightness — away
> from the text colour, in 2% steps in OKLCh, preserving hue and chroma — until it clears. The fill
> is decoration; the title is content, and content wins.**

Two supporting rules, because otherwise the ratio is not computable from what ships. **Neither the
title nor the year carries `opacity`** — compositing changes the effective ratio (by ~0.45 on the
measured pair) through a mechanism no contrast check sees, so the year gets a real colour token.
And **12 px semibold is normal text under WCAG, not large** (large is ≥18.66 px bold or ≥24 px), so
4.5:1 applies to both lines. **Asserted in CI over any `--dc` / `--dc-fg` pair that ships in a
fixture**, and in the image pipeline where the colour is produced (ARCHITECTURE §4.4.1).

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
- **Libraries grow without bound and the layout must not care.** The scope chip (§8.1) is the seam:
  a library is `?lib=` on an existing route, so a user with thirty libraries costs zero new page
  types and zero sidebar rows.
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
  logic** (ADR-0025).

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
  `sort_title` → "sort title", `no work identity` → "matched by title", `breaker open` → "paused —
  7 failed attempts, retrying 14:19".
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
  is quoted verbatim as *"Radarr 4K is unreachable — showing cached data from 14:02"*, which is
  eight words. §17 wins over this checklist, so the rule carries the exception rather than the
  banner carrying a rewrite.
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
  one colour that is data rather than a token, so it cannot be checked once.
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
| **OQ-3** *(narrowed)* | **The family is settled — IBM Plex, confirmed 2026-08-16. The *subset* is not.** `latin` alone is **103.6 KB**; `latin`+`latin-ext` is **177.2 KB** (§4.1). Neither trips the ~200 KB trigger, so the question is only whether an accented library gets `latin-ext` or falls back mid-string — and six media types make that sharper than two did, since a manga, classical-music or translated-fiction library is *full* of accented and transliterated titles. Two loose ends the family decision does not close: the argument that beat the system stack (cross-OS metric drift) is **uncited inference, unmeasured for this design** (D-32), and the mockup README asserts the opposite of it (D-30) | It is the only decision here that costs bytes on first paint, and the two documents still disagree about whether the specific face matters |
| **OQ-4** | **13 px base type.** That is the Linear/dense register. If it reads small to you, move base to 14 and shift the whole scale up one step — **do not add a seventh step** | Personal legibility; you are the only user in v0.1 |
| **OQ-5** | **Radius 0 or 2 px** on inputs and buttons. Both are within the budget; 0 is more committed | Taste, and it should be decided once and applied without exception |
| **OQ-6** | **Theme default.** Auto (Sonarr's default) or Dark (Navidrome's)? | Navidrome is the stated reference point, but Auto is the *Arr convention and UsArr sits next to three of them |
| **OQ-7** | **Tailwind's native engine.** ADR-0025 records `@tailwindcss/oxide`'s fetching postinstall as the strongest argument against, with Open Props as the pre-agreed fallback. Is that trigger condition acceptable to you as written? | It touches `make check`'s "exactly one network call" property, which is your rule |

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
