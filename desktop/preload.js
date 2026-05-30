'use strict';

const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('qgen', {
  loadSettings: () => ipcRenderer.invoke('settings:load'),
  saveSettings: (s) => ipcRenderer.invoke('settings:save', s),
  pickFolder: () => ipcRenderer.invoke('dialog:pickFolder'),
  pickSave: (defaultName) => ipcRenderer.invoke('dialog:pickSave', defaultName),
  openFile: (p) => ipcRenderer.invoke('file:open', p),
  revealFile: (p) => ipcRenderer.invoke('file:reveal', p),
  run: (opts) => ipcRenderer.invoke('run:start', opts),
  cancel: () => ipcRenderer.invoke('run:cancel'),
  onEvent: (cb) => {
    const listener = (_e, ev) => cb(ev);
    ipcRenderer.on('run:event', listener);
    return () => ipcRenderer.removeListener('run:event', listener);
  },
});
