<script lang="ts">
	import { filters } from '$lib/state/trains.svelte';
	import { routeSelection } from '$lib/state/selection.svelte';
	import type { Route } from '$lib/types';

	interface Props {
		routes: Route[];
	}
	let { routes }: Props = $props();

	const visible = $derived(routes.filter((r) => !filters.mode || r.mode === filters.mode));

	$effect(() => {
		const id = filters.route;
		if (id && !visible.some((r) => r.id === id)) filters.route = '';
	});

	$effect(() => {
		const id = filters.route;
		if (id) routeSelection.select([id]);
		else routeSelection.clear();
	});
</script>

<label class="filter">
	<span>Route</span>
	<select bind:value={filters.route}>
		<option value="">All</option>
		{#each visible as r}
			<option value={r.id}>{r.short_name || r.long_name || r.id}</option>
		{/each}
	</select>
</label>

<style>
	.filter {
		display: inline-flex;
		flex-direction: column;
		gap: 2px;
		font-size: 12px;
		color: #475569;
	}
	select {
		border: 1px solid #cbd5e1;
		border-radius: 6px;
		padding: 4px 6px;
		font-size: 13px;
	}
</style>
