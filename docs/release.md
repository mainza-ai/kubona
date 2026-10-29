# Release procedure

## Preconditions

- The release commit is on `main` and its `ci.yml` run is green.
- A signed tag exists (for example `v0.3.0`).
- GHCR package creation is allowed for the repository's `GITHUB_TOKEN`.
- The release workflow has `contents: write`, `packages: write`,
  `id-token: write`, and `actions: read`.

## Cut a release

```bash
git checkout main
git pull --ff-only
git tag -s v0.3.0 -m "Kubona v0.3.0"
git push origin v0.3.0
```

The `Release` workflow then:

1. Refuses unless the tag commit is on `origin/main` and its `ci.yml` run is
   `success`.
2. Builds `kubonad`, `console`, and `bridge` for `linux/amd64`.
3. Pushes `ghcr.io/<owner>/<repo>/{kubonad,console,bridge}:<tag>` and
   `:sha-<commit>` with BuildKit provenance and SBOM attestations.
4. Signs each image digest keyless with cosign.
5. Creates a GitHub release for the tag.

`workflow_dispatch` takes an existing tag; tag push is the normal path.

## Verify a release

```bash
IMAGE=ghcr.io/mainza-ai/kubona/kubonad:v0.3.0
cosign verify "$IMAGE" \
  --certificate-identity-regexp 'https://github.com/mainza-ai/kubona/.github/workflows/release.yml@refs/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

docker buildx imagetools inspect "$IMAGE"
```

The provenance and SBOM attestations ride with the image; `cosign
verify-attestation` checks them. The console image carries
`NEXT_PUBLIC_KUBONA_VERSION` from the tag; the Go image carries `VERSION`.

## Notes

- The release workflow only runs on tags. The regular push/PR gate still
  runs on `main` and `develop`.
- If the tag is moved or deleted, the release is not automatically rewritten;
  delete and recreate it deliberately.
- The rate-card contract is validated at daemon startup; a stale card still
  blocks a release's runtime even if the image built.
