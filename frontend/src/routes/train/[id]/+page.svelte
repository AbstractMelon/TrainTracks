<script lang="ts">
	import { TrainFront, ArrowLeft } from '@lucide/svelte';
	import VehiclePhoto from '$lib/components/VehiclePhoto.svelte';
	import type { PageData } from './$types';
	import { api } from '$lib/api/client';
	import { confidencePct, delayLabel, fmtTime } from '$lib/format';

	let { data } = $props();
	let override = $state<{ id: string; train: PageData['train'] } | null>(null);
	let train = $derived(override?.id === data.trainId ? override.train : data.train);

	$effect(() => {
		const id = data.trainId;
		const timer = setInterval(async () => {
			try {
				override = { id, train: await api.train(id) };
			} catch {
				/* train may have left the active set */
			}
		}, 10_000);
		return () => clearInterval(timer);
	});
</script>

<svelte:head>
	<title>Train {train.id} · TrainTracks</title>
</svelte:head>

<div class="page">
	<a class="back" href="/"><ArrowLeft size={14} /> Back to map</a>
	<h1 class="title">
		<TrainFront size={20} />
		{train.destination || train.id}
	</h1>

	<VehiclePhoto agency={train.agency} mode={train.route?.mode} />

	<div class="card">
		<h3>Live position</h3>
		<dl class="grid">
			<div><dt>Status</dt><dd>{train.position.type}</dd></div>
			<div><dt>Confidence</dt><dd>{confidencePct(train.position.confidence)}</dd></div>
			<div><dt>Speed</dt><dd>{train.position.speed_mph != null ? `${Math.round(train.position.speed_mph)} mph` : '—'}</dd></div>
			{#if train.position.heading != null}
				<div><dt>Heading</dt><dd>{Math.round(train.position.heading)}°</dd></div>
			{/if}
			<div><dt>Last actual</dt><dd>{fmtTime(train.position.last_actual_at)}</dd></div>
			<div><dt>Delay</dt><dd>{delayLabel(train.delay_seconds)}</dd></div>
		</dl>
	</div>

	{#if train.route}
		<div class="card">
			<h3>Route</h3>
			<p>
				<span class="badge" style="background:{train.route.color || '#334155'}"></span>
				{train.route.short_name || train.route.long_name || train.route.id}
			</p>
		</div>
	{/if}

	{#if train.next_station}
		<div class="card">
			<h3>Next stop</h3>
			<p>{train.next_station.name}</p>
		</div>
	{/if}

	<div class="card">
		<h3>Latest position</h3>
		<p class="muted">
			{fmtTime(train.position.last_actual_at)} at
			{train.position.lat.toFixed(5)}, {train.position.lon.toFixed(5)}
		</p>
	</div>
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
	.card p {
		margin: 4px 0;
	}
	.grid {
		margin: 0;
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
		gap: 8px 20px;
	}
	.grid div {
		display: flex;
		justify-content: space-between;
		gap: 8px;
	}
	.grid dt {
		color: #64748b;
	}
	.grid dd {
		margin: 0;
	}
	.badge {
		display: inline-block;
		width: 12px;
		height: 12px;
		border-radius: 3px;
	}
	.muted {
		color: #64748b;
		font-size: 13px;
	}
</style>