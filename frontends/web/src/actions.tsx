import { createContext, useContext } from 'react';
import type { CommandAction } from '@virta/ui-kit';

// The action registry, provided at the app root so surfaces deep in the dock (e.g. the Settings
// panel, rendered through a dockview portal) can read it without prop drilling.
const ActionsContext = createContext<CommandAction[]>([]);

export const ActionsProvider = ActionsContext.Provider;

export function useActions(): CommandAction[] {
  return useContext(ActionsContext);
}
