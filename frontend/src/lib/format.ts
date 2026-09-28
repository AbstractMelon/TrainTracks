export function fmtTime(iso?: string | null): string {
	if (!iso) return '—';
	return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export function fmtTimeShort(iso?: string | null): string {
	if (!iso) return '—';
	return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

export function confidencePct(c: number | null): string {
	if (c == null) return '—';
	return `${Math.round(c * 100)}%`;
}

export function delayLabel(d: number | null | undefined): string {
	if (d == null) return '—';
	if (d < 0) return `ahead ${Math.abs(d)}s`;
	return `+${d}s`;
}

/** Normalizes a GTFS route_color (bare hex or #hex) to a CSS color. */
export function routeColor(color?: string): string | undefined {
	if (!color) return undefined;
	const hex = color.startsWith('#') ? color : `#${color}`;
	return /^#[0-9a-f]{6}$/i.test(hex) ? hex : undefined;
}

export function arrivalDelayLabel(d: number | null | undefined): string {
	if (d == null) return '';
	if (d < 0) return `${Math.abs(d)}s ahead`;
	return `+${d}s late`;
}
