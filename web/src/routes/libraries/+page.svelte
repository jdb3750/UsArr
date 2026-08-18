<script lang="ts">
	/**
	 * LIBRARIES — ARCHITECTURE.md §17.8, AND IT IS NOT BUILT. The sidebar row is
	 * part of the fixed set; the screen behind it is a later slice.
	 *
	 * ⚠️ THE BLOCKER IS NOT A MISSING SERVICE, AND THIS COPY USED TO SAY IT WAS.
	 * It read "nothing can be configured here until a service exists", which was
	 * measured rendering with two services connected and healthy, so a user who
	 * had just connected one was told to go do the thing they had done.
	 *
	 * What is actually missing is this screen. `internal/httpapi/server.go`
	 * registers no libraries endpoint — `GET /api/v1/library/recent` is Home's
	 * Block C (§17.2, ADR-0028) over a different corpus, not §17.8's binding — so
	 * a control here would have nothing to call.
	 *
	 * And the libraries themselves are NOT waiting on anything. The catalogue
	 * sync already derives one per imported container and binds the container to
	 * it as a source, `managed_by = 'auto'`: `internal/store/catalogue.go`'s
	 * `bindOneContainer`, reached from `internal/libsync/importer.go`. Rows exist
	 * that nothing renders. Naming, joining and the request destination are what
	 * §17.8 adds on top, and none of them ship yet.
	 *
	 * ⚠️ THE COPY BELOW DELIBERATELY DOES NOT SAY "ROWS" OR NAME A COLUMN. A user
	 * cannot open the database and cannot see `managed_by`, so a claim about what
	 * a table holds is one they have no way to check, and an unfalsifiable claim
	 * is the same defect as a false one wearing better clothes. It says the
	 * derivation happens on every sync and that seeing or changing the result is
	 * what is absent, both of which are about behaviour rather than storage.
	 *
	 * Nor does it promise §17.8's Accept step. That conclusion stands; the
	 * reasoning that used to sit under it does not. It read: "`managed_by =
	 * 'auto'` is §17.8's OWN marker for a library the user has not touched, so
	 * the column that would record an accepted proposal is already carrying the
	 * pre-proposal state correctly." ⚠️ FALSIFIED BY THE SCHEMA — that column
	 * records no such thing, and cannot.
	 * `internal/db/migrations/00005_library_sync.sql:565` reads
	 * `managed_by TEXT NOT NULL DEFAULT 'auto' CHECK (managed_by IN ('auto','user'))`:
	 * exactly TWO values, and NEITHER means "proposed". Its own comment defines
	 * 'auto' as created by the proposal flow and still tracking its source, and
	 * 'user' as the user edited it. A library the user accepted and one they have
	 * never been shown are therefore the same row, indistinguishable.
	 *
	 * ⚠️ AND THE ROW EXISTS THE MOMENT IT IS PROPOSED, which inverts §17.8's
	 * safety. If a proposal is already a row, then Accept is a no-op and Decline
	 * is a DELETE — backwards from what a pre-checked confirmation screen
	 * implies, where saying no is supposed to be the cheap direction.
	 *
	 * ✅ THAT DECISION IS NOW TAKEN — SEE ADR-0048. This block used to end by
	 * naming three candidates and picking none. It no longer does, because the
	 * ADR picked one: A LIBRARY PROPOSAL IS NOT A ROW IN `library`. The proposal
	 * set is a value computed by the connect probe from what the instance reports
	 * and what `library_source` already binds; it is never persisted; and a
	 * `library` row is created only when the user accepts one. The other two
	 * candidates are rejected in writing there — a third `managed_by` state
	 * (a `'proposed'` row still satisfies `enabled = 1` and `include_in_search = 1`
	 * on its defaults, so every read that does not name `managed_by` sees it, and
	 * none of them names it; and it would reserve a name in `ux_library_name`
	 * before acceptance, breaking §17.8's specified join-on-existing-name), and
	 * renaming §17.8's step to a review of what already exists (which ratifies the
	 * inversion instead of repairing it).
	 *
	 * SO THE COLUMN IS NOT ASKED TO EXPRESS "proposed" ANY MORE — the state was
	 * removed from the design rather than covered by the schema. `managed_by`
	 * keeps its two values, and no migration follows from the ADR.
	 *
	 * WHAT IS STILL UNBUILT IS UNCHANGED BY ANY OF THIS, and is the reason this
	 * screen is a placeholder: Accept is a SUBTRACTION from a working path, not a
	 * screen bolted onto one. Today's import creates rows unconditionally on a
	 * first connect, so building §17.8 means removing creation from
	 * `bindOneContainer`'s reach on the bootstrap path. The ADR names that trade
	 * and deliberately does not schedule it; it belongs to whoever builds §17.8.
	 */
	import { resolve } from '$app/paths';
</script>

<svelte:head><title>Libraries · UsArr</title></svelte:head>

<div class="section">
	<div class="empty">
		<h2 class="empty__title">Libraries has not been built yet</h2>
		<p class="empty__text">
			This screen is specified in <code class="mono">docs/ARCHITECTURE.md</code> §17.8. A library is a
			name you own over the containers your services already named, together with the destination its
			requests are routed to. UsArr registers no libraries endpoint yet, so there is no list to show you
			here and no field to edit.
		</p>
		<p class="empty__text">
			Connecting a service will not change that. UsArr already derives a library from each container
			a service reports, every time it syncs, so the binding is not what you are waiting on. What
			has never existed is anywhere to see those libraries or change them. Renaming one, joining two
			sources into a single library, and choosing where its requests go are what this screen adds.
		</p>
		<!--
			NOT `btn--primary`. A primary action is the next step, and the old copy's
			next step was "go connect a service" — which is the premise that was
			false. Services is still the right destination, because it is where the
			sources feeding those auto-created libraries live, but it is somewhere to
			go rather than something to do.
		-->
		<div class="empty__actions">
			<a class="btn" href={resolve('/services')}>Services</a>
		</div>
	</div>
</div>
