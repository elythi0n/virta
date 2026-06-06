import '@virta/ui-kit/tokens.css';
import '@fontsource-variable/geist/wght.css';
import '@fontsource-variable/geist-mono/wght.css';
import 'dockview/dist/styles/dockview.css';
import './app.css';
import './dock-theme.css';

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';

const target = document.getElementById('app');
if (!target) throw new Error('missing #app mount target');

createRoot(target).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
