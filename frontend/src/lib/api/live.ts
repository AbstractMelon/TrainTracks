import { API_BASE, type Train } from '$lib/types';
import { trainStore } from '$lib/state/trains.svelte';

export type LiveFilters = { agency?: string; route?: string; bbox?: string };

function buildQuery(f: LiveFilters): string {
	const p = new URLSearchParams();
	if (f.agency) p.set('agency', f.agency);
	if (f.route) p.set('route', f.route);
	if (f.bbox) p.set('bbox', f.bbox);
	const s = p.toString();
	return s ? `?${s}` : '';
}

export interface LiveClient {
	close: () => void;
}

/**
 * Opens an SSE connection to /live. On connect the server pushes a full
 * snapshot; afterwards it streams diffs. Reconnects with exponential backoff.
 */
export function connectLive(filters: LiveFilters, onStatus?: (connected: boolean, retrying: boolean) => void): LiveClient {
	let es: EventSource | null = null;
	let closed = false;
	let attempts = 0;

	const backoff = () => Math.min(3000 * 2 ** (attempts - 1), 30_000);

	const open = () => {
		if (closed) return;
		es = new EventSource(API_BASE + '/live' + buildQuery(filters));
		onStatus?.(true, false);

		es.addEventListener('train_update', (e) => {
			attempts = 0;
			try {
				const t = JSON.parse((e as MessageEvent).data) as Train;
				trainStore.upsert(t);
			} catch (err) {
				console.error('[live] train_update parse/upsert failed', err);
			}
		});

		es.addEventListener('train_removed', (e) => {
			const data = JSON.parse((e as MessageEvent).data) as { id: string };
			trainStore.remove(data.id);
		});

		es.addEventListener('heartbeat', () => {
			attempts = 0;
		});

		es.onerror = () => {
			console.error('[live] SSE error');
			es?.close();
			if (closed) return;
			attempts++;
			onStatus?.(false, true);
			setTimeout(open, backoff());
		};
	};

	open();

	return {
		close() {
			closed = true;
			es?.close();
		}
	};
}