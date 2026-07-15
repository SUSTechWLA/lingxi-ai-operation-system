const { app, BrowserWindow } = require('electron')

const timeout = setTimeout(() => {
  console.error('electron runtime smoke timed out')
  app.exit(1)
}, 15_000)

app.whenReady()
  .then(async () => {
    const window = new BrowserWindow({
      show: false,
      webPreferences: {
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: true,
      },
    })
    await window.loadURL('data:text/html,<main id="ready">ready</main>')
    const ready = await window.webContents.executeJavaScript(
      "document.querySelector('#ready')?.textContent",
    )
    if (ready !== 'ready') {
      throw new Error(`unexpected renderer result: ${ready}`)
    }
    console.log(`electron runtime smoke passed (${process.versions.electron})`)
    window.destroy()
    clearTimeout(timeout)
    app.quit()
  })
  .catch((error) => {
    console.error(error)
    clearTimeout(timeout)
    app.exit(1)
  })
