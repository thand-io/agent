import Foundation
import LocalAuthentication

public struct LocalPresenceCheckRequest: Sendable, Equatable {
    public let challengeID: String
    public let deviceID: String
    public let workflowID: String
    public let taskName: String
    public let prompt: String
    public let timeout: TimeInterval
    public let requestedBy: String
    public let roleName: String
    public let reason: String

    public init(
        challengeID: String,
        deviceID: String,
        workflowID: String,
        taskName: String,
        prompt: String,
        timeout: TimeInterval,
        requestedBy: String = "",
        roleName: String = "",
        reason: String = ""
    ) {
        self.challengeID = challengeID
        self.deviceID = deviceID
        self.workflowID = workflowID
        self.taskName = taskName
        self.prompt = prompt
        self.timeout = timeout
        self.requestedBy = requestedBy
        self.roleName = roleName
        self.reason = reason
    }
}

public struct LocalPresenceCheckResult: Sendable, Equatable {
    public let approved: Bool
    public let authenticatedAt: Date?
    public let failureReason: String?

    public init(approved: Bool, authenticatedAt: Date? = nil, failureReason: String? = nil) {
        self.approved = approved
        self.authenticatedAt = authenticatedAt
        self.failureReason = failureReason
    }
}

public protocol LocalPresenceAuthenticating: Sendable {
    func checkPresence(_ request: LocalPresenceCheckRequest) async -> LocalPresenceCheckResult
}

private final class LAContextBox: @unchecked Sendable {
    let context: LAContext

    init(_ context: LAContext) {
        self.context = context
    }
}

public struct LocalPresenceAuthenticator: LocalPresenceAuthenticating {
    private let now: @Sendable () -> Date
    private let evaluatePolicy: @Sendable (String) async throws -> Bool

    public init(
        now: @escaping @Sendable () -> Date = { Date() },
        contextFactory: @escaping @Sendable () -> LAContext = { LAContext() }
    ) {
        self.init(now: now) { prompt in
            let box = LAContextBox(contextFactory())
            return try await Self.evaluateDeviceOwnerAuthentication(box: box, prompt: prompt)
        }
    }

    public init(
        now: @escaping @Sendable () -> Date = { Date() },
        evaluatePolicy: @escaping @Sendable (String) async throws -> Bool
    ) {
        self.now = now
        self.evaluatePolicy = evaluatePolicy
    }

    public func checkPresence(_ request: LocalPresenceCheckRequest) async -> LocalPresenceCheckResult {
        let timeout = request.timeout
        guard timeout > 0 else {
            return LocalPresenceCheckResult(
                approved: false,
                failureReason: "local presence timeout must be positive"
            )
        }

        let prompt = request.prompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? "Approve this access request on your Mac"
            : request.prompt

        return await withTaskGroup(of: LocalPresenceCheckResult.self) { group in
            group.addTask {
                do {
                    let approved = try await evaluatePolicy(prompt)
                    if approved {
                        return LocalPresenceCheckResult(approved: true, authenticatedAt: now())
                    }
                    return LocalPresenceCheckResult(
                        approved: false,
                        failureReason: "local presence was not approved"
                    )
                } catch {
                    return LocalPresenceCheckResult(
                        approved: false,
                        failureReason: Self.failureReason(for: error)
                    )
                }
            }

            group.addTask {
                try? await Task.sleep(nanoseconds: Self.timeoutNanoseconds(timeout))
                return LocalPresenceCheckResult(
                    approved: false,
                    failureReason: "timed out waiting for local presence"
                )
            }

            let result = await group.next() ?? LocalPresenceCheckResult(
                approved: false,
                failureReason: "local presence did not return a result"
            )
            group.cancelAll()
            return result
        }
    }

    private static func evaluateDeviceOwnerAuthentication(box: LAContextBox, prompt: String) async throws -> Bool {
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                box.context.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: prompt) { success, error in
                    if success {
                        continuation.resume(returning: true)
                    } else if let error {
                        continuation.resume(throwing: error)
                    } else {
                        continuation.resume(returning: false)
                    }
                }
            }
        } onCancel: {
            box.context.invalidate()
        }
    }

    private static func timeoutNanoseconds(_ timeout: TimeInterval) -> UInt64 {
        let nanoseconds = timeout * 1_000_000_000
        if nanoseconds >= Double(UInt64.max) {
            return UInt64.max
        }
        return UInt64(Swift.max(0, nanoseconds))
    }

    private static func failureReason(for error: Error) -> String {
        let nsError = error as NSError
        guard nsError.domain == LAError.errorDomain,
              let code = LAError.Code(rawValue: nsError.code)
        else {
            return error.localizedDescription
        }

        switch code {
        case .authenticationFailed:
            return "local presence authentication failed"
        case .userCancel, .systemCancel:
            return "user canceled local presence"
        case .userFallback:
            return "user requested password fallback"
        case .biometryNotAvailable:
            return "biometry is not available"
        case .biometryNotEnrolled:
            return "biometry is not enrolled"
        case .biometryLockout:
            return "biometry is locked out"
        case .passcodeNotSet:
            return "passcode is not set"
        default:
            return error.localizedDescription
        }
    }
}
