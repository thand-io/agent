import Darwin
import Dispatch
import Foundation

@objc public protocol BrokerNotifierSubscriptionXPCProtocol {
    func subscribe(withReply reply: @escaping (NSString?, NSNumber?, NSString?) -> Void)
}

@objc public protocol BrokerNotifierEventSinkXPCProtocol {
    func deliverEventData(_ payload: Data)
}

public struct ResolvedNotifierIdentity: Sendable, Equatable {
    public let username: String
    public let uid: Int
    public let auditSessionIdentifier: UInt32
    public let processIdentifier: Int32

    public init(username: String, uid: Int, auditSessionIdentifier: UInt32, processIdentifier: Int32) {
        self.username = username
        self.uid = uid
        self.auditSessionIdentifier = auditSessionIdentifier
        self.processIdentifier = processIdentifier
    }
}

public struct NotifierSubscriptionAcknowledgement: Sendable, Equatable {
    public let username: String
    public let auditSessionIdentifier: UInt32

    public init(username: String, auditSessionIdentifier: UInt32) {
        self.username = username
        self.auditSessionIdentifier = auditSessionIdentifier
    }
}

public enum NotifierXPCError: Error, CustomStringConvertible {
    case invalidMessage(String)
    case subscriptionFailed(String)
    case invalidPeer(String)

    public var description: String {
        switch self {
        case .invalidMessage(let message):
            return message
        case .subscriptionFailed(let message):
            return message
        case .invalidPeer(let message):
            return message
        }
    }
}

public enum NotifierIdentityResolver {
    public typealias UserLookup = @Sendable (uid_t) -> ValidatedLocalUser?

    public static func resolve(
        uid: uid_t,
        auditSessionIdentifier: UInt32,
        processIdentifier: Int32,
        userLookup: UserLookup = { LocalAccountValidator.defaultLookup(uid: $0) }
    ) throws -> ResolvedNotifierIdentity {
        guard let user = userLookup(uid) else {
            throw NotifierXPCError.invalidPeer("unable to resolve local user for uid \(uid)")
        }

        return ResolvedNotifierIdentity(
            username: user.username,
            uid: user.uid,
            auditSessionIdentifier: auditSessionIdentifier,
            processIdentifier: processIdentifier
        )
    }

    public static func resolve(
        connection: NSXPCConnection,
        userLookup: UserLookup = { LocalAccountValidator.defaultLookup(uid: $0) }
    ) throws -> ResolvedNotifierIdentity {
        try resolve(
            uid: connection.effectiveUserIdentifier,
            auditSessionIdentifier: UInt32(connection.auditSessionIdentifier),
            processIdentifier: Int32(connection.processIdentifier),
            userLookup: userLookup
        )
    }
}

private final class BrokerNotifierEventReceiver: NSObject, BrokerNotifierEventSinkXPCProtocol {
    private let onEvent: @Sendable (BrokerEvent) -> Void

    init(onEvent: @escaping @Sendable (BrokerEvent) -> Void) {
        self.onEvent = onEvent
    }

    func deliverEventData(_ payload: Data) {
        do {
            let event = try JSONDecoder().decode(BrokerEvent.self, from: payload)
            onEvent(event)
        } catch {
            fputs("failed to decode broker event payload: \(error)\n", stderr)
        }
    }
}

private final class NSXPCConnectionBox: @unchecked Sendable {
    weak var connection: NSXPCConnection?

    init(connection: NSXPCConnection) {
        self.connection = connection
    }
}

public final class NotifierXPCClient: @unchecked Sendable {
    private let config: BrokerConfig
    private let peerRequirementPolicy: PeerRequirementPolicy
    private let onEvent: @Sendable (BrokerEvent) -> Void

    private var connection: NSXPCConnection?
    private var eventReceiver: BrokerNotifierEventReceiver?

    public init(
        config: BrokerConfig,
        peerRequirementPolicy: PeerRequirementPolicy? = nil,
        onEvent: @escaping @Sendable (BrokerEvent) -> Void
    ) {
        self.config = config
        self.peerRequirementPolicy = peerRequirementPolicy ?? PeerRequirementPolicy()
        self.onEvent = onEvent
    }

    @discardableResult
    public func start() throws -> NotifierSubscriptionAcknowledgement {
        let connection = NSXPCConnection(machServiceName: config.notifierServiceLabel, options: .privileged)
        connection.setCodeSigningRequirement(try peerRequirementPolicy.nsxpcRequirementString(for: .broker))

        let eventReceiver = BrokerNotifierEventReceiver(onEvent: onEvent)
        connection.exportedInterface = NSXPCInterface(with: BrokerNotifierEventSinkXPCProtocol.self)
        connection.exportedObject = eventReceiver
        connection.remoteObjectInterface = NSXPCInterface(with: BrokerNotifierSubscriptionXPCProtocol.self)
        connection.interruptionHandler = {
            brokerLog("notifier xpc connection interrupted", fields: [
                "service_label": self.config.notifierServiceLabel
            ])
        }
        connection.invalidationHandler = {
            brokerLog("notifier xpc connection invalidated", fields: [
                "service_label": self.config.notifierServiceLabel
            ])
        }

        connection.activate()

        var remoteError: Error?
        let proxy = connection.synchronousRemoteObjectProxyWithErrorHandler { error in
            remoteError = error
        } as? BrokerNotifierSubscriptionXPCProtocol

        guard let proxy else {
            connection.invalidate()
            throw NotifierXPCError.invalidMessage("failed to construct notifier subscription proxy")
        }

        var subscribedUsername: NSString?
        var subscribedAuditSessionIdentifier: NSNumber?
        var failureMessage: NSString?

        proxy.subscribe { username, auditSessionIdentifier, error in
            subscribedUsername = username
            subscribedAuditSessionIdentifier = auditSessionIdentifier
            failureMessage = error
        }

        if let remoteError {
            connection.invalidate()
            throw NotifierXPCError.subscriptionFailed(String(describing: remoteError))
        }
        if let failureMessage {
            connection.invalidate()
            throw NotifierXPCError.subscriptionFailed(failureMessage as String)
        }
        guard let subscribedUsername,
              let subscribedAuditSessionIdentifier else {
            connection.invalidate()
            throw NotifierXPCError.subscriptionFailed("notifier subscription was not acknowledged")
        }

        self.eventReceiver = eventReceiver
        self.connection = connection
        return NotifierSubscriptionAcknowledgement(
            username: subscribedUsername as String,
            auditSessionIdentifier: subscribedAuditSessionIdentifier.uint32Value
        )
    }
}

private final class NotifierSubscriptionEndpoint: NSObject, BrokerNotifierSubscriptionXPCProtocol {
    private weak var server: NotifierXPCServer?
    private weak var connection: NSXPCConnection?

    init(server: NotifierXPCServer, connection: NSXPCConnection) {
        self.server = server
        self.connection = connection
    }

    func subscribe(withReply reply: @escaping (NSString?, NSNumber?, NSString?) -> Void) {
        guard let server, let connection else {
            reply(nil, nil, "notifier connection is unavailable")
            return
        }
        server.registerSubscription(for: connection, reply: reply)
    }
}

public final class NotifierXPCServer: NSObject, NSXPCListenerDelegate, @unchecked Sendable {
    private let service: PrivilegeBrokerService
    private let peerRequirementPolicy: PeerRequirementPolicy
    private let queue = DispatchQueue(label: "io.thand.agent.privilege-broker.notifier-listener")
    private let queueKey = DispatchSpecificKey<Void>()

    private var listener: NSXPCListener?
    private var subscriptionTokens: [ObjectIdentifier: UUID] = [:]

    public init(service: PrivilegeBrokerService) {
        self.service = service
        self.peerRequirementPolicy = service.peerRequirementPolicy
        self.queue.setSpecific(key: queueKey, value: ())
    }

    public func start() throws {
        brokerLog("starting broker notifier xpc listener", fields: [
            "service_label": service.config.notifierServiceLabel
        ])

        let listener = NSXPCListener(machServiceName: service.config.notifierServiceLabel)
        listener.setConnectionCodeSigningRequirement(try peerRequirementPolicy.nsxpcRequirementString(for: .notifier))
        listener.delegate = self
        self.listener = listener
        listener.activate()

        brokerLog("activated broker notifier xpc listener", fields: [
            "service_label": service.config.notifierServiceLabel
        ])
    }

    public func listener(_ listener: NSXPCListener, shouldAcceptNewConnection newConnection: NSXPCConnection) -> Bool {
        do {
            _ = try peerRequirementPolicy.validate(newConnection, role: .notifier)
        } catch {
            brokerLog("rejected notifier xpc connection", fields: [
                "service_label": service.config.notifierServiceLabel,
                "process_id": String(newConnection.processIdentifier),
                "effective_uid": String(newConnection.effectiveUserIdentifier),
                "error": String(describing: error)
            ])
            return false
        }

        let endpoint = NotifierSubscriptionEndpoint(server: self, connection: newConnection)
        newConnection.exportedInterface = NSXPCInterface(with: BrokerNotifierSubscriptionXPCProtocol.self)
        newConnection.exportedObject = endpoint
        newConnection.remoteObjectInterface = NSXPCInterface(with: BrokerNotifierEventSinkXPCProtocol.self)
        newConnection.interruptionHandler = { [weak self, weak newConnection] in
            guard let self, let newConnection else {
                return
            }
            brokerLog("notifier xpc connection interrupted", fields: [
                "service_label": self.service.config.notifierServiceLabel,
                "process_id": String(newConnection.processIdentifier)
            ])
        }
        newConnection.invalidationHandler = { [weak self, weak newConnection] in
            guard let self, let newConnection else {
                return
            }
            self.removeSubscription(for: newConnection)
        }
        newConnection.activate()

        brokerLog("accepted notifier xpc connection", fields: [
            "service_label": service.config.notifierServiceLabel,
            "process_id": String(newConnection.processIdentifier),
            "effective_uid": String(newConnection.effectiveUserIdentifier)
        ])
        return true
    }

    fileprivate func registerSubscription(
        for connection: NSXPCConnection,
        reply: @escaping (NSString?, NSNumber?, NSString?) -> Void
    ) {
        do {
            let identity = try NotifierIdentityResolver.resolve(connection: connection)
            let connectionBox = NSXPCConnectionBox(connection: connection)
            let token = service.subscribe(username: identity.username) { [weak self, connectionBox] event in
                guard let self, let connection = connectionBox.connection else {
                    return
                }
                self.sendEvent(event, to: connection)
            }

            syncOnQueue {
                let key = ObjectIdentifier(connection)
                if let existingToken = subscriptionTokens.updateValue(token, forKey: key) {
                    service.unsubscribe(existingToken)
                }
            }

            brokerLog("registered notifier subscription", fields: [
                "service_label": service.config.notifierServiceLabel,
                "username": identity.username,
                "uid": String(identity.uid),
                "audit_session_identifier": String(identity.auditSessionIdentifier),
                "process_id": String(identity.processIdentifier)
            ])
            reply(
                identity.username as NSString,
                NSNumber(value: identity.auditSessionIdentifier),
                nil
            )
        } catch {
            brokerLog("failed to register notifier subscription", fields: [
                "service_label": service.config.notifierServiceLabel,
                "process_id": String(connection.processIdentifier),
                "effective_uid": String(connection.effectiveUserIdentifier),
                "error": String(describing: error)
            ])
            reply(nil, nil, String(describing: error) as NSString)
        }
    }

    private func sendEvent(_ event: BrokerEvent, to connection: NSXPCConnection) {
        do {
            let payload = try JSONEncoder().encode(event)
            let subscriptionKey = ObjectIdentifier(connection)
            guard let sink = connection.remoteObjectProxyWithErrorHandler({ [weak self] error in
                brokerLog("failed to deliver notifier event", fields: [
                    "service_label": self?.service.config.notifierServiceLabel ?? "",
                    "broker_handle": event.brokerHandle,
                    "error": String(describing: error)
                ])
                guard let self else {
                    return
                }
                self.removeSubscription(for: subscriptionKey)
            }) as? BrokerNotifierEventSinkXPCProtocol else {
                return
            }
            sink.deliverEventData(payload)
        } catch {
            brokerLog("failed to encode notifier event", fields: [
                "service_label": service.config.notifierServiceLabel,
                "broker_handle": event.brokerHandle,
                "error": String(describing: error)
            ])
        }
    }

    private func removeSubscription(for connection: NSXPCConnection) {
        removeSubscription(for: ObjectIdentifier(connection))
    }

    private func removeSubscription(for key: ObjectIdentifier) {
        let token: UUID? = syncOnQueue {
            subscriptionTokens.removeValue(forKey: key)
        }
        if let token {
            service.unsubscribe(token)
        }
    }

    private func syncOnQueue<T>(_ work: () -> T) -> T {
        if DispatchQueue.getSpecific(key: queueKey) != nil {
            return work()
        }
        return queue.sync(execute: work)
    }
}
