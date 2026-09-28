<script lang="ts">
	import { Route as RouteIcon, ArrowLeft } from '@lucide/svelte';
	import type { PageData } from './$types';

	let { data } = $props();
	const route = $derived((data as PageData).route);
</script>

<svelte:head>
	<title>{route.short_name || route.long_name || route.id} · TrainTracks</title>
</svelte:head>

<div class="page">
	<a class="back" href="/"><ArrowLeft size={14} /> Back to map</a>
	<h1 class="title">
		<RouteIcon size={20} />
		<span class="badge" style="background:{route.color || '#334155'}"></span>
		{route.short_name || route.long_name || route.id}
	</h1>
	<p class="muted">ID {route.id}{route.long_name ? ` (${route.long_name})` : ''}</p>

	{#if route.stations?.length}
		<div class="card">
			<h3>Stations on this route ({route.stations.length})</h3>
			<ol>
				{#each route.stations as s}
					<li><a href={`/station/${encodeURIComponent(s.id)}`}>{s.name}</a></li>
				{/each}
			</ol>
		</div>
	{:else}
		<p class="muted">No station list available for this route.</p>
	{/if}
</div>

<style>
	.back {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		text-decoration: none;
		margin-bottom: 12px;
	}
	.title {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 22px;
	}
	.badge {
		display: inline-block;
		width: 14px;
		height: 14px;
		border-radius: 4px;
	}
	.card {
		background: #fff;
		border: 1px solid #e2e8f0;
		border-radius: 10px;
		padding: 14px 16px;
		margin-top: 14px;
	}
	.card h3 {
		margin: 0 0 8px;
		font-size: 13px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: #64748b;
	}
	ol {
		margin: 0;
		padding-left: 20px;
		columns: 2;
	}
	li {
		padding: 2px 0;
		font-size: 13px;
	}
	.muted {
		color: #64748b;
		font-size: 13px;
	}
</style>