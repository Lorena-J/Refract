import Foundation

/// The server's log file, `~/Library/Logs/Refract/refract.log`: the child's standard
/// error goes straight into it. At each start a file over `maxBytes` becomes
/// `refract.log.1` (and `.1` becomes `.2`, up to `keep` old files).
public struct LogStore {
    public let directory: URL
    public let maxBytes: Int
    public let keep: Int
    public var file: URL { directory.appendingPathComponent("refract.log") }

    public init(directory: URL, maxBytes: Int = 2_000_000, keep: Int = 2) {
        self.directory = directory
        self.maxBytes = maxBytes
        self.keep = keep
    }

    public static var standard: LogStore {
        let library = FileManager.default.urls(for: .libraryDirectory, in: .userDomainMask)[0]
        return LogStore(directory: library.appendingPathComponent("Logs/Refract", isDirectory: true))
    }

    /// Rotates if needed and opens the log for appending. The caller closes the descriptor.
    public func open(rotating: Bool = true) throws -> Int32 {
        let fm = FileManager.default
        try fm.createDirectory(at: directory, withIntermediateDirectories: true)
        if rotating { rotateIfNeeded() }
        let fd = Darwin.open(file.path, O_WRONLY | O_CREAT | O_APPEND | O_CLOEXEC, 0o600)
        if fd < 0 { throw CocoaError(.fileWriteNoPermission, userInfo: [NSFilePathErrorKey: file.path]) }
        return fd
    }

    func rotateIfNeeded() {
        let fm = FileManager.default
        let size = (try? fm.attributesOfItem(atPath: file.path)[.size] as? Int) ?? 0
        guard size > maxBytes else { return }
        func old(_ n: Int) -> URL { directory.appendingPathComponent("refract.log.\(n)") }
        try? fm.removeItem(at: old(keep))
        if keep > 1 {
            for n in stride(from: keep - 1, through: 1, by: -1) {
                try? fm.moveItem(at: old(n), to: old(n + 1))
            }
        }
        try? fm.moveItem(at: file, to: old(1))
    }

    /// A line of the app's own, written between the server's.
    public func note(_ text: String, fd: Int32) {
        let stamp = ISO8601DateFormatter().string(from: Date())
        let line = "\(stamp) [app] \(text)\n"
        line.withCString { _ = Darwin.write(fd, $0, strlen($0)) }
    }

    /// The last `lines` lines of the log (empty when there is none).
    public func tail(lines: Int = 20) -> String {
        guard let handle = try? FileHandle(forReadingFrom: file) else { return "" }
        defer { try? handle.close() }
        let size = (try? handle.seekToEnd()) ?? 0
        let start = size > 64_000 ? size - 64_000 : 0
        try? handle.seek(toOffset: start)
        let data = (try? handle.readToEnd()) ?? Data()
        let all = String(decoding: data, as: UTF8.self).split(separator: "\n", omittingEmptySubsequences: true)
        return all.suffix(lines).joined(separator: "\n")
    }
}
