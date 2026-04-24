import Foundation
import GRPCCore
import GRPCNIOTransportHTTP2TransportServices
import PrivilegeServicesLocalBrokerProto
import PrivilegeServicesShared
import Security

private enum HelperLaunchError: Error, CustomStringConvertible {
    case parentIdentityRejected(String)
    case processIdentityUnavailable(String)

    var description: String {
        switch self {
        case .parentIdentityRejected(let message):
            return message
        case .processIdentityUnavailable(let message):
            return message
        }
    }
}

private struct ProcessIdentity {
    let identifier: String
    let teamIdentifier: String
}

private actor OneShotServerController {
    private var shutdown: (@Sendable () -> Void)?

    func install(_ callback: @escaping @Sendable () -> Void) {
        shutdown = callback
    }

    func finishRequest() {
        let callback = shutdown
        shutdown = nil
        callback?()
    }
}

@available(macOS 15.0, *)
private final class LocalBrokerControlService: Thand_Localbroker_V1_LocalBrokerControl.SimpleServiceProtocol {
    private let config: BrokerConfig
    private let serverController: OneShotServerController
    private let presenceAuthenticator: any LocalPresenceAuthenticating

    init(
        config: BrokerConfig,
        serverController: OneShotServerController,
        presenceAuthenticator: any LocalPresenceAuthenticating = LocalPresenceAuthenticator()
    ) {
        self.config = config
        self.serverController = serverController
        self.presenceAuthenticator = presenceAuthenticator
    }

    func grantTimedSudoers(
        request: Thand_Localbroker_V1_GrantTimedSudoersRequest,
        context _: ServerContext
    ) async throws -> Thand_Localbroker_V1_GrantTimedSudoersResponse {
        defer {
            Task {
                await serverController.finishRequest()
            }
        }

        let brokerRequest = TimedSudoersGrantRequest(
            grantID: request.grantID,
            deviceID: request.deviceID,
            targetUsername: request.targetUsername,
            roleName: request.roleName,
            duration: Double(request.durationMillis) / 1_000,
            deniedUsernames: request.deniedUsernames,
            allowedUIDRanges: request.allowedUidRanges,
            requestExpiresAt: request.hasRequestExpiresAtUnixMillis ? Date(timeIntervalSince1970: Double(request.requestExpiresAtUnixMillis) / 1_000) : nil
        )

        let response = try callBroker { client in
            try client.send(BrokerControlRequest(
                operation: .timedSudoersGrant,
                timedSudoersGrant: brokerRequest
            ))
        }

        guard let grant = response.timedSudoersGrant else {
            throw RPCError(code: .internalError, message: "broker returned an empty timed sudoers grant response")
        }

        var proto = Thand_Localbroker_V1_GrantTimedSudoersResponse()
        proto.brokerHandle = grant.brokerHandle
        proto.targetUsername = grant.targetUsername
        proto.expiresAtUnixMillis = Int64((grant.expiresAt.timeIntervalSince1970 * 1_000).rounded())
        return proto
    }

    func revokeTimedGrant(
        request: Thand_Localbroker_V1_RevokeTimedGrantRequest,
        context _: ServerContext
    ) async throws -> Thand_Localbroker_V1_RevokeTimedGrantResponse {
        defer {
            Task {
                await serverController.finishRequest()
            }
        }

        let response = try callBroker { client in
            try client.send(BrokerControlRequest(
                operation: .timedSudoersRevoke,
                revokeTimedGrant: RevokeTimedGrantRequest(brokerHandle: request.brokerHandle)
            ))
        }

        guard let revoke = response.revokeTimedGrant else {
            throw RPCError(code: .internalError, message: "broker returned an empty timed sudoers revoke response")
        }

        var proto = Thand_Localbroker_V1_RevokeTimedGrantResponse()
        switch revoke.status {
        case .revoked:
            proto.status = .revoked
        case .notFound:
            proto.status = .notFound
        }
        return proto
    }

    func checkLocalPresence(
        request: Thand_Localbroker_V1_CheckLocalPresenceRequest,
        context _: ServerContext
    ) async throws -> Thand_Localbroker_V1_CheckLocalPresenceResponse {
        defer {
            Task {
                await serverController.finishRequest()
            }
        }

        let result = await presenceAuthenticator.checkPresence(LocalPresenceCheckRequest(
            challengeID: request.challengeID,
            deviceID: request.deviceID,
            workflowID: request.workflowID,
            taskName: request.taskName,
            prompt: request.prompt,
            timeout: Double(request.timeoutMillis) / 1_000,
            requestedBy: request.requestedBy,
            roleName: request.roleName,
            reason: request.reason
        ))

        var proto = Thand_Localbroker_V1_CheckLocalPresenceResponse()
        proto.approved = result.approved
        if let authenticatedAt = result.authenticatedAt {
            proto.authenticatedAtUnixMillis = Int64((authenticatedAt.timeIntervalSince1970 * 1_000).rounded())
        }
        proto.failureReason = result.failureReason ?? ""
        return proto
    }

    private func callBroker(_ body: (XPCBrokerClient) throws -> BrokerControlResponse) throws -> BrokerControlResponse {
        do {
            return try body(XPCBrokerClient(config: config))
        } catch let error as BrokerRemoteError {
            throw RPCError(code: grpcCode(for: error.code), message: error.description)
        } catch let error as BrokerServiceError {
            throw RPCError(code: grpcCode(for: error.failureCode), message: error.description)
        } catch let error as XPCTransportError {
            switch error {
            case .peerRequirementRejected(let message):
                throw RPCError(code: .permissionDenied, message: message)
            case .invalidMessage(let message):
                throw RPCError(code: .invalidArgument, message: message)
            case .xpcFailure(let message):
                throw RPCError(code: .unavailable, message: message)
            }
        } catch {
            throw RPCError(code: .internalError, message: String(describing: error))
        }
    }

    private func grpcCode(for code: BrokerFailureCode) -> RPCError.Code {
        switch code {
        case .grantIDConflict:
            return .alreadyExists
        case .grantAlreadyCompleted:
            return .failedPrecondition
        case .peerRejected:
            return .permissionDenied
        case .invalidRequest, .invalidUsername, .deniedUsername, .unknownUser, .uidOutOfRange, .invalidUIDRange:
            return .invalidArgument
        case .unavailable:
            return .unavailable
        case .missingLease, .visudoFailed:
            return .failedPrecondition
        case .internalError:
            return .internalError
        }
    }
}

private func effectiveConfig(arguments: BrokerHelperArguments) -> BrokerConfig {
    let environmentConfig = BrokerConfig.fromEnvironment()
    return BrokerConfig(
        stateDirectoryURL: environmentConfig.stateDirectoryURL,
        sudoersDirectoryURL: environmentConfig.sudoersDirectoryURL,
        visudoPath: environmentConfig.visudoPath,
        serviceLabel: arguments.serviceLabel ?? environmentConfig.serviceLabel
    )
}

private func validateLaunch(arguments: BrokerHelperArguments) throws -> String {
    let grpcSocketPath = arguments.grpcSocketPath!
    let parent = try copyProcessIdentity(pid: getppid())
    let current = try copyProcessIdentity(pid: getpid())
    let peerRequirementPolicy = PeerRequirementPolicy()

    guard parent.teamIdentifier == current.teamIdentifier else {
        throw HelperLaunchError.parentIdentityRejected("agent peer identity check failed")
    }
    guard parent.identifier == peerRequirementPolicy.signingIdentifier(for: .agent) else {
        throw HelperLaunchError.parentIdentityRejected("agent peer identity check failed")
    }

    return grpcSocketPath
}

private func copyProcessIdentity(pid: pid_t) throws -> ProcessIdentity {
    let attributes = [kSecGuestAttributePid as String: NSNumber(value: pid)] as CFDictionary

    var code: SecCode?
    let copyGuestStatus = SecCodeCopyGuestWithAttributes(nil, attributes, SecCSFlags(), &code)
    guard copyGuestStatus == errSecSuccess, let code else {
        throw HelperLaunchError.processIdentityUnavailable("unable to load code identity for pid \(pid)")
    }

    var staticCode: SecStaticCode?
    let copyStaticStatus = SecCodeCopyStaticCode(code, SecCSFlags(), &staticCode)
    guard copyStaticStatus == errSecSuccess, let staticCode else {
        throw HelperLaunchError.processIdentityUnavailable("unable to load static code identity for pid \(pid)")
    }

    var signingInfo: CFDictionary?
    let signingStatus = SecCodeCopySigningInformation(staticCode, SecCSFlags(rawValue: kSecCSSigningInformation), &signingInfo)
    guard signingStatus == errSecSuccess, let signingInfo else {
        throw HelperLaunchError.processIdentityUnavailable("unable to inspect signing identity for pid \(pid)")
    }

    let info = signingInfo as NSDictionary
    guard
        let identifier = info[kSecCodeInfoIdentifier as String] as? String,
        let teamIdentifier = info[kSecCodeInfoTeamIdentifier as String] as? String,
        !identifier.isEmpty,
        !teamIdentifier.isEmpty
    else {
        throw HelperLaunchError.processIdentityUnavailable("pid \(pid) is not signed with a usable identifier")
    }

    return ProcessIdentity(identifier: identifier, teamIdentifier: teamIdentifier)
}

@main
enum BrokerCtlMain {
    static func main() async {
        do {
            let arguments = try BrokerHelperArguments.parse(Array(CommandLine.arguments.dropFirst()))
            let config = effectiveConfig(arguments: arguments)
            let grpcSocketPath = try validateLaunch(arguments: arguments)
            let serverController = OneShotServerController()

            let transport = HTTP2ServerTransport.TransportServices(
                address: .unixDomainSocket(path: grpcSocketPath),
                transportSecurity: .plaintext
            )
            let server = GRPCServer(
                transport: transport,
                services: [LocalBrokerControlService(config: config, serverController: serverController)]
            )
            await serverController.install {
                server.beginGracefulShutdown()
            }
            try await server.serve()
        } catch {
            let arguments = try? BrokerHelperArguments.parse(Array(CommandLine.arguments.dropFirst()))
            let config = effectiveConfig(arguments: arguments ?? BrokerHelperArguments())
            let effectiveServiceLabel = arguments?.serviceLabel ?? config.serviceLabel
            fputs(
                "thand-macos-privilege-brokerctl failed: \(error) service_label=\(effectiveServiceLabel)\n",
                stderr
            )
            exit(1)
        }
    }
}
