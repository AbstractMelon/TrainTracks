// Mirror of the backend API surface (backend/internal/api/types.go and
// backend/internal/models/types.go).

export const API_BASE: string = (import.meta.env.PUBLIC_API_BASE as string | undefined) ?? '/api/v1';

export type PositionType = 'ACTUAL' | 'ESTIMATED' | 'STALE' | 'UNKNOWN';

export interface Agency {
	id: string;
	name: string;
	website?: string;
	gtfs_static_url?: string;
	gtfs_rt_url?: string;
	timezone: string;
	license_note?: string;
}

export interface RouteRef {
	id: string;
	short_name: string;
	long_name?: string;
	mode?: string;
	color?: string;
}

export interface StationRef {
	id: string;
	name: string;
}

export interface PositionView {
	lat: number;
	lon: number;
	heading?: number;
	speed_mph?: number;
	type: PositionType;
	source: string;
	confidence: number | null;
	last_actual_at: string;
	updated_at: string;
}

export interface Train {
	id: string;
	agency: string;
	route?: RouteRef;
	destination?: string;
	position: PositionView;
	current_station?: StationRef | null;
	next_station?: StationRef | null;
	delay_seconds?: number | null;
}

export interface Arrival {
	train_id: string;
	route_id: string;
	headsign: string;
	direction_id?: number;
	scheduled_at: string;
	predicted_at?: string | null;
	delay_seconds?: number | null;
}

export interface Station {
	id: string;
	agency_id: string;
	gtfs_stop_id: string;
	name: string;
	lat: number;
	lon: number;
	routes?: Route[];
	next_arrivals?: Arrival[];
}

export interface GeometryLineString {
	type: 'LineString';
	coordinates: number[][];
}

export interface Route {
	id: string;
	agency_id: string;
	gtfs_route_id: string;
	short_name?: string;
	long_name?: string;
	mode: string;
	color?: string;
	stations?: Station[];
	geometry?: { type: string; coordinates: unknown };
}

export interface RouteShapeFeature {
	type: 'Feature';
	id: string;
	geometry: GeometryLineString;
	properties: {
		id: string;
		agency_id: string;
		short_name?: string;
		long_name?: string;
		mode: string;
		color?: string;
		shape_id: string;
	};
}

export interface TrackFeature {
	type: string;
	id: number;
	geometry: GeometryLineString;
	properties: {
		osm_way_id?: number | null;
		railway_type?: string;
		name?: string;
	};
}

export interface SearchResults {
	stations: Station[];
	routes: Route[];
}