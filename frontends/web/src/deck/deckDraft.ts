// Draft persistence for the Deck form: the user's in-progress edits survive view-switches and
// reloads, so opening Deck never wipes work. Cleared after a successful sync so the next open
// reflects the server state, not a stale local draft.

const KEY = 'virta.deck.draft.v1';

export interface DeckDraft {
  title: string;
  tagsInput: string;
  selectedChannels: string[];
  selectedCategoryName: string;
  categoryIds: Record<string, string>; // platform -> category id
}

export function loadDeckDraft(): DeckDraft | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const v = JSON.parse(raw) as DeckDraft;
    // Lightweight shape check: missing fields default to empty, but a totally wrong shape
    // (e.g. someone else wrote here) yields null so we fall back to a clean prefill.
    if (typeof v !== 'object' || v === null) return null;
    return {
      title: typeof v.title === 'string' ? v.title : '',
      tagsInput: typeof v.tagsInput === 'string' ? v.tagsInput : '',
      selectedChannels: Array.isArray(v.selectedChannels) ? v.selectedChannels.filter((c) => typeof c === 'string') : [],
      selectedCategoryName: typeof v.selectedCategoryName === 'string' ? v.selectedCategoryName : '',
      categoryIds: typeof v.categoryIds === 'object' && v.categoryIds !== null ? (v.categoryIds as Record<string, string>) : {},
    };
  } catch {
    return null;
  }
}

export function saveDeckDraft(draft: DeckDraft): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(draft));
  } catch {
    // storage unavailable (private mode); the draft just won't survive a reload
  }
}

export function clearDeckDraft(): void {
  try {
    localStorage.removeItem(KEY);
  } catch {
    // ignore
  }
}

export function isEmptyDraft(d: DeckDraft | null): boolean {
  if (!d) return true;
  return (
    !d.title.trim() &&
    !d.tagsInput.trim() &&
    !d.selectedCategoryName.trim() &&
    Object.keys(d.categoryIds).length === 0
  );
}
