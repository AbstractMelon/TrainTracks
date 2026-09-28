import { describe, expect, it } from 'vitest';
import { vehicleFor } from './vehicle';

describe('vehicleFor', () => {
	it('returns a photo for each UTA mode the API reports', () => {
		for (const mode of ['bus', 'light_rail', 'commuter_rail']) {
			const photo = vehicleFor('uta', mode);
			expect(photo, mode).not.toBeNull();
			expect(photo!.src).toMatch(/^\/vehicles\/uta\//);
			expect(photo!.label).toBeTruthy();
			expect(photo!.credit).toBeTruthy();
		}
	});

	it('returns null when the agency or mode has no photo', () => {
		expect(vehicleFor('uta', 'ferry')).toBeNull();
		expect(vehicleFor('other', 'bus')).toBeNull();
		expect(vehicleFor('uta', '')).toBeNull();
		expect(vehicleFor(undefined, 'bus')).toBeNull();
	});
});
