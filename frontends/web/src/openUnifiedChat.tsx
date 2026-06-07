import { createContext, useContext } from 'react';

// Opens a unified multi-channel chat feed for a streamer. When all variants for a streamer are
// passed, the feed shows messages from every platform they stream on in one timeline.
// For a single channel it behaves identically to openChannel (stable panel id).
type OpenUnifiedChat = (channelKeys: string[], label: string) => void;

const OpenUnifiedChatContext = createContext<OpenUnifiedChat>(() => {});

export const OpenUnifiedChatProvider = OpenUnifiedChatContext.Provider;

export function useOpenUnifiedChat(): OpenUnifiedChat {
  return useContext(OpenUnifiedChatContext);
}
