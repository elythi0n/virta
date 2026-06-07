import { createContext, useContext } from 'react';

type OpenStream = (channelKey: string, label: string) => void;

const OpenStreamContext = createContext<OpenStream>(() => {});

export const OpenStreamProvider = OpenStreamContext.Provider;

export function useOpenStream(): OpenStream {
  return useContext(OpenStreamContext);
}
