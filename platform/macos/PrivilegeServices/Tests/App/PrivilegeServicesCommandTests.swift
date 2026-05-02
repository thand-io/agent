import XCTest
@testable import PrivilegeServicesAppSupport

final class PrivilegeServicesCommandTests: XCTestCase {
    func testDefaultsToStatus() throws {
        XCTAssertEqual(try PrivilegeServicesCommand.parse(arguments: []), .status)
    }

    func testParsesKnownCommand() throws {
        XCTAssertEqual(
            try PrivilegeServicesCommand.parse(arguments: ["open-settings"]),
            .openSettings
        )
    }

    func testRejectsUnknownCommand() {
        XCTAssertThrowsError(
            try PrivilegeServicesCommand.parse(arguments: ["mystery-command"])
        )
    }
}
