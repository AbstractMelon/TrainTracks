<script lang="ts">
	import { BookOpen } from '@lucide/svelte';
</script>

<svelte:head>
	<title>About · TrainTracks</title>
</svelte:head>

<div class="page">
	<h1 class="title"><BookOpen size={22} /> About TrainTracks</h1>

	<p>
		TrainTracks is a live transit status map. It ingests real-time vehicle positions and trip
		updates from transit agencies, tracks trains in near-real-time, estimates motion between
		updates, and streams the result to the browser over Server-Sent Events. No polling, no
		refresh. The map updates itself.
	</p>

	<h3>How it works</h3>
	<ul>
		<li><strong>Live</strong>. Real-time position received recently (<em>fresh</em>).</li>
		<li><strong>Estimated</strong>. Interpolated/predicted between updates, with a confidence score.</li>
		<li><strong>Stale</strong>. The train hasn't reported recently but is still on the map.</li>
		<li><strong>Unknown</strong>. Position known to be out of date or not precise.</li>
	</ul>

	<h3>Sources</h3>
	<ul>
		<li>Utah Transit Authority: GTFS-RT vehicle positions &amp; trip updates via
			<a href="https://gtfs.org/documentation/backend/usage/">gtfs.org</a>-style endpoints.</li>
		<li>OpenStreetMap: basemap tiles and rail geometry (via osmtracker-style import tooling).</li>
	</ul>

	<h3>Project</h3>
	<ul>
		<li>Backend: Go (REST + SSE). Frontend: SvelteKit + MapLibre GL.</li>
		<li>Estimation methodology and API reference are documented in <code>docs/</code>.</li>
	</ul>
</div>

<style>
	.title {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 24px;
	}
	h3 {
		margin: 20px 0 8px;
	}
	li {
		margin: 4px 0;
	}
</style>