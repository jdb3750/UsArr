/**
 * THE ONE SSE CONNECTION, SHARED, AND THE IMPORT PROGRESS CARRIED ON IT.
 *
 * # Why this is a module and not three lines inside the Services screen
 *
 * `openEventStream` builds an `EventSource`, and an `EventSource` is a socket.
 * The Requests screen opens one in its own `onMount` and closes it on unmount,
 * which is correct there and is exactly the shape that does not survive a second
 * caller: two screens each opening their own would be two connections, two
 * replay cursors and two heartbeats against one server.
 *
 * So the connection lives here, REFERENCE COUNTED. `subscribe()` opens it on the
 * first caller and returns the function that releases it; the last release
 * closes it. A screen mounting, unmounting and mounting again gets one socket at
 * a time and never two, and a second screen subscribing while the first is up
 * gets the socket that already exists.
 *
 * ⚠️ THIS IS DELIBERATELY MORE THAN THE IMPORT NEEDS. §17's steward ruled that
 * the Libraries screen should not subscribe FOR NOW, and named the revisit
 * trigger as this plumbing landing somewhere shared: a screen that wants the
 * same frames adds `subscribe((event) => ...)` and nothing else. Libraries is
 * NOT subscribed here, on purpose. It is only made cheap to.
 *
 * # What it remembers, and what it refuses to conclude
 *
 * The import state is one `ImportProgress` per `instance_id`: the LAST frame
 * that arrived for it, not an accumulation. Every frame carries the running
 * counts already, so keeping the latest is the whole state, and keeping a
 * derived total would be inventing a number the server can send itself.
 *
 * ⚠️ NOTHING HERE EVER CONCLUDES THAT AN IMPORT FINISHED. `phase === 'done'` is
 * a fact the server sent; a stream that goes quiet, a count that reaches a
 * total, and a socket that drops are all silence, and silence is what a FAILED
 * import looks like as well (internal/libsync/importer.go returns before the
 * terminal publish on every error path). `connected` is exported beside the
 * counts for exactly that reason: a reader that cannot see the connection state
 * cannot tell a stalled readout from a live one.
 */
import { openEventStream, type ImportProgress, type StreamEvent, type StreamHandles } from './api';
import { SvelteMap } from 'svelte/reactivity';

/** A listener gets every frame this client understands, in arrival order. */
export type StreamListener = (event: StreamEvent) => void;

class ImportStream {
	/**
	 * Whether the socket is up RIGHT NOW.
	 *
	 * False before the first `open`, false the instant an error fires, true
	 * again when the browser's own retry reconnects. It is not a health verdict
	 * and it is not sticky: it is the answer to "may the numbers below be
	 * believed as current", and there is no other answer available, because the
	 * SSE spec gives the error event no status at all.
	 */
	connected = $state(false);

	#handles: StreamHandles | undefined;
	#refs = 0;
	#listeners = new Set<StreamListener>();
	#progress = new SvelteMap<number, ImportProgress>();

	/**
	 * Join the shared stream. The returned function is the release, and calling
	 * it twice releases once: a component's cleanup can run after a hot reload
	 * has already torn the tree down, and a double release would close a socket
	 * another screen still holds.
	 */
	subscribe(listener?: StreamListener): () => void {
		if (listener) this.#listeners.add(listener);
		this.#refs += 1;
		if (!this.#handles) {
			this.#handles = openEventStream(
				(event) => this.#receive(event),
				(connected) => (this.connected = connected)
			);
		}
		let released = false;
		return () => {
			if (released) return;
			released = true;
			if (listener) this.#listeners.delete(listener);
			this.#refs -= 1;
			if (this.#refs > 0) return;
			this.#refs = 0;
			this.#handles?.close();
			this.#handles = undefined;
			this.connected = false;
		};
	}

	/** The last frame that arrived for this instance, or undefined for none. */
	progressFor(instanceId: number): ImportProgress | undefined {
		return this.#progress.get(instanceId);
	}

	/**
	 * Drop what is remembered for one instance.
	 *
	 * ⚠️ A PRESS MUST CALL THIS, and the reason is the terminal frame. A `done`
	 * left over from the previous import would be rendered against the new one
	 * as "finished" before the new one has read anything, which is the single
	 * most misleading thing this module could say.
	 */
	forget(instanceId: number): void {
		this.#progress.delete(instanceId);
	}

	#receive(event: StreamEvent): void {
		if (event.kind === 'import') {
			this.#progress.set(event.progress.instanceId, event.progress);
		}
		for (const listener of this.#listeners) listener(event);
	}
}

/**
 * The singleton. One module instance per page load, so the reference count and
 * the socket are process-wide rather than per component.
 */
export const importStream = new ImportStream();
