import { filters } from '$lib/state/trains.svelte';

// Routes highlighted on the map. Written by the route filter, train markers,
// route lines, stations and search; read by Map.svelte.
//
// `clear` drops the highlight and the route filter together so the drawn
// selection can never drift from what the filter selects.
class RouteSelection {
	ids = $state<string[]>([]);

	select(ids: string[]) {
		this.ids = [...new Set(ids)];
	}

	clear() {
		this.ids = [];
		filters.route = '';
	}
}

export const routeSelection = new RouteSelection();
