import { describe, expect, it } from 'vitest';
import {
	fraction,
	shouldAnimate,
	opacityFor,
	glideDuration,
	glidePosition,
	startGlide,
	type AnimatedTrainPoint,
	type Glide
} from './interpolate';

const actual = { lat: 40.7, lon: -111.8, type: 'ACTUAL' as const, confidence: null };
// ~280m from `actual`, a real move over one update gap: startGlide treats a
// move faster than a vehicle could travel as a correction, so fixtures have to
// be plausible for the glide behaviour under test to be what runs.
const est = { lat: 40.702, lon: -111.798, type: 'ESTIMATED' as const, confidence: 0.6 };

describe('fraction', () => {
	it('clamps into [0,1]', () => {
		expect(fraction(-5, 1000)).toBe(0);
		expect(fraction(500, 1000)).toBe(0.5);
		expect(fraction(5000, 1000)).toBe(1);
	});
	it('returns 1 for zero/negative gap', () => {
		expect(fraction(100, 0)).toBe(1);
		expect(fraction(100, -10)).toBe(1);
	});
});

describe('shouldAnimate', () => {
	it('animates ACTUAL and ESTIMATED only', () => {
		expect(shouldAnimate('ACTUAL')).toBe(true);
		expect(shouldAnimate('ESTIMATED')).toBe(true);
		expect(shouldAnimate('STALE')).toBe(false);
		expect(shouldAnimate('UNKNOWN')).toBe(false);
	});
});

describe('opacityFor', () => {
	it('is 1 for live trains', () => {
		expect(opacityFor(actual)).toBe(1);
	});
	it('fades estimated trains by confidence', () => {
		expect(opacityFor(est)).toBeCloseTo(0.4 + 0.6 * 0.6, 6);
	});
	it('is dimmed but visible for stale/unknown', () => {
		expect(opacityFor({ ...actual, type: 'STALE' })).toBe(0.55);
	});
});

describe('glideDuration', () => {
	it('uses the measured gap between updates but clamps it', () => {
		expect(glideDuration(300)).toBe(1_000);
		expect(glideDuration(30_000)).toBe(30_000);
		expect(glideDuration(120_000)).toBe(45_000);
	});
});

describe('glidePosition', () => {
	const g: Glide = { fromLat: 0, fromLon: 0, toLat: 10, toLon: 20, startAt: 1_000, durationMs: 1_000, vLat: 0, vLon: 0 };

	it('stays at the start before the glide begins and eases onto the target by its end', () => {
		expect(glidePosition(g, 0)).toEqual({ lat: 0, lon: 0 });
		expect(glidePosition(g, 1_500)).toEqual({ lat: 5, lon: 10 });
		expect(glidePosition(g, 2_000)).toEqual({ lat: 10, lon: 20 });
	});

	it('coasts past the target at the reported speed when the next update is late', () => {
		const coasting: Glide = { ...g, vLat: 0.001, vLon: 0.002 };
		// 1s of window, then 1s of coast past the target.
		const at = glidePosition(coasting, 3_000);
		expect(at.lat).toBeCloseTo(11, 9);
		expect(at.lon).toBeCloseTo(22, 9);
	});

	it('stops coasting at the cap so a feed that never returns parks the marker', () => {
		const coasting: Glide = { ...g, vLat: 0.001, vLon: 0.002 };
		expect(glidePosition(coasting, 62_000)).toEqual(glidePosition(coasting, 17_000));
	});
});

describe('startGlide', () => {
	it('holds the first position of a train still', () => {
		const g = startGlide(undefined, actual, 1_000);
		expect(g.fromLat).toBe(actual.lat);
		expect(g.fromLon).toBe(actual.lon);
		expect(g.toLat).toBe(actual.lat);
		expect(g.startAt).toBe(1_000);
		expect(g.vLat).toBe(0);
	});

	it('leaves from where the marker currently is, over the previous update gap', () => {
		const first = startGlide(undefined, actual, 0);
		const halfway = glidePosition(first, 15_000);
		const second = startGlide(first, est, 15_000);

		expect(second.fromLat).toBeCloseTo(halfway.lat, 9);
		expect(second.fromLon).toBeCloseTo(halfway.lon, 9);
		expect(second.toLat).toBe(est.lat);
		expect(second.durationMs).toBe(15_000);
	});

	it('carries the speed of the last reported move for coasting', () => {
		const first = startGlide(undefined, actual, 0);
		const second = startGlide(first, est, 10_000);

		expect(second.vLat).toBeCloseTo(0.002 / 10_000, 12);
		expect(second.vLon).toBeCloseTo(0.002 / 10_000, 12);
		// Past the window it keeps travelling instead of parking on the target.
		expect(glidePosition(second, 25_000).lat).toBeGreaterThan(est.lat);
	});

	it('continues from the previous target once the earlier glide has finished', () => {
		const first = startGlide(undefined, actual, 0);
		const second = startGlide(first, est, 60_000);

		expect(second.fromLat).toBe(actual.lat);
		expect(second.durationMs).toBe(45_000);
	});

	it('settles STALE/UNKNOWN positions instead of animating them', () => {
		const stale = { lat: 1, lon: 2, type: 'STALE' as const, confidence: null };
		const g = startGlide(startGlide(undefined, est, 0), stale, 5_000);

		expect(g.fromLat).toBe(stale.lat);
		expect(g.fromLon).toBe(stale.lon);
		expect(g.toLat).toBe(stale.lat);
		expect(g.toLon).toBe(stale.lon);
		expect(g.vLat).toBe(0);
		expect(g.vLon).toBe(0);
		expect(glidePosition(g, 60_000)).toEqual({ lat: stale.lat, lon: stale.lon });
	});

	it('settles a move no vehicle could have travelled instead of flying to it', () => {
		const first = startGlide(undefined, actual, 0);
		const far = { lat: 40.9, lon: -111.6, type: 'ACTUAL' as const, confidence: null };
		const g = startGlide(first, far, 30_000);

		expect(g.fromLat).toBe(far.lat);
		expect(g.fromLon).toBe(far.lon);
		expect(g.vLat).toBe(0);
		expect(g.vLon).toBe(0);
		expect(glidePosition(g, 120_000)).toEqual({ lat: far.lat, lon: far.lon });
	});

	it('caps the coast at a speed the vehicle could do', () => {
		const runaway: Glide = {
			fromLat: 40.7,
			fromLon: -111.8,
			toLat: 40.7,
			toLon: -111.8,
			startAt: 0,
			durationMs: 1_000,
			vLat: 0.00001,
			vLon: 0 // coasted ~4.4km past its target before the next update lands
		};
		const marker = glidePosition(runaway, 5_000);
		const g = startGlide(runaway, { ...actual, lat: marker.lat, lon: marker.lon }, 5_000);

		// The update lands on the marker, so there is no correction to settle;
		// the gap still carries the runaway coast, which the cap must absorb.
		const gapMs = 5_000;
		const mps =
			(Math.hypot(g.vLat * 110_540, g.vLon * 111_320 * Math.cos((40.7 * Math.PI) / 180)) * 1000) / gapMs;
		expect(mps).toBeGreaterThan(0);
		expect(mps).toBeLessThanOrEqual(40);
	});
});

// Regression test for the feed cadence: positions land in a burst every 30s,
// and every frame in between has to be part of a continuous glide.
describe('update bursts', () => {
	it('glides the whole way to the next burst without jumping or reversing', () => {
		let target: AnimatedTrainPoint = { ...actual };
		let glide = startGlide(undefined, target, 0);
		let prevLat = target.lat;
		let midGlide: number | null = null;

		for (let t = 0; t <= 90_000; t += 16) {
			if (t > 0 && t % 30_000 === 0) {
				target = { ...target, lat: target.lat + 0.005, lon: target.lon + 0.005 };
				glide = startGlide(glide, target, t);
			}
			const point = glidePosition(glide, t);

			expect(Math.abs(point.lat - prevLat)).toBeLessThan(0.005);
			expect(point.lat).toBeGreaterThanOrEqual(prevLat);
			if (t >= 45_000 && midGlide === null) midGlide = point.lat;
			prevLat = point.lat;
		}

		expect(midGlide).toBeCloseTo(40.7025, 3);
	});
});

// Regression test for jittered bursts: gaps stretched from 11s to 23s, the
// marker reached its target early and then sat still until the next update.
describe('stretched feed gaps', () => {
	it('never parks between updates', () => {
		const speed = 0.0002; // degrees of travel per second
		const stops = [11_000, 34_000, 45_000, 68_000, 79_000, 102_000, 113_000, 136_000];
		let target: AnimatedTrainPoint = { ...actual, lat: 40.7, lon: -111.8 };
		let glide = startGlide(undefined, target, 0);
		let prevLat = target.lat;
		let lastMoved = stops[0];
		let seen = 0;

		for (let now = 0; now <= 140_000; now += 200) {
			if (seen < stops.length && now >= stops[seen]) {
				target = { ...target, lat: 40.7 + (stops[seen] / 1_000) * speed };
				glide = startGlide(glide, target, now);
				seen++;
			}
			const point = glidePosition(glide, now);
			if (point.lat > prevLat) lastMoved = now;
			if (now > stops[0]) {
				expect(point.lat).toBeGreaterThanOrEqual(prevLat);
				expect(now - lastMoved).toBeLessThan(1_500);
			}
			prevLat = point.lat;
		}

		expect(seen).toBe(stops.length);
	});
});
