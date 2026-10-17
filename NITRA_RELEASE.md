# Nitra Forgejo provider 1.7.0-nitra.2

Temporary release based on upstream `6cf807d7afe390d894af65beb6672c2f267fb8de` (v1.7.0 plus its documentation update).

Includes upstream PRs [#216](https://github.com/svalabs/terraform-provider-forgejo/pull/216), [#217](https://github.com/svalabs/terraform-provider-forgejo/pull/217), [#218](https://github.com/svalabs/terraform-provider-forgejo/pull/218), [#219](https://github.com/svalabs/terraform-provider-forgejo/pull/219), [#220](https://github.com/svalabs/terraform-provider-forgejo/pull/220), [#221](https://github.com/svalabs/terraform-provider-forgejo/pull/221), [#222](https://github.com/svalabs/terraform-provider-forgejo/pull/222), [#223](https://github.com/svalabs/terraform-provider-forgejo/pull/223), and [#224](https://github.com/svalabs/terraform-provider-forgejo/pull/224).

The grouped-event fix from #224 is combined with #216's webhook replacement rules. Both acceptance tests and both parts of the attribute documentation are retained.

## Distribution

Provider source: `registry.opentofu.org/nitra/forgejo`.
OCI repository: `oci.7n.ai/public/opentofu-providers/nitra/forgejo`.
Version: `1.7.0-nitra.2`.
Platforms: `darwin_arm64`, `darwin_amd64`, `linux_arm64`, `linux_amd64`.

Use OpenTofu's `oci_mirror` installation method and commit the dependency lock file. The release is distributed as provider ZIP packages in an OCI provider index, not as a container image. Build scripts are maintained in `nitra-protected/k8s`, under `tofu/forgejo-secrets/provider/`.

Pin this release exactly. Do not replace an existing tag with different binaries. Existing roots require a state provider-address migration and a reviewed plan before use.

## Returning to upstream

Return to `svalabs/forgejo` once its release includes the features used by the root. Check compatibility with the state before replacing the provider address, then require a no-op plan. Older upstream releases cannot manage the new team-repository and tag-protection resource types.

This release was prepared with the assistance of generative AI.
