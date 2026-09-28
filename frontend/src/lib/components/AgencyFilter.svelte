<script lang="ts">
	import { filters } from '$lib/state/trains.svelte';
	import type { Agency } from '$lib/types';

	interface Props {
		agencies: Agency[];
	}
	let { agencies }: Props = $props();
</script>

<label class="filter">
	<span>Agency</span>
	<select bind:value={filters.agency}>
		<option value="">All</option>
		{#each agencies as a}
			<option value={a.id}>{a.name}</option>
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