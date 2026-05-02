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

    func testUsesLocalNotificationPayload() {
        let notification = LocalNotificationPostRequest(
            notificationID: "notification-1",
            title: "Access approved",
            subtitle: "Local Sudo",
            body: "Your sudo access is ready",
            threadID: "workflow-1"
        )
        let event = BrokerEvent(
            kind: .localNotification,
            brokerHandle: "notification-1",
            grantID: "",
            deviceID: "",
            username: "tester",
            localNotification: notification,
            occurredAt: Date(timeIntervalSince1970: 1)
        )

        XCTAssertEqual(NotificationFormatter.request(for: event), notification)
        XCTAssertEqual(NotificationFormatter.body(for: event), "Your sudo access is ready")
    }
}
