import XCTest
@testable import PrivilegeServicesShared

final class PeerRequirementPolicyTests: XCTestCase {
    func testDefaultsToSecureMode() {
        let policy = PeerRequirementPolicy()

        XCTAssertTrue(policy.combinedClientRequirement.contains("io.thand.agent"))
        XCTAssertEqual(policy.signingIdentifier(for: .agent), PeerRequirementPolicy.agentSigningIdentifier)
        XCTAssertEqual(policy.signingIdentifier(for: .notifier), PeerRequirementPolicy.notifierSigningIdentifier)
        XCTAssertEqual(policy.signingIdentifier(for: .broker), PeerRequirementPolicy.brokerSigningIdentifier)
        XCTAssertEqual(
            policy.entitlement(for: .agent, teamIdentifier: "TEAM123456"),
            "TEAM123456.io.thand.agent.privileged-broker-client"
        )
    }
}
