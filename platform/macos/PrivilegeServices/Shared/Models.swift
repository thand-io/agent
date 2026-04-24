import Foundation

public enum BrokerOperation: String, Codable, Sendable {
    case timedSudoersGrant = "timed_sudoers_grant"
    case timedSudoersRevoke = "timed_sudoers_revoke"
    case execCommand = "exec_command"
    case ptySession = "pty_session"
    case localPresenceProof = "local_presence_proof"
}

public struct TimedSudoersGrantRequest: Codable, Sendable, Equatable {
    public let grantID: String
    public let deviceID: String
    public let targetUsername: String
    public let roleName: String
    public let duration: TimeInterval
    public let deniedUsernames: [String]
    public let allowedUIDRanges: [String]
    public let requestExpiresAt: Date?

    public init(
        grantID: String,
        deviceID: String,
        targetUsername: String,
        roleName: String,
        duration: TimeInterval,
        deniedUsernames: [String] = [],
        allowedUIDRanges: [String] = [],
        requestExpiresAt: Date? = nil
    ) {
        self.grantID = grantID
        self.deviceID = deviceID
        self.targetUsername = targetUsername
        self.roleName = roleName
        self.duration = duration
        self.deniedUsernames = deniedUsernames
        self.allowedUIDRanges = allowedUIDRanges
        self.requestExpiresAt = requestExpiresAt
    }

    enum CodingKeys: String, CodingKey {
        case grantID = "grant_id"
        case deviceID = "device_id"
        case targetUsername = "target_username"
        case roleName = "role_name"
        case duration
        case deniedUsernames = "denied_usernames"
        case allowedUIDRanges = "allowed_uid_ranges"
        case requestExpiresAt = "request_expires_at"
    }
}

public struct TimedSudoersGrantResponse: Codable, Sendable, Equatable {
    public let brokerHandle: String
    public let targetUsername: String
    public let expiresAt: Date

    public init(brokerHandle: String, targetUsername: String, expiresAt: Date) {
        self.brokerHandle = brokerHandle
        self.targetUsername = targetUsername
        self.expiresAt = expiresAt
    }

    enum CodingKeys: String, CodingKey {
        case brokerHandle = "broker_handle"
        case targetUsername = "target_username"
        case expiresAt = "expires_at"
    }
}

public struct RevokeTimedGrantRequest: Codable, Sendable, Equatable {
    public let brokerHandle: String

    public init(brokerHandle: String) {
        self.brokerHandle = brokerHandle
    }

    enum CodingKeys: String, CodingKey {
        case brokerHandle = "broker_handle"
    }
}

public enum RevokeTimedGrantStatus: String, Codable, Sendable {
    case revoked = "revoked"
    case notFound = "not_found"
}

public struct RevokeTimedGrantResponse: Codable, Sendable, Equatable {
    public let status: RevokeTimedGrantStatus

    public init(status: RevokeTimedGrantStatus) {
        self.status = status
    }
}

public struct BrokerControlRequest: Codable, Sendable, Equatable {
    public let operation: BrokerOperation
    public let timedSudoersGrant: TimedSudoersGrantRequest?
    public let revokeTimedGrant: RevokeTimedGrantRequest?

    public init(
        operation: BrokerOperation,
        timedSudoersGrant: TimedSudoersGrantRequest? = nil,
        revokeTimedGrant: RevokeTimedGrantRequest? = nil
    ) {
        self.operation = operation
        self.timedSudoersGrant = timedSudoersGrant
        self.revokeTimedGrant = revokeTimedGrant
    }

    enum CodingKeys: String, CodingKey {
        case operation
        case timedSudoersGrant = "timed_sudoers_grant"
        case revokeTimedGrant = "revoke_timed_grant"
    }
}

public struct BrokerControlResponse: Codable, Sendable, Equatable {
    public let timedSudoersGrant: TimedSudoersGrantResponse?
    public let revokeTimedGrant: RevokeTimedGrantResponse?

    public init(
        timedSudoersGrant: TimedSudoersGrantResponse? = nil,
        revokeTimedGrant: RevokeTimedGrantResponse? = nil
    ) {
        self.timedSudoersGrant = timedSudoersGrant
        self.revokeTimedGrant = revokeTimedGrant
    }

    enum CodingKeys: String, CodingKey {
        case timedSudoersGrant = "timed_sudoers_grant"
        case revokeTimedGrant = "revoke_timed_grant"
    }
}

public enum BrokerFailureCode: String, Codable, Sendable {
    case invalidRequest = "invalid_request"
    case invalidUsername = "invalid_username"
    case deniedUsername = "denied_username"
    case unknownUser = "unknown_user"
    case uidOutOfRange = "uid_out_of_range"
    case invalidUIDRange = "invalid_uid_range"
    case missingLease = "missing_lease"
    case visudoFailed = "visudo_failed"
    case grantIDConflict = "grant_id_conflict"
    case grantAlreadyCompleted = "grant_already_completed"
    case peerRejected = "peer_rejected"
    case unavailable = "unavailable"
    case internalError = "internal_error"
}

public struct BrokerFailure: Codable, Sendable, Equatable {
    public let code: BrokerFailureCode
    public let message: String

    public init(code: BrokerFailureCode, message: String) {
        self.code = code
        self.message = message
    }
}

public struct BrokerWireMessage: Codable, Sendable, Equatable {
    public let controlRequest: BrokerControlRequest?
    public let controlResponse: BrokerControlResponse?
    public let failure: BrokerFailure?
    public let error: String?

    public init(
        controlRequest: BrokerControlRequest? = nil,
        controlResponse: BrokerControlResponse? = nil,
        failure: BrokerFailure? = nil,
        error: String? = nil
    ) {
        self.controlRequest = controlRequest
        self.controlResponse = controlResponse
        self.failure = failure
        self.error = error
    }

    enum CodingKeys: String, CodingKey {
        case controlRequest = "control_request"
        case controlResponse = "control_response"
        case failure
        case error
    }
}

public enum BrokerEventKind: String, Codable, Sendable {
    case grantCreated = "grant_created"
    case grantRevoked = "grant_revoked"
    case grantExpired = "grant_expired"
    case grantRevokeFailed = "grant_revoke_failed"
}

public struct BrokerEvent: Codable, Sendable, Equatable {
    public let kind: BrokerEventKind
    public let brokerHandle: String
    public let grantID: String
    public let deviceID: String
    public let username: String
    public let message: String?
    public let occurredAt: Date

    public init(
        kind: BrokerEventKind,
        brokerHandle: String,
        grantID: String,
        deviceID: String,
        username: String,
        message: String? = nil,
        occurredAt: Date
    ) {
        self.kind = kind
        self.brokerHandle = brokerHandle
        self.grantID = grantID
        self.deviceID = deviceID
        self.username = username
        self.message = message
        self.occurredAt = occurredAt
    }

    enum CodingKeys: String, CodingKey {
        case kind
        case brokerHandle = "broker_handle"
        case grantID = "grant_id"
        case deviceID = "device_id"
        case username
        case message
        case occurredAt = "occurred_at"
    }
}

public struct LeaseRecord: Codable, Sendable, Equatable {
    public let brokerHandle: String
    public let grantID: String
    public let deviceID: String
    public let username: String
    public let roleName: String
    public let sudoersFragmentPath: String
    public let expiresAt: Date
    public let createdAt: Date

    public init(
        brokerHandle: String,
        grantID: String,
        deviceID: String,
        username: String,
        roleName: String,
        sudoersFragmentPath: String,
        expiresAt: Date,
        createdAt: Date
    ) {
        self.brokerHandle = brokerHandle
        self.grantID = grantID
        self.deviceID = deviceID
        self.username = username
        self.roleName = roleName
        self.sudoersFragmentPath = sudoersFragmentPath
        self.expiresAt = expiresAt
        self.createdAt = createdAt
    }

    enum CodingKeys: String, CodingKey {
        case brokerHandle = "broker_handle"
        case grantID = "grant_id"
        case deviceID = "device_id"
        case username
        case roleName = "role_name"
        case sudoersFragmentPath = "sudoers_fragment_path"
        case expiresAt = "expires_at"
        case createdAt = "created_at"
    }
}

public enum GrantLedgerState: String, Codable, Sendable {
    case active
    case revoked
    case expired
}

public struct GrantLedgerRecord: Codable, Sendable, Equatable {
    public let grantID: String
    public let requestFingerprint: String
    public let brokerHandle: String
    public let deviceID: String
    public let username: String
    public let roleName: String
    public let sudoersFragmentPath: String
    public let expiresAt: Date
    public let createdAt: Date
    public let state: GrantLedgerState
    public let terminalAt: Date?
    public let terminalReason: String?

    public init(
        grantID: String,
        requestFingerprint: String,
        brokerHandle: String,
        deviceID: String,
        username: String,
        roleName: String,
        sudoersFragmentPath: String,
        expiresAt: Date,
        createdAt: Date,
        state: GrantLedgerState,
        terminalAt: Date? = nil,
        terminalReason: String? = nil
    ) {
        self.grantID = grantID
        self.requestFingerprint = requestFingerprint
        self.brokerHandle = brokerHandle
        self.deviceID = deviceID
        self.username = username
        self.roleName = roleName
        self.sudoersFragmentPath = sudoersFragmentPath
        self.expiresAt = expiresAt
        self.createdAt = createdAt
        self.state = state
        self.terminalAt = terminalAt
        self.terminalReason = terminalReason
    }

    public func withTerminalState(
        _ state: GrantLedgerState,
        terminalAt: Date,
        terminalReason: String
    ) -> GrantLedgerRecord {
        GrantLedgerRecord(
            grantID: grantID,
            requestFingerprint: requestFingerprint,
            brokerHandle: brokerHandle,
            deviceID: deviceID,
            username: username,
            roleName: roleName,
            sudoersFragmentPath: sudoersFragmentPath,
            expiresAt: expiresAt,
            createdAt: createdAt,
            state: state,
            terminalAt: terminalAt,
            terminalReason: terminalReason
        )
    }

    enum CodingKeys: String, CodingKey {
        case grantID = "grant_id"
        case requestFingerprint = "request_fingerprint"
        case brokerHandle = "broker_handle"
        case deviceID = "device_id"
        case username
        case roleName = "role_name"
        case sudoersFragmentPath = "sudoers_fragment_path"
        case expiresAt = "expires_at"
        case createdAt = "created_at"
        case state
        case terminalAt = "terminal_at"
        case terminalReason = "terminal_reason"
    }
}

public enum BrokerJSON {
    public static func makeEncoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        return encoder
    }

    public static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}
