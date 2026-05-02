import XCTest
@testable import PrivilegeServicesShared

final class NotifierXPCTests: XCTestCase {
    func testIdentityResolverCanonicalizesNotifierUserFromUID() throws {
        let identity = try NotifierIdentityResolver.resolve(
            uid: 501,
            auditSessionIdentifier: 42,
            processIdentifier: 9001,
            userLookup: { uid in
                XCTAssertEqual(uid, 501)
                return ValidatedLocalUser(username: "tester", uid: Int(uid))
            }
        )

        XCTAssertEqual(identity.username, "tester")
        XCTAssertEqual(identity.uid, 501)
        XCTAssertEqual(identity.auditSessionIdentifier, 42)
        XCTAssertEqual(identity.processIdentifier, 9001)
    }

    func testIdentityResolverRejectsUnknownUID() {
        XCTAssertThrowsError(try NotifierIdentityResolver.resolve(
            uid: 777,
            auditSessionIdentifier: 11,
            processIdentifier: 22,
            userLookup: { _ in nil }
        )) { error in
            guard case NotifierXPCError.invalidPeer(let message) = error else {
                return XCTFail("unexpected error: \(error)")
            }
            XCTAssertTrue(message.contains("777"))
        }
    }
}
