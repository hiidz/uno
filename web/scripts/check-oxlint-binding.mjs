// Guards a resolver quirk that silently breaks `npm run lint` on Windows.
//
// oxlint ships its native binary as a set of per-platform optional deps, and
// npm has a long-standing bug where optional deps are resolved in the lockfile
// and then dropped during install: https://github.com/npm/cli/issues/4828
// On win32/x64 this hits `@oxlint/binding-win32-x64-msvc` — npm's reify step
// logs `mark deleted` for that exact package, and a plain `npm install` will
// not restore it. The other win32-only optional bindings in this project
// (@rolldown, @tailwindcss/oxide, lightningcss) happen to be unaffected.
//
// npm's own suggested remedy is to delete package-lock.json AND node_modules
// and reinstall. The targeted fix below is cheaper and doesn't churn the lock.
//
// The failure mode is quiet: oxlint exits with `Cannot find module
// '@oxlint/binding-win32-x64-msvc'`, so `npm run lint` stops working and
// nothing else complains. This prints a warning and the one-line fix.
//
// Warn, never fail: a missing binding must not block an install, and this
// script runs on every platform including CI.

import { createRequire } from 'node:module'

// Only the platform the quirk affects. Everywhere else the win32 binding is
// *supposed* to be absent, so checking for it would be noise.
if (process.platform !== 'win32' || process.arch !== 'x64') {
  process.exit(0)
}

const PKG = '@oxlint/binding-win32-x64-msvc'
const require = createRequire(import.meta.url)

try {
  require.resolve(`${PKG}/package.json`)
} catch {
  const version =
    (() => {
      try {
        return require('../package.json').optionalDependencies?.[PKG]?.replace(/^[^\d]*/, '') ?? ''
      } catch {
        return ''
      }
    })() || 'latest'

  process.emitWarning(
    `${PKG} is missing — \`npm run lint\` will fail.\n` +
      `  npm install did not place it despite package-lock.json resolving it (known npm quirk).\n` +
      `  Fix:\n` +
      `    npm pack ${PKG}@${version} --pack-destination=.\n` +
      `    tar -xzf oxlint-binding-win32-x64-msvc-${version}.tgz\n` +
      `    mv package node_modules/${PKG}`,
    'OxlintBindingWarning',
  )
}
