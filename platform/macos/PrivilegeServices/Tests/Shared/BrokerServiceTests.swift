import Foundation
import XCTest
@testable import PrivilegeServicesShared

private final class EventRecorder: @unchecked Sendable {
    private let lock = NSLock()
    private var events: [BrokerEvent] = []

    func append(_ event: BrokerEvent) {
        lock.lock()
        defer { lock.unlock() }
        events.append(event)
    }

    func snapshot() -> [BrokerEvent] {
        lock.lock()
        defer { lock.unlock() }
        return events
    }
}

final class BrokerServiceTests: XCTestCase {
    func testLeaseStoreRoundTripsLeases() throws {
        let directoryURL = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-lease-store-tests-\(UUID().uuidString)")
        let leaseStore = LeaseStore(directoryURL: directoryURL)

        let lease = LeaseRecord(
            brokerHandle: "handle-1",
            grantID: "grant-1",
            deviceID: "device-1",
            username: "tester",
            roleName: "local_sudo",
            sudoersFragmentPath: "/tmp/fragment",
            expiresAt: Date(timeIntervalSince1970: 2_000),
            createdAt: Date(timeIntervalSince1970: 1_000)
        )

        try leaseStore.save(lease)
        let loadedLease = try XCTUnwrap(leaseStore.load(handle: "handle-1"))
        XCTAssertEqual(loadedLease, lease)
    }

    func testLeaseStoreRoundTripsGrantLedgerAndPrunesOldTombstones() throws {
        let directoryURL = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-grant-store-tests-\(UUID().uuidString)")
        let leaseStore = LeaseStore(directoryURL: directoryURL)
        try leaseStore.prepareDirectory()

        let active = GrantLedgerRecord(
            grantID: "grant-active",
            requestFingerprint: "fingerprint-active",
            brokerHandle: "handle-active",
            deviceID: "device-1",
            username: "tester",
            roleName: "local_sudo",
            sudoersFragmentPath: "/tmp/thand-active",
            expiresAt: Date(timeIntervalSince1970: 2_000),
            createdAt: Date(timeIntervalSince1970: 1_000),
            state: .active
        )
        let oldTombstone = GrantLedgerRecord(
            grantID: "grant-old",
            requestFingerprint: "fingerprint-old",
            brokerHandle: "handle-old",
            deviceID: "device-1",
            username: "tester",
            roleName: "local_sudo",
            sudoersFragmentPath: "/tmp/thand-old",
            expiresAt: Date(timeIntervalSince1970: 2_000),
            createdAt: Date(timeIntervalSince1970: 1_000),
            state: .expired,
            terminalAt: Date(timeIntervalSince1970: 1_500),
            terminalReason: "expired for pruning"
        )
        let recentTombstone = GrantLedgerRecord(
            grantID: "grant-recent",
            requestFingerprint: "fingerprint-recent",
            brokerHandle: "handle-recent",
            deviceID: "device-1",
            username: "tester",
            roleName: "local_sudo",
            sudoersFragmentPath: "/tmp/thand-recent",
            expiresAt: Date(timeIntervalSince1970: 2_000),
            createdAt: Date(timeIntervalSince1970: 1_000),
            state: .revoked,
            terminalAt: Date(timeIntervalSince1970: 1_950),
            terminalReason: "revoked recently"
        )

        try leaseStore.saveGrant(active)
        try leaseStore.saveGrant(oldTombstone)
        try leaseStore.saveGrant(recentTombstone)

        XCTAssertEqual(try leaseStore.loadGrant(grantID: "grant-active"), active)
        XCTAssertEqual(try leaseStore.loadGrant(brokerHandle: "handle-old"), oldTombstone)

        let removed = try leaseStore.pruneGrantTombstones(olderThan: Date(timeIntervalSince1970: 1_800))

        XCTAssertEqual(removed, [oldTombstone])
        XCTAssertNil(try leaseStore.loadGrant(grantID: "grant-old"))
        XCTAssertEqual(try leaseStore.loadGrant(grantID: "grant-recent"), recentTombstone)
        XCTAssertEqual(try leaseStore.loadGrant(grantID: "grant-active"), active)
    }

    func testServiceRestoresAndExpiresPersistedLeases() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-service-tests-\(UUID().uuidString)")
        let stateDirectory = temporaryRoot.appendingPathComponent("state")
        let sudoersDirectory = temporaryRoot.appendingPathComponent("sudoers")
        let config = BrokerConfig(
            stateDirectoryURL: stateDirectory,
            sudoersDirectoryURL: sudoersDirectory
        )
        let leaseStore = LeaseStore(directoryURL: stateDirectory)
        try leaseStore.prepareDirectory()

        let fragmentURL = sudoersDirectory.appendingPathComponent("thand-handle-1")
        try FileManager.default.createDirectory(at: sudoersDirectory, withIntermediateDirectories: true)
        try "# Managed by Thand\ntester ALL=(ALL:ALL) NOPASSWD: ALL\n".write(to: fragmentURL, atomically: true, encoding: .utf8)

        try leaseStore.save(LeaseRecord(
            brokerHandle: "handle-1",
            grantID: "grant-1",
            deviceID: "device-1",
            username: "tester",
            roleName: "local_sudo",
            sudoersFragmentPath: fragmentURL.path,
            expiresAt: Date(timeIntervalSince1970: 10),
            createdAt: Date(timeIntervalSince1970: 5)
        ))

        _ = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            leaseStore: leaseStore,
            grantManager: SudoersGrantManager(
                sudoersDirectoryURL: sudoersDirectory,
                visudoPath: config.visudoPath,
                runCommand: { _ in }
            ),
            now: { Date(timeIntervalSince1970: 20) }
        )

        XCTAssertFalse(FileManager.default.fileExists(atPath: fragmentURL.path))
        XCTAssertNil(try leaseStore.load(handle: "handle-1"))
    }

    func testServiceRoutesEventsToMatchingSubscribers() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-events-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )

        let service = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            grantManager: SudoersGrantManager(
                sudoersDirectoryURL: config.sudoersDirectoryURL,
                visudoPath: config.visudoPath,
                runCommand: { _ in }
            ),
            now: { Date(timeIntervalSince1970: 100) }
        )

        let testerEvents = EventRecorder()
        let testerEventsSecondSession = EventRecorder()
        let otherEvents = EventRecorder()
        let testerToken = service.subscribe(username: "tester") { testerEvents.append($0) }
        let testerSecondToken = service.subscribe(username: "tester") { testerEventsSecondSession.append($0) }
        let otherToken = service.subscribe(username: "other") { otherEvents.append($0) }

        _ = try service.grantTimedSudoers(TimedSudoersGrantRequest(
            grantID: "grant-1",
            deviceID: "device-1",
            targetUsername: "tester",
            roleName: "local_sudo",
            duration: 60
        ))

        service.unsubscribe(testerToken)
        service.unsubscribe(testerSecondToken)
        service.unsubscribe(otherToken)

        let testerSnapshot = testerEvents.snapshot()
        let testerSecondSnapshot = testerEventsSecondSession.snapshot()
        let otherSnapshot = otherEvents.snapshot()

        XCTAssertEqual(testerSnapshot.count, 1)
        XCTAssertEqual(testerSecondSnapshot.count, 1)
        XCTAssertTrue(otherSnapshot.isEmpty)
        XCTAssertEqual(testerSnapshot.first?.kind, .grantCreated)
        XCTAssertEqual(testerSecondSnapshot.first?.kind, .grantCreated)
    }

    func testServicePublishesLocalNotificationToMatchingNotifierSubscriber() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-local-notification-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )

        let service = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            grantManager: SudoersGrantManager(
                sudoersDirectoryURL: config.sudoersDirectoryURL,
                visudoPath: config.visudoPath,
                runCommand: { _ in }
            ),
            now: { Date(timeIntervalSince1970: 100) }
        )

        let testerEvents = EventRecorder()
        let otherEvents = EventRecorder()
        let testerToken = service.subscribe(username: "tester") { testerEvents.append($0) }
        let otherToken = service.subscribe(username: "other") { otherEvents.append($0) }
        defer {
            service.unsubscribe(testerToken)
            service.unsubscribe(otherToken)
        }

        let notification = LocalNotificationPostRequest(
            notificationID: "notification-1",
            title: "Access approved",
            body: "Your sudo access is ready",
            threadID: "workflow-1"
        )
        try service.postLocalNotification(BrokerLocalNotificationRequest(
            username: "tester",
            notification: notification
        ))

        let testerSnapshot = testerEvents.snapshot()
        XCTAssertEqual(testerSnapshot.count, 1)
        XCTAssertEqual(testerSnapshot.first?.kind, .localNotification)
        XCTAssertEqual(testerSnapshot.first?.localNotification, notification)
        XCTAssertTrue(otherEvents.snapshot().isEmpty)
    }

    func testServiceRejectsLocalNotificationWithoutNotifierSubscriber() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-local-notification-missing-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )
        let service = try PrivilegeBrokerService(
            config: config,
            grantManager: SudoersGrantManager(
                sudoersDirectoryURL: config.sudoersDirectoryURL,
                visudoPath: config.visudoPath,
                runCommand: { _ in }
            )
        )

        XCTAssertThrowsError(try service.postLocalNotification(BrokerLocalNotificationRequest(
            username: "tester",
            notification: LocalNotificationPostRequest(
                notificationID: "notification-1",
                title: "Access approved",
                body: "Your sudo access is ready"
            )
        ))) { error in
            guard case BrokerServiceError.unavailable = error else {
                XCTFail("unexpected error \(error)")
                return
            }
        }
    }

    func testServiceRevokesActiveGrantAndPublishesRevokedEvent() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-revoke-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )
        let leaseStore = LeaseStore(directoryURL: config.stateDirectoryURL)
        let grantManager = SudoersGrantManager(
            sudoersDirectoryURL: config.sudoersDirectoryURL,
            visudoPath: config.visudoPath,
            runCommand: { _ in }
        )
        let service = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            leaseStore: leaseStore,
            grantManager: grantManager
        )

        let events = EventRecorder()
        let token = service.subscribe(username: "tester") { events.append($0) }
        defer { service.unsubscribe(token) }

        let response = try service.grantTimedSudoers(TimedSudoersGrantRequest(
            grantID: "grant-revoke-1",
            deviceID: "device-1",
            targetUsername: "tester",
            roleName: "local_sudo",
            duration: 60
        ))

        let revokeResponse = try service.revokeTimedGrant(handle: response.brokerHandle)

        XCTAssertEqual(revokeResponse.status, .revoked)
        XCTAssertNil(try leaseStore.load(handle: response.brokerHandle))
        XCTAssertFalse(
            FileManager.default.fileExists(
                atPath: grantManager.fragmentURL(for: response.brokerHandle).path
            )
        )
        XCTAssertEqual(events.snapshot().map(\.kind), [.grantCreated, .grantRevoked])
    }

    func testGrantTimedSudoersReturnsExistingLeaseForMatchingGrantID() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-grant-replay-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )
        let leaseStore = LeaseStore(directoryURL: config.stateDirectoryURL)
        let grantManager = SudoersGrantManager(
            sudoersDirectoryURL: config.sudoersDirectoryURL,
            visudoPath: config.visudoPath,
            runCommand: { _ in }
        )
        let service = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            leaseStore: leaseStore,
            grantManager: grantManager
        )

        let events = EventRecorder()
        let token = service.subscribe(username: "tester") { events.append($0) }
        defer { service.unsubscribe(token) }

        let initialRequest = TimedSudoersGrantRequest(
            grantID: "grant-replay-1",
            deviceID: "device-1",
            targetUsername: "tester",
            roleName: "local_sudo",
            duration: 60,
            deniedUsernames: ["root", "daemon"],
            allowedUIDRanges: ["700-800", "500-600"]
        )
        let replayRequest = TimedSudoersGrantRequest(
            grantID: "grant-replay-1",
            deviceID: "device-1",
            targetUsername: "tester",
            roleName: "local_sudo",
            duration: 60,
            deniedUsernames: ["daemon", "root"],
            allowedUIDRanges: ["500-600", "700-800"]
        )

        let initialResponse = try service.grantTimedSudoers(initialRequest)
        let replayResponse = try service.grantTimedSudoers(replayRequest)

        XCTAssertEqual(replayResponse.brokerHandle, initialResponse.brokerHandle)
        XCTAssertEqual(replayResponse.targetUsername, initialResponse.targetUsername)
        XCTAssertEqual(try leaseStore.loadAll().count, 1)
        XCTAssertEqual(try leaseStore.loadAllGrants().count, 1)
        XCTAssertEqual(events.snapshot().map(\.kind), [.grantCreated])
        XCTAssertTrue(
            FileManager.default.fileExists(
                atPath: grantManager.fragmentURL(for: initialResponse.brokerHandle).path
            )
        )
    }

    func testGrantTimedSudoersRejectsGrantIDConflict() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-grant-conflict-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )
        let leaseStore = LeaseStore(directoryURL: config.stateDirectoryURL)
        let service = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            leaseStore: leaseStore,
            grantManager: SudoersGrantManager(
                sudoersDirectoryURL: config.sudoersDirectoryURL,
                visudoPath: config.visudoPath,
                runCommand: { _ in }
            )
        )

        let initialRequest = TimedSudoersGrantRequest(
            grantID: "grant-conflict-1",
            deviceID: "device-1",
            targetUsername: "tester",
            roleName: "local_sudo",
            duration: 60
        )
        _ = try service.grantTimedSudoers(initialRequest)

        XCTAssertThrowsError(try service.grantTimedSudoers(TimedSudoersGrantRequest(
            grantID: "grant-conflict-1",
            deviceID: "device-1",
            targetUsername: "tester",
            roleName: "local_sudo",
            duration: 120
        ))) { error in
            guard case BrokerServiceError.grantIDConflict(let grantID) = error else {
                return XCTFail("unexpected error: \(error)")
            }
            XCTAssertEqual(grantID, "grant-conflict-1")
        }

        XCTAssertEqual(try leaseStore.loadAll().count, 1)
        XCTAssertEqual(try leaseStore.loadAllGrants().count, 1)
    }

    func testServiceRevokingMissingHandleReturnsNotFoundWithoutPublishingEvent() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-missing-revoke-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )
        let service = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            grantManager: SudoersGrantManager(
                sudoersDirectoryURL: config.sudoersDirectoryURL,
                visudoPath: config.visudoPath,
                runCommand: { _ in }
            )
        )

        let events = EventRecorder()
        let token = service.subscribe(username: "tester") { events.append($0) }
        defer { service.unsubscribe(token) }

        let revokeResponse = try service.revokeTimedGrant(handle: "missing-handle")

        XCTAssertEqual(revokeResponse.status, .notFound)
        XCTAssertTrue(events.snapshot().isEmpty)
    }

    func testGrantTimedSudoersRejectsCompletedGrantIDAfterRevoke() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-regrant-after-revoke-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )
        let leaseStore = LeaseStore(directoryURL: config.stateDirectoryURL)
        let grantManager = SudoersGrantManager(
            sudoersDirectoryURL: config.sudoersDirectoryURL,
            visudoPath: config.visudoPath,
            runCommand: { _ in }
        )
        let service = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            leaseStore: leaseStore,
            grantManager: grantManager
        )

        let request = TimedSudoersGrantRequest(
            grantID: "grant-revoked-1",
            deviceID: "device-1",
            targetUsername: "tester",
            roleName: "local_sudo",
            duration: 60
        )

        let response = try service.grantTimedSudoers(request)
        _ = try service.revokeTimedGrant(handle: response.brokerHandle)

        XCTAssertThrowsError(try service.grantTimedSudoers(request)) { error in
            guard case BrokerServiceError.grantAlreadyCompleted(let grantID, let state) = error else {
                return XCTFail("unexpected error: \(error)")
            }
            XCTAssertEqual(grantID, "grant-revoked-1")
            XCTAssertEqual(state, .revoked)
        }

        let ledger = try XCTUnwrap(leaseStore.loadGrant(grantID: "grant-revoked-1"))
        XCTAssertEqual(ledger.state, .revoked)
        XCTAssertNil(try leaseStore.load(handle: response.brokerHandle))
        XCTAssertFalse(
            FileManager.default.fileExists(
                atPath: grantManager.fragmentURL(for: response.brokerHandle).path
            )
        )
    }

    func testServiceExpiresLiveGrantAndPublishesExpiryEvent() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-expiry-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )

        let leaseStore = LeaseStore(directoryURL: config.stateDirectoryURL)
        let grantManager = SudoersGrantManager(
            sudoersDirectoryURL: config.sudoersDirectoryURL,
            visudoPath: config.visudoPath,
            runCommand: { _ in }
        )
        let service = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            leaseStore: leaseStore,
            grantManager: grantManager
        )

        let expiredExpectation = expectation(description: "grant expired")
        let token = service.subscribe(username: "tester") { event in
            if event.kind == .grantExpired {
                expiredExpectation.fulfill()
            }
        }
        defer { service.unsubscribe(token) }

        let response = try service.grantTimedSudoers(TimedSudoersGrantRequest(
            grantID: "grant-expire-1",
            deviceID: "device-1",
            targetUsername: "tester",
            roleName: "local_sudo",
            duration: 0.1
        ))

        wait(for: [expiredExpectation], timeout: 2.0)

        XCTAssertNil(try leaseStore.load(handle: response.brokerHandle))
        XCTAssertFalse(
            FileManager.default.fileExists(
                atPath: grantManager.fragmentURL(for: response.brokerHandle).path
            )
        )
    }

    func testServiceRevokingExpiredHandleReturnsNotFoundWithoutExtraEvent() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-expired-revoke-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )
        let leaseStore = LeaseStore(directoryURL: config.stateDirectoryURL)
        let grantManager = SudoersGrantManager(
            sudoersDirectoryURL: config.sudoersDirectoryURL,
            visudoPath: config.visudoPath,
            runCommand: { _ in }
        )
        let service = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            leaseStore: leaseStore,
            grantManager: grantManager
        )

        let expiredExpectation = expectation(description: "grant expired")
        let events = EventRecorder()
        let token = service.subscribe(username: "tester") { event in
            events.append(event)
            if event.kind == .grantExpired {
                expiredExpectation.fulfill()
            }
        }
        defer { service.unsubscribe(token) }

        let response = try service.grantTimedSudoers(TimedSudoersGrantRequest(
            grantID: "grant-expire-2",
            deviceID: "device-1",
            targetUsername: "tester",
            roleName: "local_sudo",
            duration: 0.1
        ))

        wait(for: [expiredExpectation], timeout: 2.0)

        let eventCountBeforeRevoke = events.snapshot().count
        let revokeResponse = try service.revokeTimedGrant(handle: response.brokerHandle)

        XCTAssertEqual(revokeResponse.status, .notFound)
        XCTAssertEqual(events.snapshot().count, eventCountBeforeRevoke)
        XCTAssertEqual(events.snapshot().map(\.kind), [.grantCreated, .grantExpired])
        XCTAssertNil(try leaseStore.load(handle: response.brokerHandle))
        XCTAssertFalse(
            FileManager.default.fileExists(
                atPath: grantManager.fragmentURL(for: response.brokerHandle).path
            )
        )
    }

    func testGrantTimedSudoersRejectsCompletedGrantIDAfterExpiry() throws {
        let temporaryRoot = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-broker-regrant-after-expiry-tests-\(UUID().uuidString)")
        let config = BrokerConfig(
            stateDirectoryURL: temporaryRoot.appendingPathComponent("state"),
            sudoersDirectoryURL: temporaryRoot.appendingPathComponent("sudoers")
        )
        let leaseStore = LeaseStore(directoryURL: config.stateDirectoryURL)
        let grantManager = SudoersGrantManager(
            sudoersDirectoryURL: config.sudoersDirectoryURL,
            visudoPath: config.visudoPath,
            runCommand: { _ in }
        )
        let service = try PrivilegeBrokerService(
            config: config,
            validator: LocalAccountValidator { username in
                ValidatedLocalUser(username: username, uid: 501)
            },
            leaseStore: leaseStore,
            grantManager: grantManager
        )

        let expiredExpectation = expectation(description: "grant expired")
        let token = service.subscribe(username: "tester") { event in
            if event.kind == .grantExpired {
                expiredExpectation.fulfill()
            }
        }
        defer { service.unsubscribe(token) }

        let request = TimedSudoersGrantRequest(
            grantID: "grant-expired-3",
            deviceID: "device-1",
            targetUsername: "tester",
            roleName: "local_sudo",
            duration: 0.1
        )

        let response = try service.grantTimedSudoers(request)
        wait(for: [expiredExpectation], timeout: 2.0)

        XCTAssertThrowsError(try service.grantTimedSudoers(request)) { error in
            guard case BrokerServiceError.grantAlreadyCompleted(let grantID, let state) = error else {
                return XCTFail("unexpected error: \(error)")
            }
            XCTAssertEqual(grantID, "grant-expired-3")
            XCTAssertEqual(state, .expired)
        }

        let ledger = try XCTUnwrap(leaseStore.loadGrant(grantID: "grant-expired-3"))
        XCTAssertEqual(ledger.state, .expired)
        XCTAssertNil(try leaseStore.load(handle: response.brokerHandle))
        XCTAssertFalse(
            FileManager.default.fileExists(
                atPath: grantManager.fragmentURL(for: response.brokerHandle).path
            )
        )
    }
}
