-- Seed the UTA agency reference row (SPEC.md §1, §13). UTA's GTFS feeds are
-- open public feeds (no API key), so they live in the seed, not in env vars.
INSERT INTO agencies (id, name, website, gtfs_static_url, gtfs_rt_url, timezone, license_note)
VALUES (
    'uta',
    'Utah Transit Authority',
    'https://www.rideuta.com/',
    'https://apps.rideuta.com/tms/gtfs/Static',
    'https://apps.rideuta.com/tms/gtfs/Vehicle',
    'America/Denver',
    'Public open data feed, served by UTA.'
)
ON CONFLICT (id) DO NOTHING;