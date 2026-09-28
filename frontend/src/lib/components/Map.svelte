<script lang="ts">
	import maplibregl from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import { onMount } from 'svelte';

	import { api } from '$lib/api/client';
	import { routeColor } from '$lib/format';
	import { emitBounds, registerMap } from '$lib/map/registry';
	import { routeSelection } from '$lib/state/selection.svelte';
	import type { RouteShapeFeature, Station, TrackFeature } from '$lib/types';

	interface Props {
		stations: Station[];
		tracks?: TrackFeature[];
		agency?: string;
	}

	let { stations = [], tracks = [], agency = '' }: Props = $props();
	let container: HTMLDivElement;
	let map: maplibregl.Map | null = $state(null);
	let routeShapes: RouteShapeFeature[] = [];

	const ROUTE_LAYERS = ['route-lines', 'route-highlight', 'route-highlight-glow'];

	function addIfMissing(id: string, make: () => void) {
		if (map && !map.getLayer(id)) {
			make();
		}
	}

	function shapeData(shapes: RouteShapeFeature[]): GeoJSON.FeatureCollection {
		return { type: 'FeatureCollection', features: shapes as never };
	}

	function pointerCursor(mp: maplibregl.Map, layer: string) {
		mp.on('mouseenter', layer, () => (mp.getCanvas().style.cursor = 'pointer'));
		mp.on('mouseleave', layer, () => (mp.getCanvas().style.cursor = ''));
	}

	// Dims every route line except the highlighted ones, which are redrawn on
	// top by the `route-highlight` layers. Only dims when a highlighted route
	// is actually on screen, so an off-map selection never greys out the map.
	function applySelection(mp: maplibregl.Map, ids: string[]) {
		if (!mp.getLayer('route-lines')) return;
		const selected = new Set(ids);
		const visible = routeShapes.filter((f) => selected.has(f.properties.id));
		(mp.getSource('route-highlight') as maplibregl.GeoJSONSource | undefined)?.setData(
			shapeData(visible)
		);
		mp.setPaintProperty(
			'route-lines',
			'line-opacity',
			visible.length === 0
				? 0.75
				: ['case', ['any', ...ids.map((id) => ['==', ['get', 'id'], id])], 0, 0.18]
		);
	}

	function syncRouteShapes(mp: maplibregl.Map) {
		if (!mp.getLayer('route-lines')) return;
		(mp.getSource('route-lines') as maplibregl.GeoJSONSource | undefined)?.setData(
			shapeData(routeShapes)
		);
		applySelection(mp, routeSelection.ids);
	}

	$effect(() => {
		const ids = routeSelection.ids;
		if (map) applySelection(map, ids);
	});

	$effect(() => {
		agency;
		if (map) scheduleFetch(map);
	});

	$effect(() => {
		if (!map || !stations.length) return;
		const feats = stations.map((s) => ({
			type: 'Feature',
			geometry: { type: 'Point', coordinates: [s.lon, s.lat] },
			properties: { id: s.id, name: s.name }
		}));
		try {
			const src = map.getSource('stations') as maplibregl.GeoJSONSource | undefined;
			src?.setData({ type: 'FeatureCollection', features: feats as never });
		} catch {
			/* source not added yet */
		}
	});

	function selectStation(mp: maplibregl.Map, id: string, name: string, at: maplibregl.LngLatLike) {
		openStationPopup(mp, at, name, id);
		api.station(id)
			.then((st) => {
				const ids = (st.routes ?? []).map((r) => r.id);
				if (ids.length) routeSelection.select(ids);
			})
			.catch(() => {
				/* highlight nothing if the station detail is unavailable */
			});
	}

	function openStationPopup(mp: maplibregl.Map, at: maplibregl.LngLatLike, name: string, id: string) {
		const body = document.createElement('div');
		body.className = 'station-pop';
		const title = document.createElement('strong');
		title.textContent = name;
		const link = document.createElement('a');
		link.href = `/station/${encodeURIComponent(id)}`;
		link.textContent = 'Open station page';
		body.append(title, link);
		new maplibregl.Popup({ closeButton: true, offset: 14 }).setLngLat(at).setDOMContent(body).addTo(mp);
	}

	onMount(() => {
		const mp = new maplibregl.Map({
			container,
			style: 'https://tiles.openfreemap.org/styles/fiord',
			center: [-111.89, 40.75],
			zoom: 11
		});
		map = mp;
		registerMap(mp);

		mp.on('click', (e) => {
			const interactive = [...ROUTE_LAYERS, 'stations', 'trains'].filter((l) => mp.getLayer(l));
			const hits = interactive.length
				? mp.queryRenderedFeatures(e.point, { layers: interactive })
				: [];
			if (hits.some((f) => f.layer.id === 'trains')) return;
			const line = hits.find((f) => ROUTE_LAYERS.includes(f.layer.id));
			if (line) {
				routeSelection.select([line.properties.id as string]);
				return;
			}
			if (!hits.length) routeSelection.clear();
		});

		mp.on('style.load', () => {
			addIfMissing('route-lines', () => {
				mp.addSource('route-lines', { type: 'geojson', data: shapeData([]) });
				mp.addLayer({
					id: 'route-lines',
					type: 'line',
					source: 'route-lines',
					layout: { 'line-cap': 'round', 'line-join': 'round' },
					paint: {
						'line-color': ['coalesce', ['get', 'color'], '#64748b'],
						'line-width': ['interpolate', ['linear'], ['zoom'], 8, 2, 13, 4],
						'line-opacity': 0.75
					}
				});
				pointerCursor(mp, 'route-lines');
			});

			addIfMissing('tracks', () => {
				mp.addSource('tracks', {
					type: 'geojson',
					data: { type: 'FeatureCollection', features: (tracks ?? []) as never }
				});
				mp.addLayer({
					id: 'tracks',
					type: 'line',
					source: 'tracks',
					layout: { 'line-cap': 'round' },
					paint: { 'line-color': '#8fa3b8', 'line-width': 1.5 }
				});
			});

			addIfMissing('stations', () => {
				mp.addSource('stations', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } });
				mp.addLayer({
					id: 'stations',
					type: 'circle',
					source: 'stations',
					paint: {
						'circle-radius': 4,
						'circle-color': '#334155',
						'circle-stroke-color': '#ffffff',
						'circle-stroke-width': 1.5
					}
				});
				mp.on('click', 'stations', (e) => {
					const f = e.features?.[0];
					if (!f) return;
					const id = f.properties.id as string;
					selectStation(mp, id, (f.properties.name as string | undefined) ?? id, e.lngLat);
				});
				pointerCursor(mp, 'stations');
			});

			addIfMissing('route-highlight', () => {
				mp.addSource('route-highlight', { type: 'geojson', data: shapeData([]) });
				mp.addLayer(
					{
						id: 'route-highlight-glow',
						type: 'line',
						source: 'route-highlight',
						layout: { 'line-cap': 'round', 'line-join': 'round' },
						paint: {
							'line-color': ['coalesce', ['get', 'color'], '#0369a1'],
							'line-width': ['interpolate', ['linear'], ['zoom'], 8, 7, 13, 12],
							'line-opacity': 0.35
						}
					},
					'stations'
				);
				mp.addLayer(
					{
						id: 'route-highlight',
						type: 'line',
						source: 'route-highlight',
						layout: { 'line-cap': 'round', 'line-join': 'round' },
						paint: {
							'line-color': ['coalesce', ['get', 'color'], '#0369a1'],
							'line-width': ['interpolate', ['linear'], ['zoom'], 8, 3.5, 13, 6]
						}
					},
					'stations'
				);
				pointerCursor(mp, 'route-highlight');
				pointerCursor(mp, 'route-highlight-glow');
			});

			syncRouteShapes(mp);
			scheduleFetch(mp);
			mp.on('moveend', () => scheduleFetch(mp));
		});

		return () => {
			registerMap(null);
			mp.remove();
		};
	});

	let fetchTimer: ReturnType<typeof setTimeout> | undefined;

	function scheduleFetch(mp: maplibregl.Map) {
		if (fetchTimer) return;
		fetchTimer = setTimeout(() => {
			fetchTimer = undefined;
			void refresh(mp);
		}, 400);
	}

	async function refresh(mp: maplibregl.Map) {
		const b = mp.getBounds();
		const bbox = `${b.getWest()},${b.getSouth()},${b.getEast()},${b.getNorth()}`;
		emitBounds(bbox);
		try {
			const fc = await api.tracks(bbox);
			if (fc.features?.length) {
				const src = mp.getSource('tracks') as maplibregl.GeoJSONSource | undefined;
				src?.setData({ type: 'FeatureCollection', features: fc.features as never });
			}
		} catch {
			/* keep old tracks */
		}
		await loadRouteShapes(mp, bbox);
	}

	async function loadRouteShapes(mp: maplibregl.Map, bbox: string) {
		try {
			const fc = await api.routeShapes({ agency: agency || undefined, bbox });
			routeShapes = fc.features.map((f) => ({
				...f,
				properties: { ...f.properties, color: routeColor(f.properties.color) }
			}));
		} catch {
			/* keep the route lines already drawn */
			return;
		}
		syncRouteShapes(mp);
	}
</script>

<div class="map" bind:this={container}></div>

<style>
	.map {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
	}
	:global(.station-pop) {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 13px;
		min-width: 130px;
	}
	:global(.station-pop a) {
		color: #0369a1;
		font-size: 12px;
		text-decoration: none;
	}
</style>
