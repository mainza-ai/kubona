// Vite's `?raw` suffix imports a file verbatim as a string. The browser
// component suite reads shipped CSS source this way because a real Chromium
// page has no `node:fs`; the asset is inlined at bundle time.
declare module '*?raw' {
  const content: string;
  export default content;
}
