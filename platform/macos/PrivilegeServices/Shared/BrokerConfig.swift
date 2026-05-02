import Foundation

public struct BrokerConfig: Sendable, Equatable {
    public static let defaultServiceLabel = "io.thand.agent.privilege-broker"
    public static let defaultStateDirectory = "/var/db/thand/local-privilege-broker"
    public static let defaultSudoersDirectory = "/etc/sudoers.d"
    public static let defaultVisudoPath = "/usr/sbin/visudo"

    public let stateDirectoryURL: URL
    public let sudoersDirectoryURL: URL
    public let visudoPath: String
    public let serviceLabel: String

    public var notifierServiceLabel: String {
        "\(serviceLabel).notifier"
    }

    public init(
        stateDirectoryURL: URL = URL(fileURLWithPath: Self.defaultStateDirectory),
        sudoersDirectoryURL: URL = URL(fileURLWithPath: Self.defaultSudoersDirectory),
        visudoPath: String = Self.defaultVisudoPath,
        serviceLabel: String = Self.defaultServiceLabel
    ) {
        self.stateDirectoryURL = stateDirectoryURL
        self.sudoersDirectoryURL = sudoersDirectoryURL
        self.visudoPath = visudoPath
        self.serviceLabel = serviceLabel
    }

    public static func fromEnvironment(processInfo: ProcessInfo = .processInfo) -> BrokerConfig {
        let environment = processInfo.environment

        let stateDirectory = environment["THAND_PRIVILEGE_BROKER_STATE_DIR"] ?? Self.defaultStateDirectory
        let sudoersDirectory = environment["THAND_PRIVILEGE_BROKER_SUDOERS_DIR"] ?? Self.defaultSudoersDirectory
        let visudoPath = environment["THAND_PRIVILEGE_BROKER_VISUDO_PATH"] ?? Self.defaultVisudoPath
        let serviceLabel = environment["THAND_PRIVILEGE_BROKER_SERVICE_LABEL"] ?? Self.defaultServiceLabel

        return BrokerConfig(
            stateDirectoryURL: URL(fileURLWithPath: stateDirectory),
            sudoersDirectoryURL: URL(fileURLWithPath: sudoersDirectory),
            visudoPath: visudoPath,
            serviceLabel: serviceLabel
        )
    }
}
