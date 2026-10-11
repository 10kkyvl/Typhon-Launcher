# Security policy

## Supported versions

Only the latest release receives security fixes. The launcher updates itself; if you
are on an older version, update first and check whether the issue still reproduces.

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub private vulnerability reporting](https://github.com/10kkyvl/Typhon-Launcher/security/advisories/new),
not in a public issue.

Include what is affected (launcher version, OS), how to reproduce it, and what an attacker
gains. Logs help, but review them first: they are redacted of paths and account details,
yet may still contain things you would rather not share.

You will get an answer within a few days. Once a fix is released, the advisory is
published, with credit if you want it.

## Scope

In scope: the launcher in this repository, its self-update channel and its communication
with the Typhon services (account, metadata and update servers).

Out of scope: content of third-party sources added by users, games and their installers,
and the BitTorrent network itself.
