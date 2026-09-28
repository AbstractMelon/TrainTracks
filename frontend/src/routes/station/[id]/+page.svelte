<script lang="ts">
	import { MapPin, ArrowLeft } from '@lucide/svelte';
	import type { PageData } from './$types';
	import { arrivalDelayLabel, fmtTimeShort } from '$lib/format';

	let { data } = $props();
	const station = $derived((data as PageData).station);
</script>

<svelte:head>
	<title>{station.name} · TrainTracks</title>
</svelte:head>

<div class="page">
	<a class="back" href="/"><ArrowLeft size={14} /> Back to map</a>
	<h1 class="title">
		<MapPin size={20} />
		{station.name}
	</h1>
	<p class="muted">ID {station.id}</p>

	{#if station.next_arrivals?.length}
		<div class="card">
			<h3>Upcoming arrivals</h3>
			<table>
				<thead>
					<tr><th>Train</th><th>Headsign</th><th>ETA</th><th>Scheduled</th><th>Delay</th></tr>
				</thead>
				<tbody>
					{#each station.next_arrivals as a}
						<tr>
							<td><a href={`/train/${encodeURIComponent(a.train_id)}`}>{a.train_id}</a></td>
							<td>{a.headsign}</td>
							<td>{fmtTimeShort(a.predicted_at || a.scheduled_at)}</td>
							<td>{fmtTimeShort(a.scheduled_at)}</td>
							<td>{arrivalDelayLabel(a.delay_seconds)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{:else}
		<p class="muted">No upcoming arrivals right now.</p>
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
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	th,
	td {
		text-align: left;
		padding: 6px 8px;
		border-bottom: 1px solid #f1f5f9;
	}
	.muted {
		color: #64748b;
		font-size: 13px;
	}
</style>