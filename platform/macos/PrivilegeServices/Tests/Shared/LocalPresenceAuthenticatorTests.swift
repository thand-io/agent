import LocalAuthentication
import PrivilegeServicesShared
import XCTest

final class LocalPresenceAuthenticatorTests: XCTestCase {
    func testSuccessfulEvaluationApprovesPresence() async {
        let authenticatedAt = Date(timeIntervalSince1970: 1_777_777_777)
        let authenticator = LocalPresenceAuthenticator(now: { authenticatedAt }, evaluatePolicy: { prompt in
            XCTAssertEqual(prompt, "Approve this request")
            return true
        })

        let result = await authenticator.checkPresence(LocalPresenceCheckRequest(
            challengeID: "challenge-1",
            deviceID: "device-alpha",
            workflowID: "workflow-1",
            taskName: "presence",
            prompt: "Approve this request",
            timeout: 2
        ))

        XCTAssertTrue(result.approved)
        XCTAssertEqual(result.authenticatedAt, authenticatedAt)
        XCTAssertNil(result.failureReason)
    }

    func testUserCancelDeniesPresence() async {
        let authenticator = LocalPresenceAuthenticator(evaluatePolicy: { _ in
            throw NSError(domain: LAError.errorDomain, code: LAError.Code.userCancel.rawValue)
        })

        let result = await authenticator.checkPresence(testRequest())

        XCTAssertFalse(result.approved)
        XCTAssertEqual(result.failureReason, "user canceled local presence")
    }

    func testAuthenticationFailureDeniesPresence() async {
        let authenticator = LocalPresenceAuthenticator(evaluatePolicy: { _ in
            throw NSError(domain: LAError.errorDomain, code: LAError.Code.authenticationFailed.rawValue)
        })

        let result = await authenticator.checkPresence(testRequest())

        XCTAssertFalse(result.approved)
        XCTAssertEqual(result.failureReason, "local presence authentication failed")
    }

    func testUnavailableBiometryDeniesPresence() async {
        let authenticator = LocalPresenceAuthenticator(evaluatePolicy: { _ in
            throw NSError(domain: LAError.errorDomain, code: LAError.Code.biometryNotAvailable.rawValue)
        })

        let result = await authenticator.checkPresence(testRequest())

        XCTAssertFalse(result.approved)
        XCTAssertEqual(result.failureReason, "biometry is not available")
    }

    func testTimeoutDeniesPresence() async {
        let authenticator = LocalPresenceAuthenticator(evaluatePolicy: { _ in
            try await Task.sleep(nanoseconds: 5_000_000_000)
            return true
        })

        let result = await authenticator.checkPresence(testRequest(timeout: 0.001))

        XCTAssertFalse(result.approved)
        XCTAssertEqual(result.failureReason, "timed out waiting for local presence")
    }

    private func testRequest(timeout: TimeInterval = 2) -> LocalPresenceCheckRequest {
        LocalPresenceCheckRequest(
            challengeID: "challenge-1",
            deviceID: "device-alpha",
            workflowID: "workflow-1",
            taskName: "presence",
            prompt: "Approve this request",
            timeout: timeout
        )
    }
}
