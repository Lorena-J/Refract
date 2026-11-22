import XCTest

@testable import RefractCore

final class LinkPolicyTests: XCTestCase {
    let origin = URL(string: "http://127.0.0.1:51234")!

    func classify(_ s: String) -> LinkDecision { LinkPolicy.classify(URL(string: s), origin: origin) }

    func testOurOriginStays() {
        XCTAssertEqual(classify("http://127.0.0.1:51234/projects/1"), .ours)
        XCTAssertEqual(classify("blob:http://127.0.0.1:51234/abc"), .ours)
        XCTAssertEqual(classify("about:blank"), .ours)
    }

    func testOtherPortOrHostIsExternal() {
        XCTAssertEqual(classify("http://127.0.0.1:51235/"), .external)
        XCTAssertEqual(classify("http://localhost:51234/"), .external)
        XCTAssertEqual(classify("https://127.0.0.1:51234/"), .external)
        XCTAssertEqual(classify("https://example.com/help"), .external)
        XCTAssertEqual(classify("mailto:a@b.c"), .external)
    }

    func testOtherSchemesAreRefused() {
        XCTAssertEqual(classify("file:///etc/passwd"), .refused)
        XCTAssertEqual(classify("javascript:alert(1)"), .refused)
        XCTAssertEqual(classify("x-apple.systempreferences:"), .refused)
        XCTAssertEqual(classify("ssh://host"), .refused)
        XCTAssertEqual(LinkPolicy.classify(nil, origin: origin), .refused)
    }

    func testDownloadNames() {
        XCTAssertEqual(DownloadName.sanitize("My thesis.zip"), "My thesis.zip")
        XCTAssertEqual(DownloadName.sanitize("../../etc/passwd"), "passwd")
        XCTAssertEqual(DownloadName.sanitize("a:b\\c.pdf"), "c.pdf")
        XCTAssertEqual(DownloadName.sanitize(""), "download")
        XCTAssertEqual(DownloadName.sanitize("..."), "download")
        XCTAssertEqual(DownloadName.sanitize(".hidden"), "hidden")
        XCTAssertEqual(DownloadName.sanitize("a\u{0}b.pdf"), "ab.pdf")
        XCTAssertEqual(DownloadName.sanitize("x", fallback: "f"), "x")
    }
}
