import { createContext, useContext } from 'react';

type FeedDisplayValue = {
  showTimestamps: boolean;
  setShowTimestamps: (v: boolean) => void;
  /** Names that mark a message as a mention of you (drives the Mentions inbox + highlighting). */
  mentionNames: string[];
  setMentionNames: (v: string[]) => void;
  /** Mod view: show a deleted message's original text (struck) instead of a tombstone. */
  showDeleted: boolean;
  setShowDeleted: (v: boolean) => void;
  /** Saved canned messages, offered as quick replies in the composer. */
  quickReplies: string[];
  setQuickReplies: (v: string[]) => void;
  /** Auto-engage calm mode when a feed's combined rate reaches this many msg/s (0 = off). */
  autoCalmRate: number;
  setAutoCalmRate: (v: number) => void;
};

const FeedDisplayContext = createContext<FeedDisplayValue>({
  showTimestamps: true,
  setShowTimestamps: () => {},
  mentionNames: [],
  setMentionNames: () => {},
  showDeleted: false,
  setShowDeleted: () => {},
  quickReplies: [],
  setQuickReplies: () => {},
  autoCalmRate: 0,
  setAutoCalmRate: () => {},
});

export const FeedDisplayProvider = FeedDisplayContext.Provider;

// Feed display preferences (timestamp toggle, mention names) live at the app root and are read by
// feeds deep in the dock (through the dockview portal, which preserves context) and set from
// Settings.
export function useFeedDisplay(): FeedDisplayValue {
  return useContext(FeedDisplayContext);
}
