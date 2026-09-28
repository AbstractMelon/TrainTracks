import type { PageLoad } from './$types';
import { api } from '$lib/api/client';

export const load: PageLoad = async ({ params, fetch }) => {
	const id = decodeURIComponent(params.id);
	const train = await api.train(id, fetch);
	return { train, trainId: id };
};