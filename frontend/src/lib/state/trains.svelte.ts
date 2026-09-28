import { glidePosition, shouldAnimate, startGlide, type Glide } from '$lib/animation/interpolate';
import type { Train, PositionType } from '$lib/types';

// SPEC.md §12.1. Runes-based live train store with interpolation between
// consecutive updates (only the latest glide per train is kept).
//
// The feed arrives in bursts, so each update starts a glide that leaves from
// the marker's current position and eases into the new one over roughly the
// previous gap between updates, coasting past it if the next update is late.
// That keeps motion continuous: no waiting for the next burst, no jump when it
// lands.
//
// Internally keeps a plain Map as source of truth, and eagerly mirrors it into
// reactive `count`/`list` state so every component (statusbar, panels, marker
// loop) observes updates deterministically without depending on Map proxy
// instrumentation for iterators, size, or getters.
class TrainStore {
	private _trains = new Map<string, Train>();
	private glides = new Map<string, Glide>();

	count = $state(0);
	list: Train[] = $state([]);
	selectedTrainId = $state('');

	private sync() {
		this.count = this._trains.size;
		this.list = [...this._trains.values()];
	}

	upsert(t: Train) {
		const now = performance.now();
		const prev = this.glides.get(t.id);
		this.glides.set(
			t.id,
			startGlide(
				prev,
				{ lat: t.position.lat, lon: t.position.lon, type: t.position.type, confidence: t.position.confidence },
				now
			)
		);
		this._trains.set(t.id, t);
		this.sync();
	}

	remove(id: string) {
		this._trains.delete(id);
		this.glides.delete(id);
		if (this.selectedTrainId === id) this.selectedTrainId = '';
		this.sync();
	}

	select(id: string) {
		this.selectedTrainId = id;
	}

	/** Trains matching the given filters, in update order. */
	filtered(f: {
		agency?: string;
		route?: string;
		mode?: string;
		status?: PositionType | '';
		showUnknown?: boolean;
	}) {
		const out: Train[] = [];
		for (const t of this._trains.values()) {
			if (f.agency && t.agency !== f.agency) continue;
			if (f.route && t.route?.id !== f.route) continue;
			if (f.mode && t.route?.mode !== f.mode) continue;
			if (f.status && t.position.type !== f.status) continue;
			if (t.position.type === 'UNKNOWN' && !f.showUnknown) continue;
			out.push(t);
		}
		return out;
	}

	byId(id: string) {
		return this._trains.get(id);
	}

	/**
	 * Returns per-train lat/lon at the given frame time, gliding
	 * ACTUAL/ESTIMATED trains onto their latest update and holding
	 * STALE/UNKNOWN still.
	 */
	animatedPositions(now: number): Map<string, { lat: number; lon: number }> {
		const out = new Map<string, { lat: number; lon: number }>();
		for (const [id, train] of this._trains) {
			const g = this.glides.get(id);
			if (!g) {
				out.set(id, { lat: train.position.lat, lon: train.position.lon });
				continue;
			}
			const point = shouldAnimate(train.position.type)
				? glidePosition(g, now)
				: { lat: g.toLat, lon: g.toLon };
			out.set(id, point);
		}
		return out;
	}
}

export const trainStore = new TrainStore();

class FilterState {
	agency = $state('');
	route = $state('');
	mode = $state('');
	status = $state<PositionType | ''>('');
	showUnknown = $state(false);

	clear() {
		this.agency = '';
		this.route = '';
		this.mode = '';
		this.status = '';
		this.showUnknown = false;
	}
}

export const filters = $state(new FilterState());
