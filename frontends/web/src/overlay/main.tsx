import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import Overlay from './Overlay';
import '@virta/ui-kit/tokens.css';

const root = document.getElementById('overlay');
if (root) {
  createRoot(root).render(
    <StrictMode>
      <Overlay />
    </StrictMode>,
  );
}
