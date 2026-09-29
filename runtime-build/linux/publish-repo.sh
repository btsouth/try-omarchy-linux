#!/usr/bin/env bash
# Sign a build into the Try Omarchy Flatpak repository.
#   publish-repo.sh BUILD_DIR SITE_DIR RELEASE_DIR < SECRET_KEY
# BUILD_DIR is build-flatpak.sh's output. SITE_DIR receives what the site
# serves: repo/, the one-click .flatpakref, the .flatpakrepo and the landing
# page. It may hold the previous site, whose history then provides update
# deltas; a new directory starts a fresh repository, which clients update from
# just as well. RELEASE_DIR receives the release assets: a bundle that installs
# the repository as its update source, the .flatpakref, the site as
# flatpak-site.tar.gz for the Pages workflow, and SHA256SUMS. The secret key is
# read from standard input into a keyring in memory and written nowhere else.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
usage='usage: publish-repo.sh BUILD_DIR SITE_DIR RELEASE_DIR < SECRET_KEY'
build=$(cd "${1:?$usage}" && pwd)
mkdir -p "${2:?$usage}" "${3:?$usage}"
site=$(cd "$2" && pwd)
release=$(cd "$3" && pwd)
[[ -z $(ls -A "$release") ]] || { echo "$release must be empty" >&2; exit 2; }
[[ -d $build/repo ]] || { echo "No repository in $build; run build-flatpak.sh first" >&2; exit 2; }
[[ ! -t 0 ]] || { echo 'Pipe the secret signing key to standard input' >&2; exit 2; }
image=ghcr.io/flathub-infra/flatpak-github-actions@sha256:78d969b18225ae107ca29497bf09c4a3f791e5f1838a9a912a529d0024fb6210
docker run --rm -i --network=none --tmpfs /keys:mode=0700 \
  -v "$build":/build -v "$site":/site -v "$release":/release -v "$here":/linux:ro "$image" bash -c '
  set -euo pipefail
  export GNUPGHOME=/keys
  gpg --batch --quiet --import 2>/dev/null
  key=$(gpg --batch --with-colons --list-secret-keys | awk -F: "/^fpr/{print \$10; exit}")
  public=$(gpg --batch --with-colons --show-keys /linux/try-omarchy-repo.gpg | awk -F: "/^fpr/{print \$10; exit}")
  [[ -n $key && $key == "$public" ]] || { echo "The key on standard input is not the one in try-omarchy-repo.gpg" >&2; exit 2; }
  gpg --batch --quiet --import /linux/try-omarchy-repo.gpg 2>/dev/null
  source /linux/repository.env
  app=com.tryomarchy.TryOmarchy
  [[ -f /site/repo/config ]] || ostree init --mode=archive-z2 --repo=/site/repo
  flatpak build-commit-from --src-repo=/build/repo --gpg-sign="$key" --gpg-homedir=/keys \
    --no-update-summary /site/repo "app/$app/x86_64/$BRANCH"
  flatpak build-update-repo --gpg-sign="$key" --gpg-homedir=/keys \
    --title="$TITLE" --comment="$COMMENT" --homepage="$HOMEPAGE" --icon="${SITE_URL}icon.svg" \
    --default-branch="$BRANCH" --gpg-import=/linux/try-omarchy-repo.gpg \
    --generate-static-deltas --prune --prune-depth=3 /site/repo
  flatpak build-bundle --repo-url="$REPO_URL" --runtime-repo="$RUNTIME_REPO" \
    --gpg-keys=/linux/try-omarchy-repo.gpg /site/repo "/release/$app.flatpak" "$app" "$BRANCH"
  gpgkey=$(base64 -w0 /linux/try-omarchy-repo.gpg)
  cat > "/site/$app.flatpakref" <<REF
[Flatpak Ref]
Name=$app
Branch=$BRANCH
Title=$TITLE
Comment=$COMMENT
Url=$REPO_URL
SuggestRemoteName=$REMOTE_NAME
Homepage=$HOMEPAGE
Icon=${SITE_URL}icon.svg
RuntimeRepo=$RUNTIME_REPO
IsRuntime=false
GPGKey=$gpgkey
REF
  cat > "/site/$REMOTE_NAME.flatpakrepo" <<REPO
[Flatpak Repo]
Title=$TITLE
Comment=$COMMENT
Url=$REPO_URL
Homepage=$HOMEPAGE
Icon=${SITE_URL}icon.svg
DefaultBranch=$BRANCH
GPGKey=$gpgkey
REPO
  install -m644 /linux/site/index.html /site/index.html
  install -m644 /linux/site/icon.svg /site/icon.svg
  touch /site/.nojekyll
  ostree --repo=/site/repo rev-parse "app/$app/x86_64/$BRANCH" > /site/commit.txt
  cp "/site/$app.flatpakref" /release/
  tar -C /site --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - . | gzip -n > /release/flatpak-site.tar.gz
  chown -R '"$(id -u):$(id -g)"' /site /release'
(cd "$release" && sha256sum com.tryomarchy.TryOmarchy.flatpak com.tryomarchy.TryOmarchy.flatpakref flatpak-site.tar.gz > SHA256SUMS)
echo "commit: $(cat "$site/commit.txt")"
cat "$release/SHA256SUMS"
