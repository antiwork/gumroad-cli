---
name: gumroad-cli-release
description: Release gumroad-cli after a PR lands by inspecting origin/main, creating the next Go-compatible date tag, pushing only that tag, and verifying GitHub release/Homebrew follow-through. Use for requests like "cut a gumroad-cli release", "push the release tag", or "release the merged gumroad-cli changes".
---

# Gumroad CLI Release

## Use When

Use this only after the intended changes are merged to `origin/main`.

Do not use it to release from a feature branch, local `HEAD`, or an unmerged PR branch. The release workflow is tag-driven: push an annotated tag that points at the current `origin/main` commit.

## Version Format

Tags use Go-compatible date versioning:

```text
v0.YYYYMMDD.N
```

- `YYYYMMDD` is the UTC release date.
- `N` starts at `0` and increments only for multiple releases on the same UTC date.
- The binary and Homebrew formula display `v0.20260609.0` as `2026.06.09`.
- Do not create `v2026.06.09`; it is not a valid Go module release shape for this repo.

## Workflow

1. Confirm repository and state:

```bash
git status --short --branch
git remote -v
```

2. Fetch current main and tags:

```bash
git fetch origin main --tags
```

3. Inspect what will be released:

```bash
latest_tag="$(git describe --tags --abbrev=0 origin/main)"
git log --oneline --first-parent "${latest_tag}..origin/main"
git rev-parse origin/main
```

If there are no first-parent changes since `latest_tag`, stop and report that no new release is needed.

4. Choose the next tag:

```bash
release_date="$(date -u +%Y%m%d)"
release_seq=0
tag="$(make release-tag RELEASE_DATE="$release_date" RELEASE_SEQ="$release_seq")"

while git rev-parse -q --verify "refs/tags/${tag}" >/dev/null ||
  git ls-remote --exit-code --tags origin "${tag}" >/dev/null 2>&1; do
  release_seq=$((release_seq + 1))
  tag="$(make release-tag RELEASE_DATE="$release_date" RELEASE_SEQ="$release_seq")"
done
```

5. Validate and create the annotated tag on `origin/main` explicitly:

```bash
./script/validate-release-tag.sh "$tag"
git tag -a "$tag" origin/main -m "Release $tag"
```

6. Push only the tag:

```bash
git push origin "refs/tags/${tag}"
```

Never force-push or rewrite an existing release tag unless the user explicitly asks for a tag repair and understands the release artifact implications.

7. Verify the remote tag peels to `origin/main`:

```bash
main_sha="$(git rev-parse origin/main)"
local_tag_sha="$(git rev-parse "${tag}^{}")"
remote_tag_sha="$(git ls-remote --tags origin "${tag}^{}" | awk '{print $1}')"

test "$main_sha" = "$local_tag_sha"
test "$main_sha" = "$remote_tag_sha"
```

8. Check release automation:

```bash
gh run list --workflow Release --event push --limit 5
gh release view "$tag" --json tagName,name,isDraft,isPrerelease,publishedAt,url
```

If the workflow is still running, watch the relevant run:

```bash
gh run watch <run-id>
```

## Closeout

Report:

- tag pushed
- commit SHA it points to
- first-parent changes released
- release workflow status
- GitHub release URL, once available

If Homebrew publishing is part of the workflow result, verify it from the workflow logs or the `antiwork/homebrew-cli` update before calling the release complete.
