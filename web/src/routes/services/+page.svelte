<script lang="ts">
	/**
	 * Services — the SCAFFOLDING version.
	 *
	 * This is deliberately NOT the screen ARCHITECTURE.md §17.3 specifies. That
	 * one is per-instance breaker state, per-channel last-sync times, the
	 * verbatim upstream error and the single action that fixes it, and it is
	 * being designed in its own thread. What this route is, and all it is: the
	 * smallest thing that closes the last gap in the shell, which was that a
	 * fresh install had no way at all to add a service except by calling the API
	 * by hand — sign in, land on search, get 409 no_indexer_service, and stop.
	 *
	 * So: list what is configured, show whether each one works and the actual
	 * error when it does not, add one, re-test one, remove one. Nothing else.
	 * Delete this file wholesale when §17.3 lands.
	 *
	 * Three server rules it has to respect, all enforced in
	 * internal/httpapi/services.go and none of them cosmetic:
	 *
	 *  1. The API key is write-only. It is never in a response, so it is never
	 *     rendered back — `has_credential` is the whole truth this screen has.
	 *  2. Changing a base URL's scheme, host or port invalidates the stored key
	 *     and requires re-entry. The form says so while you type rather than
	 *     letting you press Save and receive a 400.
	 *  3. Every write sits behind `sudo`, which closes five minutes after you
	 *     sign in. A 403 sudo_required is answered with a password field and the
	 *     action is retried, not with an error you cannot act on.
	 */
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import {
		ApiError,
		confirmSudo,
		createService,
		deleteService,
		fetchServicesHealth,
		listServices,
		requiresCredentialReentry,
		testNewService,
		testService,
		updateService,
		urlBaseProblem,
		type ConnectionTestResult,
		type ServiceHealth,
		type ServiceInstance
	} from '$lib/api';
	import { session } from '$lib/session.svelte';

	/** One instance as this screen needs it: the health row plus has_credential. */
	type Row = { health: ServiceHealth; instance?: ServiceInstance };

	let rows = $state<Row[]>([]);
	let setupRequired = $state(false);
	let loaded = $state(false);
	let loadError = $state('');

	// Per-row transient state. Keyed by id so a slow test on one row never
	// disables the rest of the list.
	let busyRow = $state<number | undefined>(undefined);
	let rowMessage = $state<Record<number, string>>({});
	let rowProblem = $state<Record<number, string>>({});
	let editing = $state<number | undefined>(undefined);
	let removing = $state<number | undefined>(undefined);

	// The edit form, one row at a time.
	let editName = $state('');
	let editBaseUrl = $state('');
	let editUrlBase = $state('');
	let editApiKey = $state('');
	let editOriginalBaseUrl = $state('');

	// A refused sub-path belongs UNDER the sub-path field, not in the row's
	// problem line: `invalid_url_base` names exactly one input, and a message
	// that says which one is the difference between a fix and a guess.
	let editUrlBaseProblem = $state('');
	let addUrlBaseProblem = $state('');

	// The add form.
	let addName = $state('');
	let addBaseUrl = $state('');
	let addUrlBase = $state('');
	let addApiKey = $state('');
	let addBusy = $state(false);
	let addProblem = $state('');
	let addMessage = $state('');

	// The sudo prompt, and the action waiting behind it.
	let sudoPassword = $state('');
	let sudoBusy = $state(false);
	let sudoProblem = $state('');
	let pending = $state<(() => Promise<void>) | undefined>(undefined);

	const editNeedsKey = $derived(requiresCredentialReentry(editOriginalBaseUrl, editBaseUrl));
	const canAdd = $derived(
		!addBusy && addName.trim() !== '' && addBaseUrl.trim() !== '' && addApiKey !== ''
	);

	function describe(error: unknown): string {
		return error instanceof ApiError ? error.detail : String(error);
	}

	/**
	 * Run one write, and turn a 403 sudo_required into the password prompt with
	 * the same call queued behind it. The action re-runs itself on success, so a
	 * confirmed password finishes what was clicked rather than losing it.
	 */
	async function guarded(action: () => Promise<void>): Promise<void> {
		try {
			await action();
		} catch (error) {
			if (error instanceof ApiError && error.code === 'sudo_required') {
				pending = action;
				sudoProblem = '';
				sudoPassword = '';
				return;
			}
			throw error;
		}
	}

	async function load() {
		try {
			// Two calls, because they answer different questions: /health carries
			// the state and the verbatim problem, /services carries whether a
			// credential is stored. Neither carries the key.
			const [health, instances] = await Promise.all([fetchServicesHealth(), listServices()]);
			const byId = new Map(instances.map((i) => [i.id, i]));
			rows = health.services.map((h) => ({ health: h, instance: byId.get(h.id) }));
			setupRequired = health.setupRequired;
			loadError = '';
		} catch (error) {
			loadError = describe(error);
		} finally {
			loaded = true;
		}
	}

	onMount(load);

	function testSummary(result: ConnectionTestResult): string {
		const parts = [result.message];
		if (result.appName) {
			parts.push(result.appVersion ? `${result.appName} ${result.appVersion}` : result.appName);
		}
		return parts.filter((p) => p !== '').join(' — ');
	}

	async function runTest(row: Row) {
		const id = row.health.id;
		busyRow = id;
		rowMessage = { ...rowMessage, [id]: '' };
		rowProblem = { ...rowProblem, [id]: '' };
		try {
			await guarded(async () => {
				const result = await testService(id);
				rowMessage = { ...rowMessage, [id]: testSummary(result) };
				if (!result.ok) rowProblem = { ...rowProblem, [id]: testSummary(result) };
				await load();
			});
		} catch (error) {
			rowProblem = { ...rowProblem, [id]: describe(error) };
		} finally {
			busyRow = undefined;
		}
	}

	function startEdit(row: Row) {
		editing = row.health.id;
		removing = undefined;
		editName = row.health.name;
		editBaseUrl = row.health.baseUrl;
		editOriginalBaseUrl = row.health.baseUrl;
		// The health row carries no sub-path; GET /api/v1/services does. Blank
		// when the instance is served from the root, which is the common case.
		editUrlBase = row.instance?.urlBase ?? '';
		editApiKey = '';
		editUrlBaseProblem = '';
	}

	function cancelEdit() {
		editing = undefined;
		editApiKey = '';
	}

	async function saveEdit(event: SubmitEvent, row: Row) {
		event.preventDefault();
		const id = row.health.id;
		busyRow = id;
		rowProblem = { ...rowProblem, [id]: '' };
		rowMessage = { ...rowMessage, [id]: '' };
		editUrlBaseProblem = '';
		try {
			await guarded(async () => {
				await updateService(id, {
					name: editName,
					baseUrl: editBaseUrl,
					// Always sent, blank included: on PATCH the field is
					// present-means-set, so emptying it is how a sub-path is
					// removed once it is no longer behind a proxy.
					urlBase: editUrlBase,
					apiKey: editApiKey,
					currentBaseUrl: editOriginalBaseUrl
				});
				editing = undefined;
				editApiKey = '';
				rowMessage = { ...rowMessage, [id]: 'Saved.' };
				await load();
			});
		} catch (error) {
			// The form stays open on a refused sub-path, with the reason under the
			// field: the value is still in the input, so the fix is one edit away.
			editUrlBaseProblem = urlBaseProblem(error) ?? '';
			if (editUrlBaseProblem === '') rowProblem = { ...rowProblem, [id]: describe(error) };
		} finally {
			busyRow = undefined;
		}
	}

	async function confirmRemove(row: Row) {
		const id = row.health.id;
		busyRow = id;
		rowProblem = { ...rowProblem, [id]: '' };
		try {
			await guarded(async () => {
				await deleteService(id);
				removing = undefined;
				await load();
			});
		} catch (error) {
			rowProblem = { ...rowProblem, [id]: describe(error) };
		} finally {
			busyRow = undefined;
		}
	}

	function newServiceInput() {
		return {
			kind: 'prowlarr',
			name: addName,
			baseUrl: addBaseUrl,
			urlBase: addUrlBase,
			apiKey: addApiKey
		};
	}

	async function testBeforeAdd() {
		addBusy = true;
		addProblem = '';
		addMessage = '';
		addUrlBaseProblem = '';
		try {
			await guarded(async () => {
				const result = await testNewService(newServiceInput());
				addMessage = testSummary(result);
				if (!result.ok) addProblem = testSummary(result);
			});
		} catch (error) {
			addUrlBaseProblem = urlBaseProblem(error) ?? '';
			if (addUrlBaseProblem === '') addProblem = describe(error);
		} finally {
			addBusy = false;
		}
	}

	async function add(event: SubmitEvent) {
		event.preventDefault();
		if (!canAdd) return;
		addBusy = true;
		addProblem = '';
		addMessage = '';
		addUrlBaseProblem = '';
		try {
			await guarded(async () => {
				const created = await createService(newServiceInput());
				addMessage = `Added ${created.name}.`;
				addName = '';
				addBaseUrl = '';
				addUrlBase = '';
				addApiKey = '';
				await load();
			});
		} catch (error) {
			addUrlBaseProblem = urlBaseProblem(error) ?? '';
			if (addUrlBaseProblem === '') addProblem = describe(error);
		} finally {
			addBusy = false;
		}
	}

	async function submitSudo(event: SubmitEvent) {
		event.preventDefault();
		sudoBusy = true;
		sudoProblem = '';
		try {
			session.set(await confirmSudo(sudoPassword));
			sudoPassword = '';
			const action = pending;
			pending = undefined;
			if (action) await action();
		} catch (error) {
			sudoProblem = describe(error);
		} finally {
			sudoBusy = false;
		}
	}
</script>

<svelte:head><title>Services — UsArr</title></svelte:head>

<h2>Services</h2>

<p>
	UsArr searches through Prowlarr and reads its results from the local replica. Until an indexer
	service is configured there is nothing to search, which is why a fresh install answers
	<code>no_indexer_service</code> on <a href={resolve('/search')}>Search</a>.
</p>

{#if loadError}
	<div class="banner banner-error" role="alert">
		The configured services could not be read.
		<p class="banner-detail">{loadError}</p>
	</div>
{/if}

{#if pending}
	<!--
		Not a modal. `sudo` (internal/httpapi/auth.go) gates every write that
		touches a stored credential and the window closes five minutes after
		sign-in, so this is a normal part of the screen rather than an error.
	-->
	<form class="stack" onsubmit={submitSudo}>
		<p>
			That change touches a stored credential, so it needs a fresh password confirmation. Confirm
			and it will be applied.
		</p>
		{#if sudoProblem}
			<div class="banner banner-error" role="alert">
				<p class="banner-detail">{sudoProblem}</p>
			</div>
		{/if}
		<div class="field">
			<label for="sudo-password">Password</label>
			<input
				id="sudo-password"
				type="password"
				autocomplete="current-password"
				required
				bind:value={sudoPassword}
			/>
		</div>
		<div class="row-actions">
			<button type="submit" disabled={sudoBusy || sudoPassword === ''}>
				{sudoBusy ? 'Confirming' : 'Confirm password'}
			</button>
			<button type="button" onclick={() => (pending = undefined)} disabled={sudoBusy}>Cancel</button
			>
		</div>
	</form>
{/if}

<h3>Configured</h3>

{#if !loaded}
	<p class="empty">Reading the configured services.</p>
{:else if rows.length === 0}
	<p class="empty">
		{setupRequired
			? 'Nothing is configured yet. Add a Prowlarr instance below and search will start working.'
			: 'No services are visible to this account.'}
	</p>
{:else}
	<ul class="results">
		{#each rows as row (row.health.id)}
			{@const health = row.health}
			<li class="result">
				<div class="result-main">
					<div class="result-title">
						{health.name}
						<span class={`state state-${health.state.replace(/\s+/g, '-')}`}>{health.state}</span>
					</div>
					<div class="result-meta">
						<span>{health.kind}</span>
						<span>{health.baseUrl}</span>
						<!--
							The sub-path, when there is one. It is not on the health row —
							only GET /api/v1/services carries it — and it is shown because a
							wrong one is otherwise invisible: the address above looks right
							while every request goes to the wrong path.
						-->
						{#if row.instance?.urlBase}
							<span>sub-path {row.instance.urlBase}</span>
						{/if}
						{#if row.instance}
							<span>{row.instance.hasCredential ? 'API key stored' : 'no API key stored'}</span>
						{/if}
						{#if health.appVersion}<span>{health.appVersion}</span>{/if}
						{#if !health.enabled}<span>disabled</span>{/if}
						{#if health.stale}<span>not probed yet</span>{/if}
					</div>

					<!--
						The upstream's own words, never a generic sentence — §17.3 is
						explicit that "an error occurred" is not acceptable here.
					-->
					{#if health.problem}
						<p class="banner-detail">{health.problem}</p>
					{/if}
					{#if health.action}
						<p class="row-note">Suggested fix: {health.action}</p>
					{/if}
					{#each health.warnings as warning, i (i)}
						<p class="banner-detail">
							{warning.source ? `${warning.source}: ` : ''}{warning.message}
						</p>
					{/each}
					{#if health.blockedIndexers.length > 0}
						<p class="row-note">
							{health.blockedIndexers.length}
							{health.blockedIndexers.length === 1 ? 'indexer is' : 'indexers are'} blocked upstream,
							so results from them will be missing.
						</p>
					{/if}

					{#if rowProblem[health.id]}
						<p class="banner-detail">{rowProblem[health.id]}</p>
					{:else if rowMessage[health.id]}
						<p class="row-note">{rowMessage[health.id]}</p>
					{/if}
				</div>

				<div class="result-action">
					<button type="button" onclick={() => runTest(row)} disabled={busyRow === health.id}>
						{busyRow === health.id ? 'Working' : 'Test connection'}
					</button>
					{#if editing === health.id}
						<button type="button" onclick={cancelEdit}>Cancel</button>
					{:else}
						<button type="button" onclick={() => startEdit(row)}>Edit</button>
					{/if}
					{#if removing === health.id}
						<button
							type="button"
							onclick={() => confirmRemove(row)}
							disabled={busyRow === health.id}>Confirm removal</button
						>
						<button type="button" onclick={() => (removing = undefined)}>Keep</button>
					{:else}
						<button type="button" onclick={() => (removing = health.id)}>Remove</button>
					{/if}
				</div>
			</li>

			{#if editing === health.id}
				<li class="result">
					<form class="stack" onsubmit={(event) => saveEdit(event, row)}>
						<div class="field">
							<label for={`name-${health.id}`}>Name</label>
							<input id={`name-${health.id}`} type="text" required bind:value={editName} />
						</div>
						<div class="field">
							<label for={`base-${health.id}`}>Base URL</label>
							<input id={`base-${health.id}`} type="text" required bind:value={editBaseUrl} />
						</div>
						<div class="field">
							<label for={`urlbase-${health.id}`}>Sub-path (optional)</label>
							<input
								id={`urlbase-${health.id}`}
								type="text"
								autocomplete="off"
								placeholder="/prowlarr"
								aria-invalid={editUrlBaseProblem !== '' ? 'true' : undefined}
								bind:value={editUrlBase}
							/>
							{#if editUrlBaseProblem}
								<p class="field-problem" role="alert">{editUrlBaseProblem}</p>
							{/if}
							<p class="field-hint">
								Needed only behind a reverse proxy, for a Prowlarr reachable at
								<code>https://host/prowlarr</code> rather than at the root. It is Prowlarr's own Settings
								→ General → URL Base. Clear it to go back to the root. Changing only the sub-path does
								not require the API key again.
							</p>
						</div>
						<div class="field">
							<label for={`key-${health.id}`}>API key</label>
							<!--
								type=password, and empty on open. The server never returns the
								key, so there is nothing to prefill and nothing to reveal.
							-->
							<input
								id={`key-${health.id}`}
								type="password"
								autocomplete="off"
								placeholder={editNeedsKey
									? 'Required: the address changed'
									: 'Leave blank to keep the stored key'}
								required={editNeedsKey}
								bind:value={editApiKey}
							/>
							<p class="field-hint">
								{#if editNeedsKey}
									This address has a different scheme, host or port from the stored one. The saved
									key is bound to the old address and cannot be moved, so it has to be entered
									again.
								{:else}
									Leave this blank to keep the stored key. UsArr never shows a stored key back.
								{/if}
							</p>
						</div>
						<div class="row-actions">
							<!--
								The label says what the server will actually do. handleUpdateService
								runs the connection test only when the body carries an api_key —
								with no key there is nothing to test WITH, since the stored one is
								never decrypted on a write path, and refusing to save while a
								service is down would block the very edit that fixes it. So a save
								with a key is "Test and save" and a save without one is "Save";
								"Test connection" above is how you check the rest.
							-->
							<button type="submit" disabled={busyRow === health.id}>
								{#if busyRow === health.id}
									Saving
								{:else if editApiKey.trim() !== ''}
									Test and save
								{:else}
									Save
								{/if}
							</button>
							<button type="button" onclick={cancelEdit}>Cancel</button>
						</div>
					</form>
				</li>
			{/if}
		{/each}
	</ul>
{/if}

<h3>Add a service</h3>

{#if addProblem}
	<div class="banner banner-error" role="alert">
		That service was not added.
		<p class="banner-detail">{addProblem}</p>
	</div>
{:else if addMessage}
	<div class="banner">{addMessage}</div>
{/if}

<form class="stack" onsubmit={add}>
	<div class="field">
		<label for="add-name">Name</label>
		<input id="add-name" type="text" autocomplete="off" required bind:value={addName} />
		<p class="field-hint">Whatever you want to call this instance in UsArr.</p>
	</div>
	<div class="field">
		<label for="add-base">Base URL</label>
		<input
			id="add-base"
			type="text"
			autocomplete="off"
			placeholder="http://prowlarr:9696"
			required
			bind:value={addBaseUrl}
		/>
	</div>
	<div class="field">
		<label for="add-urlbase">Sub-path (optional)</label>
		<input
			id="add-urlbase"
			type="text"
			autocomplete="off"
			placeholder="/prowlarr"
			aria-invalid={addUrlBaseProblem !== '' ? 'true' : undefined}
			bind:value={addUrlBase}
		/>
		{#if addUrlBaseProblem}
			<p class="field-problem" role="alert">{addUrlBaseProblem}</p>
		{/if}
		<p class="field-hint">
			Needed only behind a reverse proxy, for a Prowlarr reachable at
			<code>https://host/prowlarr</code> rather than at the root. It is Prowlarr's own Settings → General
			→ URL Base. Leave it blank if the base URL above is the whole address.
		</p>
	</div>
	<div class="field">
		<label for="add-key">API key</label>
		<input id="add-key" type="password" autocomplete="off" required bind:value={addApiKey} />
		<p class="field-hint">
			Prowlarr's Settings → General → API Key. It is stored encrypted and is never shown again — not
			in full and not masked.
		</p>
	</div>
	<div class="row-actions">
		<!--
			Saving runs the connection test server-side and refuses to store a
			credential that fails it, so this button is "check before I commit",
			not the thing that makes the save safe.
		-->
		<button type="submit" disabled={!canAdd}>{addBusy ? 'Working' : 'Test and add'}</button>
		<button type="button" onclick={testBeforeAdd} disabled={!canAdd}>Test connection only</button>
	</div>
	<p class="field-hint">v0.1 talks to Prowlarr. Other services are not wired up yet.</p>
</form>

<style>
	h3 {
		margin: var(--space-7) 0 var(--space-5);
		font-size: var(--text-md);
		line-height: var(--leading-md);
		font-weight: var(--weight-semibold);
	}

	.stack {
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
		max-width: 420px;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	.field label {
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
		color: var(--fg-secondary);
	}

	.field-hint {
		margin: 0;
		color: var(--fg-muted);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}

	.field-problem {
		margin: 0;
		color: var(--status-error);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}

	.row-actions {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
	}

	.row-note {
		margin: var(--space-2) 0 0;
		color: var(--fg-muted);
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
	}

	.state {
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
		color: var(--status-unset);
	}

	.state-healthy {
		color: var(--status-ok);
	}

	.state-degraded {
		color: var(--status-warn);
	}

	.state-down,
	.state-needs-re-identification {
		color: var(--status-error);
	}
</style>
