"""Explicit local OLTP benchmark settings, not production sizing defaults."""
BASE = {
    'max_connections': '256',
    'shared_preload_libraries': 'pg_stat_statements',
    'pg_stat_statements.track': 'all',
    'track_io_timing': 'on',
    'track_wal_io_timing': 'on',
}
# The host is shared with application pods. Allocate a bounded cache rather than
# taking a percentage of all host RAM just because the container has no limit.
TUNED = {
    'shared_buffers': '1GB',
    'effective_cache_size': '4GB',
    'work_mem': '4MB',
    'maintenance_work_mem': '128MB',
    'autovacuum_work_mem': '128MB',
    'wal_buffers': '32MB',
    'min_wal_size': '1GB',
    'max_wal_size': '4GB',
    'checkpoint_timeout': '15min',
    'checkpoint_completion_target': '0.9',
    'autovacuum_naptime': '10s',
    'autovacuum_vacuum_scale_factor': '0.05',
    'autovacuum_analyze_scale_factor': '0.02',
    'jit': 'off',
    # Preserve crash recovery and acknowledged strict-audit WAL durability.
    'fsync': 'on',
    'synchronous_commit': 'on',
    'full_page_writes': 'on',
}

def settings(tuned=False):
    return dict(BASE, **(TUNED if tuned else {}))

def arguments(tuned=False):
    args=['postgres']
    for key,value in settings(tuned).items():args.extend(['-c',f'{key}={value}'])
    return args
