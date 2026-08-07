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
   private keys in a profile or this repository.
5. Validate the selected profile on a clean managed image before broad rollout.
