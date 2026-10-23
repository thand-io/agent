---
layout: default
title: Development
nav_order: 9
has_children: true
description: Developer documentation for Thand Agent
---

# Development

Documentation for developers contributing to or extending the Thand Agent.

## Config Mutation Invariant

Configuration definition maps should be treated as immutable snapshots.
When config changes, prefer replacing whole entries or whole definition maps
instead of mutating nested state in place. Some older code paths still perform
mutation-prone updates; keep new code aligned with the invariant and track
cleanup of legacy exceptions in follow-up issues rather than extending them.
Current cleanup work is tracked in [#306](https://github.com/thand-io/agent/issues/306).
