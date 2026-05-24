---
layout: default
title: Migration Guide
parent: Roles
grand_parent: Configuration
nav_order: 2
description: Guide for migrating roles to the current permission model with statements, allow/deny scopes, and consolidated resource targets
---

# Role Model Migration Guide

This guide covers the changes to the role model and how existing configurations are handled.

{: .highlight}
**No manual migration is required.** Thand automatically migrates old-format role configurations at load time. Your existing YAML files will continue to work without changes. However, we recommend updating your files to the new format for clarity and to take advantage of new features like conditions and domain-based scopes.

---

## What Changed

Three structural changes were made to the role model:

| Change | Old Model | New Model |
|--------|-----------|-----------|
| **Permissions** | Flat string lists | Statement objects with `operations`, `targets`, and `conditions` |
| **Resources** | Separate top-level `resources` field | Folded into statement `targets` |
| **Scopes** | Flat `users`/`groups` lists | Allow/deny structure with `users`, `groups`, and `domains` |

---

## 1. Permissions: Strings → Statements

### What Changed

Permissions were previously flat lists of operation strings. They are now **statement objects** that combine operations with optional targets and conditions.

### Before

```yaml
permissions:
  allow:
    - ec2:DescribeInstances
    - s3:GetObject
    - s3:PutObject
  deny:
    - ec2:TerminateInstances
```

### After

```yaml
permissions:
  allow:
    - operations:
        - ec2:DescribeInstances
        - s3:GetObject
        - s3:PutObject
      targets:
        - "arn:aws:s3:::dev-bucket/*"
      conditions:
        IpAddress:
          "aws:SourceIp": "10.0.0.0/8"
  deny:
    - operations:
        - ec2:TerminateInstances
```

### Auto-Migration Behavior

When Thand encounters a plain string in a permission `allow` or `deny` list, it automatically converts it to a statement:

```
"ec2:DescribeInstances"  →  { operations: ["ec2:DescribeInstances"], targets: [], conditions: {} }
```

This happens transparently at load time. No data is lost.

---

## 2. Resources: Removed (Folded into Targets)

### What Changed

The top-level `resources` field has been removed. Resource identifiers now live inside permission statements as `targets`, directly alongside the operations they apply to.

### Before

```yaml
permissions:
  allow:
    - s3:GetObject
    - s3:PutObject
  deny:
    - s3:DeleteObject

resources:
  allow:
    - "arn:aws:s3:::dev-bucket/*"
    - "arn:aws:s3:::staging-bucket/*"
  deny:
    - "arn:aws:s3:::prod-bucket/*"
```

### After

```yaml
permissions:
  allow:
    - operations:
        - s3:GetObject
        - s3:PutObject
      targets:
        - "arn:aws:s3:::dev-bucket/*"
        - "arn:aws:s3:::staging-bucket/*"
  deny:
    - operations:
        - s3:DeleteObject
      targets:
        - "arn:aws:s3:::prod-bucket/*"
```

### Auto-Migration Behavior

When Thand encounters a top-level `resources` field on a role, it migrates the resource entries into permission statement targets:

- `resources.allow` entries are added as targets to the permission `allow` statements
- `resources.deny` entries are added as targets to the permission `deny` statements

The top-level `resources` field is then ignored.

---

## 3. Scopes: Flat → Allow/Deny

### What Changed

Scopes previously had flat `users` and `groups` lists at the top level. They now use an **allow/deny** structure with support for `users`, `groups`, and the new `domains` identity type.

### Before

```yaml
scopes:
  users:
    - alice@example.com
    - bob@example.com
  groups:
    - developers
    - engineering
```

### After

```yaml
scopes:
  allow:
    users:
      - alice@example.com
      - bob@example.com
    groups:
      - developers
      - engineering
    domains:
      - example.com
  deny:
    groups:
      - contractors
```

### Auto-Migration Behavior

When Thand encounters the old flat scope format (with `users` or `groups` directly under `scopes`), it automatically moves them into the `allow` block:

```yaml
# Old format at load time:
scopes:
  users: [alice@example.com]
  groups: [developers]

# Automatically becomes:
scopes:
  allow:
    users: [alice@example.com]
    groups: [developers]
```

### New: Domain Scopes

The new model adds `domains` as a scope identity type. Domain scopes match based on the domain portion of a user's email address:

```yaml
scopes:
  allow:
    domains:
      - example.com           # Matches any @example.com user
  deny:
    domains:
      - external-vendor.com   # Blocks any @external-vendor.com user
```

### New: Deny Scopes

The allow/deny structure enables explicit denial rules. **Deny always takes precedence over allow:**

```yaml
scopes:
  allow:
    groups: [engineering]     # All engineers can request
  deny:
    groups: [interns]         # Except interns, even if in engineering
```

---

## 4. New: Composite Field

The `composite` field is a **system-managed** boolean that Thand sets to `true` when a role's permissions are assembled from inherited local roles at runtime. You will see this in API responses and debug output, but you should never set it manually in configuration files.

---

## 5. New: Conditions

Statements support an optional `conditions` field for provider-specific constraints. Currently, only AWS maps conditions to IAM policy `Condition` blocks:

```yaml
permissions:
  allow:
    - operations:
        - s3:GetObject
      targets:
        - "arn:aws:s3:::sensitive-bucket/*"
      conditions:
        IpAddress:
          "aws:SourceIp": "10.0.0.0/8"
```

See the [Conditions documentation](./index#conditions) for full details and AWS examples.

---

## 6. New: Binding Field on Statements

The `binding` field is an optional property on permission statements that declares the explicit CSP resource where a custom role should be created and where the resulting IAM binding should be applied.

### Why it exists

Some providers create custom roles at a different scope than the request tenant. The most common case is **GCP**: custom roles can only be created at the `projects/{id}` or `organizations/{id}` level, but a request tenant may be a folder (`folders/{id}`). Without `binding`, the GCP provider would fail when asked to create a custom role for a folder tenant.

The field exists to resolve this explicitly and cleanly, regardless of provider. See [Binding](./index#binding) in the reference documentation for the full format per provider.

### No migration required — but a deprecation warning may appear

If your existing roles have `permissions.allow` statements that include `targets` pointing at a specific project (e.g. `projects/my-project/...`), the GCP provider previously attempted to infer the binding project from those targets whenever the request tenant was a folder. This inference still works, but it now emits a **deprecation warning** in the agent logs:

```
level=warning msg="permissions.allow statements are missing 'binding'; inferring project from targets. Set an explicit 'binding' on each statement to remove this warning."
```

If only some statements in the role set `binding`, a different warning is emitted:

```
level=warning msg="some permissions.allow statements have 'binding' set but not all; the explicit binding values will be ignored and the project will be inferred from targets instead. Set 'binding' on every statement or remove it from all statements."
```

To silence the warning and make the intent explicit, add `binding` to the relevant statements:

```yaml
# Before (still works, but logs a deprecation warning)
permissions:
  allow:
    - operations:
        - "gcp-prod:secretmanager.secrets.get"
      targets:
        - "projects/my-project/secrets/*"

# After (explicit, no warning)
permissions:
  allow:
    - binding: "projects/my-project"
      operations:
        - "gcp-prod:secretmanager.secrets.get"
      targets:
        - "projects/my-project/secrets/*"
```

### Validation

- All statements within the same role that set `binding` must agree on the same value. Conflicting values produce a configuration error.
- `binding: "folders/..."` is rejected by the GCP provider — folders are not a valid scope for custom role creation.
- `binding` does not restrict which resources the operations act on; `targets` continues to control that.

---

## 7. New: Statement ID Field

The `id` field is an optional property on permission statements that provides a stable identifier for per-statement custom role naming. It is most useful when a role contains **multiple statements**, each requiring its own custom role in the provider.

### Why it exists

When a role has multiple `allow` statements, the provider must create a separate custom role for each one. Without `id`, the generated role names use a positional index suffix (e.g., `thand_my_role_s0`, `thand_my_role_s1`). This means reordering or inserting statements changes the generated names, which can cause unnecessary role deletions and recreations.

Setting `id` on each statement produces **stable, human-readable** names:

```yaml
# Without id — positional names (fragile)
permissions:
  allow:
    - operations:                          # → thand_my_role_s0
        - secretmanager.secrets.get
    - operations:                          # → thand_my_role_s1
        - storage.buckets.get

# With id — stable names
permissions:
  allow:
    - id: secrets_read                     # → thand_my_role_secrets_read
      operations:
        - secretmanager.secrets.get
    - id: storage_read                     # → thand_my_role_storage_read
      operations:
        - storage.buckets.get
```

### Validation

- Must be **snake_case**: lowercase letters, digits, and underscores only (regex: `^[a-z][a-z0-9_]*$`).
- Maximum **64 characters**.
- Optional — roles with a single statement do not need an `id` (the base role name is used directly).
- The value is **not shown** in notifications or user-facing messages; it is an internal identifier only.

---

## 8. Provider-Specific Behavior: GCP Targets

### GCP-Specific Note: Targets Are Metadata-Only

For GCP roles, the `targets` field within permission statements is **preserved in the role definition but not enforced**. Only the `operations` field is used when building custom IAM roles.

This means:
- **`targets` are metadata**: You can include targets for documentation or reference, but GCP's role creation system (IAM API) ignores them entirely.
- **`operations` are enforced**: Only the operations you list in `operations` are included in the generated custom role.
- **Use `binding` to scope IAM bindings**: To explicitly control which project or resource a custom role is assigned to a user, use the `binding` field. `binding` determines the IAM assignment scope, not `targets`.

### Example: GCP Role with Targets (Targets Ignored)

```yaml
permissions:
  allow:
    - binding: "projects/my-project"
      operations:
        - secretmanager.secrets.get
        - secretmanager.secrets.list
      targets:
        - "projects/my-project/secrets/*"    # This line is metadata only; GCP ignores it
```

The custom role created in GCP will only include `secretmanager.secrets.get` and `secretmanager.secrets.list`. The `targets` line provides documentation to users or tools about which resources these operations apply to, but it does not limit the role itself.

### Avoid Relying on Targets for GCP Resource Scoping

**Incorrect approach** (targets won't enforce scope):
```yaml
# DON'T do this—targets alone won't restrict scope
permissions:
  allow:
    - operations:
        - storage.buckets.get
      targets:
        - "projects/my-project/buckets/my-bucket"   # This is ignored!
```

**Correct approach** (use binding for scope control):
```yaml
# DO this—binding controls both role creation scope and IAM binding scope
permissions:
  allow:
    - binding: "projects/my-project"   # Role created here; binding applied here
      operations:
        - storage.buckets.get
      targets:
        - "projects/my-project/buckets/my-bucket"   # Optional: documentation
```

---

## Summary

| Feature | Manual Action Required | Notes |
|---------|----------------------|-------|
| String permissions → statements | No | Auto-converted at load time |
| Top-level `resources` → `targets` | No | Auto-migrated at load time |
| Flat scopes → allow/deny | No | Auto-migrated to `allow` block |
| Domain scopes | Optional | New feature, add when ready |
| Deny scopes | Optional | New feature, add when ready |
| Conditions | Optional | New feature, AWS-only currently |
| Composite field | No | System-managed, do not set |
| Binding field | Recommended | Silences deprecation warning when tenant ≠ role creation scope |
| Statement ID | Optional | Stable per-statement custom role names for multi-statement roles |

{: .note}
While auto-migration ensures your existing configurations continue to work, we recommend updating your YAML files to the new format when convenient. The new format is more expressive, supports conditions and domain scopes, and makes the relationship between operations and their target resources explicit.
