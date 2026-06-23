import DeckDock from './DeckDock';
import DeckHero from './DeckHero';
import { DeckProvider } from './DeckContext';
import styles from './DeckView.module.css';

// Deck: the live broadcast control workspace. A hero header carries the always-visible "Go live"
// CTA and the live-state pills; a dockview tree below holds Stream Info, Chat, Stats, Gifts, Mod
// queue, and Mentions — all rearrangeable, none closeable. Everything reads from one DeckProvider
// so the CTA and the editor pane share the form state.
export default function DeckView() {
  return (
    <DeckProvider>
      <div className={styles.deck}>
        <DeckHero />
        <DeckDock />
      </div>
    </DeckProvider>
  );
}
