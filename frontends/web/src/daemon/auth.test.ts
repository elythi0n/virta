import { describe, expect, it } from 'vitest';
import { isTerminal, pollSession } from './auth';

const noSleep = () => Promise.resolve();

describe('isTerminal', () => {
  it('treats pending as non-terminal and the rest as terminal', () => {
    expect(isTerminal('pending')).toBe(false);
    expect(isTerminal('authorized')).toBe(true);
    expect(isTerminal('denied')).toBe(true);
    expect(isTerminal('expired')).toBe(true);
    expect(isTerminal('error')).toBe(true);
  });
});

describe('pollSession', () => {
  it('polls until a terminal state and returns it', async () => {
    const states = ['pending', 'pending', 'authorized'];
    let i = 0;
    const final = await pollSession(() => Promise.resolve({ state: states[i++], login: 'bob' }), {
      intervalMs: 1,
      sleep: noSleep,
    });
    expect(final.state).toBe('authorized');
    expect(i).toBe(3); // two pendings then the terminal
  });

  it('throws AbortError when the signal is already aborted', async () => {
    const ctrl = new AbortController();
    ctrl.abort();
    await expect(
      pollSession(() => Promise.resolve({ state: 'pending' }), { intervalMs: 1, sleep: noSleep, signal: ctrl.signal }),
    ).rejects.toMatchObject({ name: 'AbortError' });
  });
});
