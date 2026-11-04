#!/usr/bin/env node
// Runs the snoop binary from the platform package npm installed beside this
// one (@jzills/snoop-cli-<platform>-<arch>), passing the arguments through.
"use strict";

const { spawnSync } = require("node:child_process");

const pkg = `@jzills/snoop-cli-${process.platform}-${process.arch}`;
const exe = process.platform === "win32" ? "snoop.exe" : "snoop";

let bin;
try {
  bin = require.resolve(`${pkg}/bin/${exe}`);
} catch {
  console.error(
    `snoop: no build for ${process.platform}/${process.arch} was installed (${pkg}). ` +
      "Builds exist for Linux, macOS and Windows on x64 and arm64; if npm skipped " +
      "optional dependencies, reinstall without --omit=optional, or see " +
      "https://jzills.github.io/snoop/docs/install/",
  );
  process.exit(1);
}

const run = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
if (run.error) {
  console.error(`snoop: ${run.error.message}`);
  process.exit(1);
}
if (run.signal) {
  process.kill(process.pid, run.signal);
}
process.exit(run.status ?? 1);
