import Foundation

public enum NotificationFormatter {
    public static let title = "Thand Privileged Access"

    public static func request(for event: BrokerEvent) -> LocalNotificationPostRequest {
        if event.kind == .localNotification, let notification = event.localNotification {
            return notification
        }
        return LocalNotificationPostRequest(
            notificationID: "thand-privilege-broker-\(event.kind.rawValue)-\(event.brokerHandle)",
            title: title,
            body: body(for: event),
            threadID: event.grantID
        )
    }

    public static func body(for event: BrokerEvent) -> String {
        switch event.kind {
        case .grantCreated:
            return "Timed sudo access is active for \(event.username)."
        case .grantRevoked:
            return "Timed sudo access was revoked for \(event.username)."
        case .grantExpired:
            return "Timed sudo access expired for \(event.username)."
        case .grantRevokeFailed:
            return "Timed sudo cleanup failed for \(event.username): \(event.message ?? "unknown error")."
        case .localNotification:
            return event.localNotification?.body ?? event.message ?? ""
        }
    }
}
