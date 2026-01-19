---
paths:
  - "electron/**/*.js"
  - "electron/**/*.ts"
  - "electron/**/*.json"
  - "electron/**/*.html"
---

# Electron Rules for weave

## Philosophy

Electron is the container, not the application. Keep it thin. Every line of code needs to justify why it can't live in the Go backend instead.

**Priorities:**
1. Security (non-negotiable)
2. Native feel (why we use Electron)
3. Simplicity (minimal code)
4. Performance (don't bloat startup)

## Security Requirements

These are not suggestions. They are requirements.

### Window Creation

Every BrowserWindow must have these settings:

```javascript
const win = new BrowserWindow({
    webPreferences: {
        contextIsolation: true,      // Required
        nodeIntegration: false,      // Required
        sandbox: true,               // Required
        preload: path.join(__dirname, 'preload.js')
    }
});
```

**Never disable:**
- `contextIsolation` - Prevents renderer from accessing Node.js
- `nodeIntegration: false` - Keeps Node.js out of renderer
- `sandbox` - OS-level process isolation
- `webSecurity` - Same-origin policy protection

### Preload Scripts

Preload scripts are the **only** bridge between main and renderer.

```javascript
// GOOD - Minimal, explicit API
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('weave', {
    generate: (prompt) => ipcRenderer.invoke('weave:generate', prompt),
    cancelGeneration: () => ipcRenderer.invoke('weave:cancel'),
    onProgress: (callback) => {
        ipcRenderer.on('weave:progress', (event, data) => callback(data));
    }
});
```

```javascript
// BAD - Exposing raw Electron APIs
contextBridge.exposeInMainWorld('electron', {
    ipcRenderer: ipcRenderer,  // Never
    shell: require('electron').shell,  // Never
    fs: require('fs')  // Absolutely never
});
```

**Rules:**
- Expose functions, not modules
- Validate inputs in the main process
- Use `invoke` for request/response, `on` for events
- Prefix IPC channels with `weave:` for clarity

### Remote Module

**Never use the remote module.** It's deprecated and insecure.

```javascript
// BAD
const { BrowserWindow } = require('@electron/remote');

// GOOD - Use IPC instead
ipcRenderer.invoke('weave:open-dialog');
```

## IPC Patterns

### Channel Naming

Use namespaced channel names:

```javascript
// GOOD
'weave:generate'
'weave:cancel'
'weave:progress'
'weave:settings:get'
'weave:settings:set'

// BAD
'generate'
'doThing'
'msg'
```

### Request/Response (invoke/handle)

For operations that return a result:

```javascript
// Main process
ipcMain.handle('weave:generate', async (event, prompt) => {
    // Validate input
    if (typeof prompt !== 'string' || prompt.length === 0) {
        throw new Error('Invalid prompt');
    }
    // Do work
    return await generateImage(prompt);
});

// Renderer (via preload)
const result = await window.weave.generate(prompt);
```

### Events (send/on)

For one-way notifications:

```javascript
// Main process
win.webContents.send('weave:progress', { percent: 50 });

// Renderer (via preload)
window.weave.onProgress((data) => {
    console.log(data.percent);
});
```

### Input Validation

**Always validate in the main process:**

```javascript
ipcMain.handle('weave:generate', async (event, prompt) => {
    // Validate type
    if (typeof prompt !== 'string') {
        throw new Error('Prompt must be a string');
    }
    // Validate length
    if (prompt.length === 0 || prompt.length > 2048) {
        throw new Error('Prompt must be 1-2048 characters');
    }
    // Now safe to use
    return await generateImage(prompt);
});
```

## Code Style

### Formatting

- Use Prettier with project config
- 2 space indentation (Electron/Node convention)
- Single quotes for strings
- Semicolons required

### Naming

```javascript
// Files: kebab-case
main.js
preload.js
window-manager.js

// Functions: camelCase
function createMainWindow() {}
function handleGenerate() {}

// Constants: SCREAMING_SNAKE_CASE
const DEFAULT_WIDTH = 1200;
const IPC_GENERATE = 'weave:generate';

// Classes: PascalCase (rare in Electron)
class WindowManager {}
```

### Error Handling

```javascript
// GOOD - Specific error handling
ipcMain.handle('weave:generate', async (event, prompt) => {
    try {
        return await generateImage(prompt);
    } catch (err) {
        // Log full error for debugging
        console.error('Generation failed:', err);
        // Return safe error to renderer
        throw new Error('Generation failed. Check logs for details.');
    }
});

// BAD - Exposing internal errors
ipcMain.handle('weave:generate', async (event, prompt) => {
    return await generateImage(prompt);  // Raw errors leak to renderer
});
```

## Platform Handling

### Clean Platform Checks

```javascript
// GOOD - Centralized platform logic
const isMac = process.platform === 'darwin';
const isWindows = process.platform === 'win32';
const isLinux = process.platform === 'linux';

const menu = Menu.buildFromTemplate([
    ...(isMac ? [{ role: 'appMenu' }] : []),
    { role: 'fileMenu' },
    { role: 'editMenu' },
]);
```

```javascript
// BAD - Scattered platform checks
if (process.platform === 'darwin') {
    // 50 lines of macOS code
} else if (process.platform === 'win32') {
    // 50 lines of Windows code
} else {
    // 50 lines of Linux code
}
```

### Keyboard Shortcuts

```javascript
// GOOD - Platform-appropriate modifier
const modifier = process.platform === 'darwin' ? 'Cmd' : 'Ctrl';
const shortcut = `${modifier}+Q`;

// Or use Electron's built-in handling
{ role: 'quit' }  // Automatically uses Cmd+Q on Mac, Ctrl+Q elsewhere
```

## Window Management

### Single Window (Current)

```javascript
let mainWindow = null;

function createMainWindow() {
    if (mainWindow) {
        mainWindow.focus();
        return;
    }

    mainWindow = new BrowserWindow({
        width: 1200,
        height: 800,
        webPreferences: {
            contextIsolation: true,
            nodeIntegration: false,
            sandbox: true,
            preload: path.join(__dirname, 'preload.js')
        }
    });

    mainWindow.on('closed', () => {
        mainWindow = null;
    });

    mainWindow.loadURL('http://localhost:PORT');
}
```

### Window State Persistence

If saving window position/size:

```javascript
// Use electron-store or similar
const store = new Store();

function getWindowState() {
    return store.get('windowState', {
        width: 1200,
        height: 800
    });
}

function saveWindowState(win) {
    store.set('windowState', win.getBounds());
}
```

## Dependencies

### Be Conservative

Every dependency is:
- A security risk (supply chain)
- A maintenance burden (updates)
- Startup time (more to load)

**Ask:**
- Can this be done with Electron's built-in APIs?
- Is this dependency actively maintained?
- Do we really need it?

### Acceptable Dependencies

- `electron-store` - Settings persistence
- `electron-updater` - Auto-updates (when needed)

### Avoid

- UI frameworks in main process
- Heavy utility libraries (lodash, etc.)
- Anything that duplicates Electron APIs

## Testing

### Main Process Testing

```javascript
// test/main.test.js
const { ipcMain } = require('electron');

describe('IPC handlers', () => {
    test('weave:generate validates prompt', async () => {
        // Test that empty prompts are rejected
        await expect(
            invokeHandler('weave:generate', '')
        ).rejects.toThrow('Prompt must be');
    });
});
```

### Preload Testing

Preload scripts are hard to test in isolation. Focus on:
- Testing the main process handlers thoroughly
- E2E tests for critical paths

## Temporary Files

**Always use `./tmp/` (project-local), never system temp directories.**

```javascript
const fs = require('fs');
const path = require('path');

// ✅ GOOD - Project-local temp directory
const tmpDir = path.join(__dirname, '..', 'tmp');
fs.mkdirSync(tmpDir, { recursive: true });
const tmpFile = path.join(tmpDir, 'workfile.tmp');

// ❌ BAD - System temp directory
const os = require('os');
const tmpFile = path.join(os.tmpdir(), 'workfile.tmp');  // Uses /tmp
```

**Why:**
- Keeps test artifacts contained to the project
- Easier cleanup
- Avoids permission issues in sandboxed environments (Flatpak)
- Project `.gitignore` already ignores `./tmp/`

## Anti-Patterns

**Avoid:**
- Business logic in Electron (belongs in Go backend)
- Storing secrets in renderer-accessible storage
- Using `remote` module
- Disabling security for "convenience"
- Synchronous IPC (`sendSync`)
- Direct file system access from renderer
- Loading remote content without validation

## Debugging

### DevTools

```javascript
// Development only
if (process.env.NODE_ENV === 'development') {
    mainWindow.webContents.openDevTools();
}
```

### Logging

```javascript
// Use console with prefixes
console.log('[main]', 'Window created');
console.error('[main]', 'Failed to start backend:', err);
```

## When in Doubt

1. Can this live in the Go backend instead?
2. Is this the minimal solution?
3. Are security settings intact?
4. Does this work on all target platforms?

If you're adding significant code to Electron, question whether it belongs there.
