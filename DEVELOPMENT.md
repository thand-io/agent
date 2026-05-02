# Development Notes

This file documents the supported developer loop for repo features that need local setup beyond normal Go build and test commands.

## macOS Privilege Services

The macOS timed-sudo path now lives under `platform/macos/PrivilegeServices`.

### Scope

- `xcodebuild` is the only supported native macOS build and test backend.
- XcodeGen is the source of truth for the generated Xcode project.
- `make build` and `make test` remain the top-level commands on Darwin.
- The Go agent stays unprivileged.
- The native app bundle owns `SMAppService` registration, the login item, and the privileged broker daemon.
- `brokerctl` remains installed at `/Library/Application Support/Thand/PrivilegeBroker/bin/thand-macos-privilege-brokerctl`.
- the installed app bundle and `brokerctl` payload are normalized to `root:wheel` ownership and non-user-writable modes during dev install

### Prerequisites

- macOS with full Xcode installed, not Command Line Tools only.
- Xcode selected via `xcode-select`.
- `xcodegen` installed locally:

```bash
brew install xcodegen
sudo xcode-select -s /Applications/Xcode.app/Contents/Developer
```

- Root access for install and end-to-end broker testing.
- For full local `SMAppService` integration testing:
  - Xcode signed into an Apple team that can perform Apple Development signing
  - `APPLE_TEAM_ID` exported in your shell as the signing TeamIdentifier
  - an Apple Development signing identity that shows up as valid in the login keychain
  - if your Apple Development certificate is present but marked untrusted, install the Apple Worldwide Developer Relations intermediate certificate for the current Apple Development chain (`WWDR G3`) from Apple PKI into the `System` keychain and leave trust at the default system settings

Example:

```bash
export APPLE_TEAM_ID=ABCDE12345
security find-identity -v -p codesigning ~/Library/Keychains/login.keychain-db
```

That identity check should show at least one valid Apple Development identity for your team. `APPLE_TEAM_ID` should be the signing TeamIdentifier from the certificate subject `OU`, not the display-name suffix in the certificate common name if those differ on your machine. If the identity check still says `0 valid identities found`, install or repair the WWDR G3 intermediate and re-check before attempting the signed dev install. The `0 provisioned devices` indicator in Xcode account settings is not relevant for this macOS flow.

To install the WWDR G3 intermediate on the local Mac:

1. Download the current Apple Worldwide Developer Relations intermediate certificate for the Apple Development chain (`WWDR G3`) from Apple PKI.
2. Open the downloaded certificate in Keychain Access.
3. When prompted for a keychain, install it into the `System` keychain.
4. Leave the trust settings at `Use System Defaults`.
5. Re-run the identity check above and confirm it now shows at least one valid Apple Development identity.

### Supported Build Outputs

- Generated project:
  - `platform/macos/PrivilegeServices/ThandPrivilegeServices.xcodeproj`
- Generated protobuf/gRPC stubs:
  - `internal/localbroker/proto/localbroker/v1/*.pb.go`
  - `platform/macos/PrivilegeServices/Generated/LocalBroker/*.swift`
- Repo-local native build cache:
  - `.build/macos/DerivedData`
- Repo-local staging and packaging outputs:
  - `.build/macos/PrivilegeServices`

`make clean` removes those generated native build outputs.

The checked-in protobuf contract lives in `proto/localbroker/v1/localbroker.proto`. Generated Go and Swift stubs are intentionally ignored. Use `make gen` or `make gen-buf` to regenerate them explicitly; supported build, test, package, and install targets run that generation before compiling.

### Fast Local Loop

Use this loop for normal development. It does not require Apple release secrets or notarization.

```bash
make build
make test
```

On Darwin, those commands do all of the following:

- build the Go agent
- generate localbroker protobuf/gRPC stubs
- Apple Development-sign `bin/thand` when `APPLE_TEAM_ID` is set for strict local broker testing
- generate the Xcode project from `platform/macos/PrivilegeServices/project.yml`
- run `xcodebuild build`
- run `xcodebuild test`

The Make targets use coarse repo-local stamp files around the native build and test outputs, while Xcode owns the real Swift-level incrementality underneath. Re-running `make build` or `make test` with no relevant file changes should be a no-op.

For layout-only packaging verification without local signing:

```bash
THAND_MACOS_SKIP_SIGNING=1 make package-macos-privilege-services-dev
```

That verifies the staged bundle structure and the installed `brokerctl` payload location, but it is not a supported broker runtime path and is not sufficient for real `SMAppService` integration.

### Full Local Integration Loop

Use this loop when you need to test the real login item, daemon registration, notifications, brokered sudo, reboot handling, and reconnect behavior on your own Mac.

1. Export your Apple team ID:

```bash
export APPLE_TEAM_ID=ABCDE12345
```

2. Confirm your Apple Development identity is trusted locally:

```bash
security find-identity -v -p codesigning ~/Library/Keychains/login.keychain-db
```

If you see `0 valid identities found` but your Apple Development certificate exists in Keychain Access, install the current Apple WWDR G3 intermediate certificate into the `System` keychain with default trust settings, then retry the check.

3. Build and run native tests:

```bash
make build
make test
```

When `APPLE_TEAM_ID` is set, `make build` also Apple Development-signs the local `bin/thand` binary with the `io.thand.agent` identifier so the installed helper can verify the parent agent identity during strict local broker testing.

4. Package and install the Apple Development-signed dev payload:

```bash
sudo -E make install-macos-privilege-services-dev
```

The install target intentionally splits responsibilities:

- packaging and Apple Development signing happen as the invoking desktop user so the user keychain identities are available
- copying into `/Applications` and `/Library/Application Support/...` happens as root
- the installed app bundle, embedded helpers, daemon plist, and installed `brokerctl` are normalized to `root:wheel` ownership with non-user-writable modes
- the installed app is asked to `register` after install when `SUDO_USER` is available
- the staged and installed daemon keep strict peer entitlement enforcement enabled
- the staged and installed helper binaries carry the same broker peer entitlements as the release packaging path

5. Open the installed app registration manually if needed:

```bash
/Applications/ThandPrivilegeServices.app/Contents/MacOS/ThandPrivilegeServices register
```

6. Approve background items and the daemon in System Settings when macOS prompts for them.

7. Submit and verify a timed sudo request:

```bash
thand request sudo --device device-alpha --duration 5m --reason "privilege services smoke test"
```

Validate all of the following locally:

- grant succeeds and returns broker-backed metadata
- notifier shows grant, revoke, and expiry notifications
- sudoers fragment appears under `/etc/sudoers.d`
- lease state appears under `/var/db/thand/local-privilege-broker`
- revoke removes the active grant
- expiry still happens if the agent disconnects
- reboot and reconnect reconciliation still converge
- direct helper execution outside the signed agent path is still rejected

### Useful Commands

Show the selected Xcode:

```bash
xcodebuild -version
xcode-select -p
```

Check the locally trusted Apple Development signing identities:

```bash
security find-identity -v -p codesigning ~/Library/Keychains/login.keychain-db
```

Open System Settings to the login-items area:

```bash
/Applications/ThandPrivilegeServices.app/Contents/MacOS/ThandPrivilegeServices open-settings
```

Check current native registration status:

```bash
/Applications/ThandPrivilegeServices.app/Contents/MacOS/ThandPrivilegeServices status
```

Tail the broker log:

```bash
sudo tail -f /var/log/thand-privilege-broker.log
```

### Release Packaging

Public-distribution packaging is separate from local development:

- local integration uses Apple Development signing
- release packaging uses Developer ID signing and notarization

Build the release installer locally only if you have the required Developer ID identities and notarization setup:

```bash
export APPLE_TEAM_ID=ABCDE12345
./scripts/package-macos-privilege-services-release.sh
```

That produces a signed `.pkg` under `.build/macos/PrivilegeServices/release`.

### Cleanup

Remove the locally installed dev payload with:

```bash
sudo make uninstall-macos-privilege-services-dev
```

### CI Requirements

The supported macOS CI path assumes:

- a GitHub macOS runner with full Xcode available
- Xcode selected explicitly in the workflow
- XcodeGen installed in the workflow before `make build` and `make test`
- Go available so the repo-local buf and protobuf codegen tools can be bootstrapped into `.build`

PR and mainline validation do not require Apple signing secrets. They run unsigned native build, test, and layout verification only.

The optional macOS release-sign/notarize lane is separate:

- it skips cleanly when Apple release secrets are absent
- once those secrets exist, it signs and notarizes the public `.pkg`

### Unsupported Old Paths

The old `platform/macos/PrivilegeBroker` SwiftPM-first layout is no longer the supported developer path.

`swift build` and `swift test` are no longer the supported native macOS workflow for this feature.
