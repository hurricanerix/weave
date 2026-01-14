const { app, BrowserWindow, dialog } = require('electron');
const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');

// Configuration constants
const PORT = 19600;
const SERVER_URL = `http://localhost:${PORT}`;
const POLL_INTERVAL_MS = 100;
const STARTUP_TIMEOUT_MS = 10000;
const MIN_SPLASH_DURATION_MS = 1500;

// Global reference to Go server child process
let goProcess = null;

// Flag to prevent re-entrant shutdown
let shuttingDown = false;

/**
 * Spawns the Go server binary as a child process.
 * Returns a Promise that resolves with the child process reference,
 * or rejects if spawning fails.
 */
function spawnGoServer() {
  return new Promise((resolve, reject) => {
    // Binary path is fixed to project location for security (no env var injection)
    const binaryPath = path.join(__dirname, '..', 'bin', 'weave');

    // Check if binary exists before spawning
    if (!fs.existsSync(binaryPath)) {
      const err = new Error(`Binary not found at ${binaryPath}`);
      err.binaryPath = binaryPath;
      err.isBinaryMissing = true;
      reject(err);
      return;
    }

    console.log(`[electron] Spawning Go server: ${binaryPath}`);

    // Spawn Go process with port flag
    const child = spawn(binaryPath, [`--port=${PORT}`]);

    // Track if we've already resolved/rejected
    let settled = false;

    // Pipe stdout to console with prefix
    child.stdout.on('data', (data) => {
      const lines = data.toString().trim().split('\n').filter(Boolean);
      lines.forEach(line => console.log(`[weave] ${line}`));
    });

    // Pipe stderr to console with prefix
    child.stderr.on('data', (data) => {
      const lines = data.toString().trim().split('\n').filter(Boolean);
      lines.forEach(line => console.error(`[weave] ${line}`));
    });

    // Log when process exits
    child.on('exit', (code, signal) => {
      console.log(`[electron] Go server exited with code ${code}, signal ${signal}`);
    });

    // Handle spawn errors (e.g., binary not found)
    child.on('error', (err) => {
      console.error(`[electron] Failed to spawn Go server: ${err.message}`);
      if (!settled) {
        settled = true;
        reject(err);
      }
    });

    // If we get any stdout/stderr, the process spawned successfully
    // Use a short delay to ensure error events fire first if spawn failed
    setTimeout(() => {
      if (!settled) {
        settled = true;
        resolve(child);
      }
    }, 50);
  });
}

/**
 * Polls the /ready endpoint until the server responds successfully.
 * Returns a Promise that resolves when server is ready, or rejects on timeout.
 */
async function waitForReady() {
  const startTime = Date.now();

  while (true) {
    const elapsed = Date.now() - startTime;

    // Check for timeout
    if (elapsed >= STARTUP_TIMEOUT_MS) {
      throw new Error(`Go server failed to start within ${STARTUP_TIMEOUT_MS}ms`);
    }

    try {
      // Use native fetch API (available in Node.js 18+)
      const response = await fetch(`${SERVER_URL}/ready`);

      if (response.ok) {
        try {
          const data = await response.json();
          console.log('[electron] Server ready');
          return;
        } catch (parseErr) {
          // Invalid JSON response, continue polling
        }
      }
    } catch (err) {
      // Server not ready yet, continue polling
      // Only log progress in final 20% of timeout to reduce noise
      if (elapsed > STARTUP_TIMEOUT_MS * 0.8) {
        console.log(`[electron] Waiting for server... (${elapsed}ms elapsed)`);
      }
    }

    // Wait before next poll
    await new Promise(r => setTimeout(r, POLL_INTERVAL_MS));
  }
}

/**
 * Kills the Go server process if it exists.
 * Sends SIGTERM for graceful shutdown.
 */
function killGoServer() {
  if (goProcess) {
    console.log('[electron] Killing Go server...');
    goProcess.kill('SIGTERM');
    goProcess = null;
  }
}

/**
 * Shows an error dialog to the user with actionable guidance.
 */
function showErrorDialog(title, message) {
  dialog.showErrorBox(title, message);
}

/**
 * Creates the splash screen window.
 * Returns a BrowserWindow configured to display splash.html.
 */
function createSplashWindow() {
  const splash = new BrowserWindow({
    width: 300,
    height: 200,
    frame: false,
    center: true,
    alwaysOnTop: true,
    resizable: false,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      nodeIntegration: false,
      contextIsolation: true
    }
  });

  splash.loadFile(path.join(__dirname, 'splash.html'));

  return splash;
}

/**
 * Creates the main application window.
 * Returns a BrowserWindow configured to load the Go server content.
 */
function createMainWindow() {
  const mainWindow = new BrowserWindow({
    width: 1200,
    height: 800,
    center: true,
    show: false,  // Don't show until ready-to-show event (prevents white flash)
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      nodeIntegration: false,
      contextIsolation: true
    }
  });

  // Load content from Go server, log any errors
  mainWindow.loadURL(SERVER_URL).catch(err => {
    console.error(`[electron] Failed to load main window: ${err.message}`);
  });

  return mainWindow;
}

// Main Electron lifecycle
app.whenReady().then(async () => {
  let splash = null;

  try {
    console.log('[electron] Electron ready, starting Go server...');

    // Create and show splash window
    splash = createSplashWindow();
    const splashStartTime = Date.now();

    // Spawn Go server and store reference
    goProcess = await spawnGoServer();

    // Wait for server to be ready
    await waitForReady();

    console.log('[electron] Go server is ready');

    // Calculate remaining splash duration
    const splashElapsed = Date.now() - splashStartTime;
    const remainingSplashTime = MIN_SPLASH_DURATION_MS - splashElapsed;

    // Wait for remaining splash time if needed
    if (remainingSplashTime > 0) {
      console.log(`[electron] Enforcing minimum splash duration (${remainingSplashTime}ms remaining)`);
      await new Promise(resolve => setTimeout(resolve, remainingSplashTime));
    }

    // Create main window
    const mainWindow = createMainWindow();

    // Show main window when ready, then close splash
    mainWindow.once('ready-to-show', () => {
      try {
        mainWindow.show();
        // Close splash if still open (defensive check for edge cases)
        if (splash) {
          splash.close();
          splash = null;
        }
        console.log('[electron] Main window displayed');
      } catch (err) {
        console.error(`[electron] Error showing main window: ${err.message}`);
      }
    });

  } catch (err) {
    console.error(`[electron] Startup failed: ${err.message}`);
    if (splash) {
      splash.close();
    }
    killGoServer();

    // Show user-friendly error dialog based on error type
    if (err.isBinaryMissing) {
      showErrorDialog(
        'Failed to start Weave server',
        `The weave binary was not found at:\n${err.binaryPath}\n\nPlease build the project first:\n  make weave`
      );
    } else if (err.message.includes('failed to start within')) {
      showErrorDialog(
        'Weave server failed to start',
        `The server did not respond within ${STARTUP_TIMEOUT_MS / 1000} seconds.\n\nPossible causes:\n- Port ${PORT} may already be in use\n- Check the terminal for error messages\n- Try restarting the application`
      );
    } else {
      showErrorDialog(
        'Weave startup error',
        `An error occurred during startup:\n${err.message}\n\nCheck the terminal for more details.`
      );
    }

    app.quit();
  }
});

// Graceful shutdown when all windows are closed
// Note: Uses POSIX signals (SIGTERM, SIGKILL) - Linux only
app.on('window-all-closed', () => {
  // Prevent re-entrant shutdown
  if (shuttingDown) {
    console.log('[electron] Shutdown already in progress');
    return;
  }
  shuttingDown = true;

  console.log('[electron] All windows closed, shutting down...');

  if (!goProcess) {
    console.log('[electron] No Go process to clean up');
    app.quit();
    return;
  }

  // Track if process has exited
  let processExited = false;
  let killTimeout = null;

  // Listen for process exit
  const exitHandler = (code, signal) => {
    if (!processExited) {
      processExited = true;
      clearTimeout(killTimeout);
      console.log(`[electron] Go process exited (code ${code}, signal ${signal})`);
      goProcess = null;
      app.quit();
    }
  };

  goProcess.once('exit', exitHandler);

  // Send SIGTERM for graceful shutdown
  console.log('[electron] Sending SIGTERM to Go process...');
  goProcess.kill('SIGTERM');

  // Set timeout for SIGKILL fallback (5 second grace period)
  killTimeout = setTimeout(() => {
    if (!processExited && goProcess) {
      console.warn('[electron] Graceful shutdown timeout (5000ms), sending SIGKILL');
      goProcess.kill('SIGKILL');
      // SIGKILL is non-ignorable - the exit handler will fire when process dies
    }
  }, 5000);
});
