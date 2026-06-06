import { request } from './http';
import type { FilterRule } from './wire.gen';

// The active profile's filter ruleset.
export function listFilters(): Promise<FilterRule[]> {
  return request<{ filters: FilterRule[] }>('/v1/filters').then((r) => r.filters);
}

// Replace the ruleset; the daemon validates (a bad regex throws) and hot-swaps it. Returns the
// stored rules.
export function saveFilters(filters: FilterRule[]): Promise<FilterRule[]> {
  return request<{ filters: FilterRule[] }>('/v1/filters', {
    method: 'PUT',
    body: JSON.stringify({ filters }),
  }).then((r) => r.filters);
}
