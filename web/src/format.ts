export const fmt = (iso?: string | null) => (iso ? new Date(iso).toLocaleString() : '—')
