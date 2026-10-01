#!/usr/bin/env bash
# 把设计稿 HTML 渲染成 PNG（412×915 dp，2 倍像素密度）。需要 google-chrome 与 Noto Sans CJK 字体；图标字体来自 Google Fonts（需联网）。
# 用法：docs/design/v03/render.sh [输出目录]（默认 build/design/v03）
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
out="${1:-$here/../../../build/design/v03}"
mkdir -p "$out"
tmp="$(mktemp -d)"
cp "$here/common.css" "$tmp/"
for f in "$here"/[0-9]*.html; do
  name="$(basename "$f" .html)"
  # 用公共头部替换 <!--INC-->
  awk -v inc="$here/_head.inc" '/<!--INC-->/{while((getline l < inc)>0) print l; next} {print}' "$f" > "$tmp/$name.html"
  google-chrome --headless=new --no-sandbox --disable-gpu --hide-scrollbars --force-device-scale-factor=2 \
    --virtual-time-budget=10000 --window-size=412,915 --screenshot="$out/$name.png" "file://$tmp/$name.html" >/dev/null 2>&1
  echo "$out/$name.png"
done
rm -rf "$tmp"
