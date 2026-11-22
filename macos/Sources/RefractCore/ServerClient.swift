import Foundation

/// Asks the running server which bundled tools it couldn't find (`missing_tools` of
/// `/api/me`). Returns nothing when it can't be asked: that isn't a damaged install.
public func fetchMissingTools(url: URL, token: String?) async -> [String] {
    var request = URLRequest(url: url.appendingPathComponent("api/me"), timeoutInterval: 10)
    request.httpShouldHandleCookies = false
    if let token, !token.isEmpty { request.setValue("\(embedCookieName)=\(token)", forHTTPHeaderField: "Cookie") }
    guard let (data, response) = try? await URLSession.shared.data(for: request),
        (response as? HTTPURLResponse)?.statusCode == 200,
        let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
    else { return [] }
    return object["missing_tools"] as? [String] ?? []
}

/// The cookie that carries the per-launch token (README, "For packagers").
public let embedCookieName = "refract_embed"
