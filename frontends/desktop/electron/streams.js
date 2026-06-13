'use strict';

// Native stream player windows — the Electron equivalent of the old Go shell's App.OpenStreamWindow.
// Loads the platform's own watch page top-level (not the embed iframe) in a separate window. With
// the inline embed now working in the main panel this is a secondary "pop out" path, kept for
// platforms without an inline player (YouTube) and as an explicit detached-player option.

const { BrowserWindow } = require('electron');

function streamPageURL(platform, slug) {
  switch (platform) {
    case 'twitch':
      return `https://www.twitch.tv/${encodeURIComponent(slug)}`;
    case 'kick':
      return `https://kick.com/${encodeURIComponent(slug)}`;
    case 'youtube':
      // /live resolves the handle's current livestream (or the channel page when offline).
      return `https://www.youtube.com/@${encodeURIComponent(String(slug).replace(/^@/, ''))}/live`;
    default:
      return null;
  }
}

function openStreamWindow(platform, slug) {
  const url = streamPageURL(platform, slug);
  if (!url) return;
  const win = new BrowserWindow({
    width: 1280,
    height: 720,
    title: `${slug} — ${platform}`,
    backgroundColor: '#0e0f12',
    autoHideMenuBar: true,
    webPreferences: {
      // Isolated, persistent session so logins/cookies for the player survive across opens, kept
      // separate from the app's own session.
      partition: 'persist:streams',
    },
  });
  win.loadURL(url);
}

module.exports = { openStreamWindow, streamPageURL };
