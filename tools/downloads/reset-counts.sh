#!/usr/bin/env bash
# Starts the download counts of MPCvibedRPC's release files again from 0, by replacing each file with its identical
# bytes. Names, links, checksums and build records stay the same; only the counts start again.
#
# It REWRITES EVERY ASSET of the releases it is given (it uploads a copy, deletes the original, renames the copy), so
# it needs `gh` signed in with write access to the repository. Run it only deliberately, from a terminal, never from
# CI. Afterwards run the `downloads` workflow by hand with "baseline" ticked (docs/maintaining.md).
#
#   bash reset-counts.sh              every public release, oldest first, the latest one last
#   bash reset-counts.sh v0.9.0       only the releases named
#
# For each file: download it, check it against GitHub's checksum, upload the same bytes under a temporary name
# (NAME.reset), check that copy's checksum, delete the old file, rename the copy, check name, checksum and count 0.
# Any surprise stops it with a message; the old file is only deleted once its identical copy is in place. A left-over
# NAME.reset from an interrupted run must be finished or removed by hand first.
set -euo pipefail
repo=CptPakundo/MPCvibedRPC
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
if [ $# -eq 0 ]; then
  set -- $(gh api --paginate "repos/$repo/releases?per_page=100" --jq '.[] | select(.draft | not) | .tag_name' | tac)
fi
for tag in "$@"; do
  rid=$(gh api "repos/$repo/releases/tags/$tag" --jq .id)
  gh api "repos/$repo/releases/tags/$tag" --jq '.assets[] | "\(.id)\t\(.name)\t\(.content_type)\t\(.digest)"' > "$work/assets.tsv"
  while IFS=$'\t' read -r id name ctype digest; do
    case "$name" in *.reset) echo "STOP $tag: a left-over $name; finish or remove it first"; exit 1 ;; esac
    f="$work/file"
    gh api -H "Accept: application/octet-stream" "repos/$repo/releases/assets/$id" > "$f"
    have="sha256:$(sha256sum "$f" | cut -d' ' -f1)"
    [ "$have" = "$digest" ] || { echo "STOP $tag $name: downloaded $have, GitHub says $digest"; exit 1; }
    new=$(gh api --method POST -H "Content-Type: $ctype" --input "$f" \
      "https://uploads.github.com/repos/$repo/releases/$rid/assets?name=$name.reset" --jq '"\(.id) \(.digest)"')
    nid=${new%% *}; ndig=${new#* }
    [ "$ndig" = "$digest" ] || { echo "STOP $tag $name: the copy ($name.reset) has $ndig, want $digest; old file kept"; exit 1; }
    gh api --method DELETE "repos/$repo/releases/assets/$id" > /dev/null
    gh api --method PATCH "repos/$repo/releases/assets/$nid" -f name="$name" > /dev/null
    got=$(gh api "repos/$repo/releases/assets/$nid" --jq '"\(.name) \(.digest) \(.download_count)"')
    [ "$got" = "$name $digest 0" ] || { echo "STOP $tag $name: now '$got'"; exit 1; }
    echo "ok $tag $name"
  done < "$work/assets.tsv"
done
echo "done: every file named above now counts from 0"
