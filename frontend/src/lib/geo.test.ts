import { describe, expect, it } from 'vitest';
import { interpolatePoint, haversineMeters, lerp } from './geo';

describe('lerp', () => {
	it('interpolates linearly', () => {
		expect(lerp(0, 10, 0.5)).toBe(5);
		expect(lerp(0, 10, 0)).toBe(0);
		expect(lerp(0, 10, 1)).toBe(10);
	});
});

describe('interpolatePoint', () => {
	it('interpolates lat/lon toward the destination', () => {
		expect(interpolatePoint(0, 0, 1, 1, 0.5)).toEqual([0.5, 0.5]);
	});
	it('clamps the fraction', () => {
		expect(interpolatePoint(0, 0, 10, 10, 5)).toEqual([10, 10]);
	});
});

describe('haversineMeters', () => {
	it('is ~0 for equal points', () => {
		expect(haversineMeters(40.75, -111.88, 40.75, -111.88)).toBeCloseTo(0, 3);
	});
	it('known distance: ~111km per degree of latitude', () => {
		const d = haversineMeters(40, -111.88, 41, -111.88);
		expect(Math.abs(d - 111_195)).toBeLessThan(200);
	});
});