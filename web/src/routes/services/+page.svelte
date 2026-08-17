<script lang="ts">
	/**
	 * SERVICES — setup and health. ARCHITECTURE.md §17.3, §17.3.1, §17.3.2,
	 * §17.3.3, and §17.7's first-run and error states.
	 *
	 * This is the screen that makes the pluggable claim visible: "if there's ever
	 * an issue somewhere in the pipeline, we should be able to see what's having
	 * issues and fix it", made concrete. §10 puts it plainly — it must be MORE
	 * informative when things are broken than when they are fine, and a wall of
	 * green dots is a failure.
	 *
	 * THE THREE COLUMN CONTRACTS, because they are the rules this screen is most
	 * often got wrong on. Their implementations are in `$lib/services`, which is
	 * DOM-free and unit-tested; what is here is the rendering:
	 *
	 *   Problem   the upstream's own words and NOTHING else. One object per cell:
	 *             the first line of what the upstream said with the full text on
	 *             `title` and, untruncated, in the expander — or `—`. No
	 *             rendering decision, no rebuttal, and never a sentence beginning
	 *             "Nothing is wrong.", which inverts the meaning of the one
	 *             column a user scans for what is broken.
	 *   State     UsArr speaking, in plain language. The verbatim rule stops at
	 *             Problem. The breaker vocabulary is real and it is behind the
	 *             expander.
	 *   Action    the one button that fixes it, or `—`. Never "No action needed".
	 *
	 * A PROBLEM IS STATED CANONICALLY ONCE PER SCREEN. The System status roll-up
	 * below the table is worth keeping and is Sonarr's shape, so it carries the
	 * title and a LINK TO THE ROW rather than a second copy of the row's action.
	 * There is one place the fix is pressed.
	 *
	 * TWO ENDPOINTS, NOT ONE. GET /api/v1/services/health carries the state, the
	 * verbatim problem and the breaker; it does NOT carry `url_base`. Only GET
	 * /api/v1/services does, and a wrong sub-path is otherwise invisible — the
	 * address reads correctly while every request goes to the wrong path. So both
	 * are fetched and joined on id.
	 *
	 * THE THREE 403s ARE TOLD APART ON THE BODY'S `error` FIELD AND NEVER ON THE
	 * STATUS (§17.3.3). `sudo_required` is a prompt that retries the pending
	 * action; `forbidden` is stated plainly with no retry, because retrying
	 * cannot succeed; `csrf` is the only retryable one and $lib/api has already
	 * re-bootstrapped and retried it once before it reaches here, so a second one
	 * is a reload. An unknown code surfaces. Blind-retrying a 403 as a stale CSRF
	 * token is a bug this codebase has already fixed once.
	 *
	 * THE CSP FORBIDS INLINE STYLE ATTRIBUTES, so nothing here writes one. The
	 * list primitive sets its custom properties through the CSSOM; everything
	 * else is a class.
	 */
	import { onMount, tick } from 'svelte';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import {
		ApiError,
		SERVICE_KINDS,
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
		wasAborted,
		type ConnectionTestResult,
		type ServiceHealth
	} from '$lib/api';
	import Icon from '$lib/Icon.svelte';
	import List from '$lib/List.svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { NOTHING, type ListColumn } from '$lib/list';
	import { session } from '$lib/session.svelte';
	import {
		applicationName,
		classify403,
		connectedHeadline,
		itemsCell,
		likelyCauses,
		mechanics,
		normaliseUrlBase,
		problemCell,
		rollup,
		rollupCount,
		stampOf,
		stateLabel,
		syncCell,
		testOutcome,
		testTitle,
		testTone,
		type ServiceRow
	} from '$lib/services';

	/**
	 * The columns, declared. §9.1: `table-layout: auto` has to measure every cell
	 * in every row, which is the one layout mode no containment can help.
	 *
	 * ⚠️ THE ACTION TRACK IS A FIXED RESERVE AND MUST NEVER BE CONTENT-SIZED.
	 * It read `minmax(max-content, auto)`, on the argument that a fixed track
	 * shears the buttons attached to exactly the rows that are broken. ADR-0029
	 * makes EVERY ROW ITS OWN GRID, so that argument buys nothing it claims to:
	 * a content-sized track resolves against the contents of its own row, and
	 * the header row's contents are the word "Action". Measured in Chromium at
	 * 1440 px with the Plex faces served, the action track came out 56 px in the
	 * header against 201 / 194 / 159 / 151 / 136 / 133 / 127 / 34 px in the eight
	 * states a body row can take — so every body row sat 145 to 22 px away from
	 * its own header, and the four tracks left of it drifted 7.66 to 85.11 px
	 * with them.
	 *
	 * THE RESERVE IS MEASURED, NOT CHOSEN. The widest state is `Choose how to
	 * resolve this`, the re-identification row's button, and it measures 177.00 px
	 * with `document.fonts.check('600 13px "IBM Plex Sans"')` true and 215.97 px
	 * with it false — the Plex faces are `font-display: block`, so a build that
	 * cannot serve them renders on fallback metrics FOREVER and the wider number
	 * is the one that has to fit. 215.97 + 2 × --row-pad-x (12 px at all three
	 * densities) = 239.97, plus one --space-4 of headroom rounded up to the next
	 * --space-4 = 248 px; 8 px is about one character at --text-base, so a
	 * server-supplied label one character longer still clears the reserve.
	 *
	 * The remaining states measure 170 / 135 / 127 / 112 / 109 / 103 / 10 px on
	 * Plex, and `Working` (the busy label) is narrower than all of them.
	 *
	 * §9.1's overflow policy survives the change: `.cell-actions` wraps
	 * (app.css), so a cell that ever holds two controls drops the second to a
	 * new line rather than pushing it under `.tablewrap`'s `overflow-x: clip`.
	 * Nothing wraps at this reserve — it is the floor under it, not a layout
	 * anything relies on.
	 */
	const COLUMNS: ListColumn[] = [
		{ id: 'service', header: 'Service', width: 'minmax(0, 1.6fr)' },
		{ id: 'state', header: 'State', width: 'minmax(0, 1.1fr)' },
		{ id: 'sync', header: 'Last successful sync', width: '150px' },
		{ id: 'items', header: 'Items', width: '110px', align: 'end' },
		{ id: 'problem', header: 'Problem', width: 'minmax(0, 1.9fr)' },
		{ id: 'action', header: 'Action', width: '248px' }
	];

	/**
	 * A services row is two lines, not one, so `contain-intrinsic-size` gets a
	 * value that matches. ROW_INTRINSIC's default is measured on a ONE-line row
	 * and would be wrong by half here, which shows up as scroll-height jitter on
	 * a list long enough to virtualise.
	 */
	const ROW_INTRINSIC_SERVICES = 44;

	let rows = $state<ServiceRow[]>([]);
	let loaded = $state(false);
	let loadError = $state('');
	let setupRequired = $state(false);

	/** Re-read on a timer so the relative times stay true rather than freezing. */
	let now = $state(new Date());

	let openRows = new SvelteSet<number>();
	let busyRow = $state<number | undefined>(undefined);
	let testingAll = $state(false);

	/** What the polite live region says. One region, one politeness (§9.4). */
	let announcement = $state('');

	// ── the form ────────────────────────────────────────────────────────────
	//
	// One dialog for add and for edit. Native <dialog> opened with showModal(),
	// so the focus trap, the inert background, Escape and the top layer are the
	// platform's rather than reimplemented (§17.1: standard controls). What is
	// NOT free is §9.4's rule that Escape over unsaved credential input confirms
	// before discarding, and that is handled on the `cancel` event.

	let dialogEl = $state<HTMLDialogElement | null>(null);
	let keyInputEl = $state<HTMLInputElement | null>(null);
	let sudoInputEl = $state<HTMLInputElement | null>(null);

	/** `undefined` = closed, `'add'` = the wizard, a number = editing that id. */
	let editing = $state<'add' | number | undefined>(undefined);

	let fKind = $state<string>(SERVICE_KINDS[0]);
	let fName = $state('');
	let fBaseUrl = $state('');
	let fUrlBase = $state('');
	let fApiKey = $state('');
	let fOriginalBaseUrl = $state('');
	let fUrlBaseProblem = $state('');
	let formProblem = $state('');
	let saving = $state(false);
	let escapeArmed = $state(false);

	/**
	 * The server said `credential_reentry_required`. Held separately from the
	 * form's own comparison because §17.3.2 requires BOTH: a form that only
	 * pre-empts the state turns the server's refusal into an unexplained
	 * failure, and a form that only renders the response makes the user press
	 * Save to find out.
	 */
	let reentryFromServer = $state(false);

	// ── the connection test ─────────────────────────────────────────────────

	type TestPhase = 'idle' | 'testing' | 'done';
	let testPhase = $state<TestPhase>('idle');
	let testResult = $state<ConnectionTestResult | undefined>(undefined);
	let testFailure = $state('');
	let testController: AbortController | undefined;

	// ── the three 403s, and the pending write behind a sudo prompt ───────────

	let pending = $state<(() => Promise<void>) | undefined>(undefined);
	let sudoPassword = $state('');
	let sudoBusy = $state(false);
	let sudoProblem = $state('');
	/** The raw status line, for the `for the record` line. Diagnostic, not the message. */
	let sudoRecord = $state('');
	let forbidden = $state('');
	let csrfStale = $state('');

	// ── removal ─────────────────────────────────────────────────────────────

	let removing = $state<ServiceRow | undefined>(undefined);

	/** Rows whose identity changed. Loud on purpose: this one blocks (§17.7). */
	const reidentRows = $derived(rows.filter((r) => r.health.state === 'needs re-identification'));
	const entries = $derived(rollup(rows, now));
	const entryCount = $derived(rollupCount(entries));

	const originChanged = $derived(
		typeof editing === 'number' && requiresCredentialReentry(fOriginalBaseUrl, fBaseUrl)
	);

	/**
	 * §17.3.2's named state. It clears the moment a key is typed, and it clears
	 * when the original address is put back — the escape hatch for someone who
	 * edited the wrong field.
	 */
	const reentry = $derived(
		typeof editing === 'number' && fApiKey.trim() === '' && (originChanged || reentryFromServer)
	);

	// Restoring the address clears the server's answer too, or the form would go
	// on refusing a save the server would now accept.
	$effect(() => {
		if (!originChanged && fApiKey.trim() !== '') reentryFromServer = false;
	});

	/**
	 * Why Save cannot be pressed, or ''. §17.3's `idle` test state is "Save is
	 * disabled and labelled with the reason", and §9.3 forbids a real `disabled`
	 * — a keyboard user never meets a control that Tab skips, so they get no
	 * signal that a blocked commit exists at all.
	 */
	const saveBlocked = $derived.by(() => {
		if (saving) return 'Saving.';
		if (fName.trim() === '') return 'Give this instance a name.';
		if (fBaseUrl.trim() === '') return 'Enter the base URL.';
		if (reentry) return 'Enter the API key for the new address.';
		if (fUrlBaseProblem !== '') return 'Fix the URL base.';
		if (editing === 'add') {
			if (fApiKey === '') return 'Paste the API key.';
			if (testPhase !== 'done' || testResult?.ok !== true) {
				return 'Run the connection test. Save is enabled once it passes.';
			}
		}
		return '';
	});

	const testBlocked = $derived.by(() => {
		if (testPhase === 'testing') return 'A test is already running.';
		if (fBaseUrl.trim() === '') return 'Enter the base URL.';
		if (reentry) return 'Enter the API key for the new address.';
		if (editing === 'add' && fApiKey === '') return 'Paste the API key.';
		return '';
	});

	const outcome = $derived(
		testResult ? testOutcome(testResult) : testFailure !== '' ? 'failed' : 'connected'
	);

	function describe(error: unknown): string {
		return error instanceof ApiError ? error.detail : String(error);
	}

	/**
	 * Run one write, and branch on WHICH 403 came back.
	 *
	 * `sudo_required` queues the same call behind a password prompt and re-runs
	 * it on success, because making the user find and press the original button
	 * again is the failure mode the state exists to avoid (§17.3.3). It is a
	 * prompt and not an error: UsArr's own words, no verbatim block, and the raw
	 * status line only on the muted `for the record` line.
	 */
	async function guarded(action: () => Promise<void>, what: string): Promise<boolean> {
		try {
			await action();
			return true;
		} catch (error) {
			if (error instanceof ApiError && error.status === 403) {
				switch (classify403(error.code)) {
					case 'sudo':
						pending = action;
						sudoProblem = '';
						sudoPassword = '';
						sudoRecord = `${what} → 403 sudo_required`;
						void focusSudo();
						return false;
					case 'forbidden':
						forbidden = error.detail;
						return false;
					case 'csrf':
						// $lib/api has already re-bootstrapped the token and retried
						// once. A second one is not a token that has rotated.
						csrfStale = error.action || 'Reload the page';
						return false;
					default:
						break;
				}
			}
			throw error;
		}
	}

	async function focusSudo() {
		await tick();
		sudoInputEl?.focus();
	}

	async function load() {
		try {
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

	/**
	 * `#service-<id>` in the URL lands on that instance's row.
	 *
	 * IT IS THE OTHER HALF OF §17.3's "a problem is stated canonically once per
	 * screen". Home's Block B reports what is wrong and links HERE rather than
	 * growing its own copy of the fix, and a link that lands at the top of a
	 * table of six instances has not finished the sentence: the user still has
	 * to find the row Home was talking about. So the fragment names the row and
	 * this focuses it, which is exactly what the in-page roll-up already does
	 * through `focusRow`.
	 *
	 * Focus rather than scroll: a `tabindex="-1"` target is not reliably focused
	 * by a fragment navigation across engines, and moving focus is what puts a
	 * keyboard and screen-reader user on the row too. Awaited after `load()`
	 * because the row does not exist until the health response has rendered.
	 */
	function focusHashRow() {
		const id = Number(page.url.hash.replace(/^#service-/, ''));
		if (!page.url.hash.startsWith('#service-') || !Number.isFinite(id)) return;
		void focusRow(id);
	}

	onMount(() => {
		void load().then(focusHashRow);
		// The table renders from the replica and never blocks on a probe, so this
		// is a cheap re-read of SQLite rather than an upstream call. It also keeps
		// "6 minutes ago" from freezing at whatever it said on arrival.
		const timer = setInterval(() => {
			now = new Date();
			void load();
		}, 60_000);
		return () => clearInterval(timer);
	});

	function isOpen(id: number): boolean {
		return openRows.has(id);
	}

	function toggleRow(id: number) {
		if (!openRows.delete(id)) openRows.add(id);
	}

	function addressOf(row: ServiceRow): string {
		return `${row.health.baseUrl}${row.instance?.urlBase ?? ''}`;
	}

	// ── the form ────────────────────────────────────────────────────────────

	function resetTest() {
		testController?.abort();
		testController = undefined;
		testPhase = 'idle';
		testResult = undefined;
		testFailure = '';
	}

	async function openForm(target: 'add' | ServiceRow, focusKey = false) {
		resetTest();
		formProblem = '';
		fUrlBaseProblem = '';
		reentryFromServer = false;
		escapeArmed = false;
		fApiKey = '';

		if (target === 'add') {
			editing = 'add';
			fKind = SERVICE_KINDS[0];
			fName = '';
			fBaseUrl = '';
			fUrlBase = '';
			fOriginalBaseUrl = '';
		} else {
			editing = target.health.id;
			fKind = target.health.kind;
			fName = target.health.name;
			fBaseUrl = target.health.baseUrl;
			fOriginalBaseUrl = target.health.baseUrl;
			// Only GET /api/v1/services carries the sub-path; the health row does not.
			fUrlBase = target.instance?.urlBase ?? '';
		}

		await tick();
		dialogEl?.showModal();
		if (focusKey) {
			await tick();
			keyInputEl?.focus();
		}
	}

	function closeForm() {
		resetTest();
		editing = undefined;
		fApiKey = '';
		dialogEl?.close();
	}

	/**
	 * §9.4: Escape over unsaved credential input confirms before discarding.
	 * Reflexively hitting Esc to dismiss a password manager's popup should not
	 * silently throw away a pasted 32-character API key.
	 */
	function onDialogCancel(event: Event) {
		if (fApiKey !== '' && !escapeArmed) {
			event.preventDefault();
			escapeArmed = true;
			return;
		}
		closeForm();
	}

	/** §17.3.1: the form repairs what it can and complains only about the rest. */
	function onUrlBaseBlur() {
		const fixed = normaliseUrlBase(fUrlBase);
		fUrlBase = fixed.value;
		fUrlBaseProblem = fixed.problem;
	}

	function formInput() {
		return {
			kind: fKind,
			name: fName,
			baseUrl: fBaseUrl,
			urlBase: fUrlBase,
			apiKey: fApiKey
		};
	}

	function noteReentry(error: unknown): boolean {
		if (!(error instanceof ApiError) || error.code !== 'credential_reentry_required') return false;
		reentryFromServer = true;
		fApiKey = '';
		void (async () => {
			await tick();
			keyInputEl?.focus();
		})();
		return true;
	}

	async function runFormTest() {
		if (testBlocked !== '') return;
		const controller = new AbortController();
		testController = controller;
		testPhase = 'testing';
		testResult = undefined;
		testFailure = '';
		formProblem = '';
		try {
			await guarded(async () => {
				const result =
					editing === 'add'
						? await testNewService(formInput(), controller.signal)
						: await testService(
								editing as number,
								{
									baseUrl: fBaseUrl,
									urlBase: fUrlBase,
									apiKey: fApiKey,
									currentBaseUrl: fOriginalBaseUrl
								},
								controller.signal
							);
				testResult = result;
				testPhase = 'done';
				announcement = testTitle(testOutcome(result));
			}, testPath());
		} catch (error) {
			if (wasAborted(error, controller.signal)) {
				resetTest();
				return;
			}
			if (noteReentry(error)) {
				resetTest();
				return;
			}
			fUrlBaseProblem = urlBaseProblem(error) ?? '';
			if (fUrlBaseProblem === '') {
				testFailure = describe(error);
				testPhase = 'done';
			} else {
				resetTest();
			}
		} finally {
			if (testController === controller) testController = undefined;
			if (testPhase === 'testing') testPhase = 'idle';
		}
	}

	function testPath(): string {
		return editing === 'add'
			? 'POST /api/v1/services/test'
			: `POST /api/v1/services/${String(editing)}/test`;
	}

	function cancelTest() {
		testController?.abort();
		resetTest();
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		if (saveBlocked !== '') return;
		saving = true;
		formProblem = '';
		fUrlBaseProblem = '';
		try {
			if (editing === 'add') {
				const ok = await guarded(async () => {
					const created = await createService(formInput());
					announcement = `${created.name} was added.`;
				}, 'POST /api/v1/services');
				if (!ok) return;
			} else {
				const id = editing as number;
				const ok = await guarded(
					async () => {
						await updateService(id, {
							name: fName,
							baseUrl: fBaseUrl,
							// Present-means-set on PATCH: sending '' is the only way to
							// clear a sub-path once the service is no longer behind a proxy.
							urlBase: fUrlBase,
							apiKey: fApiKey,
							currentBaseUrl: fOriginalBaseUrl
						});
						announcement = `${fName.trim()} was saved.`;
					},
					`PATCH /api/v1/services/${String(id)}`
				);
				if (!ok) return;
			}
			closeForm();
			await load();
		} catch (error) {
			if (noteReentry(error)) return;
			fUrlBaseProblem = urlBaseProblem(error) ?? '';
			if (fUrlBaseProblem === '') formProblem = describe(error);
		} finally {
			saving = false;
		}
	}

	// ── row actions ─────────────────────────────────────────────────────────

	async function runRowTest(row: ServiceRow) {
		const id = row.health.id;
		busyRow = id;
		try {
			await guarded(
				async () => {
					const result = await testService(id);
					announcement = `${row.health.name}: ${testTitle(testOutcome(result))}`;
					await load();
				},
				`POST /api/v1/services/${String(id)}/test`
			);
		} catch (error) {
			loadError = describe(error);
		} finally {
			busyRow = undefined;
		}
	}

	async function testAll() {
		testingAll = true;
		try {
			for (const row of rows) await runRowTest(row);
			announcement = 'Every connection was tested.';
		} finally {
			testingAll = false;
		}
	}

	async function setEnabled(row: ServiceRow, enabled: boolean) {
		const id = row.health.id;
		busyRow = id;
		try {
			await guarded(
				async () => {
					await updateService(id, { enabled });
					await load();
				},
				`PATCH /api/v1/services/${String(id)}`
			);
		} catch (error) {
			loadError = describe(error);
		} finally {
			busyRow = undefined;
		}
	}

	async function confirmRemove(row: ServiceRow) {
		const id = row.health.id;
		busyRow = id;
		try {
			await guarded(
				async () => {
					await deleteService(id);
					announcement = `${row.health.name} was removed.`;
					removing = undefined;
					await load();
				},
				`DELETE /api/v1/services/${String(id)}`
			);
		} catch (error) {
			loadError = describe(error);
		} finally {
			busyRow = undefined;
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
			sudoRecord = '';
			if (action) await action();
			await load();
		} catch (error) {
			// "That password does not match" inline, and the prompt stays open.
			sudoProblem = describe(error);
		} finally {
			sudoBusy = false;
		}
	}

	/**
	 * The one button that fixes this row.
	 *
	 * `health.action` is the SERVER's own name for the fix, so the label is not
	 * invented here; what is decided here is which control that label attaches
	 * to. An action the server names but this build has no control for opens the
	 * expander, where the upstream's own words about it are — which for the one
	 * case that reaches it, a TLS fingerprint, is the only fingerprint
	 * information UsArr holds.
	 */
	type RowAction = { label: string; primary: boolean; run: () => void };

	function actionFor(row: ServiceRow): RowAction | undefined {
		const h = row.health;
		if (h.state === 'needs re-identification') {
			return {
				label: 'Choose how to resolve this',
				primary: true,
				run: () => focusBanner(h.id)
			};
		}
		if (!h.enabled) {
			return { label: 'Turn it back on', primary: false, run: () => void setEnabled(row, true) };
		}
		const action = h.action ?? '';
		if (action === '') return undefined;
		if (action === 'Update API key') {
			return { label: action, primary: true, run: () => void openForm(row, true) };
		}
		if (action === 'Test connection') {
			return { label: action, primary: false, run: () => void runRowTest(row) };
		}
		if (action === 'Enable this service') {
			return { label: action, primary: false, run: () => void setEnabled(row, true) };
		}
		return {
			label: action,
			primary: false,
			run: () => {
				if (!isOpen(h.id)) toggleRow(h.id);
			}
		};
	}

	async function focusBanner(id: number) {
		await tick();
		document.getElementById(`reident-${String(id)}`)?.focus();
	}

	async function focusRow(id: number) {
		await tick();
		document.querySelector<HTMLElement>(`[data-key="${CSS.escape(String(id))}"]`)?.focus();
	}

	/**
	 * What to say where the *Arr's own warnings would go when there are none.
	 *
	 * "None reported" is a claim about what the instance said, so it may only be
	 * made when the instance actually answered. An unreachable or disabled
	 * instance reported nothing because it was never asked, and drawing that as a
	 * clean bill of health is the same failure as a green tick on a row nothing
	 * has probed.
	 */
	function warningsOf(health: ServiceHealth): string {
		if (health.warnings.length > 0) return '';
		if (health.stale) return 'Not readable: no probe has run yet.';
		if (!health.enabled) return 'Not readable: UsArr is not contacting this instance.';
		if ((health.problem ?? '') !== '') return 'Not readable while the instance is not answering.';
		return 'None reported';
	}
</script>

<svelte:head><title>Services · UsArr</title></svelte:head>

<!--
	NO <h2>Services</h2> HERE, DELIBERATELY. The toolbar already renders the page
	name and the layout renders it as the h1 inside <main>. A third copy costs a
	density row and makes the heading structure claim a section boundary that does
	not exist: a screen-reader user navigating by heading lands on "Services"
	twice and learns nothing the second time.
-->
<div class="pagehead">
	<span class="pagehead__meta">
		{#if !loaded}
			Reading the configured services.
		{:else if rows.length === 0}
			No service is connected yet.
		{:else}
			{rows.length}
			{rows.length === 1 ? 'service' : 'services'}{entryCount
				? `. ${entryCount}`
				: '. All healthy'}. Health is probed per instance, never globally.
		{/if}
	</span>
</div>

{#if loadError}
	<div class="section">
		<div class="banner banner--err" role="alert">
			<Icon name="x-circle" />
			<div class="banner__body">
				<div class="banner__title">The configured services could not be read</div>
				<p class="verbatim">{loadError}</p>
			</div>
		</div>
	</div>
{/if}

<!--
	§17.3.3's re-authentication state. It is a PROMPT, not an error: UsArr's own
	words, no verbatim block, and the raw status line only on the muted `for the
	record` line beneath the body. Rendered here when no dialog is open and
	inside the dialog when one is, because a modal in the top layer would
	otherwise hide the one control that unblocks it.
-->
{#snippet authNotice()}
	{#if pending}
		<div class="banner banner--warn" role="alert">
			<Icon name="alert" />
			<div class="banner__body">
				<div class="banner__title">Confirm your password to change a service</div>
				<p class="banner__text">
					Changing a service credential needs a password confirmation from the last five minutes,
					and this session's has expired. Nothing is wrong and nothing was lost: the change you made
					is still in the form.
				</p>
				<form class="form sudo-form" onsubmit={submitSudo}>
					<div class="field">
						<label class="field__label" for="sudo-password">Password</label>
						<input
							bind:this={sudoInputEl}
							bind:value={sudoPassword}
							class="input"
							id="sudo-password"
							type="password"
							autocomplete="current-password"
							aria-invalid={sudoProblem !== '' ? 'true' : undefined}
							aria-describedby={sudoProblem !== '' ? 'sudo-problem' : undefined}
						/>
						{#if sudoProblem}
							<span class="field__err" id="sudo-problem">
								<Icon name="alert" size="sm" /><span class="sr">Error:</span>{sudoProblem}
							</span>
						{/if}
					</div>
					<div class="form__row">
						<button type="submit" class="btn btn--primary" disabled={sudoBusy}>
							{sudoBusy ? 'Confirming' : 'Confirm'}
						</button>
						<button
							type="button"
							class="btn"
							onclick={() => {
								pending = undefined;
								sudoRecord = '';
							}}>Cancel</button
						>
					</div>
				</form>
				{#if sudoRecord}
					<p class="cell-sub for-the-record">
						The server's answer, for the record: <span class="mono">{sudoRecord}</span>
					</p>
				{/if}
			</div>
		</div>
	{/if}

	{#if forbidden}
		<div class="banner banner--err" role="alert">
			<Icon name="x-circle" />
			<div class="banner__body">
				<div class="banner__title">This account may not change services</div>
				<p class="banner__text">
					There is no retry, because retrying cannot succeed. This is not the sudo window closing
					and it is not a stale page token.
				</p>
				<p class="verbatim">{forbidden}</p>
			</div>
			<div class="banner__actions">
				<button type="button" class="btn" onclick={() => (forbidden = '')}>Dismiss</button>
			</div>
		</div>
	{/if}

	{#if csrfStale}
		<div class="banner banner--warn" role="alert">
			<Icon name="alert" />
			<div class="banner__body">
				<div class="banner__title">Reload the page</div>
				<p class="banner__text">
					This page's request token no longer matches the one the server issued. UsArr fetched a
					fresh token and tried once more, and the second attempt was refused too, so a reload is
					what fixes it. Nothing was changed.
				</p>
			</div>
			<div class="banner__actions">
				<button type="button" class="btn btn--primary" onclick={() => location.reload()}>
					Reload the page
				</button>
			</div>
		</div>
	{/if}
{/snippet}

{#if !editing}
	<div class="section">{@render authNotice()}</div>
{/if}

<!--
	NEEDS RE-IDENTIFICATION — a blocking banner, loud on purpose, because §7.4's
	guard 2 exists precisely to stop a silent library-destroying sweep.

	TWO BRANCHES, NOT ONE. The banner's own copy used to say "Re-link only if you
	know this is the same library" and then offer `Re-link` and nothing else — so
	a user who knows it is NOT the same instance was told to stop and given no way
	to proceed. §17.3 calls this the most destructive decision in the product and
	records the one-way prompt as a finding.

	⚠️ AND THE SAFE BRANCH IS THE ONE THIS BUILD CANNOT PERFORM. There is no
	re-link endpoint: the HTTP surface is create, list, health, test, update and
	delete. `needs_reidentification` is written by §7.4 guard 2, which belongs to
	the reconciliation sweep — channel 4, and `internal/libsync/doc.go` names it
	as NOT here, so nothing sets the state today either. Do not read that as "the
	sync engine has not shipped": channel 1 imports a catalogue and Home renders
	it. The button is therefore aria-disabled and says why (§9.3: never
	`disabled`, or a keyboard user never meets the blocked control) — and a third,
	genuinely safe control is offered so the only live button on a destructive
	prompt is not the destructive one (§9.4).
-->
{#each reidentRows as row (row.health.id)}
	{@const app = applicationName(row.health.kind)}
	<div class="section">
		<div
			class="banner banner--err"
			role="alert"
			id={`reident-${String(row.health.id)}`}
			tabindex="-1"
		>
			<Icon name="x-circle" />
			<div class="banner__body">
				<div class="banner__title">
					{row.health.name} may be a different {app} from the one UsArr synced with
				</div>
				<p class="banner__text">
					Sync for this instance is paused deliberately. Re-link only if you know this is the same
					library. If it is not the same instance, removing it and adding the new one keeps every
					work UsArr already holds and attaches the new instance as a separate source, so nothing
					you named or corrected is lost either way.
				</p>
				{#if row.health.problem}
					<p class="verbatim">{row.health.problem}</p>
				{/if}
				<p class="cell-sub for-the-record" id={`reident-why-${String(row.health.id)}`}>
					Re-linking is not built yet, so this build can only remove and re-add.
				</p>
			</div>
			<div class="banner__actions">
				<button type="button" class="btn">Leave it paused</button>
				<button
					type="button"
					class="btn btn--primary"
					aria-disabled="true"
					aria-describedby={`reident-why-${String(row.health.id)}`}
					onclick={(event) => event.preventDefault()}
				>
					Re-link — this is the same {app}
				</button>
				<button type="button" class="btn" onclick={() => (removing = row)}>
					Not the same instance — remove and re-add
				</button>
			</div>
		</div>
	</div>
{/each}

<div class="toolbar">
	<button type="button" class="btn btn--primary" onclick={() => void openForm('add')}>
		Add service
	</button>
	{#if rows.length > 0}
		<button
			type="button"
			class="btn"
			aria-disabled={testingAll ? 'true' : undefined}
			onclick={() => {
				if (!testingAll) void testAll();
			}}
		>
			{testingAll ? 'Testing' : 'Test all connections'}
		</button>
	{/if}
	<span class="toolbar__spacer"></span>
	<span class="toolbar__label">
		The table renders from the replica and never blocks on a probe.
	</span>
</div>

<List
	label="Configured services"
	columns={COLUMNS}
	{rows}
	key={(row) => String(row.health.id)}
	total={rows.length}
	state={loaded ? (rows.length === 0 ? 'empty' : 'default') : 'loading'}
	loadingNote="Reading the configured services."
	rowIntrinsic={ROW_INTRINSIC_SERVICES}
	class="tbl--services"
	emptyTitle={setupRequired ? 'No services configured' : 'No services are visible to this account'}
	emptyText={setupRequired
		? 'Adding a service takes four things: which application it is, a name for it, its base URL and an API key.'
		: 'Nothing was returned for this account, which is not the same as nothing being configured.'}
	expanded={(row) => isOpen(row.health.id)}
	{rowExtra}
	{cell}
	{emptyActions}
/>

{#snippet emptyActions()}
	<button type="button" class="btn btn--primary" onclick={() => void openForm('add')}>
		Add service
	</button>
{/snippet}

{#snippet cell(row: ServiceRow, column: ListColumn)}
	{@const h = row.health}
	{#if column.id === 'service'}
		<div class="cell-title">
			<button
				type="button"
				class="btn btn--icon btn--sm"
				aria-expanded={isOpen(h.id)}
				aria-controls={`svc-detail-${String(h.id)}`}
				aria-label={`${isOpen(h.id) ? 'Hide' : 'Show'} detail for ${h.name}`}
				onclick={() => toggleRow(h.id)}
			>
				<Icon name={isOpen(h.id) ? 'chevron-down' : 'chevron-right'} size="sm" />
			</button>
			{h.name}
			<span class="chip">{h.kind}</span>
		</div>
		<!--
			base_url AND url_base. The health row carries only the first, so a wrong
			sub-path is otherwise invisible: the address reads correctly while every
			request goes to the wrong path.
		-->
		<div class="cell-sub mono trunc" title={addressOf(row)}>{addressOf(row)}</div>
	{:else if column.id === 'state'}
		{@const label = stateLabel(row, now)}
		<span class={`st st--${label.tone}`}>
			<Icon name={label.icon} size="sm" />{label.word}
		</span>
		{#if label.detail}<div class="cell-sub">{label.detail}</div>{/if}
	{:else if column.id === 'sync'}
		{@const sync = syncCell(row)}
		<span class:muted={sync.muted}>{sync.text}</span>
		{#if sync.sub}<div class="cell-sub">{sync.sub}</div>{/if}
	{:else if column.id === 'items'}
		{@const items = itemsCell(row)}
		<span class:muted={items.muted}>{items.text}</span>
	{:else if column.id === 'problem'}
		{@const problem = problemCell(row)}
		{#if problem.muted}
			<span class="muted">{problem.text}</span>
		{:else}
			<div class="cell-problem">
				<span class="prob trunc" title={h.problem}>{problem.text}</span>
				<button
					type="button"
					class="linkish"
					aria-expanded={isOpen(h.id)}
					aria-controls={`svc-detail-${String(h.id)}`}
					onclick={() => toggleRow(h.id)}
				>
					{isOpen(h.id) ? 'Hide detail' : 'Show detail'}
				</button>
			</div>
		{/if}
	{:else if column.id === 'action'}
		{@const action = actionFor(row)}
		{#if action}
			<div class="cell-actions">
				<button
					type="button"
					class={action.primary ? 'btn btn--primary' : 'btn'}
					aria-disabled={busyRow === h.id ? 'true' : undefined}
					onclick={() => {
						if (busyRow !== h.id) action.run();
					}}
				>
					{busyRow === h.id ? 'Working' : action.label}
				</button>
			</div>
		{:else}
			<span class="muted">{NOTHING.empty}</span>
		{/if}
	{/if}
{/snippet}

{#snippet rowExtra(row: ServiceRow)}
	{@const h = row.health}
	<div id={`svc-detail-${String(h.id)}`}>
		{#if h.problem}
			<!-- Verbatim, untruncated, in mono. This is the one place the whole
			     upstream body lives; the cell above carries its first line. -->
			<p class="verbatim">{h.problem}</p>
		{/if}
		<div class="detail">
			<div>
				<h4>Probe and retry</h4>
				<ul>
					{#each mechanics(row, now) as part, i (i)}
						<li>{part}</li>
					{/each}
				</ul>
			</div>
			<div>
				<h4>Health warnings from {h.name}</h4>
				<ul>
					{#if warningsOf(h)}
						<li class="muted">{warningsOf(h)}</li>
					{/if}
					{#each h.warnings as warning, i (i)}
						<li>
							<span class="st st--warn"><Icon name="alert" size="sm" />warning</span>
							{warning.source ? `${warning.source}: ` : ''}{warning.message}
						</li>
					{/each}
				</ul>
			</div>
			{#if h.blockedIndexers.length > 0}
				<div>
					<h4>Indexers {h.name} is refusing to query</h4>
					<ul>
						{#each h.blockedIndexers as blocked (blocked.indexerId)}
							<li>
								{blocked.name ?? `indexer ${String(blocked.indexerId)}`}
								{#if blocked.disabledTill}
									<span class="muted">until {stampOf(blocked.disabledTill, now)}</span>
								{/if}
							</li>
						{/each}
					</ul>
				</div>
			{/if}
			<div>
				<h4>Instance</h4>
				<ul>
					<li>Kind <span class="mono">{h.kind}</span></li>
					<li>Role <span class="mono">{h.role || 'unknown'}</span></li>
					{#if h.appVersion}<li>Version <span class="mono">{h.appVersion}</span></li>{/if}
					<li>
						Credential
						<span class="muted">
							{row.instance?.hasCredential ? 'stored, encrypted' : 'none stored'}
						</span>
					</li>
					{#if h.lastOkAt}
						<li>Last answered <span class="num">{stampOf(h.lastOkAt, now)}</span></li>
					{:else}
						<li class="muted">It has never answered</li>
					{/if}
				</ul>
			</div>
		</div>
		<div class="cell-actions detail-actions">
			<button type="button" class="btn" onclick={() => void openForm(row)}>Edit</button>
			<button type="button" class="btn" onclick={() => void runRowTest(row)}>Test connection</button
			>
			{#if h.enabled}
				<button type="button" class="btn" onclick={() => void setEnabled(row, false)}>
					Turn it off
				</button>
			{/if}
			<button type="button" class="btn" onclick={() => (removing = row)}>Remove</button>
		</div>
	</div>
{/snippet}

{#if loaded && rows.length === 0 && setupRequired}
	<!--
		§9.6 rule 4: the empty state itself is one sentence, and the extra
		explanation the first-run case genuinely needs goes BELOW the buttons at
		--text-base rather than as a wider centred measure above them.
	-->
	<div class="section">
		<p class="note">
			The connection is tested before anything is saved, and a service that fails its test is never
			stored. v0.1 connects to Prowlarr; the other services are not wired up yet.
		</p>
	</div>
{/if}

<!--
	THE ROLL-UP. Sonarr's shape — a list of named problems, each `severity ·
	message · fix` — and §17.3's amendment to it: the second rendering LINKS TO
	THE ROW rather than duplicating its action, so a problem is stated canonically
	once per screen and there is one place the fix is pressed.
-->
{#if entries.length > 0}
	<section class="section" id="services-system">
		<div class="section__head">
			<h2>System status</h2>
			<span class="section__count">{entryCount}</span>
		</div>
		<ul>
			{#each entries as entry (entry.id)}
				<li class={`banner rollup ${entry.tone === 'err' ? 'banner--err' : 'banner--warn'}`}>
					<Icon name={entry.tone === 'err' ? 'x-circle' : 'alert'} />
					<div class="banner__body">
						<div class="banner__title">{entry.title}</div>
					</div>
					<div class="banner__actions">
						<button type="button" class="btn btn--sm" onclick={() => void focusRow(entry.id)}>
							Go to the {entry.name} row
						</button>
					</div>
				</li>
			{/each}
		</ul>
	</section>
{/if}

<!--
	§17.3: Services and Libraries are TOP-LEVEL SCREENS and are not also items
	inside a Settings navigation. Both statements shipped on one screen at once —
	the sidebar marking Services `aria-current` and an in-page nav listing it
	again — so two navigation trees claimed one node with two different parents.

	⚠️ THE NAV ITSELF IS A SENTENCE RATHER THAN A LIST OF LINKS, and that is a
	deliberate honesty call rather than the rule being skipped. §17.3 names its
	three entries as `General · Tags · UI`; of the three only UI exists, as the
	appearance and keyboard preferences on /settings. Drawing `General` and
	`Tags` as nav rows would put two links to screens that do not exist on the
	screen whose whole job is saying what is and is not connected.
-->
<section class="section" id="services-settings">
	<div class="section__head"><h2>Settings</h2></div>
	<p class="note">
		Services and Libraries are top-level screens, not sections of Settings. Settings holds General,
		Tags and UI, and of those only <a href={resolve('/settings')}>UI</a> has shipped.
	</p>
</section>

<!--
	REMOVAL. §9.4: a destructive confirmation always offers the safe option as a
	CONTROL rather than as an escape, and the safe option takes focus.
-->
{#if removing}
	{@const target = removing}
	<div class="section">
		<div class="banner banner--err" role="alert">
			<Icon name="x-circle" />
			<div class="banner__body">
				<div class="banner__title">Remove {target.health.name}?</div>
				<p class="banner__text">
					Removing it deletes the stored credential and leaves UsArr with nothing to search through.
					Nothing is removed from {applicationName(target.health.kind)} itself, and no catalogue is lost,
					because an indexer contributes none.
				</p>
			</div>
			<div class="banner__actions">
				<!-- svelte-ignore a11y_autofocus -->
				<button
					type="button"
					class="btn btn--primary"
					autofocus
					onclick={() => (removing = undefined)}
				>
					Keep {target.health.name}
				</button>
				<button
					type="button"
					class="btn"
					aria-disabled={busyRow === target.health.id ? 'true' : undefined}
					onclick={() => {
						if (busyRow !== target.health.id) void confirmRemove(target);
					}}
				>
					{busyRow === target.health.id ? 'Removing' : 'Remove anyway'}
				</button>
			</div>
		</div>
	</div>
{/if}

<!--
	THE ADD AND EDIT FORM. §17.7's "three fields" is corrected to four — kind,
	name, base URL, API key — plus `URL base` as an optional fifth which is
	NOT behind a Show advanced toggle (§17.3.1): a reverse-proxy sub-path is how
	a large share of this audience reaches its *Arrs and it is the single most
	likely reason a first connection test fails, so hiding it would mean not
	having it.
-->
<dialog
	bind:this={dialogEl}
	class="dialog"
	aria-labelledby="service-form-title"
	oncancel={onDialogCancel}
>
	<h2 class="dialog__head" id="service-form-title">
		{editing === 'add' ? 'Add service' : `Edit ${fName || 'service'}`}
	</h2>
	<div class="dialog__body">
		{@render authNotice()}

		{#if reentry}
			<!--
				§17.3.2's named form state. NOT the `error` state of §10: nothing was
				sent upstream, so there is no verbatim text and inventing one would be
				a fabrication. It is UsArr's own words about a rule UsArr enforces.
			-->
			<div class="banner banner--warn" role="alert">
				<Icon name="alert" />
				<div class="banner__body">
					<div class="banner__title">The stored API key does not move to a new address</div>
					<p class="banner__text">
						This key is stored encrypted against <span class="mono">{fOriginalBaseUrl}</span>, so it
						cannot be sent to <span class="mono">{fBaseUrl}</span>. Paste the key for the new
						address, or put the old address back. The stored key is not deleted until you save.
						Changing only the URL base is not a new address and clears nothing.
					</p>
					{#if reentryFromServer}
						<p class="cell-sub for-the-record">
							The server's answer, for the record:
							<span class="mono">400 credential_reentry_required</span>
						</p>
					{/if}
				</div>
			</div>
		{/if}

		{#if formProblem}
			<div class="banner banner--err" role="alert">
				<Icon name="x-circle" />
				<div class="banner__body">
					<div class="banner__title">That change was not saved</div>
					<p class="verbatim">{formProblem}</p>
				</div>
			</div>
		{/if}

		<form class="form" id="service-form" onsubmit={save} novalidate>
			<div class="field">
				<label class="field__label" for="f-kind">Kind</label>
				<select class="select" id="f-kind" bind:value={fKind} disabled={editing !== 'add'}>
					{#each SERVICE_KINDS as kind (kind)}
						<option value={kind}>{kind}</option>
					{/each}
				</select>
				<span class="field__help">
					Which application this is. v0.1 binds <span class="mono">prowlarr</span>, which uses
					<span class="mono">/api/v1</span> — the API version does not track the application
					version, and it is probed rather than assumed — and
					<span class="mono">kavita</span>, whose paths carry no version segment at all and whose
					credential is an Auth Key from User Settings → Manage Auth Keys.
				</span>
			</div>

			<div class="field">
				<label class="field__label" for="f-name">Name</label>
				<input class="input" id="f-name" type="text" autocomplete="off" bind:value={fName} />
				<span class="field__help">
					Your label for this instance, shown everywhere in UsArr. It must be unique, because it is
					how you tell two instances of the same application apart.
				</span>
			</div>

			<div class="field">
				<label class="field__label" for="f-url">Base URL</label>
				<input
					class="input"
					id="f-url"
					type="text"
					autocomplete="off"
					placeholder="http://10.0.0.4:9696"
					bind:value={fBaseUrl}
					aria-describedby="f-url-help"
				/>
				<span class="field__help" id="f-url-help">
					The address <em>this host</em> can reach, which is not always the one your browser uses.
					Scheme, host and port only; a path goes in <strong>URL base</strong> below. Changing any of
					the three clears the API key, because the key is encrypted against this address and cannot be
					opened for another one.
				</span>
			</div>

			<div class="field">
				<label class="field__label" for="f-urlbase"
					>URL base <span class="muted">optional</span></label
				>
				<input
					class="input"
					id="f-urlbase"
					type="text"
					autocomplete="off"
					placeholder="/prowlarr"
					bind:value={fUrlBase}
					onblur={onUrlBaseBlur}
					aria-invalid={fUrlBaseProblem !== '' ? 'true' : undefined}
					aria-describedby={fUrlBaseProblem !== '' ? 'f-urlbase-err' : 'f-urlbase-help'}
				/>
				{#if fUrlBaseProblem}
					<span class="field__err" id="f-urlbase-err">
						<Icon name="alert" size="sm" /><span class="sr">Error:</span>{fUrlBaseProblem}
					</span>
				{/if}
				<span class="field__help" id="f-urlbase-help">
					Leave this empty unless a reverse proxy serves the application under a path. If you reach
					Prowlarr at <span class="mono">https://home.tailnet.ts.net/prowlarr</span>, the base URL
					is <span class="mono">https://home.tailnet.ts.net</span> and the URL base is
					<span class="mono">/prowlarr</span>. It is the same value the application calls
					<span class="mono">urlBase</span> in its own settings. A leading slash is added for you
					and a trailing one is trimmed. Changing it does <em>not</em> clear the API key.
				</span>
			</div>

			<div class="field">
				<label class="field__label" for="f-apikey">API key</label>
				<!--
					type=password, and empty on open. The server never returns the key —
					not in full and not masked — so there is nothing to prefill and
					nothing to reveal.
				-->
				<input
					bind:this={keyInputEl}
					bind:value={fApiKey}
					class="input"
					id="f-apikey"
					type="password"
					autocomplete="off"
					placeholder={reentry
						? 'Re-enter the API key'
						: editing === 'add'
							? "Paste the key from the application's General settings"
							: 'Leave blank to keep the stored key'}
					aria-invalid={reentry ? 'true' : undefined}
					aria-describedby="f-apikey-help"
				/>
				<span class="field__help" id="f-apikey-help">
					Stored encrypted and never sent back to the browser. An *Arr API key is a full-admin
					credential.
				</span>
				{#if escapeArmed}
					<span class="field__err" role="alert">
						<Icon name="alert" size="sm" /><span class="sr">Warning:</span>Escape will discard the
						key you pasted. Press Escape again to close anyway, or Cancel.
					</span>
				{/if}
			</div>

			<!--
				THE CONNECTION TEST'S RESULT, in all four states (§17.3).
				`idle` shows nothing at all — Save carries the reason instead.
			-->
			{#if testPhase === 'testing'}
				<div class="testresult" role="status">
					<div class="testresult__title">Testing the connection</div>
					<p>
						UsArr is calling {fBaseUrl}{fUrlBase} from the host it runs on. The request has its own timeout,
						so this state is bounded.
					</p>
					<div class="form__row">
						<button type="button" class="btn btn--sm" onclick={cancelTest}>Cancel the test</button>
					</div>
				</div>
			{:else if testPhase === 'done' && testFailure !== ''}
				<div class="testresult testresult--err" role="status">
					<div class="testresult__title">
						<Icon name="x-circle" size="sm" />Could not connect
					</div>
					<p>Nothing was saved, and no credential went anywhere except to the address above.</p>
					<p class="verbatim">{testFailure}</p>
					<p>The most likely causes, in the order they happen:</p>
					<ul>
						{#each likelyCauses(testFailure, fKind) as cause, i (i)}
							<li>{cause}</li>
						{/each}
					</ul>
				</div>
			{:else if testPhase === 'done' && testResult}
				<div class={`testresult testresult--${testTone(outcome)}`} role="status">
					<div class="testresult__title">
						<Icon
							name={outcome === 'connected'
								? 'check'
								: outcome === 'reachable'
									? 'alert'
									: 'x-circle'}
							size="sm"
						/>{testTitle(outcome)}
					</div>
					{#if outcome === 'failed'}
						<p>Nothing was saved, and no credential went anywhere except to the address above.</p>
						<p class="verbatim">{testResult.message}</p>
						<p>The most likely causes, in the order they happen:</p>
						<ul>
							{#each likelyCauses(testResult.message, fKind) as cause, i (i)}
								<li>{cause}</li>
							{/each}
						</ul>
					{:else}
						<p>
							{connectedHeadline(testResult.appName, testResult.appVersion, testResult.apiVersion)}.
							{#if outcome === 'connected'}
								Naming what actually answered is the only thing that catches a URL pasted under the
								wrong kind.
							{/if}
						</p>
						{#if outcome === 'reachable'}
							<!-- The server's own wording for this case, used as it stands.
							     A cheerier one would claim a verification that did not happen. -->
							<p class="verbatim">{testResult.message}</p>
						{/if}
						<ul>
							<li>
								An indexer contributes no catalogue items, so there is no count to report here.
							</li>
							{#if outcome === 'connected'}
								<li>Save is enabled.</li>
							{:else}
								<li>
									Save is enabled, because the host answered. The key is still unverified and a
									wrong one will surface as a health failure on this screen.
								</li>
							{/if}
						</ul>
					{/if}
				</div>
			{/if}
		</form>
	</div>

	<div class="dialog__foot">
		<button type="button" class="btn" onclick={closeForm}>Cancel</button>
		<button
			type="button"
			class="btn"
			aria-disabled={testBlocked !== '' ? 'true' : undefined}
			aria-describedby={testBlocked !== '' ? 'form-blocked' : undefined}
			onclick={() => void runFormTest()}
		>
			Test connection
		</button>
		<button
			type="submit"
			form="service-form"
			class="btn btn--primary"
			aria-disabled={saveBlocked !== '' ? 'true' : undefined}
			aria-describedby={saveBlocked !== '' ? 'form-blocked' : undefined}
		>
			{saving ? 'Saving' : 'Save'}
		</button>
		{#if saveBlocked || testBlocked}
			<span class="muted form-blocked" id="form-blocked">{saveBlocked || testBlocked}</span>
		{/if}
	</div>
</dialog>

<div class="sr" role="status" aria-live="polite" aria-atomic="true">{announcement}</div>

<style>
	/*
	 * Screen-only rules. Anything with a design-system name lives in app.css;
	 * what is here belongs to this screen and to nothing else.
	 */

	/* The muted `for the record` line §17.3.3 puts the raw status on: diagnostic,
	 * beneath the body, and never the message itself. */
	.for-the-record {
		margin-top: var(--space-3);
	}

	/* The sudo prompt is a form inside a banner, so it does not take the form's
	 * own 560px measure or its inter-field gap. */
	.sudo-form {
		margin-top: var(--space-4);
		gap: var(--space-4);
		max-width: 320px;
	}

	/* The roll-up's entries are list items, and the banner's page gutter would
	 * double up with the section's. */
	.rollup {
		margin-left: 0;
		margin-right: 0;
	}

	/* The expander's own actions sit under its fact lists rather than beside
	 * them, so a narrow viewport wraps them instead of shearing the row. */
	.detail-actions {
		flex-wrap: wrap;
		padding-top: var(--space-3);
		border-top: 1px solid var(--border);
	}

	/* The blocked reason takes its own line at the end of the footer rather than
	 * squeezing between two buttons and breaking their wrap. */
	.form-blocked {
		flex: 1 0 100%;
		font-size: var(--text-sm);
		line-height: var(--leading-sm);
		text-align: right;
	}
</style>
