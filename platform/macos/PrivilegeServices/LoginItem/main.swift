import Foundation
import PrivilegeServicesShared
import UserNotifications

private struct CLIArguments {
    var serviceLabel: String?
}

private func parseArguments() throws -> CLIArguments {
    var parsed = CLIArguments()
    var index = 1
    let arguments = CommandLine.arguments

    while index < arguments.count {
        switch arguments[index] {
        case "--service-label":
            index += 1
            guard index < arguments.count else {
                throw BrokerServiceError.invalidRequest("--service-label requires a value")
            }
            parsed.serviceLabel = arguments[index]
            index += 1
        default:
            throw BrokerServiceError.invalidRequest("unsupported argument \(arguments[index])")
        }
    }

    return parsed
}

private func requestNotificationAuthorization() async throws {
    _ = try await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge])
}

do {
    let arguments = try parseArguments()

    var config = BrokerConfig.fromEnvironment()
    config = BrokerConfig(
        stateDirectoryURL: config.stateDirectoryURL,
        sudoersDirectoryURL: config.sudoersDirectoryURL,
        visudoPath: config.visudoPath,
        serviceLabel: arguments.serviceLabel ?? config.serviceLabel
    )

    brokerLog("starting privilege notifier", fields: [
        "service_label": config.serviceLabel
    ])

    Task {
        brokerLog("requesting notification authorization")
        do {
            try await requestNotificationAuthorization()
            brokerLog("notification authorization request completed")
        } catch {
            fputs("notification authorization failed: \(error)\n", stderr)
            brokerLog("notification authorization failed", fields: [
                "error": String(describing: error)
            ])
        }
    }

    brokerLog("privilege notifier login item running without broker event subscription")
    RunLoop.main.run()
} catch {
    fputs("thand-macos-privilege-notifier failed: \(error)\n", stderr)
    exit(1)
}
