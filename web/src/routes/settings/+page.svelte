<script lang="ts">
	/**
	 * Settings, with the three chrome preferences that exist today.
	 *
	 * These are the only settings that are real: theme, density and the
	 * single-key shortcut toggle are device-local and need no backend, so they
	 * work on a fresh install with nothing configured. Everything else this
	 * screen will eventually hold — libraries, providers, users — is a later
	 * slice and is not stubbed out here, because a control that does nothing is
	 * worse than an absent one.
	 *
	 * The shortcuts toggle is not a convenience. SC 2.1.4 is Level A and
	 * requires that a single-character shortcut can be turned off, remapped, or
	 * confined to focus; this one control turns all of them off at once, and it
	 * is the cheapest of the three routes (DESIGN-DIRECTION §11).
	 *
	 * Every control is native — a native <select>, a native checkbox — so Space
	 * toggles, Tab traverses, and the state is announced with no ARIA at all.
	 */
	import { prefs, type Density, type Theme } from '$lib/prefs.svelte';

	function onTheme(event: Event) {
		prefs.setTheme((event.currentTarget as HTMLSelectElement).value as Theme);
	}

	function onDensity(event: Event) {
		prefs.setDensity((event.currentTarget as HTMLSelectElement).value as Density);
	}

	function onShortcuts(event: Event) {
		prefs.setShortcuts((event.currentTarget as HTMLInputElement).checked);
	}
</script>

<svelte:head><title>Settings · UsArr</title></svelte:head>

<div class="section">
	<div class="section__head"><h2>Appearance</h2></div>

	<div class="settings-form">
		<div class="field">
			<label class="field__label" for="pref-theme">Theme</label>
			<select id="pref-theme" class="select" value={prefs.theme} onchange={onTheme}>
				<option value="auto">Auto</option>
				<option value="light">Light</option>
				<option value="dark">Dark</option>
			</select>
			<p class="field__help">Auto follows the setting your operating system reports.</p>
		</div>

		<div class="field">
			<label class="field__label" for="pref-density">Density</label>
			<select id="pref-density" class="select" value={prefs.density} onchange={onDensity}>
				<option value="compact">Compact</option>
				<option value="standard">Standard</option>
				<option value="relaxed">Relaxed</option>
			</select>
			<p class="field__help">
				Compact fits the most rows on a screen. Relaxed gives rows with inline buttons enough room
				to hit them.
			</p>
		</div>
	</div>

	<div class="section__head"><h2>Keyboard</h2></div>

	<div class="settings-form">
		<div class="field">
			<label class="check">
				<input
					type="checkbox"
					id="pref-shortcuts"
					checked={prefs.shortcuts}
					onchange={onShortcuts}
				/>
				Single-key shortcuts
			</label>
			<!--
				SPECIFIED AND UNBUILT — the keyboard-shortcut sheet. This help text closed with
				"and ? always opens the shortcut list", which was a present-tense promise with
				nothing behind it: web/src has no `?` handler and no sheet, and
				shellkeys.test.ts asserts `?` is not a handler on purpose. The sheet is
				specified in docs/design/DESIGN-DIRECTION.md:2863-2864 ("`?` opens a shortcut
				sheet"), and :2878 carries the rule that `?` stays ungated by this toggle when
				it lands, because the sheet is where the toggle is discovered. ARCHITECTURE
				§16 assigns the sheet no milestone, so the sentence comes back with the sheet
				and not before it — a feature earns its place in a milestone rather than in a
				help string.
			-->
			<p class="field__help">
				Turns off the single-key shortcuts as a set. Shortcuts that need a modifier are not
				affected.
			</p>
		</div>
	</div>
</div>

<style>
	.settings-form {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		max-width: 560px;
		margin-bottom: var(--space-7);
	}
</style>
