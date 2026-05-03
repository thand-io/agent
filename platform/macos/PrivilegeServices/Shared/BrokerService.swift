import Dispatch
import Foundation

public enum XPCTransportError: Error, CustomStringConvertible {
    case invalidMessage(String)
    case peerRequirementRejected(String)
    case xpcFailure(String)

    public var description: String {
        switch self {
        case .invalidMessage(let message):
            return message
        case .peerRequirementRejected(let message):
            return message
        case .xpcFailure(let message):
            return message
        }
    }
}

public final class PrivilegeBrokerService {
    public typealias EventSink = @Sendable (BrokerEvent) -> Void

    private struct Subscriber {
        let username: String
        let sink: EventSink
    }

    public let config: BrokerConfig
    public let peerRequirementPolicy: PeerRequirementPolicy

    private let validator: LocalAccountValidator
    private let leaseStore: LeaseStore
    private let grantManager: SudoersGrantManager
    private let now: @Sendable () -> Date
    private let tombstoneRetention: TimeInterval
    private let queue = DispatchQueue(label: "io.thand.agent.privilege-broker.service")
    private let queueKey = DispatchSpecificKey<Void>()

    private var timers: [String: DispatchSourceTimer] = [:]
    private var subscribers: [UUID: Subscriber] = [:]

    public init(
        config: BrokerConfig,
        validator: LocalAccountValidator? = nil,
        leaseStore: LeaseStore? = nil,
        grantManager: SudoersGrantManager? = nil,
        tombstoneRetention: TimeInterval = 24 * 60 * 60,
        now: @escaping @Sendable () -> Date = Date.init
    ) throws {
        self.config = config
        self.peerRequirementPolicy = PeerRequirementPolicy()
        self.validator = validator ?? LocalAccountValidator()
        self.leaseStore = leaseStore ?? LeaseStore(directoryURL: config.stateDirectoryURL)
        self.grantManager = grantManager ?? SudoersGrantManager(
            sudoersDirectoryURL: config.sudoersDirectoryURL,
            visudoPath: config.visudoPath
        )
        self.now = now
        self.tombstoneRetention = tombstoneRetention
        self.queue.setSpecific(key: queueKey, value: ())

        try self.leaseStore.prepareDirectory()
        try pruneTombstones(at: now())
        try restorePersistedLeases()
        brokerLog("initialized privilege broker service", fields: [
            "service_label": config.serviceLabel,
            "state_dir": config.stateDirectoryURL.path
        ])
    }

    public func handle(_ request: BrokerControlRequest) throws -> BrokerControlResponse {
        brokerLog("handling broker control request", fields: [
            "operation": request.operation.rawValue
        ])

        switch request.operation {
        case .timedSudoersGrant:
            guard let grantRequest = request.timedSudoersGrant else {
                throw BrokerServiceError.invalidRequest("timed sudoers grant payload is required")
            }
            return BrokerControlResponse(timedSudoersGrant: try grantTimedSudoers(grantRequest))
        case .timedSudoersRevoke:
            guard let revokeRequest = request.revokeTimedGrant else {
                throw BrokerServiceError.invalidRequest("revoke payload is required")
            }
            return BrokerControlResponse(revokeTimedGrant: try revokeTimedGrant(handle: revokeRequest.brokerHandle))
        case .postLocalNotification:
            guard let notificationRequest = request.localNotification else {
                throw BrokerServiceError.invalidRequest("local notification payload is required")
            }
            try postLocalNotification(notificationRequest)
            return BrokerControlResponse()
        default:
            throw BrokerServiceError.invalidRequest("unsupported broker operation \(request.operation.rawValue)")
        }
    }

    public func grantTimedSudoers(_ request: TimedSudoersGrantRequest) throws -> TimedSudoersGrantResponse {
        brokerLog("granting timed sudoers access", fields: [
            "grant_id": request.grantID,
            "device_id": request.deviceID,
            "target_username": request.targetUsername,
            "role_name": request.roleName,
            "duration_seconds": String(Int(request.duration))
        ])

        guard request.duration > 0 else {
            throw BrokerServiceError.invalidRequest("grant duration must be positive")
        }

        let currentTime = now()
        if let requestDeadline = request.requestExpiresAt, requestDeadline <= currentTime {
            throw BrokerServiceError.invalidRequest("grant request expired before broker approval")
        }

        try pruneTombstones(at: currentTime)

        let fingerprint = try request.requestFingerprint()
        if let existing = try leaseStore.loadGrant(grantID: request.grantID) {
            switch existing.state {
            case .active:
                guard existing.requestFingerprint == fingerprint else {
                    throw BrokerServiceError.grantIDConflict(request.grantID)
                }

                let lease = try ensureActiveGrantMaterialized(existing, at: currentTime)
                syncOnQueue {
                    self.scheduleLease(lease)
                }
                return TimedSudoersGrantResponse(
                    brokerHandle: lease.brokerHandle,
                    targetUsername: lease.username,
                    expiresAt: lease.expiresAt
                )
            case .revoked, .expired:
                throw BrokerServiceError.grantAlreadyCompleted(request.grantID, existing.state)
            }
        }

        let validatedUser = try validator.validate(
            username: request.targetUsername,
            deniedUsernames: request.deniedUsernames,
            allowedUIDRanges: request.allowedUIDRanges
        )

        let brokerHandle = UUID().uuidString.lowercased()
        let fragmentURL = grantManager.fragmentURL(for: brokerHandle)
        let expiresAt = currentTime.addingTimeInterval(request.duration)
        let lease = LeaseRecord(
            brokerHandle: brokerHandle,
            grantID: request.grantID,
            deviceID: request.deviceID,
            username: validatedUser.username,
            roleName: request.roleName,
            sudoersFragmentPath: fragmentURL.path,
            expiresAt: expiresAt,
            createdAt: currentTime
        )
        let ledger = GrantLedgerRecord(
            grantID: request.grantID,
            requestFingerprint: fingerprint,
            brokerHandle: brokerHandle,
            deviceID: request.deviceID,
            username: validatedUser.username,
            roleName: request.roleName,
            sudoersFragmentPath: fragmentURL.path,
            expiresAt: expiresAt,
            createdAt: currentTime,
            state: .active
        )

        try leaseStore.saveGrant(ledger)
        do {
            try materializeGrant(lease)
        } catch {
            try? leaseStore.removeGrant(grantID: ledger.grantID)
            throw error
        }

        syncOnQueue {
            self.scheduleLease(lease)
        }

        publish(BrokerEvent(
            kind: .grantCreated,
            brokerHandle: brokerHandle,
            grantID: request.grantID,
            deviceID: request.deviceID,
            username: validatedUser.username,
            occurredAt: currentTime
        ))

        return TimedSudoersGrantResponse(
            brokerHandle: brokerHandle,
            targetUsername: validatedUser.username,
            expiresAt: expiresAt
        )
    }

    public func revokeTimedGrant(handle: String) throws -> RevokeTimedGrantResponse {
        brokerLog("revoking timed sudoers access", fields: [
            "broker_handle": handle
        ])
        return try revokeTimedGrant(handle: handle, eventKind: .grantRevoked)
    }

    public func postLocalNotification(_ request: BrokerLocalNotificationRequest) throws {
        let username = request.username.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !username.isEmpty else {
            throw BrokerServiceError.invalidRequest("local notification username is required")
        }
        guard !request.notification.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw BrokerServiceError.invalidRequest("local notification title is required")
        }
        guard !request.notification.body.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            throw BrokerServiceError.invalidRequest("local notification body is required")
        }

        let delivered = publish(BrokerEvent(
            kind: .localNotification,
            brokerHandle: request.notification.notificationID,
            grantID: "",
            deviceID: "",
            username: username,
            localNotification: request.notification,
            occurredAt: now()
        ))
        guard delivered > 0 else {
            throw BrokerServiceError.unavailable("no active privilege notifier subscription for \(username)")
        }
    }

    public func subscribe(username: String, sink: @escaping EventSink) -> UUID {
        let token = UUID()
        syncOnQueue {
            subscribers[token] = Subscriber(username: username, sink: sink)
        }
        return token
    }

    public func unsubscribe(_ token: UUID) {
        _ = syncOnQueue {
            subscribers.removeValue(forKey: token)
        }
    }

    private func revokeTimedGrant(handle: String, eventKind: BrokerEventKind) throws -> RevokeTimedGrantResponse {
        let currentTime = now()

        if let lease = try leaseStore.load(handle: handle) {
            syncOnQueue {
                self.cancelTimer(for: handle)
            }
            try grantManager.revoke(fragmentURL: URL(fileURLWithPath: lease.sudoersFragmentPath))
            try leaseStore.remove(handle: handle)
            try markGrantCompleted(
                grantID: lease.grantID,
                fallbackHandle: handle,
                state: eventKind == .grantExpired ? .expired : .revoked,
                terminalAt: currentTime,
                terminalReason: eventKind.rawValue
            )

            publish(BrokerEvent(
                kind: eventKind,
                brokerHandle: handle,
                grantID: lease.grantID,
                deviceID: lease.deviceID,
                username: lease.username,
                occurredAt: currentTime
            ))

            return RevokeTimedGrantResponse(status: .revoked)
        }

        if let record = try leaseStore.loadGrant(brokerHandle: handle), record.state == .active {
            syncOnQueue {
                self.cancelTimer(for: handle)
            }
            try leaseStore.saveGrant(record.withTerminalState(
                eventKind == .grantExpired ? .expired : .revoked,
                terminalAt: currentTime,
                terminalReason: "lease already absent when handling \(eventKind.rawValue)"
            ))
        }

        return RevokeTimedGrantResponse(status: .notFound)
    }

    private func restorePersistedLeases() throws {
        let currentTime = now()
        try pruneTombstones(at: currentTime)

        let leases = try leaseStore.loadAll()
        brokerLog("restoring persisted leases", fields: [
            "lease_count": String(leases.count)
        ])

        for record in try leaseStore.loadAllGrants() where record.state == .active && record.expiresAt <= currentTime {
            if try leaseStore.load(handle: record.brokerHandle) == nil {
                try leaseStore.saveGrant(record.withTerminalState(
                    .expired,
                    terminalAt: currentTime,
                    terminalReason: "expired while inactive during broker startup"
                ))
            }
        }

        for lease in leases {
            if lease.expiresAt <= currentTime {
                do {
                    try grantManager.revoke(fragmentURL: URL(fileURLWithPath: lease.sudoersFragmentPath))
                    try leaseStore.remove(handle: lease.brokerHandle)
                    try markGrantCompleted(
                        grantID: lease.grantID,
                        fallbackHandle: lease.brokerHandle,
                        state: .expired,
                        terminalAt: currentTime,
                        terminalReason: "expired during broker startup restore"
                    )
                } catch {
                    publish(BrokerEvent(
                        kind: .grantRevokeFailed,
                        brokerHandle: lease.brokerHandle,
                        grantID: lease.grantID,
                        deviceID: lease.deviceID,
                        username: lease.username,
                        message: String(describing: error),
                        occurredAt: currentTime
                    ))
                }
                continue
            }

            syncOnQueue {
                self.scheduleLease(lease)
            }
        }
    }

    private func materializeGrant(_ lease: LeaseRecord) throws {
        let fragmentURL = try grantManager.installTimedGrant(
            username: lease.username,
            grantID: lease.grantID,
            brokerHandle: lease.brokerHandle
        )

        let materializedLease = LeaseRecord(
            brokerHandle: lease.brokerHandle,
            grantID: lease.grantID,
            deviceID: lease.deviceID,
            username: lease.username,
            roleName: lease.roleName,
            sudoersFragmentPath: fragmentURL.path,
            expiresAt: lease.expiresAt,
            createdAt: lease.createdAt
        )

        do {
            try leaseStore.save(materializedLease)
        } catch {
            try? grantManager.revoke(fragmentURL: fragmentURL)
            throw error
        }
    }

    private func ensureActiveGrantMaterialized(_ record: GrantLedgerRecord, at currentTime: Date) throws -> LeaseRecord {
        guard record.state == .active else {
            throw BrokerServiceError.grantAlreadyCompleted(record.grantID, record.state)
        }
        if record.expiresAt <= currentTime {
            try leaseStore.saveGrant(record.withTerminalState(
                .expired,
                terminalAt: currentTime,
                terminalReason: "authorize replay arrived after expiry"
            ))
            try leaseStore.remove(handle: record.brokerHandle)
            syncOnQueue {
                self.cancelTimer(for: record.brokerHandle)
            }
            throw BrokerServiceError.grantAlreadyCompleted(record.grantID, .expired)
        }

        let lease: LeaseRecord
        if let existingLease = try leaseStore.load(handle: record.brokerHandle) {
            lease = existingLease
        } else {
            lease = LeaseRecord(
                brokerHandle: record.brokerHandle,
                grantID: record.grantID,
                deviceID: record.deviceID,
                username: record.username,
                roleName: record.roleName,
                sudoersFragmentPath: record.sudoersFragmentPath,
                expiresAt: record.expiresAt,
                createdAt: record.createdAt
            )
            try materializeGrant(lease)
        }

        if !FileManager.default.fileExists(atPath: lease.sudoersFragmentPath) {
            try materializeGrant(lease)
        }

        return lease
    }

    private func markGrantCompleted(
        grantID: String,
        fallbackHandle: String,
        state: GrantLedgerState,
        terminalAt: Date,
        terminalReason: String
    ) throws {
        if let record = try leaseStore.loadGrant(grantID: grantID) {
            try leaseStore.saveGrant(record.withTerminalState(
                state,
                terminalAt: terminalAt,
                terminalReason: terminalReason
            ))
            return
        }

        if let record = try leaseStore.loadGrant(brokerHandle: fallbackHandle) {
            try leaseStore.saveGrant(record.withTerminalState(
                state,
                terminalAt: terminalAt,
                terminalReason: terminalReason
            ))
        }
    }

    private func pruneTombstones(at currentTime: Date) throws {
        let cutoff = currentTime.addingTimeInterval(-tombstoneRetention)
        _ = try leaseStore.pruneGrantTombstones(olderThan: cutoff)
    }

    private func scheduleLease(_ lease: LeaseRecord) {
        brokerLog("scheduling lease expiry", fields: [
            "broker_handle": lease.brokerHandle,
            "username": lease.username,
            "expires_at": lease.expiresAt.ISO8601Format()
        ])
        cancelTimer(for: lease.brokerHandle)

        let timer = DispatchSource.makeTimerSource(queue: queue)
        let delay = max(0, lease.expiresAt.timeIntervalSince(now()))
        timer.schedule(deadline: .now() + delay)
        timer.setEventHandler { [weak self] in
            self?.expireLease(handle: lease.brokerHandle)
        }
        timers[lease.brokerHandle] = timer
        timer.resume()
    }

    private func cancelTimer(for handle: String) {
        guard let timer = timers.removeValue(forKey: handle) else {
            return
        }
        timer.cancel()
    }

    private func expireLease(handle: String) {
        brokerLog("expiring lease", fields: [
            "broker_handle": handle
        ])
        do {
            _ = try revokeTimedGrant(handle: handle, eventKind: .grantExpired)
        } catch {
            brokerLog("failed to expire lease", fields: [
                "broker_handle": handle,
                "error": String(describing: error)
            ])
            if let lease = try? leaseStore.load(handle: handle) {
                publish(BrokerEvent(
                    kind: .grantRevokeFailed,
                    brokerHandle: handle,
                    grantID: lease.grantID,
                    deviceID: lease.deviceID,
                    username: lease.username,
                    message: String(describing: error),
                    occurredAt: now()
                ))
            }
        }
    }

    @discardableResult
    private func publish(_ event: BrokerEvent) -> Int {
        brokerLog("publishing broker event", fields: [
            "event_kind": event.kind.rawValue,
            "broker_handle": event.brokerHandle,
            "username": event.username
        ])
        let sinks: [EventSink] = syncOnQueue {
            subscribers.values
                .filter { $0.username == event.username }
                .map(\.sink)
        }

        for sink in sinks {
            sink(event)
        }
        return sinks.count
    }

    private func syncOnQueue<T>(_ work: () throws -> T) rethrows -> T {
        if DispatchQueue.getSpecific(key: queueKey) != nil {
            return try work()
        }
        return try queue.sync(execute: work)
    }
}
