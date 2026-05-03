import Foundation
import XCTest
@testable import PrivilegeServicesShared

final class LocalPrivilegeManagerTests: XCTestCase {
    func testValidatorRejectsDeniedUsernames() {
        let validator = LocalAccountValidator { username in
            ValidatedLocalUser(username: username, uid: 501)
        }

        XCTAssertThrowsError(
            try validator.validate(username: "root", deniedUsernames: [], allowedUIDRanges: [])
        )
    }

    func testValidatorRejectsUIDOutsideConfiguredRanges() {
        let validator = LocalAccountValidator { username in
            ValidatedLocalUser(username: username, uid: 400)
        }

        XCTAssertThrowsError(
            try validator.validate(username: "tester", deniedUsernames: [], allowedUIDRanges: ["500-60000"])
        )
    }

    func testSudoersGrantManagerWritesAndRevokesFragments() throws {
        let temporaryDirectory = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("thand-privilege-broker-tests-\(UUID().uuidString)")
        let manager = SudoersGrantManager(
            sudoersDirectoryURL: temporaryDirectory,
            visudoPath: "/usr/sbin/visudo",
            runCommand: { arguments in
                XCTAssertEqual(arguments.count, 4)
            }
        )

        let fragmentURL = try manager.installTimedGrant(username: "tester", grantID: "grant-1", brokerHandle: "handle-1")
        let content = try String(contentsOf: fragmentURL, encoding: .utf8)
        XCTAssertTrue(content.contains("tester ALL=(ALL:ALL) NOPASSWD: ALL"))

        try manager.revoke(fragmentURL: fragmentURL)
        XCTAssertFalse(FileManager.default.fileExists(atPath: fragmentURL.path))
    }
}
