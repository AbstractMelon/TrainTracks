import type { PageLoad } from './$types';
import { api } from '$lib/api/client';

export const load: PageLoad = async ({ params, fetch }) => {
	const id = decodeURIComponent(params.id);
	const route = await api.route(id, fetch);
	return { route, routeId: id };
};