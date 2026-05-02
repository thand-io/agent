import Foundation
import ServiceManagement

public struct ServiceManagementStatus: Encodable, Equatable {
    public let loginItem: String
    public let daemon: String

    public init(loginItem: String, daemon: String) {
        self.loginItem = loginItem
        self.daemon = daemon
    }
}

public struct ServiceManagementController {
    public static let loginItemIdentifier = "io.thand.agent.privilege-notifier"
    public static let daemonPlistName = "io.thand.agent.privilege-broker.plist"

    public let loginItemService: SMAppService
    public let daemonService: SMAppService

    public init(
        loginItemService: SMAppService = .loginItem(identifier: Self.loginItemIdentifier),
        daemonService: SMAppService = .daemon(plistName: Self.daemonPlistName)
    ) {
        self.loginItemService = loginItemService
        self.daemonService = daemonService
    }

    public func register() throws {
        try loginItemService.register()
        try daemonService.register()
    }

    public func unregister() throws {
        try daemonService.unregister()
        try loginItemService.unregister()
    }

    public func currentStatus() -> ServiceManagementStatus {
        ServiceManagementStatus(
            loginItem: describe(loginItemService.status),
            daemon: describe(daemonService.status),
        )
    }

    public func openSettings() {
        SMAppService.openSystemSettingsLoginItems()
    }

    private func describe(_ status: SMAppService.Status) -> String {
        switch status {
        case .enabled:
            return "enabled"
        case .notRegistered:
            return "not-registered"
        case .requiresApproval:
            return "requires-approval"
        case .notFound:
            return "not-found"
        @unknown default:
            return "unknown"
        }
    }
}
