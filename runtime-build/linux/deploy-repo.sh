#!/usr/bin/env bash
# Deploy a Linux app release's signed repository site to flatpak.tryomarchy.com.
#   deploy-repo.sh TAG
# Downloads flatpak-site.tar.gz from the GitHub release, checks it against the
# release's SHA256SUMS and uploads it to the Cloudflare Pages project in
# repository.env with wrangler, which must be logged in to the account that
# owns tryomarchy.com. Nothing is built or signed here.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
tag=${1:?usage: deploy-repo.sh TAG}
[[ $tag =~ ^linux-app-v[0-9]+\.[0-9]+\.[0-9]+(-preview\.[0-9]+)?$ ]] || { echo "Not an app release tag: $tag" >&2; exit 2; }
source "$here/repository.env"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
gh release download "$tag" --repo "$GITHUB_REPO" --dir "$work" --pattern flatpak-site.tar.gz --pattern SHA256SUMS
(cd "$work" && grep ' flatpak-site.tar.gz$' SHA256SUMS | sha256sum -c -)
mkdir "$work/site"
tar -C "$work/site" -xzf "$work/flatpak-site.tar.gz"
for f in repo/config repo/summary repo/summary.sig com.tryomarchy.TryOmarchy.flatpakref 404.html; do
  [[ -f $work/site/$f ]] || { echo "The release site lacks $f" >&2; exit 1; }
done
wrangler pages deploy "$work/site" --project-name "$PAGES_PROJECT" --branch main \
  --commit-message "$tag" --commit-dirty=true
curl -fsS "${REPO_URL}config" >/dev/null && echo "Deployed $tag to $REPO_URL"
