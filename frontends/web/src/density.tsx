import { createContext, useContext } from 'react';
import type { Density } from '@virta/feed-core';

type DensityContextValue = { density: Density; setDensity: (d: Density) => void };

const DensityContext = createContext<DensityContextValue>({ density: 'cozy', setDensity: () => {} });

export const DensityProvider = DensityContext.Provider;

// Feed row density lives at the app root and is read by feeds deep in the dock (through the
// dockview portal, which preserves context) and set from Settings.
export function useDensity(): DensityContextValue {
  return useContext(DensityContext);
}
