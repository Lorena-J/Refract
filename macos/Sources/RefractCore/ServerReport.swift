import Foundation

/// What `refract local --embedded` is: the contract in the README, "For packagers".
public struct ServerInfo: Equatable {
    public var url: URL?
    public var pid: Int?
    public var version: String?
    public var dataDir: String?
    public var embedded: Bool
}

/// The one JSON line the server prints on standard output.
public enum ServerReport: Equatable {
    /// The server answers at `url`.
    case ready(ServerInfo)
    /// Another `refract local` holds the data folder. With `embedded` it is another app
    /// launch's instance (its token is unknown here), otherwise one started from a terminal.
    case running(ServerInfo)
    /// The lock is held but nothing answers.
    case unresponsive(ServerInfo)
    /// Starting failed; the message says what to fix.
    case error(String)
}

public enum ServerReportError: Error, Equatable {
    case notJSON
    case unknownState(String)
    case missingURL(String)
}

public func parseServerReport(_ line: String) throws -> ServerReport {
    guard let data = line.data(using: .utf8),
        let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
        let state = object["state"] as? String
    else { throw ServerReportError.notJSON }
    let info = ServerInfo(
        url: (object["url"] as? String).flatMap(URL.init(string:)),
        pid: object["pid"] as? Int,
        version: object["version"] as? String,
        dataDir: object["dataDir"] as? String,
        embedded: object["embedded"] as? Bool ?? false)
    switch state {
    case "ready", "running":
        guard info.url != nil else { throw ServerReportError.missingURL(state) }
        return state == "ready" ? .ready(info) : .running(info)
    case "unresponsive":
        return .unresponsive(info)
    case "error":
        return .error(object["error"] as? String ?? "")
    default:
        throw ServerReportError.unknownState(state)
    }
}

/// A random per-launch token: 32 random bytes, base64url (what the server asks for).
public func makeLaunchToken() -> String {
    var bytes = [UInt8](repeating: 0, count: 32)
    let status = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
    precondition(status == errSecSuccess, "no random bytes")
    return Data(bytes).base64EncodedString()
        .replacingOccurrences(of: "+", with: "-")
        .replacingOccurrences(of: "/", with: "_")
        .replacingOccurrences(of: "=", with: "")
}
