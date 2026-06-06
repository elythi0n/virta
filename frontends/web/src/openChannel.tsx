import { createContext, useContext } from 'react';

// Opens a single-channel chat feed in the dock. Provided at the app root so panels rendered through
// the dockview portal (e.g. Search) can jump to a channel without prop-drilling through the dock.
type OpenChannel = (channelKey: string, label: string) => void;

const OpenChannelContext = createContext<OpenChannel>(() => {});

export const OpenChannelProvider = OpenChannelContext.Provider;

export function useOpenChannel(): OpenChannel {
  return useContext(OpenChannelContext);
}
