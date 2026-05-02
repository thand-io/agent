import Dispatch
import Foundation
import XPC

@objc private protocol BrokerControlXPCProtocol {
    func sendBrokerWireMessageData(_ payload: Data, withReply reply: @escaping (Data?, NSString?) -> Void)
}

private final class LegacyBrokerControlEndpoint: NSObject, BrokerControlXPCProtocol {
    private weak var server: XPCBrokerServer?
    private weak var connection: NSXPCConnection?

    init(server: XPCBrokerServer, connection: NSXPCConnection) {
        self.server = server
        self.connection = connection
    }

    func sendBrokerWireMessageData(_ payload: Data, withReply reply: @escaping (Data?, NSString?) -> Void) {
        guard let server, let connection else {
            reply(nil, "broker unavailable")
            return
        }

        let (responseData, errorMessage) = server.handleLegacyMessageData(payload, connection: connection)
        reply(responseData, errorMessage as NSString?)
    }
}

public struct BrokerRemoteError: Error, CustomStringConvertible, Sendable, Equatable {
    public let failure: BrokerFailure

    public init(failure: BrokerFailure) {
        self.failure = failure
    }

    public var code: BrokerFailureCode {
        failure.code
    }

    public var description: String {
        failure.message
    }
}

private func shouldLogCancellation(_ description: String) -> Bool {
    !description.lowercased().contains("session manually canceled")
}

private func encodeWireMessage(_ message: BrokerWireMessage) throws -> Data {
    try JSONEncoder().encode(message)
}

private func decodeWireMessage(_ payload: Data) throws -> BrokerWireMessage {
    try JSONDecoder().decode(BrokerWireMessage.self, from: payload)
}

public final class XPCBrokerClient: @unchecked Sendable {
    private let config: BrokerConfig
    private let peerRequirementPolicy: PeerRequirementPolicy

    public init(config: BrokerConfig, peerRequirementPolicy: PeerRequirementPolicy? = nil) {
        self.config = config
        self.peerRequirementPolicy = peerRequirementPolicy ?? PeerRequirementPolicy()
    }

    public func send(_ request: BrokerControlRequest) throws -> BrokerControlResponse {
        let reply: BrokerWireMessage
        if #available(macOS 26.0, *) {
            let session = try makeModernSession(incomingMessageHandler: nil)
            defer { session.cancel(reason: "request complete") }
            reply = try session.sendSync(BrokerWireMessage(controlRequest: request))
        } else {
            reply = try sendViaLegacyConnection(BrokerWireMessage(controlRequest: request))
        }

        if let failure = reply.failure {
            throw BrokerRemoteError(failure: failure)
        }
        if let error = reply.error {
            throw XPCTransportError.xpcFailure(error)
        }
        return reply.controlResponse ?? BrokerControlResponse()
    }

    @available(macOS 26.0, *)
    fileprivate func makeModernSession(
        incomingMessageHandler: (@Sendable (BrokerWireMessage) -> (any Encodable)?)?
    ) throws -> XPCSession {
        let session: XPCSession
        brokerLog("creating broker xpc client session", fields: [
            "service_label": config.serviceLabel
        ])
        session = try XPCSession(
            machService: config.serviceLabel,
            options: [.inactive, .privileged],
            requirement: peerRequirementPolicy.sessionRequirement(for: .broker),
            incomingMessageHandler: incomingMessageHandler,
            cancellationHandler: { richError in
                guard shouldLogCancellation(richError.debugDescription) else {
                    return
                }
                fputs("broker session cancelled: \(richError.debugDescription)\n", stderr)
            }
        )

        try session.activate()
        brokerLog("activated broker xpc client session", fields: [
            "service_label": config.serviceLabel
        ])
        return session
    }

    private func sendViaLegacyConnection(_ message: BrokerWireMessage) throws -> BrokerWireMessage {
        brokerLog("creating legacy broker nsxpc client connection", fields: [
            "service_label": config.serviceLabel
        ])

        let connection = NSXPCConnection(machServiceName: config.serviceLabel, options: .privileged)
        connection.setCodeSigningRequirement(try peerRequirementPolicy.nsxpcRequirementString(for: .broker))
        connection.remoteObjectInterface = NSXPCInterface(with: BrokerControlXPCProtocol.self)
        connection.activate()
        defer { connection.invalidate() }

        var remoteError: Error?
        let proxy = connection.synchronousRemoteObjectProxyWithErrorHandler { error in
            remoteError = error
        } as? BrokerControlXPCProtocol

        guard let proxy else {
            throw XPCTransportError.invalidMessage("failed to construct broker control proxy")
        }

        let payload = try encodeWireMessage(message)
        var responseData: Data?
        var responseError: NSString?

        proxy.sendBrokerWireMessageData(payload) { data, error in
            responseData = data
            responseError = error
        }

        if let remoteError {
            throw XPCTransportError.xpcFailure(String(describing: remoteError))
        }
        if let responseError {
            throw XPCTransportError.xpcFailure(responseError as String)
        }
        guard let responseData else {
            throw XPCTransportError.invalidMessage("broker returned an empty response")
        }

        brokerLog("activated legacy broker nsxpc client connection", fields: [
            "service_label": config.serviceLabel
        ])
        return try decodeWireMessage(responseData)
    }
}

public final class XPCBrokerServer: NSObject, NSXPCListenerDelegate, @unchecked Sendable {
    private let service: PrivilegeBrokerService
    private let queue = DispatchQueue(label: "io.thand.agent.privilege-broker.listener")

    private var modernListener: XPCListener?
    private var legacyListener: NSXPCListener?

    public init(service: PrivilegeBrokerService) {
        self.service = service
    }

    public func start() throws {
        brokerLog("starting broker xpc listener", fields: [
            "service_label": service.config.serviceLabel
        ])

        if #available(macOS 26.0, *) {
            try startModernListener()
        } else {
            try startLegacyListener()
        }
    }

    @available(macOS 26.0, *)
    private func startModernListener() throws {
        let listener = try XPCListener(
            service: service.config.serviceLabel,
            targetQueue: queue,
            options: [.inactive],
            incomingSessionHandler: { [weak self] request in
                guard let self else {
                    return request.reject(reason: "broker unavailable")
                }

                brokerLog("received incoming xpc session")

                let (decision, _) = request.accept(
                    incomingMessageHandler: { [weak self] (message: XPCReceivedMessage) -> (any Encodable)? in
                        guard let self else {
                            return BrokerWireMessage(error: "broker unavailable")
                        }
                        return self.handleModernMessage(message: message)
                    },
                    cancellationHandler: { _ in }
                )
                return decision
            }
        )
        self.modernListener = listener
        try listener.activate()
        brokerLog("activated broker xpc listener", fields: [
            "service_label": service.config.serviceLabel,
            "transport": "xpc"
        ])
    }

    private func startLegacyListener() throws {
        let listener = NSXPCListener(machServiceName: service.config.serviceLabel)
        listener.setConnectionCodeSigningRequirement(try service.peerRequirementPolicy.nsxpcRequirementString(for: .agent))
        listener.delegate = self
        self.legacyListener = listener
        listener.activate()
        brokerLog("activated broker xpc listener", fields: [
            "service_label": service.config.serviceLabel,
            "transport": "nsxpc"
        ])
    }

    public func listener(_ listener: NSXPCListener, shouldAcceptNewConnection newConnection: NSXPCConnection) -> Bool {
        do {
            _ = try service.peerRequirementPolicy.validate(newConnection, role: .agent)
        } catch {
            brokerLog("rejected broker nsxpc connection", fields: [
                "service_label": service.config.serviceLabel,
                "process_id": String(newConnection.processIdentifier),
                "effective_uid": String(newConnection.effectiveUserIdentifier),
                "error": String(describing: error)
            ])
            return false
        }

        let endpoint = LegacyBrokerControlEndpoint(server: self, connection: newConnection)
        newConnection.exportedInterface = NSXPCInterface(with: BrokerControlXPCProtocol.self)
        newConnection.exportedObject = endpoint
        newConnection.interruptionHandler = {
            brokerLog("broker nsxpc connection interrupted", fields: [
                "service_label": self.service.config.serviceLabel,
                "process_id": String(newConnection.processIdentifier)
            ])
        }
        newConnection.invalidationHandler = {
            brokerLog("broker nsxpc connection invalidated", fields: [
                "service_label": self.service.config.serviceLabel,
                "process_id": String(newConnection.processIdentifier)
            ])
        }
        newConnection.activate()
        return true
    }

    fileprivate func handleLegacyMessageData(_ payload: Data, connection: NSXPCConnection) -> (Data?, String?) {
        do {
            _ = try service.peerRequirementPolicy.validate(connection, role: .agent)
            let wireMessage = try decodeWireMessage(payload)
            let response = responseMessage(for: wireMessage, senderAllowed: true)
            return (try encodeWireMessage(response), nil)
        } catch let error as PeerValidationError {
            let response = BrokerWireMessage(failure: BrokerFailure(
                code: .peerRejected,
                message: String(describing: error)
            ))
            return (try? encodeWireMessage(response), nil)
        } catch let error as XPCTransportError {
            let response = responseMessage(forTransportError: error)
            return (try? encodeWireMessage(response), nil)
        } catch {
            return (nil, String(describing: error))
        }
    }

    @available(macOS 26.0, *)
    private func handleModernMessage(message: XPCReceivedMessage) -> (any Encodable)? {
        do {
            let wireMessage: BrokerWireMessage = try message.decode()
            let senderAllowed = service.peerRequirementPolicy.senderSatisfies(message, role: .agent)
            return responseMessage(for: wireMessage, senderAllowed: senderAllowed)
        } catch let error as XPCTransportError {
            return responseMessage(forTransportError: error)
        } catch {
            return BrokerWireMessage(failure: BrokerFailure(
                code: .internalError,
                message: String(describing: error)
            ))
        }
    }

    private func responseMessage(for wireMessage: BrokerWireMessage, senderAllowed: Bool) -> BrokerWireMessage {
        do {
            if let controlRequest = wireMessage.controlRequest {
                brokerLog("handling control request", fields: [
                    "operation": controlRequest.operation.rawValue,
                    "sender_allowed": senderAllowed ? "true" : "false"
                ])
                guard senderAllowed else {
                    return BrokerWireMessage(failure: BrokerFailure(
                        code: .peerRejected,
                        message: "agent peer identity check failed"
                    ))
                }
                let response = try service.handle(controlRequest)
                return BrokerWireMessage(controlResponse: response)
            }

            return BrokerWireMessage(failure: BrokerFailure(
                code: .invalidRequest,
                message: "unsupported broker message"
            ))
        } catch let error as BrokerServiceError {
            return BrokerWireMessage(failure: error.failure)
        } catch {
            return BrokerWireMessage(failure: BrokerFailure(
                code: .internalError,
                message: String(describing: error)
            ))
        }
    }

    private func responseMessage(forTransportError error: XPCTransportError) -> BrokerWireMessage {
        let failure: BrokerFailure
        switch error {
        case .invalidMessage(let message):
            failure = BrokerFailure(code: .invalidRequest, message: message)
        case .peerRequirementRejected(let message):
            failure = BrokerFailure(code: .peerRejected, message: message)
        case .xpcFailure(let message):
            failure = BrokerFailure(code: .unavailable, message: message)
        }
        return BrokerWireMessage(failure: failure)
    }
}
