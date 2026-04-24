import XCTest
@testable import PrivilegeServicesShared

final class NotificationFormatterTests: XCTestCase {
    func testFormatsExpiryNotification() {
        let event = BrokerEvent(
            kind: .grantExpired,
            brokerHandle: "broker-handle",
            grantID: "grant-1",
            deviceID: "device-1",
            username: "tester",
            occurredAt: Date(timeIntervalSince1970: 1)
        )

        XCTAssertEqual(NotificationFormatter.title, "Thand Privileged Access")
        XCTAssertEqual(
            NotificationFormatter.body(for: event),
            "Timed sudo access expired for tester."
        )
    }
}
