#!/bin/sh
# Sign a macOS build so the keychain keeps trusting it across upgrades.
#
# gokey binds the stored API token to the code identity of the binary that
# stored it. Go ad-hoc signs by default, and an ad-hoc signature has no
# certificate to anchor to, so the identity falls back to the binary's cdhash -
# which is different for every build. Every upgrade would then raise the
# keychain dialog again, and a scripted run would fail until someone approved it.
#
# A certificate fixes it. Signed, the designated requirement becomes the
# identifier plus the certificate, with no cdhash in it, so any later build
# signed by the same certificate satisfies the same requirement.
#
#   ATL_SIGN_IDENTITY  the identity to sign with: a name or SHA-1 hash as
#                      `security find-identity -p codesigning` prints it.
#                      Required.
#
# Usage: scripts/sign.sh <binary>
set -eu

BINARY="${1:-}"
[ -n "$BINARY" ] || { echo "usage: $0 <binary>" >&2; exit 2; }
[ -f "$BINARY" ] || { echo "error: no such file: $BINARY" >&2; exit 2; }

# The identifier is pinned here rather than taken from the file name, because it
# is half of the designated requirement: the binary Homebrew installs and one
# built locally must present the same identity or the keychain treats them as
# different programs.
IDENTIFIER="com.branow.atl"

# Unlike a missing identity, a non-macOS host cannot be fixed with a variable,
# but it must not pass either: it would publish ad-hoc darwin binaries.
if [ "$(uname -s)" != "Darwin" ]; then
  echo "error: darwin builds can only be signed on macOS" >&2
  exit 1
fi

if [ -z "${ATL_SIGN_IDENTITY:-}" ]; then
  cat >&2 <<'EOF'
error: ATL_SIGN_IDENTITY is not set, so this build would be ad-hoc signed and
       the keychain would stop trusting it on the next upgrade.

  Pick an identity:

    security find-identity -p codesigning

  A release signs with the release certificate, which the release workflow
  imports from the MACOS_SIGN_P12 and MACOS_SIGN_P12_PASSWORD secrets.
EOF
  exit 1
fi

codesign --force --options runtime \
  --identifier "$IDENTIFIER" \
  --sign "$ATL_SIGN_IDENTITY" \
  "$BINARY"

# The requirement is what the keychain actually stores, so print it: a cdhash in
# here means the signature did not take. Some codesign versions prefix the line
# with "# " and some do not, and a pattern that misses it reads an empty
# requirement and passes everything.
requirement=$(codesign -d -r- "$BINARY" 2>/dev/null | sed -n 's/^\(# \)\{0,1\}designated => //p')
echo "signed $BINARY"
echo "  requirement: $requirement"

case "$requirement" in
  "")
    echo "error: could not read the designated requirement back from $BINARY" >&2
    exit 1
    ;;
  *cdhash*)
    echo "error: the requirement is still cdhash-based, so an upgrade will lose" >&2
    echo "       keychain access. Check that the identity is a real certificate." >&2
    exit 1
    ;;
esac
