import XCTest
@testable import RefractCore

final class UpdateSettingsTests: XCTestCase {
    private let key = Data(repeating: 7, count: 32).base64EncodedString()
    private let feed = "https://github.com/ponytojas/Refract/releases/latest/download/appcast-arm64.xml"

    private func info(key: String? = nil, feed: String? = nil) -> [String: Any] {
        var d: [String: Any] = [:]
        if let key { d["SUPublicEDKey"] = key }
        if let feed { d["SUFeedURL"] = feed }
        return d
    }

    func testEnabledWithKeyAndHTTPSFeed() {
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: key, feed: feed)),
                       .enabled(feedURL: URL(string: feed)!, publicKey: key))
    }

    func testDevelopmentBuildWithoutKeyIsDisabled() {
        XCTAssertEqual(UpdateSettings.resolve(info: info(feed: feed)), .disabled(.noPublicKey))
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: "  ", feed: feed)), .disabled(.noPublicKey))
    }

    func testKeyThatIsNotAnEd25519PublicKeyIsDisabled() {
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: "not base64!", feed: feed)), .disabled(.invalidPublicKey))
        let short = Data(repeating: 1, count: 31).base64EncodedString()
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: short, feed: feed)), .disabled(.invalidPublicKey))
    }

    func testFeedIsRequiredAndMustBeSecure() {
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: key)), .disabled(.noFeed))
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: key, feed: "not a url")), .disabled(.noFeed))
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: key, feed: "http://example.com/appcast.xml")),
                       .disabled(.insecureFeed("http://example.com/appcast.xml")))
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: key, feed: "file:///tmp/appcast.xml")),
                       .disabled(.insecureFeed("file:///tmp/appcast.xml")))
    }

    func testEnvironmentOverrideAllowsLoopbackHTTPOnly() {
        let name = UpdateSettings.feedOverrideVariable
        let local = "http://127.0.0.1:8099/appcast.xml"
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: key, feed: feed), environment: [name: local]),
                       .enabled(feedURL: URL(string: local)!, publicKey: key))
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: key, feed: feed), environment: [name: "http://evil.example/a.xml"]),
                       .disabled(.insecureFeed("http://evil.example/a.xml")))
        // An empty override is no override; it never turns updates on without a key.
        XCTAssertEqual(UpdateSettings.resolve(info: info(key: key, feed: feed), environment: [name: ""]),
                       .enabled(feedURL: URL(string: feed)!, publicKey: key))
        XCTAssertEqual(UpdateSettings.resolve(info: info(feed: feed), environment: [name: local]), .disabled(.noPublicKey))
    }
}
