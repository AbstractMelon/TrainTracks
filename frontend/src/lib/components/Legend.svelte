<script lang="ts">
	const items = [
		{ label: 'Live', color: '#0d9488' },
		{ label: 'Estimated', color: '#f59e0b' },
		{ label: 'Stale', color: '#64748b' },
		{ label: 'Unknown', color: '#94a3b8' }
	];
</script>

<div class="legend">
	{#each items as it}
		<span class="item">
			<span class="swatch" style="background: {it.color}"></span>
			{it.label}
		</span>
	{/each}
</div>

<style>
	.legend {
		display: flex;
		gap: 10px;
		font-size: 12px;
		color: #475569;
	}
	.item {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.swatch {
		width: 10px;
		height: 10px;
		border-radius: 3px;
	}
</style>