import PrivilegeServicesShared
import XCTest

final class LocalNotificationPosterTests: XCTestCase {
    func testPostSuccessRequestsAuthorizationAndAddsNotification() async throws {
        let captured = CapturedLocalNotification()
        let poster = LocalNotificationPoster(
            requestAuthorization: { true },
            addNotification: { request in
                await captured.set(request)
            }
        )

        let request = LocalNotificationPostRequest(
            notificationID: "notification-1",
            title: "Access approved",
            subtitle: "Local Sudo",
            body: "Your sudo access is ready",
            threadID: "workflow-1"
        )
        try await poster.post(request)

        let added = await captured.value()
        XCTAssertEqual(added, request)
    }

    func testPermissionDeniedFailsWithoutPosting() async {
        let poster = LocalNotificationPoster(
            requestAuthorization: { false },
            addNotification: { _ in
                XCTFail("notification should not be posted when permission is denied")
            }
        )

        await assertPostFailure(poster, expected: .permissionDenied)
    }

    func testUnavailableNotificationCenterFailsBeforePosting() async {
        let poster = LocalNotificationPoster(
            requestAuthorization: {
                throw NSError(domain: "UNUserNotificationCenter", code: 1)
            },
            addNotification: { _ in
                XCTFail("notification should not be posted when authorization cannot be checked")
            }
        )

        let failure = await capturePostFailure(poster)
        guard case .notificationCenterUnavailable = failure else {
            return XCTFail("failure = \(String(describing: failure)), want notificationCenterUnavailable")
        }
    }

    func testPostingFailureIsMapped() async {
        let poster = LocalNotificationPoster(
            requestAuthorization: { true },
            addNotification: { _ in
                throw NSError(domain: "UNUserNotificationCenter", code: 2)
            }
        )

        let failure = await capturePostFailure(poster)
        guard case .postingFailed = failure else {
            return XCTFail("failure = \(String(describing: failure)), want postingFailed")
        }
    }

    func testInvalidRequestRequiresTitleAndBody() async {
        let poster = LocalNotificationPoster(
            requestAuthorization: { true },
            addNotification: { _ in
                XCTFail("invalid notification should not be posted")
            }
        )

        let failure = await capturePostFailure(
            poster,
            request: LocalNotificationPostRequest(title: "", body: "body")
        )
        guard case .invalidRequest(let message) = failure else {
            return XCTFail("failure = \(String(describing: failure)), want invalidRequest")
        }
        XCTAssertEqual(message, "local notification title is required")
    }

    private func assertPostFailure(
        _ poster: LocalNotificationPoster,
        expected: LocalNotificationPostFailure,
        file: StaticString = #filePath,
        line: UInt = #line
    ) async {
        let failure = await capturePostFailure(poster)
        XCTAssertEqual(failure, expected, file: file, line: line)
    }

    private func capturePostFailure(
        _ poster: LocalNotificationPoster,
        request: LocalNotificationPostRequest = LocalNotificationPostRequest(
            title: "Access approved",
            body: "Your sudo access is ready"
        )
    ) async -> LocalNotificationPostFailure? {
        do {
            try await poster.post(request)
            XCTFail("post unexpectedly succeeded")
            return nil
        } catch let failure as LocalNotificationPostFailure {
            return failure
        } catch {
            XCTFail("unexpected error: \(error)")
            return nil
        }
    }
}

private actor CapturedLocalNotification {
    private var request: LocalNotificationPostRequest?

    func set(_ request: LocalNotificationPostRequest) {
        self.request = request
    }

    func value() -> LocalNotificationPostRequest? {
        request
    }
}
