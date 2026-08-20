/*
 * THE SCOPE SELECT — changing `?lib=` from the screen you are already on.
 *
 * WHAT IT IS. `/library` and `/library/[type]` could already ARRIVE at a library
 * scope (the Libraries screen authors the `?lib=` link) and could already CLEAR
 * one ("Show every library"). Neither could CHANGE one: switching from Kids to
 * Vinyl rips meant navigating back to Libraries and picking the other row. This
 * module is the decision half of the one control that closes that gap — a plain
 * `<select>` in the browse toolbar, beside the Sort select it deliberately looks
 * exactly like.
 *
 * ⚠️ THIS IS NOT DESIGN-DIRECTION §8.1's SCOPE CHIP, AND §8.1's CHIP REMAINS
 * UNBUILT. Nothing here discharges it, and a later reader must not read this
 * file as though something did. The chip is a SHELL-level control: it is hoisted
 * into the top bar, it is a popover, it MULTI-selects, it carries three
 * hand-written keyboard behaviours and a live region, and it propagates the
 * scope across navigation so the sidebar and every screen agree about it. This
 * is a SCREEN-LOCAL, SINGLE-SELECT `<select>` that lives in two toolbars, writes
 * one query parameter with `pushState`, and claims nothing about scope anywhere
 * else in the app. It is a strict subset, built inside the chip's outline
 * without being it — the sidebar, the `scope-empty` state and the top-bar hoist
 * are all still untouched, and §8.1 still has a wire precondition
 * (DESIGN-DIRECTION §9.7) that this control does not need and does not meet.
 * The name is `scopeSelect` everywhere — in this module, in the screens, in the
 * tests — precisely so that a search for the chip does not find it.
 *
 * WHY THE LOGIC IS HERE RATHER THAN IN THE TWO `.svelte` FILES.
 * `web/vitest.config.ts` is `environment: 'node'` with no Svelte plugin, so a
 * component cannot be compiled in a test at all — a rule written inside an
 * `{#if}` can only be guarded by reading the template as text. Every decision
 * this control makes is therefore a pure function here, called and asserted
 * directly, and the templates keep only the markup.
 */

import type { LibraryNames } from './librarygrid';

/** One entry in the select. `value` is the `?lib=` the entry writes; `''` is the
 * unscoped view, which is the parameter's ABSENCE and never its empty spelling —
 * see `scopeSelectSearch`. */
export interface ScopeOption {
	value: string;
	label: string;
}

/**
 * The unscoped entry's words.
 *
 * It matches the existing "Show every library" affordance's noun rather than
 * inventing a second vocabulary for the same state: one screen must not call the
 * same thing "every library" in a link and "All" in a control beside it.
 */
export const EVERY_LIBRARY = 'Every library';

/** The select's own label. A noun, because the control names what it scopes to
 * rather than the act of scoping. */
export const SCOPE_SELECT_LABEL = 'Library';

/**
 * SLUGS, TRIMMED, EMPTIES DROPPED, AND DEDUPED — in first-seen order.
 *
 * ⚠️ THE DEDUPE IS THIS CONTROL'S OWN AND IS NOT A FIX TO `readLibraryScope`,
 * WHICH STILL DOES NOT DEDUPE. `?lib=a,a` is expressible by hand and today reads
 * as `['a','a']`: it counts twice against `MAX_LIBRARY_SLUGS` and renders as
 * "Scoped to Kids, Kids". That is a reader question and it is left alone
 * deliberately (the commit message carries the reasoning). What is settled HERE
 * is narrower and is a hard requirement: THIS CONTROL NEVER WRITES A DUPLICATE.
 * It matters because the one option whose value is not a single slug is the
 * synthetic "current scope" entry below, which is built FROM the address — so on
 * `?lib=a,a` an un-deduped entry would write the duplicate straight back.
 */
export function distinctSlugs(parts: readonly string[]): string[] {
	const out: string[] = [];
	for (const part of parts) {
		const slug = part.trim();
		if (slug === '' || out.includes(slug)) continue;
		out.push(slug);
	}
	return out;
}

/**
 * WHICH OPTION IS SELECTED, as the `<select>`'s value.
 *
 * `''` for the unscoped view. For a scope it is the deduped slug list joined the
 * way the address spells it, so it matches an option built by
 * `scopeSelectOptions` from the same address exactly — a `<select>` whose value
 * matches no option renders as its first entry, which here would silently claim
 * an unscoped view was scoped, or the reverse.
 */
export function scopeSelectValue(current: readonly string[]): string {
	return distinctSlugs(current).join(',');
}

/**
 * THE OPTION LIST: every library this install has, plus the unscoped entry, plus
 * — only when it is needed — the scope the address already carries.
 *
 * ⚠️ THE ADDRESS IS ALWAYS REPRESENTABLE, INCLUDING WHEN THE LIBRARY LIST CANNOT
 * REPRESENT IT. Two cases, and both are real rather than defensive:
 *
 *   - A SLUG THE LIST DOES NOT HAVE. `?lib=ghost` after the library was renamed
 *     or deleted, or a scope read from an address before `GET /api/v1/libraries`
 *     has answered. `libraryScopeLine` already falls back to the slug for this,
 *     and so does this: the entry is labelled with the slug it could not resolve.
 *     Dropping it would make the control state a scope the screen is not in.
 *
 *   - TWO OR MORE LIBRARIES. `?lib=a,b` is expressible in the URL although no
 *     control writes one, and this control is single-select and cannot express
 *     it either. So the multi-library scope appears as ONE entry naming both,
 *     selected; picking any library REPLACES it with that single library, and
 *     picking the unscoped entry clears it. That is a switcher honestly saying
 *     what it can and cannot do, rather than a popover grown here by accident.
 *
 * ⚠️ AND `names` IS ALLOWED TO BE EMPTY OR LATE. It is a `ReadonlyMap` in
 * `GET /api/v1/libraries`'s own order (`libraryNames` builds it by insertion
 * from `fetchLibraries`, which does not re-sort), and a read that failed is
 * indistinguishable from an install with no libraries — deliberately so
 * (`readLibraryNames` cannot reject). An empty map yields no library entries,
 * which is what `scopeSelectWorthShowing` reads to keep a control that can do
 * nothing off the screen entirely.
 */
export function scopeSelectOptions(names: LibraryNames, current: readonly string[]): ScopeOption[] {
	const scope = distinctSlugs(current);
	const options: ScopeOption[] = [{ value: '', label: EVERY_LIBRARY }];

	// A one-library scope whose slug the list DOES carry needs no entry of its
	// own: the library's own option below already is it, and adding a second
	// option with the same value would put the same library in the list twice.
	const representable = scope.length <= 1 && scope.every((slug) => names.has(slug));
	if (scope.length > 0 && !representable) {
		options.push({
			value: scope.join(','),
			label: scope.map((slug) => names.get(slug) ?? slug).join(', ')
		});
	}

	for (const [slug, name] of names) options.push({ value: slug, label: name });
	return options;
}

/**
 * WHETHER TO RENDER THE CONTROL AT ALL.
 *
 * ⚠️ AT ZERO LIBRARIES THERE IS NO CONTROL, and that is the whole of this
 * function. An install with no libraries would otherwise get a `<select>` whose
 * only entry is "Every library" — a control offering the state the screen is
 * already in, which is the empty dropdown that reads as a broken screen rather
 * than as an install with nothing configured. Principle 3's honest degradation
 * is the Libraries screen's job to state, and it states it there.
 *
 * AT ONE LIBRARY THE CONTROL SHIPS. Two entries — that library and every
 * library — are two states the user can actually be in and can actually move
 * between, so the control has work to do.
 *
 * ⚠️ THIS GATES THE CONTROL AND NOTHING ELSE. The table, its empty state and its
 * failure banner are driven by the browse read alone and never by this — a
 * libraries read that is slow, empty or dead costs this one control and never a
 * row (ARCHITECTURE §17.7).
 */
export function scopeSelectWorthShowing(names: LibraryNames | undefined): boolean {
	return names !== undefined && names.size > 0;
}

/**
 * THE ADDRESS THE CONTROL WRITES, as a query string with no leading `?`.
 *
 * ⚠️ THE PARAMETER IS DELETED, NEVER EMPTIED, and this is the one invariant in
 * this file that is a wire fact rather than a taste. `?lib=` with nothing in it
 * is a `400`, not "no scope": `resolveBrowseLibraries` tests `query.Has("lib")`
 * — PRESENCE — so `?lib=`, `?lib=%20` and `?lib=,,` are all refusals, while an
 * empty `media_type` is silently unfiltered. Routing every empty spelling
 * through `distinctSlugs` and deleting on an empty result makes the refusal
 * unreachable from this control by construction rather than by a caller
 * remembering to check.
 *
 * The params object is passed in and MUTATED rather than built here, so the
 * screens can hand it the `SvelteURLSearchParams` that
 * `svelte/prefer-svelte-reactivity` asks for while this stays callable from a
 * node test with a plain `URLSearchParams`. Every other parameter on the address
 * — `sort` above all — survives untouched, so switching library keeps the order
 * you were reading.
 */
export function scopeSelectSearch(params: URLSearchParams, value: string): string {
	const slugs = distinctSlugs(value.split(','));
	if (slugs.length === 0) params.delete('lib');
	else params.set('lib', slugs.join(','));
	return params.toString();
}
