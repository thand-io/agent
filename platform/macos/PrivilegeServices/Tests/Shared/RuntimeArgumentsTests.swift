import XCTest
@testable import PrivilegeServicesShared

final class RuntimeArgumentsTests: XCTestCase {
    func testBrokerHelperArgumentsRejectUnsupportedFlag() {
        XCTAssertThrowsError(try BrokerHelperArguments.parse(["serve", "--unexpected-flag"])) { error in
            XCTAssertEqual(error as? RuntimeArgumentError, .unsupportedArgument("--unexpected-flag"))
        }
    }

    func testNotifierArgumentsRejectUnsupportedFlag() {
        XCTAssertThrowsError(try NotifierRuntimeArguments.parse(["--unexpected-flag"])) { error in
            XCTAssertEqual(error as? RuntimeArgumentError, .unsupportedArgument("--unexpected-flag"))
        }
    }
}
