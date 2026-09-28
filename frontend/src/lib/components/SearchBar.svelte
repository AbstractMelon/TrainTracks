<script lang="ts">
	import { Search } from '@lucide/svelte';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { routeSelection } from '$lib/state/selection.svelte';
	import type { Station, Route } from '$lib/types';

	let q = $state('');
	let results = $state<{ stations: Station[]; routes: Route[] }>({ stations: [], routes: [] });
	let open = $state(false);
	let timer: ReturnType<typeof setTimeout> | undefined;

	function search() {
		if (!q.trim()) {
			results = { stations: [], routes: [] };
			return;
		}
		clearTimeout(timer);
		timer = setTimeout(async () => {
			try {
				results = await api.search(q.trim());
				open = true;
			} catch {
				results = { stations: [], routes: [] };
			}
		}, 250);
	}

	function goStation(id: string) {
		open = false;
		goto(`/station/${encodeURIComponent(id)}`);
	}

	function pickRoute(r: Route) {
		open = false;
		routeSelection.select([r.id]);
	}
</script>

<div class="search">
	<div class="box">
		<Search size={14} />
		<input
			type="search"
			bind:value={q}
			placeholder="Search stations & routes…"
			oninput={search}
			onfocus={search}
			onblur={() => setTimeout(() => (open = false), 150)}
		/>
	</div>
	{#if open && q.trim()}
		<div class="dropdown">
			{#if results.stations.length}
				<div class="group">Stations</div>
				{#each results.stations as s}
					<button onmousedown={() => goStation(s.id)}>{s.name}</button>
				{/each}
			{/if}
			{#if results.routes.length}
				<div class="group">Routes</div>
				{#each results.routes as r}
					<button onmousedown={() => pickRoute(r)}>
						{r.short_name || r.long_name || r.id}
					</button>
				{/each}
			{/if}
			{#if !results.stations.length && !results.routes.length}
				<div class="empty">No results for “{q}”</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.search {
		position: relative;
	}
	.box {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		border: 1px solid #cbd5e1;
		border-radius: 8px;
		padding: 4px 8px;
		background: #fff;
	}
	.box input {
		border: 0;
		outline: none;
		font-size: 14px;
		min-width: 180px;
	}
	.dropdown {
		position: absolute;
		top: 34px;
		left: 0;
		width: 280px;
		background: #fff;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12);
		max-height: 320px;
		overflow: auto;
		z-index: 50;
	}
	.group {
		padding: 6px 10px;
		font-size: 11px;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: #64748b;
	}
	.dropdown button {
		display: block;
		width: 100%;
		text-align: left;
		padding: 8px 10px;
		border: 0;
		background: none;
		cursor: pointer;
	}
	.dropdown button:hover {
		background: #f1f5f9;
	}
	.empty {
		padding: 10px;
		color: #94a3b8;
		font-size: 13px;
	}
</style>