import { createContext, useContext } from 'react';

type FeedDisplayValue = {
  showTimestamps: boolean;
  setShowTimestamps: (v: boolean) => void;
  /** Names that mark a message as a mention of you (drives the Mentions inbox + highlighting). */
  mentionNames: string[];
  setMentionNames: (v: string[]) => void;
};

const FeedDisplayContext = createContext<FeedDisplayValue>({
  showTimestamps: true,
  setShowTimestamps: () => {},
  mentionNames: [],
  setMentionNames: () => {},
});

export const FeedDisplayProvider = FeedDisplayContext.Provider;

// Feed display preferences (timestamp toggle, mention names) live at the app root and are read by
// feeds deep in the dock (through the dockview portal, which preserves context) and set from
// Settings.
export function useFeedDisplay(): FeedDisplayValue {
  return useContext(FeedDisplayContext);
}
