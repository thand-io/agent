import Foundation
import Security
import XPC

public enum PeerRole {
    case agent
    case notifier
    case broker
}

public struct PeerRequirementPolicy: Sendable, Equatable {
    public static let agentSigningIdentifier = "io.thand.agent"
    public static let notifierSigningIdentifier = "io.thand.agent.privilege-notifier"
    public static let brokerSigningIdentifier = "io.thand.agent.privilege-broker"
    public static let appGroupsEntitlement = "com.apple.security.application-groups"
    public static let agentAppGroupSuffix = "io.thand.agent.privileged-broker-client"
    public static let notifierAppGroupSuffix = "io.thand.agent.privileged-broker-notifier"
    public static let brokerAppGroupSuffix = "io.thand.agent.privileged-broker-server"

    public init() {}

    public var combinedClientRequirement: String {
        "\(Self.agentSigningIdentifier)|\(Self.notifierSigningIdentifier)"
    }

    public func signingIdentifier(for role: PeerRole) -> String {
        switch role {
        case .agent:
            return Self.agentSigningIdentifier
        case .notifier:
            return Self.notifierSigningIdentifier
        case .broker:
            return Self.brokerSigningIdentifier
        }
    }

    @available(macOS 26.0, *)
    public func sessionRequirement(for role: PeerRole) -> XPCPeerRequirement {
        .isFromSameTeam(andMatchesSigningIdentifier: signingIdentifier(for: role))
    }

    @available(macOS 26.0, *)
    public func senderSatisfies(_ message: XPCReceivedMessage, role: PeerRole) -> Bool {
        let identityRequirement = XPCPeerRequirement.isFromSameTeam(andMatchesSigningIdentifier: signingIdentifier(for: role))
        guard message.senderSatisfies(identityRequirement) else {
            return false
        }
        // App Group entitlements are array-valued, and the modern XPC peer API can only
        // assert presence of the entitlement key, not membership of a specific array entry.
        // We keep exact value enforcement in the NSXPC fallback path below.
        return message.senderSatisfies(.hasEntitlement(Self.appGroupsEntitlement))
    }

    public func nsxpcRequirementString(for role: PeerRole) throws -> String {
        let teamIdentifier = try currentProcessTeamIdentifier()
        return """
        anchor apple generic and identifier "\(escapeRequirementValue(signingIdentifier(for: role)))" and certificate leaf[subject.OU] = "\(escapeRequirementValue(teamIdentifier))"
        """
    }

    public func connectionSatisfies(_ connection: NSXPCConnection, role: PeerRole) -> Bool {
        do {
            return try validate(connection, role: role)
        } catch {
            return false
        }
    }

    public func validate(_ connection: NSXPCConnection, role: PeerRole) throws -> Bool {
        let requirementString = try nsxpcRequirementString(for: role)
        let requirement = try makeRequirement(from: requirementString)
        let code = try copyCode(for: connection.processIdentifier)
        let validityStatus = SecCodeCheckValidity(code, SecCSFlags(), requirement)
        guard validityStatus == errSecSuccess else {
            throw PeerValidationError.invalidCodeSignature(message: copySecurityMessage(for: validityStatus))
        }

        let teamIdentifier = try currentProcessTeamIdentifier()
        let signingInformation = try copySigningInformation(for: code)
        guard let entitlements = signingInformation[kSecCodeInfoEntitlementsDict as String] as? [String: Any] else {
            throw PeerValidationError.missingEntitlement(expectedAppGroup(for: role, teamIdentifier: teamIdentifier))
        }
        let expectedAppGroup = expectedAppGroup(for: role, teamIdentifier: teamIdentifier)
        let appGroups = try appGroups(from: entitlements)
        guard appGroups.contains(expectedAppGroup) else {
            throw PeerValidationError.missingEntitlement(expectedAppGroup)
        }

        return true
    }

    public func entitlement(for role: PeerRole, teamIdentifier: String) -> String {
        expectedAppGroup(for: role, teamIdentifier: teamIdentifier)
    }

    public func defaultEntitlement(for role: PeerRole) -> String {
        let teamIdentifier = (try? currentProcessTeamIdentifier()) ?? "TEAMID"
        return expectedAppGroup(for: role, teamIdentifier: teamIdentifier)
    }

    private func expectedAppGroup(for role: PeerRole, teamIdentifier: String) -> String {
        switch role {
        case .agent:
            return "\(teamIdentifier).\(Self.agentAppGroupSuffix)"
        case .notifier:
            return "\(teamIdentifier).\(Self.notifierAppGroupSuffix)"
        case .broker:
            return "\(teamIdentifier).\(Self.brokerAppGroupSuffix)"
        }
    }

    private func currentProcessTeamIdentifier() throws -> String {
        var selfCode: SecCode?
        let selfStatus = SecCodeCopySelf(SecCSFlags(), &selfCode)
        guard selfStatus == errSecSuccess, let selfCode else {
            throw PeerValidationError.currentProcessTeamIdentifierUnavailable(message: copySecurityMessage(for: selfStatus))
        }

        let signingInformation = try copySigningInformation(for: selfCode)
        guard let teamIdentifier = signingInformation[kSecCodeInfoTeamIdentifier as String] as? String,
              !teamIdentifier.isEmpty else {
            throw PeerValidationError.currentProcessTeamIdentifierUnavailable(message: "missing team identifier")
        }
        return teamIdentifier
    }

    private func makeRequirement(from requirementString: String) throws -> SecRequirement {
        var requirement: SecRequirement?
        let status = SecRequirementCreateWithString(requirementString as CFString, SecCSFlags(), &requirement)
        guard status == errSecSuccess, let requirement else {
            throw PeerValidationError.invalidRequirement(message: copySecurityMessage(for: status))
        }
        return requirement
    }

    private func copyCode(for processIdentifier: pid_t) throws -> SecCode {
        let attributes = [kSecGuestAttributePid as String: processIdentifier] as CFDictionary
        var code: SecCode?
        let status = SecCodeCopyGuestWithAttributes(nil, attributes, SecCSFlags(), &code)
        guard status == errSecSuccess, let code else {
            throw PeerValidationError.invalidPeer(message: copySecurityMessage(for: status))
        }
        return code
    }

    private func copySigningInformation(for code: SecCode) throws -> [String: Any] {
        var staticCode: SecStaticCode?
        let copyStaticStatus = SecCodeCopyStaticCode(code, SecCSFlags(), &staticCode)
        guard copyStaticStatus == errSecSuccess, let staticCode else {
            throw PeerValidationError.invalidPeer(message: copySecurityMessage(for: copyStaticStatus))
        }

        var signingInformation: CFDictionary?
        let status = SecCodeCopySigningInformation(staticCode, SecCSFlags(rawValue: kSecCSSigningInformation), &signingInformation)
        guard status == errSecSuccess,
              let info = signingInformation as? [String: Any] else {
            throw PeerValidationError.invalidPeer(message: copySecurityMessage(for: status))
        }
        return info
    }

    private func copySecurityMessage(for status: OSStatus) -> String {
        (SecCopyErrorMessageString(status, nil) as String?) ?? "OSStatus \(status)"
    }

    private func appGroups(from entitlements: [String: Any]) throws -> [String] {
        guard let rawGroups = entitlements[Self.appGroupsEntitlement] else {
            throw PeerValidationError.missingEntitlement(Self.appGroupsEntitlement)
        }
        guard let appGroups = rawGroups as? [String], !appGroups.isEmpty else {
            throw PeerValidationError.missingEntitlement(Self.appGroupsEntitlement)
        }
        return appGroups
    }

    private func escapeRequirementValue(_ rawValue: String) -> String {
        rawValue.replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: "\"", with: "\\\"")
    }
}

public enum PeerValidationError: Error, CustomStringConvertible {
    case currentProcessTeamIdentifierUnavailable(message: String)
    case invalidRequirement(message: String)
    case invalidPeer(message: String)
    case invalidCodeSignature(message: String)
    case missingEntitlement(String)

    public var description: String {
        switch self {
        case .currentProcessTeamIdentifierUnavailable(let message):
            return "unable to resolve current process team identifier: \(message)"
        case .invalidRequirement(let message):
            return "invalid code-signing requirement: \(message)"
        case .invalidPeer(let message):
            return "unable to inspect peer identity: \(message)"
        case .invalidCodeSignature(let message):
            return "peer code-signing validation failed: \(message)"
        case .missingEntitlement(let entitlement):
            return "peer is missing required entitlement \(entitlement)"
        }
    }
}
