<script lang="ts">
	import { vehicleFor } from '$lib/vehicle';

	interface Props {
		agency?: string;
		mode?: string;
	}
	let { agency, mode }: Props = $props();

	const photo = $derived(vehicleFor(agency, mode));
</script>

{#if photo}
	<figure class="shot">
		<img src={photo.src} alt={photo.label} loading="lazy" />
		<figcaption>
			<span class="type">{photo.label}</span>
			{#if photo.source}
				<a href={photo.source} target="_blank" rel="noreferrer">{photo.credit}</a>
			{:else}
				<span>{photo.credit}</span>
			{/if}
		</figcaption>
	</figure>
{/if}

<style>
	.shot {
		margin: 0 0 10px;
	}
	img {
		display: block;
		width: 100%;
		aspect-ratio: 16 / 9;
		object-fit: cover;
		border: 1px solid #e2e8f0;
		border-radius: 8px;
		background: #f1f5f9;
	}
	figcaption {
		display: flex;
		flex-direction: column;
		gap: 1px;
		margin-top: 4px;
		font-size: 11px;
		color: #64748b;
	}
	.type {
		font-size: 12px;
		font-weight: 500;
		color: #334155;
	}
	a {
		color: #0369a1;
		text-decoration: none;
	}
	a:hover {
		text-decoration: underline;
	}
</style>
