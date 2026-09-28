// Photo of the vehicle behind a train, keyed by agency and GTFS route mode,
// so a selection can show "that is a UTA bus / TRAX / FrontRunner"

export interface VehiclePhoto {
	src: string;
	label: string;
	credit: string;
	source?: string;
}

// Images live in static/vehicles and are credited in static/vehicles/CREDITS.md.
const PHOTOS: Record<string, Record<string, VehiclePhoto>> = {
	uta: {
		bus: {
			src: '/vehicles/uta/bus.jpg',
			label: 'UTA bus',
			credit: 'Utah Transit, CC BY-SA 4.0',
			source: 'https://commons.wikimedia.org/wiki/File:Utah_Transit_Authority_route_72.jpg'
		},
		light_rail: {
			src: '/vehicles/uta/light_rail.jpg',
			label: 'TRAX light rail',
			credit: 'GregMcCollum, CC0',
			source: 'https://commons.wikimedia.org/wiki/File:UTA_Trax_train_at_Stadium_Station,_October_15,_2023.jpg'
		},
		commuter_rail: {
			src: '/vehicles/uta/commuter_rail.jpg',
			label: 'FrontRunner commuter rail',
			credit: '© Jeffrey D. Allred / Deseret News'
		}
	}
};

/** The photo for an agency and route mode, or null when none is registered. */
export function vehicleFor(agency?: string, mode?: string): VehiclePhoto | null {
	if (!agency || !mode) return null;
	return PHOTOS[agency]?.[mode] ?? null;
}
