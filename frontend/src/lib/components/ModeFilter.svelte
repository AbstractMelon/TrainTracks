<script lang="ts">
	import { filters } from '$lib/state/trains.svelte';
	import type { Route } from '$lib/types';

	interface Props {
		routes: Route[];
	}
	let { routes }: Props = $props();

	const modes = $derived([...new Set(routes.map((r) => r.mode).filter(Boolean))].sort());

	function label(mode: string): string {
		return mode.replace(/_/g, ' ');
	}
</script>

<label class="filter">
	<span>Type</span>
	<select bind:value={filters.mode}>
		<option value="">All</option>
		{#each modes as m}
			<option value={m}>{label(m)}</option>
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
