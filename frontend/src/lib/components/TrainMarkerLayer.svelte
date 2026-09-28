<script lang="ts">
	import maplibregl from 'maplibre-gl';

	import { opacityFor } from '$lib/animation/interpolate';
	import { filters, trainStore } from '$lib/state/trains.svelte';
	import { getMap } from '$lib/map/registry';
	import { routeSelection } from '$lib/state/selection.svelte';
	import type { Train } from '$lib/types';

	const TYPE_COLORS: Record<string, string> = {
		ACTUAL: '#0d9488',
		ESTIMATED: '#f59e0b',
		STALE: '#64748b',
		UNKNOWN: '#94a3b8'
	};

	const SHAPES = ['rail', 'bus'] as const;
	type Shape = (typeof SHAPES)[number];

	function shapeFor(mode?: string): Shape {
		return mode === 'bus' || mode === 'trolleybus' ? 'bus' : 'rail';
	}

	function markerSvg(color: string, shape: Shape): string {
		const body =
			shape === 'bus'
				? `<rect x="6" y="2" width="12" height="20" rx="4" fill="${color}" stroke="#ffffff" stroke-width="1.5"/>` +
					`<path d="M9 8 L12 5 L15 8" fill="none" stroke="#ffffff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>`
				: `<path d="M12 2 L20 20 L12 15.5 L4 20 Z" fill="${color}" stroke="#ffffff" stroke-width="1"/>`;
		const svg =
			`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24">` +
			body +
			`</svg>`;
		return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg);
	}

	function toFeature(t: Train, lat: number, lon: number) {
		return {
			type: 'Feature',
			id: t.id,
			geometry: { type: 'Point', coordinates: [lon, lat] },
			properties: {
				id: t.id,
				routeId: t.route?.id ?? '',
				heading: t.position.heading ?? 0,
				type: t.position.type,
				shape: shapeFor(t.route?.mode),
				opacity: opacityFor(t.position)
			}
		};
	}

	function loadIcons(mp: maplibregl.Map) {
		for (const shape of SHAPES) {
			for (const [type, color] of Object.entries(TYPE_COLORS)) {
				const name = `${shape}-${type}`;
				if (mp.hasImage(name)) continue;
				const img = document.createElement('img');
				img.src = markerSvg(color, shape);
				img.onload = () => {
					if (!mp.hasImage(name)) mp.addImage(name, img as never);
				};
			}
		}
	}

	function initLayers(mp: maplibregl.Map) {
		mp.addSource('trains', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } });
		mp.addLayer({
			id: 'trains',
			type: 'symbol',
			source: 'trains',
			layout: {
				'icon-image': [
					'concat',
					['coalesce', ['get', 'shape'], 'rail'],
					'-',
					['coalesce', ['get', 'type'], 'ACTUAL']
				],
				'icon-size': ['interpolate', ['linear'], ['zoom'], 9, 0.65, 14, 1.1],
				'icon-rotate': ['coalesce', ['get', 'heading'], 0],
				'icon-rotation-alignment': 'map',
				'icon-allow-overlap': true
			},
			paint: {
				'icon-opacity': ['coalesce', ['get', 'opacity'], 1]
			}
		});
		mp.on('click', 'trains', (e) => {
			const id = e.features?.[0]?.properties?.id as string | undefined;
			if (!id) return;
			trainStore.select(id);
			const routeId = e.features?.[0]?.properties?.routeId as string | undefined;
			if (routeId) routeSelection.select([routeId]);
		});
		mp.on('mouseenter', 'trains', () => (mp.getCanvas().style.cursor = 'pointer'));
		mp.on('mouseleave', 'trains', () => (mp.getCanvas().style.cursor = ''));
	}

	// Svelte 5 runs $effect before onMount, so we must not gate on mount flags.
	// This effect runs exactly once (no reactive deps): a persistent rAF loop
	// lazily initialises the map layers once the style is ready, then repaints
	// train markers every frame. getMap() is re-read per frame so ordering with
	// Map.svelte's own mount doesn't matter.
	$effect(() => {
		let disposed = false;
		let initialized = false;

		const frame = (now: number) => {
			if (disposed) return;
			raf = requestAnimationFrame(frame);

			const mp = getMap();
			if (!mp || !mp.isStyleLoaded()) return;

			if (!initialized) {
				if (!mp.getLayer('trains')) {
					initLayers(mp);
				}
				loadIcons(mp);
				initialized = true;
			}

			const animated = trainStore.animatedPositions(now);
			const trains = trainStore.filtered({
				agency: filters.agency,
				route: filters.route,
				mode: filters.mode,
				status: filters.status,
				showUnknown: filters.showUnknown
			});
			const features = trains.filter((t) => animated.has(t.id)).map((t) => {
				const { lat, lon } = animated.get(t.id)!;
				return toFeature(t, lat, lon);
			});
			try {
				const src = mp.getSource('trains') as maplibregl.GeoJSONSource | undefined;
				src?.setData({ type: 'FeatureCollection', features: features as never });
			} catch {
				/* source not ready */
			}
		};

		let raf = requestAnimationFrame(frame);

		return () => {
			disposed = true;
			cancelAnimationFrame(raf);
		};
	});
</script>

<!-- Marker layer renders onto the map owned by Map.svelte -->