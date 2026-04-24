import CryptoKit
import Foundation

extension TimedSudoersGrantRequest {
    public func requestFingerprint() throws -> String {
        let normalizedDeniedUsernames = Array(
            Set(
                deniedUsernames
                    .map { $0.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() }
                    .filter { !$0.isEmpty }
            )
        ).sorted()

        let normalizedUIDRanges = try LocalAccountValidator.parseUIDRanges(allowedUIDRanges)
            .map { "\($0.min)-\($0.max)" }
            .sorted()

        let normalizedDurationMillis = Int64((duration * 1_000).rounded())
        let requestDeadlineMillis: Int64
        if let requestExpiresAt {
            requestDeadlineMillis = Int64((requestExpiresAt.timeIntervalSince1970 * 1_000).rounded())
        } else {
            requestDeadlineMillis = 0
        }

        let canonical = [
            "grant_id=\(grantID)",
            "device_id=\(deviceID.trimmingCharacters(in: .whitespacesAndNewlines))",
            "target_username=\(targetUsername.trimmingCharacters(in: .whitespacesAndNewlines))",
            "role_name=\(roleName.trimmingCharacters(in: .whitespacesAndNewlines))",
            "duration_millis=\(normalizedDurationMillis)",
            "request_expires_at_unix_millis=\(requestDeadlineMillis)",
            "denied_usernames=\(normalizedDeniedUsernames.joined(separator: ","))",
            "allowed_uid_ranges=\(normalizedUIDRanges.joined(separator: ","))"
        ].joined(separator: "\u{1F}")

        return Self.sha256Hex(canonical)
    }

    private static func sha256Hex(_ value: String) -> String {
        SHA256.hash(data: Data(value.utf8)).map { String(format: "%02x", $0) }.joined()
    }
}

extension BrokerServiceError {
    public var failureCode: BrokerFailureCode {
        switch self {
        case .invalidRequest:
            return .invalidRequest
        case .invalidUsername:
            return .invalidUsername
        case .deniedUsername:
            return .deniedUsername
        case .unknownUser:
            return .unknownUser
        case .uidOutOfRange:
            return .uidOutOfRange
        case .invalidUIDRange:
            return .invalidUIDRange
        case .missingLease:
            return .missingLease
        case .visudoFailed:
            return .visudoFailed
        case .grantIDConflict:
            return .grantIDConflict
        case .grantAlreadyCompleted:
            return .grantAlreadyCompleted
        case .unavailable:
            return .unavailable
        }
    }

    public var failure: BrokerFailure {
        BrokerFailure(code: failureCode, message: description)
    }
}
