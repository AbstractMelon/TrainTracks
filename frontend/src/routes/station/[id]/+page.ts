import type { PageLoad } from './$types';
import { api } from '$lib/api/client';

export const load: PageLoad = async ({ params, fetch }) => {
	const id = decodeURIComponent(params.id);
	const station = await api.station(id, fetch);
	return { station, stationId: id };
};