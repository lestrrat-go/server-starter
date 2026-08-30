Releasing v2
============

Release v2 from the `v2` branch, not from `develop/v2`.

1. Merge `develop/v2` into `v2` through a pull request after its checks pass.
2. Check out the merged `v2` branch and create an annotated version tag.
3. Push the tag. The release workflow verifies that the tagged commit belongs
   to `v2`, then publishes the GitHub release and its artifacts.

For the first prerelease:

```sh
git switch v2
git pull --ff-only origin v2
git tag -a v2.0.0-alpha1 -m v2.0.0-alpha1
git push origin v2.0.0-alpha1
```

Tags matching `v2.*` start the workflow. A tag with a prerelease suffix, such
as `v2.0.0-alpha1`, is published as a GitHub prerelease.
