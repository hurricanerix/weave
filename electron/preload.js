// Preload script for Electron renderer process.
//
// Preload scripts run in a privileged context and can expose APIs to the
// web page via contextBridge. This file is currently a placeholder since
// all application logic is served from the Go HTTP server at localhost:19600.
//
// If future requirements need IPC communication between the Electron main
// process and the renderer (web content), implement it here using:
//   const { contextBridge, ipcRenderer } = require('electron');
//
// For now, specifying this file in BrowserWindow webPreferences follows
// Electron security best practices even when not actively used.
