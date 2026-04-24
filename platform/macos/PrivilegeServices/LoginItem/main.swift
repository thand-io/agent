import Foundation
import PrivilegeServicesShared
import UserNotifications

private func requestNotificationAuthorization() async throws {
    _ = try await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge])
}

private func postNotification(for event: BrokerEvent) async {
    let notification = NotificationFormatter.request(for: event)

    let content = UNMutableNotificationContent()
    content.title = notification.title
    content.body = notification.body
    content.sound = .default
    if !notification.subtitle.isEmpty {
        content.subtitle = notification.subtitle
    }
    if !notification.threadID.isEmpty {
        content.threadIdentifier = notification.threadID
    }

    let request = UNNotificationRequest(
        identifier: notification.notificationID.isEmpty
            ? "thand-local-notification-\(UUID().uuidString)"
            : notification.notificationID,
        content: content,
        trigger: nil
    )

    do {
        try await UNUserNotificationCenter.current().add(request)
    } catch {
        fputs("failed to post notification: \(error)\n", stderr)
    }
}

do {
    let arguments = try NotifierRuntimeArguments.parse(Array(CommandLine.arguments.dropFirst()))

    var config = BrokerConfig.fromEnvironment()
    config = BrokerConfig(
        stateDirectoryURL: config.stateDirectoryURL,
        sudoersDirectoryURL: config.sudoersDirectoryURL,
        visudoPath: config.visudoPath,
        serviceLabel: arguments.serviceLabel ?? config.serviceLabel
    )

    brokerLog("starting privilege notifier", fields: [
        "service_label": config.serviceLabel,
        "notifier_service_label": config.notifierServiceLabel
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

    let notifier = NotifierXPCClient(config: config) { event in
        brokerLog("received broker event in notifier", fields: [
            "event_kind": event.kind.rawValue,
            "broker_handle": event.brokerHandle,
            "username": event.username
        ])
        Task {
            await postNotification(for: event)
        }
    }
    let acknowledgement = try notifier.start()
    brokerLog("privilege notifier subscribed to broker events", fields: [
        "username": acknowledgement.username,
        "audit_session_identifier": String(acknowledgement.auditSessionIdentifier)
    ])
    RunLoop.main.run()
} catch {
    fputs("thand-macos-privilege-notifier failed: \(error)\n", stderr)
    exit(1)
}
