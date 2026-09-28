import type maplibregl from 'maplibre-gl';

let map: maplibregl.Map | null = null;
const boundsListeners: ((bbox: string) => void)[] = [];

export function registerMap(mp: maplibregl.Map | null) {
	map = mp;
}

export function getMap(): maplibregl.Map | null {
	return map;
}

export function onBoundsChange(cb: (bbox: string) => void): () => void {
	boundsListeners.push(cb);
	return () => {
		const i = boundsListeners.indexOf(cb);
		if (i >= 0) boundsListeners.splice(i, 1);
	};
}

export function emitBounds(bbox: string) {
	for (const cb of boundsListeners) cb(bbox);
}