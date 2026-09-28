<script lang="ts">
	import { TrainTrack } from '@lucide/svelte';
	import { connection } from '$lib/state/connection.svelte';
</script>

<header class="app-header">
	<a class="brand" href="/">
		<TrainTrack size={22} />
		<span>TrainTracks</span>
	</a>
	<nav>
		<a href="/">Live map</a>
		<a href="/about">About</a>
	</nav>
	<div class="conn" title="Live connection status">
		<span class:dot-green={connection.connected} class:dot-red={!connection.connected}></span>
		{connection.connected ? 'live' : connection.retrying ? 'reconnecting' : 'offline'}
	</div>
</header>

<style>
	.app-header {
		display: flex;
		align-items: center;
		gap: 16px;
		padding: 0 16px;
		height: 48px;
		background: #0f172a;
		color: #e2e8f0;
	}
	.brand {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		font-weight: 700;
		color: inherit;
		text-decoration: none;
	}
	nav {
		display: flex;
		gap: 14px;
		margin-left: 8px;
	}
	nav a {
		color: #cbd5e1;
		text-decoration: none;
	}
	nav a:hover {
		color: #fff;
	}
	.conn {
		margin-left: auto;
		display: inline-flex;
		align-items: center;
		gap: 6px;
		font-size: 12px;
		color: #94a3b8;
	}
	.dot-green,
	.dot-red {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		display: inline-block;
	}
	.dot-green {
		background: #22c55e;
	}
	.dot-red {
		background: #ef4444;
	}
</style>
