# Company Deployment

Company-specific packages and shell settings must stay outside the public
repository. Store them in a private configuration repository or endpoint and
inject them during provisioning.

A local profile contains only `package` records:

```text
package macos required formula company-cli
package ubuntu required apt company-cli
```

```sh
selfishell install --profile developer \
  --local-profile /path/to/company.conf --yes
```

Set `SELFISHELL_LOCAL_PROFILE` when the same private profile should also be used
by subsequent updates. Put private shell initialization in
`${XDG_CONFIG_HOME:-$HOME/.config}/selfishell/local.zsh`; Selfishell sources but
does not manage or remove this file.

Recommended deployment controls:

1. Pin the Selfishell release with `install.sh --version`.
2. Mirror release archives and checksums when public GitHub access is restricted.
3. Provision Homebrew separately if executing its upstream bootstrap is not an
   acceptable trust decision.
4. Never place credentials, tokens, kubeconfigs, internal URLs, or certificate
   private keys in a profile or this repository.
5. Validate the selected profile on a clean managed image before broad rollout.
