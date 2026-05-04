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

public enum LocalNotificationPostFailure: Error, Equatable, CustomStringConvertible, LocalizedError {
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

    // LocalizedError conformance ensures the bridged NSError surfaces the
    // case-specific message instead of the generic "The operation couldn't
    // be completed. (... error N.)" — which otherwise hides whether the
    // failure was permission, bundle context, or a posting error.
    public var errorDescription: String? { description }
}

public protocol LocalNotificationPosting: Sendable {
    func post(_ request: LocalNotificationPostRequest) async throws
}

public struct LocalNotificationPoster: LocalNotificationPosting {
    private let requestAuthorization: @Sendable () async throws -> Bool
    private let addNotification: @Sendable (LocalNotificationPostRequest) async throws -> Void

    public init(
        requestAuthorization: @escaping @Sendable () async throws -> Bool = {
            // UNUserNotificationCenter.current() aborts via NSException
            // (`bundleProxyForCurrentProcess is nil`) when invoked from a
            // non-bundled CLI — which is exactly how the privilege broker
            // is launched (a bare executable under .../PrivilegeBroker/bin/).
            // Throw a typed error before we trigger that abort so the
            // caller surfaces a clean gRPC error instead of crashing the
            // whole broker process.
            try Self.requireBundleContext()
            return try await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge])
        },
        addNotification: @escaping @Sendable (LocalNotificationPostRequest) async throws -> Void = { request in
            try Self.requireBundleContext()
            try await Self.addSystemNotification(request)
        }
    ) {
        self.requestAuthorization = requestAuthorization
        self.addNotification = addNotification
    }

    /// Throws `notificationCenterUnavailable` when the host process does
    /// not have a `CFBundleIdentifier`. `UserNotifications` requires a
    /// bundled app context; without it, calls to
    /// `UNUserNotificationCenter.current()` abort the process with an
    /// `NSInternalInconsistencyException` rather than throwing a Swift
    /// error. Detect that condition early so the broker keeps running.
    ///
    /// `public` so default-argument expressions in the `public init`
    /// (which Swift treats as part of the public API surface) can call
    /// it; the helper is otherwise an implementation detail.
    public static func requireBundleContext() throws {
        if Bundle.main.bundleIdentifier == nil {
            throw LocalNotificationPostFailure.notificationCenterUnavailable(
                "host process is not running inside an application bundle; \(Bundle.main.bundleURL.path)"
            )
        }
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
