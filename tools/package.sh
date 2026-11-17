#!/bin/bash
# Build OpLarik.exe and the release zips into dist/. Version comes from main.go.
set -e
cd "$(dirname "$0")/.."
V=$(grep -oP 'const appVersion = "\K[^"]+' main.go)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-H windowsgui -s -w" -o dist/OpLarik.exe .
(cd dist && sha256sum OpLarik.exe > OpLarik.exe.sha256)   # GitHub release assets: OpLarik.exe + OpLarik.exe.sha256
rm -f dist/Larik-*.zip dist/OpLarik-*.zip dist/Larik.exe dist/Larik.exe.sha256
(cd dist && zip -q -X "OpLarik-$V-win-x64.zip" OpLarik.exe README.md THIRD-PARTY-NOTICES.txt)
zip -q -X -r "dist/OpLarik-$V-source.zip" go.mod *.go catalog.json BUILD.md CHANGELOG.md \
  rsrc/larik.rc rsrc/larik.manifest rsrc/larik.ico rsrc_windows_amd64.syso overlay \
  tools/catalog.py tools/icons.js tools/render_icons.py tools/atlas.py tools/package.sh tools/demo.sh tools/flow107.py tools/flow108.py tools/flow109.py tools/flow111.py tools/flow112.py tools/themes_light.py tools/catalog_i18n web \
  -x '*.res'
ls -la dist/*.zip
