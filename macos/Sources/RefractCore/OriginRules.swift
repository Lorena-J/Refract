import Foundation

/// A WebKit content rule list (JSON) that blocks every load outside `origin`, subresources
/// included, so the token cookie (host only, which browsers send to every port of the host)
/// never reaches another service on this computer. `blob:`, `data:` and `about:` stay
/// allowed: the PDF viewer and the editor make them. README, "For packagers".
public func originOnlyRules(origin: URL) -> String? {
    guard let scheme = origin.scheme, let host = origin.host, let port = origin.port else { return nil }
    let hostPart = host.contains(":") ? "[\(host)]" : host  // an IPv6 address
    let prefix = "^" + regexEscaped("\(scheme)://\(hostPart):\(port)/")
    // WebKit's rule regexes have no alternation: one rule per allowed prefix.
    let allowed = [prefix, "^blob:", "^data:", "^about:"]
    let rules: [[String: Any]] =
        [["trigger": ["url-filter": ".*"], "action": ["type": "block"]]]
        + allowed.map { ["trigger": ["url-filter": $0], "action": ["type": "ignore-previous-rules"]] }
    guard let data = try? JSONSerialization.data(withJSONObject: rules, options: [.sortedKeys]) else { return nil }
    return String(decoding: data, as: UTF8.self)
}

private func regexEscaped(_ s: String) -> String {
    var out = ""
    for c in s {
        if "\\^$.|?*+()[]{}".contains(c) { out.append("\\") }
        out.append(c)
    }
    return out
}
