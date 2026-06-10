import { createContext, useContext } from 'react';

// Jumps to a specific message: scrolls + flashes it in an open feed that still has it buffered,
// or falls back to opening that channel's chat. Provided at the app root so panels rendered
// through the dockview portal (e.g. Search) can use it without prop-drilling through the dock.
type JumpToMessage = (channelKey: string, messageId: string, label: string) => void;

const JumpToMessageContext = createContext<JumpToMessage>(() => {});

export const JumpToMessageProvider = JumpToMessageContext.Provider;

export function useJumpToMessage(): JumpToMessage {
  return useContext(JumpToMessageContext);
}
