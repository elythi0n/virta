import '../../ui-kit/tokens.css';
import '@fontsource-variable/geist/wght.css';
import '@fontsource-variable/geist-mono/wght.css';
import 'dockview-core/dist/styles/dockview.css';
import './app.css';
import './dock-theme.css';

import { mount } from 'svelte';
import App from './App.svelte';

const target = document.getElementById('app');
if (!target) throw new Error('missing #app mount target');

mount(App, { target });
