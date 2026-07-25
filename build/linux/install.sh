#!/usr/bin/env bash
#
# Installs Kaa Rada for the current user. Run this from inside the
# extracted release tarball.
#
# Everything goes under $HOME by default, so no root is required and nothing
# outside the user's own directories is touched. Pass --system to install to
# /usr/local for all users instead.
#
# The application stores its settings and recordings in the per-user config
# directory, never beside the binary, so the install location does not need to
# be writable after installation.

set -euo pipefail

readonly APPLICATION_NAME="kaa-rada"
readonly COMMAND_LINE_NAME="kaa-rada-cli"

source_directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly source_directory

install_scope="user"
if [[ "${1:-}" == "--system" ]]; then
  install_scope="system"
elif [[ -n "${1:-}" ]]; then
  echo "Usage: $0 [--system]" >&2
  exit 2
fi

if [[ "$install_scope" == "system" ]]; then
  binary_directory="/usr/local/bin"
  desktop_directory="/usr/local/share/applications"
  icon_directory="/usr/local/share/icons/hicolor/256x256/apps"
  metainfo_directory="/usr/local/share/metainfo"
  if [[ "$EUID" -ne 0 ]]; then
    echo "A system install writes to /usr/local. Re-run with: sudo $0 --system" >&2
    exit 1
  fi
else
  binary_directory="$HOME/.local/bin"
  desktop_directory="$HOME/.local/share/applications"
  icon_directory="$HOME/.local/share/icons/hicolor/256x256/apps"
  metainfo_directory="$HOME/.local/share/metainfo"
fi

echo "Installing Kaa Rada (${install_scope} scope)..."

mkdir -p "$binary_directory" "$desktop_directory" "$icon_directory" "$metainfo_directory"

install -m 0755 "${source_directory}/${APPLICATION_NAME}" "${binary_directory}/${APPLICATION_NAME}"
if [[ -f "${source_directory}/${COMMAND_LINE_NAME}" ]]; then
  install -m 0755 "${source_directory}/${COMMAND_LINE_NAME}" "${binary_directory}/${COMMAND_LINE_NAME}"
fi

install -m 0644 "${source_directory}/${APPLICATION_NAME}.desktop" "${desktop_directory}/${APPLICATION_NAME}.desktop"

if [[ -f "${source_directory}/${APPLICATION_NAME}.metainfo.xml" ]]; then
  install -m 0644 "${source_directory}/${APPLICATION_NAME}.metainfo.xml" \
    "${metainfo_directory}/${APPLICATION_NAME}.metainfo.xml"
fi

if [[ -f "${source_directory}/appicon.png" ]]; then
  install -m 0644 "${source_directory}/appicon.png" "${icon_directory}/${APPLICATION_NAME}.png"
fi

# Menus cache .desktop files; without this the entry may not appear until the
# next login. Both tools are optional, hence the guards.
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "$desktop_directory" >/dev/null 2>&1 || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -f -t "$(dirname "$(dirname "$icon_directory")")" >/dev/null 2>&1 || true
fi

echo "Installed:"
echo "  ${binary_directory}/${APPLICATION_NAME}"
echo "  ${desktop_directory}/${APPLICATION_NAME}.desktop"

# A user-scope install is useless if ~/.local/bin is not on PATH, and on some
# distributions it is not.
if [[ ":$PATH:" != *":${binary_directory}:"* ]]; then
  echo ""
  echo "Note: ${binary_directory} is not on your PATH."
  echo "Add this to your shell profile:"
  echo "  export PATH=\"${binary_directory}:\$PATH\""
fi

echo ""
echo "Kaa Rada needs OpenCV 4.6 at run time. On Ubuntu 24.04:"
echo "  sudo apt install libopencv-dev"
