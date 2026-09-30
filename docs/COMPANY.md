# Company Deployment

Company-specific packages and shell settings must stay outside the public
repository. Store them in a private configuration repository or endpoint and
inject them during provisioning.

Put private shell initialization directly in the user's `~/.zshrc`, outside the
marked Selfishell loader block. Selfishell also manages only its marked mise
shims block within `~/.zprofile`; both files remain user-owned.

Recommended deployment controls:

1. Pin the Selfishell release with `install.sh --version`.
2. Mirror release archives and checksums when public GitHub access is restricted.
3. Provision Homebrew separately if executing its upstream bootstrap is not an
   acceptable trust decision.
4. Never place credentials, tokens, kubeconfigs, internal URLs, or certificate
   private keys in package configuration or this repository.
5. Validate the environment on a clean managed image before broad rollout.

## Release mirror

`SELFISHELL_RELEASE_ROOT` replaces `https://github.com/jiminu/selfishell/releases`
for the bootstrap and the installed CLI (`update`, `version --available`, and
update notices). Set it when running `install.sh` and export it in `~/.zshrc`,
outside the Selfishell block. Mirror each release's asset set unchanged; the
mirror must serve:

```text
<root>/latest/download/VERSION
<root>/download/v<version>/selfishell-<version>-<platform>-<architecture>.tar.gz
<root>/download/v<version>/SHA256SUMS
```

Without `latest/download/VERSION`, latest-release discovery fails; pass
`--version VERSION` to both `install.sh` and `selfishell update`. Archives are
verified only against the mirrored `SHA256SUMS`, so verify
[attestations](SECURITY.md) before publishing to the mirror. The variable does
not redirect mise, Zinit, plugin, or tool downloads.
