import WebKit
import XCTest
@testable import RefractCore

final class OriginRulesTests: XCTestCase {
    private func allowed(_ rules: String, _ url: String) throws -> Bool {
        let list = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(rules.utf8)) as? [[String: [String: String]]])
        var blocked = false
        for rule in list {
            let filter = try NSRegularExpression(pattern: try XCTUnwrap(rule["trigger"]?["url-filter"]))
            guard filter.firstMatch(in: url, range: NSRange(url.startIndex..., in: url)) != nil else { continue }
            blocked = rule["action"]?["type"] == "block"
        }
        return !blocked
    }

    // Only the exact origin (scheme, host and port) and in-page URLs load: another port of
    // the same host would receive the token cookie.
    func testOnlyTheOriginLoads() throws {
        let rules = try XCTUnwrap(originOnlyRules(origin: URL(string: "http://127.0.0.1:51234")!))
        for url in ["http://127.0.0.1:51234/", "http://127.0.0.1:51234/api/projects", "blob:http://127.0.0.1:51234/x", "data:image/png;base64,AA", "about:blank"] {
            XCTAssertTrue(try allowed(rules, url), url)
        }
        for url in ["http://127.0.0.1:8080/", "http://127.0.0.1:512345/", "http://127.0.0.1:5123/", "https://127.0.0.1:51234/",
            "http://127.0.0.1:51234.evil.example/", "http://127x0x0x1:51234/", "http://localhost:51234/", "https://example.com/"] {
            XCTAssertFalse(try allowed(rules, url), url)
        }
    }

    func testIPv6Origin() throws {
        let rules = try XCTUnwrap(originOnlyRules(origin: URL(string: "http://[::1]:8080")!))
        XCTAssertTrue(try allowed(rules, "http://[::1]:8080/"))
        XCTAssertFalse(try allowed(rules, "http://[::1]:8081/"))
    }

    func testNeedsAPort() {
        XCTAssertNil(originOnlyRules(origin: URL(string: "http://127.0.0.1")!))
    }

    // WebKit accepts the list (its regex dialect is narrower than NSRegularExpression's).
    @MainActor
    func testWebKitCompilesTheRules() async throws {
        let rules = try XCTUnwrap(originOnlyRules(origin: URL(string: "http://127.0.0.1:51234")!))
        let store = try XCTUnwrap(WKContentRuleListStore(url: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)))
        let list = try await store.compileContentRuleList(forIdentifier: "test", encodedContentRuleList: rules)
        XCTAssertNotNil(list)
    }
}
