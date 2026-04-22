#!/usr/bin/env bash
# cut-release.sh — cut a release branch for Isley.
#
# Usage:
#   scripts/cut-release.sh <version>
#
# Given a target version (strict SemVer, e.g. 0.1.42 or 1.0.0-rc.1), this
# script:
#   1. Validates the version string.
#   2. Confirms the working tree is clean and we're on `dev`.
#   3. Creates a release/v<version> branch off the current HEAD.
#   4. Writes <version> into the VERSION file.
#   5. Rewrites CHANGELOG.md: renames `## [Unreleased]` to
#      `## [<version>] - <YYYY-MM-DD>` and prepends a fresh empty Unreleased
#      block with the standard subsections.
#   6. Stages both files and opens an editor on the commit so the maintainer
#      can review the curated changelog entry before committing.
#
# After the commit, push the branch and open a PR against `main`. The GitHub
# release workflow will extract the new section from CHANGELOG.md and use it
# as the release body.

set -euo pipefail

die() {
  echo "error: $*" >&2
  exit 1
}

[ "$#" -eq 1 ] || die "usage: $(basename "$0") <version>"

VERSION="$1"
SEMVER_RE='^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'

[[ "$VERSION" =~ $SEMVER_RE ]] \
  || die "version '$VERSION' is not strict SemVer (MAJOR.MINOR.PATCH[-prerelease][+build])"

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

[ -f VERSION ]      || die "VERSION file not found at $REPO_ROOT/VERSION"
[ -f CHANGELOG.md ] || die "CHANGELOG.md not found at $REPO_ROOT/CHANGELOG.md"

# Only allow clean working tree.
if ! git diff --quiet || ! git diff --cached --quiet; then
  die "working tree is not clean — commit or stash first"
fi

CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [ "$CURRENT_BRANCH" != "dev" ]; then
  echo "warning: not on dev (currently on '$CURRENT_BRANCH'). Press enter to continue, Ctrl-C to abort." >&2
  read -r _
fi

# Refuse if the tag already exists locally or on any remote.
TAG="v$VERSION"
if git rev-parse "$TAG" >/dev/null 2>&1; then
  die "tag $TAG already exists locally — pick a new version"
fi
for remote in $(git remote); do
  if git ls-remote --tags "$remote" "$TAG" 2>/dev/null | grep -q .; then
    die "tag $TAG already exists on remote '$remote' — pick a new version"
  fi
done

# Refuse if CHANGELOG has no Unreleased section.
if ! grep -q '^## \[Unreleased\]' CHANGELOG.md; then
  die "CHANGELOG.md has no '## [Unreleased]' section to stamp"
fi

# Refuse if a section for this version already exists.
if grep -qE "^## \[$VERSION\]" CHANGELOG.md; then
  die "CHANGELOG.md already has a section for [$VERSION]"
fi

BRANCH="release/v$VERSION"
if git show-ref --verify --quiet "refs/heads/$BRANCH"; then
  die "branch $BRANCH already exists — delete it or pick a different version"
fi

echo "Cutting release $VERSION on branch $BRANCH..."
git checkout -b "$BRANCH"

# Write the new VERSION
printf '%s\n' "$VERSION" > VERSION

# Stamp CHANGELOG: replace the first '## [Unreleased]' line with a fresh
# empty Unreleased block followed by the stamped version heading.
TODAY="$(date -u +%Y-%m-%d)"

python3 - "$VERSION" "$TODAY" <<'PY'
import re
import sys
from pathlib import Path

version, today = sys.argv[1], sys.argv[2]
path = Path("CHANGELOG.md")
text = path.read_text()

unreleased_block = "\n".join([
    "## [Unreleased]",
    "",
    "### Added",
    "",
    "### Changed",
    "",
    "### Deprecated",
    "",
    "### Removed",
    "",
    "### Fixed",
    "",
    "### Security",
    "",
    f"## [{version}] - {today}",
])

new_text, count = re.subn(
    r"^## \[Unreleased\][ \t]*$",
    unreleased_block,
    text,
    count=1,
    flags=re.MULTILINE,
)
if count != 1:
    sys.exit("cut-release.sh: failed to find a single '## [Unreleased]' heading")

path.write_text(new_text)
PY

git add VERSION CHANGELOG.md

cat <<EOF

Release branch prepared: $BRANCH
  VERSION       -> $VERSION
  CHANGELOG.md  -> stamped [$VERSION] - $TODAY, fresh [Unreleased] seeded

Next steps:
  1. Review CHANGELOG.md and tidy the [$VERSION] section.
  2. git commit -m "Release v$VERSION"
  3. git push -u origin $BRANCH         # (or whichever remote hosts main)
  4. Open a PR into main.

The GitHub release workflow will tag, build, sign, and publish once the PR
merges to main.
EOF
