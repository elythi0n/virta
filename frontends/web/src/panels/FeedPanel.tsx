import { useEffect, useState } from 'react';
import { Feed, type FeedMessage, type Platform } from '@virta/feed-core';
import { Segmented } from '@virta/ui-kit';
import styles from './FeedPanel.module.css';

// Synthetic stream so the feed can be driven at velocity by hand until the daemon feeds it live.
const SAMPLE = [
  'gg that was clean',
  'no shot he actually hit that',
  'first time catching the stream live, this is sick',
  'what headset is that',
  'chat is going feral lol',
  'W player',
  'the pacing on this run is unreal, no notes',
  'someone clip that',
  'how is he this consistent',
  'incoming raid, hold the line',
];
const PLATFORMS: Platform[] = ['twitch', 'kick', 'x'];

let seq = 0;
function makeMessage(): FeedMessage {
  const i = seq++;
  return {
    id: `m${i}`,
    ts: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
    platform: PLATFORMS[i % PLATFORMS.length],
    author: `viewer_${Math.floor(Math.random() * 900)}`,
    body: SAMPLE[Math.floor(Math.random() * SAMPLE.length)],
  };
}

type Rate = 'off' | 'live' | 'stress';
const RATES: Record<Rate, number> = { off: 0, live: 12, stress: 200 }; // messages/second
const MAX_MESSAGES = 8000; // bound memory; oldest drop once exceeded
const TICK_MS = 100;

export default function FeedPanel() {
  const [messages, setMessages] = useState<FeedMessage[]>([]);
  const [rate, setRate] = useState<Rate>('live');

  useEffect(() => {
    const perSecond = RATES[rate];
    if (perSecond === 0) return;
    const perTick = Math.max(1, Math.round((perSecond * TICK_MS) / 1000));
    const id = setInterval(() => {
      setMessages((prev) => {
        const additions = Array.from({ length: perTick }, makeMessage);
        const combined = prev.concat(additions);
        return combined.length > MAX_MESSAGES ? combined.slice(combined.length - MAX_MESSAGES) : combined;
      });
    }, TICK_MS);
    return () => clearInterval(id);
  }, [rate]);

  return (
    <div className={styles.panel}>
      <div className={styles.toolbar}>
        <Segmented
          ariaLabel="Stream rate"
          value={rate}
          onValueChange={(v) => setRate(v as Rate)}
          options={[
            { value: 'off', label: 'Off' },
            { value: 'live', label: 'Live' },
            { value: 'stress', label: '200/s' },
          ]}
        />
      </div>
      <div className={styles.feedWrap}>
        <Feed messages={messages} />
      </div>
    </div>
  );
}
