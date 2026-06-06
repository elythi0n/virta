import { createContext, useContext } from 'react';

type FeedDisplayValue = {
  showTimestamps: boolean;
  setShowTimestamps: (v: boolean) => void;
};

const FeedDisplayContext = createContext<FeedDisplayValue>({
  showTimestamps: true,
  setShowTimestamps: () => {},
});

export const FeedDisplayProvider = FeedDisplayContext.Provider;

// Feed display preferences (currently the timestamp toggle) live at the app root and are read by
// feeds deep in the dock (through the dockview portal, which preserves context) and set from
// Settings.
export function useFeedDisplay(): FeedDisplayValue {
  return useContext(FeedDisplayContext);
}
