import Foundation

/// What the app does with a URL the web view wants to open. Pure, so it is tested.
public enum LinkDecision: Equatable {
    /// The server's own origin (and about:, blob:, data: pages the web app makes): stays in the app.
    case ours
    /// http, https or mailto elsewhere: the default browser (or mail app).
    case external
    /// Any other scheme (file:, javascript:, custom app schemes): never opened.
    case refused
}

public enum LinkPolicy {
    public static let externalSchemes: Set<String> = ["http", "https", "mailto"]

    public static func classify(_ url: URL?, origin: URL) -> LinkDecision {
        guard let url, let scheme = url.scheme?.lowercased() else { return .refused }
        switch scheme {
        case "about", "blob", "data": return .ours
        default: break
        }
        if scheme == origin.scheme?.lowercased(), url.host?.lowercased() == origin.host?.lowercased(),
            effectivePort(url) == effectivePort(origin) {
            return .ours
        }
        return externalSchemes.contains(scheme) ? .external : .refused
    }

    private static func effectivePort(_ url: URL) -> Int? {
        if let port = url.port { return port }
        switch url.scheme?.lowercased() {
        case "http": return 80
        case "https": return 443
        default: return nil
        }
    }
}

public enum DownloadName {
    /// A file name that is safe to offer in a save panel: no path parts, no control characters,
    /// never empty or hidden-only. The server's names come from project names.
    public static func sanitize(_ suggested: String, fallback: String = "download") -> String {
        let last = suggested.split(whereSeparator: { $0 == "/" || $0 == "\\" || $0 == ":" }).last.map(String.init) ?? ""
        let cleaned = String(last.unicodeScalars.filter { !CharacterSet.controlCharacters.contains($0) })
            .trimmingCharacters(in: .whitespaces)
        let stripped = cleaned.drop(while: { $0 == "." })
        if stripped.isEmpty { return fallback }
        return cleaned.hasPrefix(".") ? String(stripped) : cleaned
    }
}
