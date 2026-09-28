import {
	API_BASE,
	type Agency,
	type Route,
	type RouteShapeFeature,
	type Station,
	type Train,
	type TrackFeature,
	type SearchResults
} from '$lib/types';

type Fetcher = typeof globalThis.fetch;

function buildUrl(path: string, params?: Record<string, string | undefined>): string {
	const query = new URLSearchParams();
	if (params) {
		for (const [k, v] of Object.entries(params)) {
			if (v) query.set(k, v);
		}
	}
	const qs = query.toString();
	return API_BASE + path + (qs ? `?${qs}` : '');
}

async function get<T>(
	path: string,
	params?: Record<string, string | undefined>,
	fetcher?: Fetcher
): Promise<T> {
	const url = buildUrl(path, params);
	const res = await (fetcher ?? fetch)(url, {
		headers: { Accept: 'application/json', 'User-Agent': 'TrainTracks.web/1.0' }
	});
	if (!res.ok) throw new Error(`GET ${path} → ${res.status}`);
	return (await res.json()) as T;
}

export const api = {
	agencies: () => get<{ agencies: Agency[] }>('/agencies').then((r) => r.agencies),
	routes: (agency?: string) => get<{ routes: Route[] }>('/routes', { agency }).then((r) => r.routes),
	route: (id: string, fetcher?: Fetcher) => get<Route>(`/routes/${id}`, undefined, fetcher),
	routeShapes: (params?: { agency?: string; bbox?: string }) =>
		get<{ type: string; features: RouteShapeFeature[] }>('/routes/geo', params),
	stations: (agency?: string, q?: string) =>
		get<{ stations: Station[] }>('/stations', { agency, q }).then((r) => r.stations),
	station: (id: string, fetcher?: Fetcher) => get<Station>(`/stations/${id}`, undefined, fetcher),
	trains: (params?: { agency?: string; route?: string; bbox?: string; status?: string }) =>
		get<{ trains: Train[] }>('/trains', params).then((r) => r.trains),
	train: (id: string, fetcher?: Fetcher) => get<Train>(`/trains/${id}`, undefined, fetcher),
	tracks: (bbox: string) => get<{ type: string; features: TrackFeature[] }>(`/tracks?bbox=${bbox}`),
	search: (q: string) => get<SearchResults>('/search', { q })
};