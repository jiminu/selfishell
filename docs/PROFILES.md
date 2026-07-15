# Profiles

Profiles are cumulative. Choose the smallest profile that covers the machine's
role.

| Profile | Purpose |
| --- | --- |
| `minimal` | Zsh, Git, Curl, Starship, and core configuration |
| `developer` | Minimal plus navigation, search, runtimes, and Vim tooling |
| `kubernetes` | Developer plus kubectl and context tooling |
| `full` | Kubernetes plus supported desktop, font, and Java integrations |

Preview without changing the machine:

```sh
selfishell install --profile kubernetes --dry-run
```

Install or change the selected profile explicitly:

```sh
selfishell install --profile kubernetes --yes
```

The active profile is recorded in the XDG state directory. `selfishell update`
uses that recorded profile when updating approved direct tools and managed
configuration. Apt and Homebrew retain responsibility for system package
versions.
