export function lerp(a: number, b: number, f: number): number {
	return a + (b - a) * f;
}

function clamp01(f: number): number {
	return f < 0 ? 0 : f > 1 ? 1 : f;
}

/** Linear interpolation between two lat/lon points by fraction f in [0,1]. */
export function interpolatePoint(lat1: number, lon1: number, lat2: number, lon2: number, f: number): [number, number] {
	f = clamp01(f);
	return [lat1 + (lat2 - lat1) * f, lon1 + (lon2 - lon1) * f];
}

const EARTH_RADIUS_M = 6_371_000;

function deg2rad(d: number): number {
	return (d * Math.PI) / 180;
}

export function haversineMeters(lat1: number, lon1: number, lat2: number, lon2: number): number {
	const dLat = deg2rad(lat2 - lat1);
	const dLon = deg2rad(lon2 - lon1);
	const a =
		Math.sin(dLat / 2) ** 2 +
		Math.cos(deg2rad(lat1)) * Math.cos(deg2rad(lat2)) * Math.sin(dLon / 2) ** 2;
	return EARTH_RADIUS_M * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}