# build/

Despite the name, this folder is mostly hand-written packaging files, not build
output. Wails requires it (`build:dir` in `apps/desktop/wails.json` points here).

- `appicon.png`: source icon for every platform. A copy lives in `apps/desktop/`
  because `//go:embed` cannot reach files outside its own package directory.
- `linux/`: `.desktop` entry, AppStream metadata, and `install.sh` for the
  release tarball.
- `windows/`: icon, manifest, version info, and the NSIS installer script.
- `bin/`: compiled binaries. The only generated part, and ignored by git.
