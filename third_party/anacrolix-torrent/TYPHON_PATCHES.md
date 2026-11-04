# Typhon patches to anacrolix/torrent v1.61.0

This directory is github.com/anacrolix/torrent v1.61.0 as published in the Go
module cache, wired in through the `replace` directive in the launcher's go.mod.
It is licensed under the Mozilla Public License 2.0 (see LICENSE). Only the
files listed below differ from the release; both changes are backports of
upstream commits that are in master but in no tagged release yet.

| File | Upstream commit | What it fixes |
|---|---|---|
| peer-conn-msg-writer.go, peer-conn-msg-writer_test.go | aa71e8d2 (#1078, fixes #1070) | The peer writer took its wakeup channel after `fillWriteBuffer` released the client lock, so a `tickleWriter` landing in between was lost and the writer slept until its keep-alive timer, a minute by default. |
| peer-conn-msg-writer.go, peer-conn-msg-writer_test.go | 70072f1a (#1071) | The keep-alive timer was rearmed only after a write. Once it fired while the connection was not useful, nothing rearmed it, and a lost wakeup could park the writer for good. |

Drop this directory and the `replace` line once a tagged release contains both
commits.
