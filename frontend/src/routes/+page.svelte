<script lang="ts">
	import { onMount } from 'svelte';
	import { X } from '@lucide/svelte';

	import Map from '$lib/components/Map.svelte';
	import TrainMarkerLayer from '$lib/components/TrainMarkerLayer.svelte';
	import TrainDetailPanel from '$lib/components/TrainDetailPanel.svelte';
	import SearchBar from '$lib/components/SearchBar.svelte';
	import AgencyFilter from '$lib/components/AgencyFilter.svelte';
	import ModeFilter from '$lib/components/ModeFilter.svelte';
	import RouteFilter from '$lib/components/RouteFilter.svelte';
	import Legend from '$lib/components/Legend.svelte';

	import { api } from '$lib/api/client';
	import { connectLive } from '$lib/api/live';
	import { filters, trainStore } from '$lib/state/trains.svelte';
	import { routeSelection } from '$lib/state/selection.svelte';
	import { connection } from '$lib/state/connection.svelte';
	import { routeColor } from '$lib/format';
	import { onBoundsChange } from '$lib/map/registry';
	import type { Agency, Route, Station } from '$lib/types';

	let agencies = $state<Agency[]>([]);
	let routes = $state<Route[]>([]);
	let stations = $state<Station[]>([]);
	let bbox = $state('');
	let live: { close: () => void } | null = null;

	const highlighted = $derived(
		routeSelection.ids.map((id) => {
			const route = routes.find((r) => r.id === id);
			return {
				label: route?.short_name || route?.long_name || id,
				color: routeColor(route?.color) ?? '#334155'
			};
		})
	);

	function trainCount(): number {
		return trainStore.count;
	}

	function reconnect() {
		live?.close();
		live = connectLive(
			{ agency: filters.agency || undefined, route: filters.route || undefined, bbox: bbox || undefined },
			(c, r) => connection.set(c, r)
		);
	}

	onMount(() => {
		void api
			.agencies()
			.then((a) => {
				agencies = a;
			})
			.catch(() => {});
		const offBounds = onBoundsChange((b) => {
			bbox = b;
		});
		return () => {
			offBounds();
			live?.close();
		};
	});

	$effect(() => {
		filters.agency;
		filters.route;
		bbox;
		reconnect();
	});

	$effect(() => {
		const agency = filters.agency;
		void api
			.routes(agency || undefined)
			.then((r) => {
				routes = r;
			})
			.catch(() => {});
		void api
			.stations(agency || undefined)
			.then((s) => {
				stations = s;
			})
			.catch(() => {});
	});
</script>

<svelte:head>
	<title>TrainTracks</title>
</svelte:head>

<div class="map-wrap">
	<Map {stations} agency={filters.agency} />
	<TrainMarkerLayer />

	<div class="toolbar">
		<SearchBar />
		<AgencyFilter {agencies} />
		<ModeFilter {routes} />
		<RouteFilter {routes} />
		<button class="clear" onclick={() => filters.clear()}>Reset</button>
	</div>

	<div class="statusbar">
		<Legend />
		{#if highlighted.length}
			<span class="highlight">
				<span class="hl_dot" style="background: {highlighted[0].color}"></span>
				<span>Highlighting {highlighted.map((h) => h.label).join(', ')}</span>
				<button class="hl_clear" aria-label="Clear highlighted route" onclick={() => routeSelection.clear()}>
					<X size={12} />
				</button>
			</span>
		{/if}
		<span class="count">{trainCount()} trains</span>
	</div>

	<TrainDetailPanel />
</div>

<style>
	.map-wrap {
		position: relative;
		flex: 1;
		min-height: 0;
	}
	.toolbar {
		position: absolute;
		top: 12px;
		left: 12px;
		display: flex;
		align-items: flex-end;
		gap: 12px;
		background: rgba(255, 255, 255, 0.95);
		border: 1px solid #e2e8f0;
		border-radius: 10px;
		padding: 10px 12px;
		box-shadow: 0 6px 20px rgba(15, 23, 42, 0.1);
		z-index: 10;
	}
	.statusbar {
		position: absolute;
		bottom: 12px;
		left: 12px;
		display: flex;
		align-items: center;
		gap: 14px;
		background: rgba(255, 255, 255, 0.95);
		border: 1px solid #e2e8f0;
		border-radius: 10px;
		padding: 8px 12px;
		box-shadow: 0 6px 20px rgba(15, 23, 42, 0.1);
		font-size: 13px;
		z-index: 10;
	}
	.count {
		color: #475569;
	}
	.highlight {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		color: #0f172a;
		font-weight: 500;
	}
	.hl_dot {
		width: 10px;
		height: 10px;
		border-radius: 3px;
	}
	.hl_clear {
		display: inline-flex;
		border: 0;
		background: none;
		padding: 2px;
		cursor: pointer;
		color: #64748b;
	}
	.hl_clear:hover {
		color: #0f172a;
	}
	.clear {
		border: 1px solid #cbd5e1;
		background: #fff;
		border-radius: 6px;
		padding: 4px 10px;
		font-size: 13px;
		cursor: pointer;
	}
	.clear:hover {
		background: #f1f5f9;
	}
</style>