/**
 * THE SHELL'S ONE CHARACTER-KEY SHORTCUT, AS A PURE DECISION.
 *
 * `$lib/shellscroll`'s `scrollDeltaFor` is the same shape and the same reason:
 * the shell's window keydown handler lives in a `.svelte` file that this
 * repo's test setup cannot compile — `vitest.config.ts` is `environment:
 * 'node'` with no Svelte plugin — so the part that is a decision is a function
 * here, and the part that touches the DOM stays in the layout.
 *
 * The split is drawn at exactly the DOM boundary: everything below reads
 * booleans and an event's own fields, and `focusTakesText` (which needs
 * `HTMLElement`) is applied by the caller.
 */

/**
 * The fields of a `KeyboardEvent` this decision reads, and nothing else, so a
 * test can hand it a plain object.
 */
export type SearchKeyEvent = Pick<
	KeyboardEvent,
	'key' | 'ctrlKey' | 'altKey' | 'metaKey' | 'repeat' | 'defaultPrevented'
> & { isComposing?: boolean };

/** What the shell knows that the event does not. */
export interface SearchKeyContext {
	/**
	 * `prefs.shortcuts`. `/` is a single-key shortcut with no modifier, which
	 * is precisely what SC 2.1.4 (Level A) requires a way to turn off, and the
	 * Settings toggle is that way for all of them at once. A toggle the shell
	 * did not read was a control reporting a state it did not have.
	 */
	shortcuts: boolean;
	/**
	 * Whether the shell is rendering its nav. Search is reachable by key where
	 * it is reachable by click: signed out, on /login, or on the unreachable
	 * screen, `/` is an ordinary character and stays one.
	 */
	showNav: boolean;
	/** Already on Search: there is nothing to navigate to. */
	onSearchScreen: boolean;
}

/**
 * True when this key press is §17.4's `/` shortcut to the Search screen, and
 * the caller should consume it.
 *
 * ⚠️ SHIFT IS DELIBERATELY NOT EXCLUDED. On a US layout Shift+/ produces `?`
 * and never arrives here; on the German and French layouts `/` IS a shifted
 * key, so `event.key === '/'` comes in with `shiftKey` true and excluding it
 * would delete the shortcut on those keyboards. The test is the character the
 * layout produced — which is what `key` reports — not the keys pressed to
 * produce it. That is the opposite call from `scrollDeltaFor`, which drops
 * Shift because Shift is a selection modifier on a NAMED key; here the key is
 * a character and Shift may be part of typing it.
 *
 * ⚠️ `l` IS DELIBERATELY NOT HERE. DESIGN-DIRECTION specifies `l` for the
 * scope popover; §8.1's scope chip is unbuilt, so the key would open nothing.
 * Adding it would repeat the defect this module exists to fix — a control that
 * claims a behaviour it does not have — rather than fix it.
 */
export function opensSearch(event: SearchKeyEvent, ctx: SearchKeyContext): boolean {
	if (event.key !== '/' || event.defaultPrevented || event.repeat) return false;
	// Ctrl, Alt and Meta combinations are browser and OS shortcuts — Meta+/ and
	// Ctrl+/ both mean something to somebody — and mid-IME the key belongs to
	// the composition, whatever it looks like.
	if (event.ctrlKey || event.altKey || event.metaKey || event.isComposing) return false;
	if (!ctx.shortcuts) return false;
	if (!ctx.showNav || ctx.onSearchScreen) return false;
	return true;
}
