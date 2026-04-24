import Foundation

public enum NotificationFormatter {
    public static let title = "Thand Privileged Access"

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
        }
    }
}
