/**
 * Who is signed in, shared by the whole shell.
 *
 * It lives in a rune module rather than in the root layout because the sign-in
 * page has to hand the answer back UP: signing in is a client-side navigation,
 * the layout's onMount does not run again, and without this the header would go
 * on showing "signed out" until a full reload.
 *
 * There is exactly one owner in v0.1 (Principle 4), so this holds one session
 * and no user list. The shape is SessionState from $lib/api, which is the
 * server's own sessionResponse — nothing is derived or invented here.
 */
import { fetchSession, SIGNED_OUT, type SessionState } from './api';

let current = $state<SessionState>(SIGNED_OUT);
let loaded = $state(false);

export const session = {
	get current(): SessionState {
		return current;
	},

	/** False until the bootstrap call has answered once. Not the same as signed out. */
	get loaded(): boolean {
		return loaded;
	},

	get authenticated(): boolean {
		return current.authenticated;
	},

	set(next: SessionState): void {
		current = next;
		loaded = true;
	},

	clear(): void {
		current = SIGNED_OUT;
		loaded = true;
	},

	async refresh(): Promise<SessionState> {
		const next = await fetchSession();
		current = next;
		loaded = true;
		return next;
	}
};
