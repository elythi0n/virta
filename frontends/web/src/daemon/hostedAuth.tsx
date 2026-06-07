import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { getHostedStatus, getMe, type VirtaUser } from './account';

export interface HostedAuthState {
  hosted: boolean;
  user: VirtaUser | null;
  /** True once the initial auth check has resolved (success or failure). */
  ready: boolean;
  setUser: (user: VirtaUser | null) => void;
}

const HostedAuthContext = createContext<HostedAuthState>({
  hosted: false,
  user: null,
  ready: false,
  setUser: () => {},
});

export function HostedAuthProvider({ children }: { children: ReactNode }) {
  const [hosted, setHosted] = useState(false);
  const [user, setUser] = useState<VirtaUser | null>(null);
  const [ready, setReady] = useState(false);
  const retries = useRef(0);

  useEffect(() => {
    let cancelled = false;
    const load = () => {
      getHostedStatus()
        .then(async s => {
          if (cancelled) return;
          setHosted(s.hosted);
          if (s.hosted) {
            await getMe().then(u => { if (!cancelled) setUser(u); }).catch(() => { if (!cancelled) setUser(null); });
          }
          if (!cancelled) setReady(true);
        })
        .catch(() => {
          if (cancelled) return;
          if (retries.current < 3) {
            retries.current++;
            setTimeout(load, 800 * retries.current);
          } else {
            setReady(true);
          }
        });
    };
    load();
    return () => { cancelled = true; };
  }, []);

  const stableSetUser = useCallback((u: VirtaUser | null) => setUser(u), []);

  return (
    <HostedAuthContext.Provider value={{ hosted, user, ready, setUser: stableSetUser }}>
      {children}
    </HostedAuthContext.Provider>
  );
}

export function useHostedAuth(): HostedAuthState {
  return useContext(HostedAuthContext);
}

/** True when we are in hosted mode and the user is NOT logged in. */
export function useIsGuest(): boolean {
  const { hosted, user, ready } = useHostedAuth();
  return hosted && ready && user === null;
}
