# Distribution Channel Evaluation

Selfishell's canonical distribution channel is the checksum-verified GitHub
Release bootstrap documented in `docs/INSTALLATION.md`. Additional package
manager channels are optional conveniences and must not become a separate
version source or configuration lifecycle.

## Homebrew Tap decision

A Homebrew Tap remains deferred until there is demonstrated user demand and a
maintainer willing to own formula updates, audits, and support. Revisit the
decision when at least one of these is true:

- users request Homebrew installation through public issues or feedback;
- bootstrap adoption shows repeated use beyond maintainer verification; or
- a deployment environment requires a Tap and has an owner for its maintenance.

Any future formula must consume the existing immutable release archive and
checksum. It must expose the same `selfishell` CLI, must not define another
version source, and must not install profiles or modify user configuration as a
package installation side effect.

## Apt and Debian packages

`.deb`, PPA, and APT repository evaluation remains deferred. The same demand,
ownership, artifact reuse, and configuration-safety requirements apply.
