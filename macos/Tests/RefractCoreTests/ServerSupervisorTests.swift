import XCTest
@testable import RefractCore

/// Runs the supervisor against a shell script that behaves like `refract local --embedded`
/// in the way FAKE_MODE says.
final class ServerSupervisorTests: XCTestCase {
    var dir: URL!
    var supervisor: ServerSupervisor!

    static let script = #"""
    #!/bin/sh
    read -r tok <&3
    printf '%s' "$tok" > "$OUT/token"
    echo "$$" > "$OUT/pid"
    case "$FAKE_MODE" in
    ready) echo '{"state":"ready","url":"http://127.0.0.1:4567","pid":1,"version":"t"}' ;;
    terminal) echo '{"state":"running","url":"http://127.0.0.1:4568","pid":9}'; exit 0 ;;
    otherapp) echo '{"state":"running","url":"http://127.0.0.1:4568","pid":9,"embedded":true}'; exit 0 ;;
    unresponsive) echo '{"state":"unresponsive","pid":9}'; exit 1 ;;
    error) echo '{"state":"error","error":"the key is unreadable"}'; exit 1 ;;
    crash) echo "panic: boom" >&2; exit 2 ;;
    portbusy)
      case "$*" in *--port*) echo '{"state":"error","error":"address in use"}'; exit 1 ;; esac
      echo '{"state":"ready","url":"http://127.0.0.1:4569","pid":1}' ;;
    stubborn) echo '{"state":"ready","url":"http://127.0.0.1:4567","pid":1}'
      trap '' TERM; sleep 60 & wait; exit 0 ;;
    dies) echo '{"state":"ready","url":"http://127.0.0.1:4567","pid":1}'; echo "about to die" >&2; kill -9 $$ ;;
    esac
    cat >/dev/null
    echo "stdin closed, bye" >&2
    exit 0
    """#

    override func setUpWithError() throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("refract-sup-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let exe = dir.appendingPathComponent("fake-refract")
        try Self.script.write(to: exe, atomically: true, encoding: .utf8)
        try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: exe.path)
    }

    override func tearDown() {
        supervisor?.stop()
        spin(until: { true }, for: 0.2)
        try? FileManager.default.removeItem(at: dir)
    }

    func make(_ mode: String, stopTimeout: TimeInterval = 5, readyTimeout: TimeInterval = 10) {
        var config = ServerSupervisor.Config(
            executable: dir.appendingPathComponent("fake-refract"),
            environment: ["FAKE_MODE": mode, "OUT": dir.path, "PATH": "/usr/bin:/bin"],
            log: LogStore(directory: dir.appendingPathComponent("logs")))
        config.stopTimeout = stopTimeout
        config.readyTimeout = readyTimeout
        supervisor = ServerSupervisor(config: config)
    }

    @discardableResult
    func spin(until done: () -> Bool, for seconds: TimeInterval = 10) -> Bool {
        let end = Date().addingTimeInterval(seconds)
        while !done() && Date() < end { RunLoop.main.run(until: Date().addingTimeInterval(0.02)) }
        return done()
    }

    func waitForSettled() {
        spin { ![ServerState.idle, .starting].contains(self.supervisor.state) }
    }

    func alive(_ pid: pid_t) -> Bool { Darwin.kill(pid, 0) == 0 }
    func childPID() -> pid_t { pid_t(((try? String(contentsOf: dir.appendingPathComponent("pid"))) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)) ?? 0 }

    func testReadyGivesTheURLAndTheTokenThroughFD3() throws {
        make("ready")
        supervisor.start()
        waitForSettled()
        XCTAssertEqual(supervisor.state, .ready(url: URL(string: "http://127.0.0.1:4567")!, ownsServer: true))
        let token = try String(contentsOf: dir.appendingPathComponent("token"))
        XCTAssertEqual(token, supervisor.token)
        XCTAssertGreaterThanOrEqual(token.count, 16)
        // The token never reaches the environment or the arguments.
        XCTAssertFalse(supervisor.config.arguments.joined().contains(token))
        XCTAssertNil(supervisor.config.environment["REFRACT_EMBED_TOKEN"])
    }

    func testStopClosesStdinAndTheServerExitsGracefully() throws {
        make("ready")
        supervisor.start(); waitForSettled()
        let pid = childPID()
        XCTAssertTrue(alive(pid))
        var outcome: StopOutcome?
        supervisor.stop { outcome = $0 }
        XCTAssertTrue(spin { outcome != nil })
        XCTAssertEqual(outcome, .graceful)
        XCTAssertEqual(supervisor.state, .stopped)
        XCTAssertFalse(alive(pid), "no server process remains")
        XCTAssertTrue(supervisor.config.log.tail(lines: 50).contains("stdin closed, bye"))
    }

    func testAServerThatDoesNotStopIsKilledAfterTheTimeout() throws {
        make("stubborn", stopTimeout: 0.5)
        supervisor.start(); waitForSettled()
        let pid = childPID()
        var outcome: StopOutcome?
        supervisor.stop { outcome = $0 }
        XCTAssertTrue(spin { outcome != nil })
        XCTAssertEqual(outcome, .killed)
        XCTAssertFalse(alive(pid))
    }

    func testKillingTheServerShowsTheErrorAndRestartRecovers() throws {
        make("ready")
        supervisor.start(); waitForSettled()
        let first = childPID()
        Darwin.kill(first, SIGKILL)
        spin { if case .failed = self.supervisor.state { return true } else { return false } }
        guard case .failed(.exited(let status, let signaled, _)) = supervisor.state else { return XCTFail("state \(supervisor.state)") }
        XCTAssertTrue(signaled)
        XCTAssertEqual(status, SIGKILL)

        supervisor.restart()
        spin { if case .ready = self.supervisor.state { return true } else { return false } }
        XCTAssertEqual(supervisor.state, .ready(url: URL(string: "http://127.0.0.1:4567")!, ownsServer: true))
        XCTAssertNotEqual(childPID(), first)
    }

    func testUnexpectedExitKeepsTheEndOfTheLog() {
        make("dies")
        supervisor.start()
        spin { if case .failed = self.supervisor.state { return true } else { return false } }
        guard case .failed(.exited(_, _, let tail)) = supervisor.state else { return XCTFail("state \(supervisor.state)") }
        XCTAssertTrue(tail.contains("about to die"))
    }

    func testATerminalInstanceIsUsedWithoutOwningIt() {
        make("terminal")
        supervisor.start(); waitForSettled()
        XCTAssertEqual(supervisor.state, .ready(url: URL(string: "http://127.0.0.1:4568")!, ownsServer: false))
        spin(until: { false }, for: 0.3)  // its exit isn't a failure
        XCTAssertEqual(supervisor.state, .ready(url: URL(string: "http://127.0.0.1:4568")!, ownsServer: false))
        var outcome: StopOutcome?
        supervisor.stop { outcome = $0 }
        XCTAssertEqual(outcome, .notRunning)
    }

    func testAnotherAppInstanceIsAnError() {
        make("otherapp")
        supervisor.start(); waitForSettled()
        XCTAssertEqual(supervisor.state, .failed(.anotherAppInstance))
    }

    func testUnresponsiveAndErrorLines() {
        make("unresponsive")
        supervisor.start(); waitForSettled()
        XCTAssertEqual(supervisor.state, .failed(.unresponsive(pid: 9)))
        make("error")
        supervisor.start(); waitForSettled()
        XCTAssertEqual(supervisor.state, .failed(.startFailed("the key is unreadable")))
    }

    func testExit2WithoutALineShowsStandardError() {
        make("crash")
        supervisor.start(); waitForSettled()
        guard case .failed(.startFailed(let message)) = supervisor.state else { return XCTFail("state \(supervisor.state)") }
        XCTAssertTrue(message.contains("panic: boom"))
    }

    func testARememberedPortThatCantBeUsedFallsBackToAFreeOne() {
        make("portbusy")
        supervisor.preferredPort = 5555
        supervisor.start(); waitForSettled()
        XCTAssertEqual(supervisor.state, .ready(url: URL(string: "http://127.0.0.1:4569")!, ownsServer: true))
        XCTAssertTrue(supervisor.config.log.tail(lines: 50).contains("retrying without --port 5555"))
    }

    func testAMissingServerOrToolIsADamagedInstallAndNothingStarts() {
        make("ready")
        supervisor.config.layout = BundleLayout(contents: dir.appendingPathComponent("Nothing"))
        supervisor.start()
        guard case .failed(.installDamaged(let names)) = supervisor.state else { return XCTFail("state \(supervisor.state)") }
        XCTAssertTrue(names.contains("refract"))
        XCTAssertFalse(FileManager.default.fileExists(atPath: dir.appendingPathComponent("pid").path))
    }

    func testReadyTimeoutKillsTheServer() {
        make("stubborn", readyTimeout: 0.01)
        // stubborn prints a line at once; use a mode that stays silent instead:
        supervisor.config.arguments = []
        supervisor.config.environment["FAKE_MODE"] = "silent"
        supervisor.start()
        spin { if case .failed = self.supervisor.state { return true } else { return false } }
        guard case .failed(.startFailed(let message)) = supervisor.state else { return XCTFail("state \(supervisor.state)") }
        XCTAssertTrue(message.contains("did not answer"))
    }
}
