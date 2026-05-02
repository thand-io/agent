import Foundation
import UserNotifications

public struct LocalNotificationPostRequest: Codable, Sendable, Equatable {
    public let notificationID: String
    public let title: String
    public let subtitle: String
    public let body: String
    public let threadID: String

    public init(
        notificationID: String = "",
        title: String,
        subtitle: String = "",
        body: String,
        threadID: String = ""
    ) {
        self.notificationID = notificationID
        self.title = title
        self.subtitle = subtitle
        self.body = body
        self.threadID = threadID
    }
}

public enum LocalNotificationPostFailure: Error, Equatable, CustomStringConvertible {
    case invalidRequest(String)
    case permissionDenied
    case notificationCenterUnavailable(String)
    case postingFailed(String)

    public var description: String {
        switch self {
        case .invalidRequest(let message):
            return message
        case .permissionDenied:
            return "local notification permission denied"
        case .notificationCenterUnavailable(let message):
            return "local notification center unavailable: \(message)"
        case .postingFailed(let message):
            return "failed to post local notification: \(message)"
        }
    }
}

public protocol LocalNotificationPosting: Sendable {
    func post(_ request: LocalNotificationPostRequest) async throws
}

public struct LocalNotificationPoster: LocalNotificationPosting {
    private let requestAuthorization: @Sendable () async throws -> Bool
    private let addNotification: @Sendable (LocalNotificationPostRequest) async throws -> Void

    public init(
        requestAuthorization: @escaping @Sendable () async throws -> Bool = {
            try await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge])
        },
        addNotification: @escaping @Sendable (LocalNotificationPostRequest) async throws -> Void = { request in
            try await Self.addSystemNotification(request)
        }
    ) {
        self.requestAuthorization = requestAuthorization
        self.addNotification = addNotification
    }

    public func post(_ request: LocalNotificationPostRequest) async throws {
        guard !request.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw LocalNotificationPostFailure.invalidRequest("local notification title is required")
        }
        guard !request.body.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw LocalNotificationPostFailure.invalidRequest("local notification body is required")
        }

        let authorized: Bool
        do {
            authorized = try await requestAuthorization()
        } catch {
            throw LocalNotificationPostFailure.notificationCenterUnavailable(error.localizedDescription)
        }
        guard authorized else {
            throw LocalNotificationPostFailure.permissionDenied
        }

        do {
            try await addNotification(request)
        } catch let failure as LocalNotificationPostFailure {
            throw failure
        } catch {
            throw LocalNotificationPostFailure.postingFailed(error.localizedDescription)
        }
    }

    public static func addSystemNotification(_ request: LocalNotificationPostRequest) async throws {
        let content = UNMutableNotificationContent()
        content.title = request.title
        content.body = request.body
        content.sound = .default
        if !request.subtitle.isEmpty {
            content.subtitle = request.subtitle
        }
        if !request.threadID.isEmpty {
            content.threadIdentifier = request.threadID
        }

        let identifier = request.notificationID.isEmpty
            ? "thand-local-notification-\(UUID().uuidString)"
            : request.notificationID
        let notificationRequest = UNNotificationRequest(
            identifier: identifier,
            content: content,
            trigger: nil
        )
        try await UNUserNotificationCenter.current().add(notificationRequest)
    }
}
