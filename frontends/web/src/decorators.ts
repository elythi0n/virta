import { useState, useEffect } from 'react';

export interface DecoratorPattern {
  regex: string;
  type: string;
}

// Global registry: pluginId → patterns. Module-level so all components share the same instance.
const registry = new Map<string, DecoratorPattern[]>();
const listeners = new Set<() => void>();

export function setPluginPatterns(pluginId: string, patterns: DecoratorPattern[]): void {
  if (patterns.length === 0) {
    registry.delete(pluginId);
  } else {
    registry.set(pluginId, patterns);
  }
  listeners.forEach(l => l());
}

export function removePluginPatterns(pluginId: string): void {
  registry.delete(pluginId);
  listeners.forEach(l => l());
}

export function getAllPatterns(): DecoratorPattern[] {
  const out: DecoratorPattern[] = [];
  for (const pats of registry.values()) out.push(...pats);
  return out;
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

/** React hook: returns all currently registered patterns. Re-renders when any plugin updates its patterns. */
export function useDecorators(): DecoratorPattern[] {
  const [patterns, setPatterns] = useState<DecoratorPattern[]>(getAllPatterns);
  useEffect(() => subscribe(() => setPatterns(getAllPatterns())), []);
  return patterns;
}
