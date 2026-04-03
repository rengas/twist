#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ICONS_DIR="$PROJECT_ROOT/public/icons"

SIZES=(16 24 32 48 64 96 128 192 256 384 512)

# Check for available SVG-to-PNG conversion tool
if command -v rsvg-convert &>/dev/null; then
  TOOL="rsvg-convert"
elif command -v inkscape &>/dev/null; then
  TOOL="inkscape"
elif command -v npx &>/dev/null; then
  TOOL="sharp-cli"
else
  echo "ERROR: No SVG-to-PNG tool found. Install librsvg (brew install librsvg), inkscape, or sharp-cli (npm)." >&2
  exit 1
fi

echo "Using tool: $TOOL"
echo ""

convert_svg() {
  local input="$1"
  local output="$2"
  local size="$3"

  case "$TOOL" in
    rsvg-convert)
      rsvg-convert -w "$size" -h "$size" "$input" -o "$output"
      ;;
    inkscape)
      inkscape "$input" --export-type=png --export-width="$size" --export-filename="$output" 2>/dev/null
      ;;
    sharp-cli)
      npx sharp-cli -i "$input" -o "$output" --width "$size" --height "$size"
      ;;
  esac
}

FAIL=0

for size in "${SIZES[@]}"; do
  # With background
  OUTPUT="$ICONS_DIR/vessel-icon-${size}.png"
  if convert_svg "$ICONS_DIR/vessel-icon.svg" "$OUTPUT" "$size"; then
    FILE_SIZE=$(wc -c < "$OUTPUT" | tr -d ' ')
    echo "  ✓ vessel-icon-${size}.png (${FILE_SIZE} bytes)"
  else
    echo "  ✗ vessel-icon-${size}.png FAILED" >&2
    FAIL=1
  fi

  # Transparent
  OUTPUT="$ICONS_DIR/vessel-icon-transparent-${size}.png"
  if convert_svg "$ICONS_DIR/vessel-icon-transparent.svg" "$OUTPUT" "$size"; then
    FILE_SIZE=$(wc -c < "$OUTPUT" | tr -d ' ')
    echo "  ✓ vessel-icon-transparent-${size}.png (${FILE_SIZE} bytes)"
  else
    echo "  ✗ vessel-icon-transparent-${size}.png FAILED" >&2
    FAIL=1
  fi
done

echo ""
TOTAL=$(find "$ICONS_DIR" -name '*.png' | wc -l | tr -d ' ')
echo "Generated $TOTAL PNG files in $ICONS_DIR"

if [ "$FAIL" -ne 0 ]; then
  echo "ERROR: Some conversions failed." >&2
  exit 1
fi

echo "Done."
