import { haversineMeters, lerp } from '$lib/geo';

/**
 * Client-side interpolation (SPEC.md §12.2): the server pushes discrete
 * updates; the client lerps ACTUAL/ESTIMATED positions between updates and
 * holds still for STALE/UNKNOWN.
 */
export interface AnimatedTrainPoint {
	lat: number;
	lon: number;
	heading?: number;
	type: 'ACTUAL' | 'ESTIMATED' | 'STALE' | 'UNKNOWN';
	confidence: number | null;
}

export function shouldAnimate(type: AnimatedTrainPoint['type']): boolean {
	return type === 'ACTUAL' || type === 'ESTIMATED';
}

/** Fraction helper (pure, unit-testable). */
export function fraction(elapsedMs: number, gapMs: number): number {
	if (gapMs <= 0) return 1;
	const f = elapsedMs / gapMs;
	return f < 0 ? 0 : f > 1 ? 1 : f;
}

/** One train's movement between two updates, started when the update arrives. */
export interface Glide {
	fromLat: number;
	fromLon: number;
	toLat: number;
	toLon: number;
	startAt: number;
	durationMs: number;
	/** Degrees/ms of the train's last reported move; 0 when it isn't moving. */
	vLat: number;
	vLon: number;
}

// The feed arrives in bursts (30s vehicle poll, 5s estimation sweep), so the
// glide window is measured per train from its own update interval and clamped
// so one update can neither ease in over minutes nor snap. The default covers
// only a train's very first position, which is held still regardless.
const MIN_GLIDE_MS = 1_000;
const MAX_GLIDE_MS = 45_000;
const DEFAULT_GLIDE_MS = 30_000;

/**
 * How long a marker keeps travelling past its window when the next update is
 * late: it coasts at the train's last reported speed instead of parking on the
 * target, so feed jitter doesn't read as stop-and-go. Capped so a feed that
 * never comes back can't drag the marker off the map.
 */
const MAX_OVERRUN_MS = 15_000;

/**
 * Ceiling for marker motion (~90 mph, more than these vehicles do between
 * updates). A move faster than this is a correction rather than travel: the
 * train reappeared somewhere new, or a bad estimate was replaced.
 */
const MAX_ANIMATE_MPS = 40;

/** A move of `dLat`/`dLon` degrees over `ms` milliseconds, expressed as m/s. */
function speedMps(dLat: number, dLon: number, refLat: number, ms: number): number {
	if (ms <= 0) return Infinity;
	const mLat = dLat * 110_540;
	const mLon = dLon * 111_320 * Math.cos((refLat * Math.PI) / 180);
	return (Math.hypot(mLat, mLon) * 1000) / ms;
}

/** Clamps a measured update interval into a usable glide window. */
export function glideDuration(intervalMs: number): number {
	if (intervalMs < MIN_GLIDE_MS) return MIN_GLIDE_MS;
	return intervalMs > MAX_GLIDE_MS ? MAX_GLIDE_MS : intervalMs;
}

/**
 * Where a glide has reached at `now`: `from` before it starts, easing onto `to`
 * for the length of the window, then coasting past `to` at the train's last
 * reported speed until the next update (or the coast cap) lands.
 */
export function glidePosition(g: Glide, now: number): { lat: number; lon: number } {
	const elapsed = now - g.startAt;
	if (elapsed <= g.durationMs) {
		const f = fraction(elapsed, g.durationMs);
		return { lat: lerp(g.fromLat, g.toLat, f), lon: lerp(g.fromLon, g.toLon, f) };
	}
	const overrun = Math.min(elapsed - g.durationMs, MAX_OVERRUN_MS);
	return { lat: g.toLat + g.vLat * overrun, lon: g.toLon + g.vLon * overrun };
}

/**
 * Starts the glide for an incoming position. It leaves from wherever the
 * marker currently is, so consecutive updates never jump, and lasts about as
 * long as the previous gap between updates so trains keep moving at the speed
 * the feed reports them at. That reported speed is carried as a coast, used
 * when the window closes before the next update lands. Two guards keep bad
 * data from looking like motion: an update farther than the marker could have
 * travelled inside the window settles at once instead of flying across the
 * map, and the coast is capped at a speed the vehicle could actually do, since
 * the gap it is measured over also carries whatever correction the glide
 * absorbed. STALE/UNKNOWN positions settle immediately (`from` equals `to`, no
 * coast) instead of animating, per SPEC.md §12.2.
 */
export function startGlide(prev: Glide | undefined, to: AnimatedTrainPoint, now: number): Glide {
	const animatable = shouldAnimate(to.type);
	const from = prev && animatable ? glidePosition(prev, now) : { lat: to.lat, lon: to.lon };
	const gapMs = prev ? now - prev.startAt : 0;
	const durationMs = prev ? glideDuration(gapMs) : DEFAULT_GLIDE_MS;

	const corrected = haversineMeters(from.lat, from.lon, to.lat, to.lon) > MAX_ANIMATE_MPS * (durationMs / 1000);
	const start = corrected ? { lat: to.lat, lon: to.lon } : from;

	let vLat = 0;
	let vLon = 0;
	if (animatable && !corrected && prev && gapMs > 0) {
		vLat = (to.lat - prev.toLat) / gapMs;
		vLon = (to.lon - prev.toLon) / gapMs;
		const mps = speedMps(to.lat - prev.toLat, to.lon - prev.toLon, to.lat, gapMs);
		if (mps > MAX_ANIMATE_MPS) {
			const k = MAX_ANIMATE_MPS / mps;
			vLat *= k;
			vLon *= k;
		}
	}
	return {
		fromLat: start.lat,
		fromLon: start.lon,
		toLat: to.lat,
		toLon: to.lon,
		startAt: now,
		durationMs,
		vLat,
		vLon
	};
}

/** Opacity knob from SPEC.md §12.4: ESTIMATED fades with confidence. */
export function opacityFor(t: AnimatedTrainPoint): number {
	if (t.type === 'STALE' || t.type === 'UNKNOWN') return 0.55;
	if (t.type === 'ESTIMATED') {
		const c = t.confidence ?? 0;
		return 0.4 + 0.6 * c;
	}
	return 1;
}