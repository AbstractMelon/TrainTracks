<script lang="ts">
	import { ExternalLink, X } from '@lucide/svelte';
	import VehiclePhoto from '$lib/components/VehiclePhoto.svelte';
	import { trainStore } from '$lib/state/trains.svelte';
	import { confidencePct, delayLabel, fmtTime } from '$lib/format';

	const train = $derived(trainStore.byId(trainStore.selectedTrainId));
</script>

{#if train}
	<aside class="panel">
		<div class="head">
			<h2>{train.destination || train.id}</h2>
			<button class="close" aria-label="Close" onclick={() => trainStore.select('')}>
				<X size={14} />
			</button>
		</div>

		<VehiclePhoto agency={train.agency} mode={train.route?.mode} />

		<dl>
			<div><dt>Agency</dt><dd>{train.agency}</dd></div>
			{#if train.route}
				<div>
					<dt>Route</dt>
					<dd class="route"><span class="route_color" style="background:{train.route.color || '#334155'}"></span>{train.route.short_name || train.route.long_name || train.route.id}</dd>
				</div>
			{/if}
			<div><dt>Status</dt><dd>{train.position.type}</dd></div>
			<div><dt>Confidence</dt><dd>{confidencePct(train.position.confidence)}</dd></div>
			<div><dt>Speed</dt><dd>{train.position.speed_mph != null ? `${Math.round(train.position.speed_mph)} mph` : '—'}</dd></div>
			{#if train.position.heading != null}
				<div><dt>Heading</dt><dd>{Math.round(train.position.heading)}°</dd></div>
			{/if}
			<div><dt>Last actual</dt><dd>{fmtTime(train.position.last_actual_at)}</dd></div>
			<div><dt>Delay</dt><dd>{delayLabel(train.delay_seconds)}</dd></div>
			{#if train.next_station}
				<div>
					<dt>Next stop</dt>
					<dd>{train.next_station.name}</dd>
				</div>
			{/if}
		</dl>

		<a class="more" href={`/train/${encodeURIComponent(train.id)}`}>
			Track details <ExternalLink size={12} />
		</a>
	</aside>
{/if}

<style>
	.panel {
		position: absolute;
		top: 12px;
		right: 12px;
		width: 260px;
		background: #fff;
		border: 1px solid #e2e8f0;
		border-radius: 10px;
		box-shadow: 0 10px 30px rgba(15, 23, 42, 0.15);
		padding: 12px;
		z-index: 20;
		font-size: 13px;
	}
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 8px;
	}
	h2 {
		font-size: 15px;
		margin: 0;
	}
	.close {
		border: 0;
		background: none;
		cursor: pointer;
		color: #64748b;
		padding: 2px;
	}
	dl {
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	dl > div {
		display: flex;
		justify-content: space-between;
		gap: 8px;
	}
	dt {
		color: #64748b;
	}
	dd {
		margin: 0;
	}
	.route {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.route_color {
		width: 10px;
		height: 10px;
		border-radius: 3px;
	}
	.more {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		margin-top: 10px;
		color: #0369a1;
		text-decoration: none;
	}
</style>