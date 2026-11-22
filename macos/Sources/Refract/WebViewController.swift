import AppKit
import RefractCore
import WebKit

/// The web app in a WKWebView, kept to the server's own origin.
final class WebViewController: NSViewController, WKNavigationDelegate, WKUIDelegate {
    private let origin: URL
    private let webView: WKWebView
    private var loaded = false

    init(origin: URL) {
        self.origin = origin
        let config = WKWebViewConfiguration()
        // The default store keeps what the web app saves in the browser (its settings), per
        // origin; the app asks for the same port every launch so the origin stays the same.
        config.websiteDataStore = .default()
        webView = WKWebView(frame: .zero, configuration: config)
        super.init(nibName: nil, bundle: nil)
        webView.navigationDelegate = self
        webView.uiDelegate = self
    }

    required init?(coder: NSCoder) { fatalError("not used") }

    override func loadView() { view = webView }

    /// Sets the token cookie in the web view's own store, then loads the app: the very
    /// first request already carries it. token is nil for a terminal instance, which has none.
    func load(token: String?) {
        Task { @MainActor in
            if let token {
                let store = webView.configuration.websiteDataStore.httpCookieStore
                let cookie = HTTPCookie(properties: [
                    .name: embedCookieName,
                    .value: token,
                    .path: "/",
                    .originURL: origin,  // host only: no Domain attribute
                    .sameSitePolicy: HTTPCookieStringPolicy.sameSiteStrict,
                    HTTPCookiePropertyKey("HttpOnly"): "TRUE",
                ])
                if let cookie { await store.setCookie(cookie) }
            }
            webView.load(URLRequest(url: origin))
        }
    }

    /// Calls window.refractFlush(): true when everything typed is saved, false when some
    /// of it couldn't be (or the page didn't answer in time), nil when no page is loaded.
    func flush() async -> Bool? {
        guard loaded else { return nil }
        return await withTaskGroup(of: Bool.self) { group in
            group.addTask { @MainActor in
                let result = try? await self.webView.callAsyncJavaScript(
                    "return await window.refractFlush?.() ?? true", arguments: [:], in: nil, contentWorld: .page)
                return (result as? Bool) ?? false
            }
            group.addTask {
                try? await Task.sleep(nanoseconds: 8_000_000_000)
                return false
            }
            let first = await group.next() ?? false
            group.cancelAll()
            return first
        }
    }

    // MARK: Navigation

    private func isOurs(_ url: URL?) -> Bool {
        guard let url else { return false }
        switch url.scheme {
        case "about", "blob", "data": return true
        default: return url.scheme == origin.scheme && url.host == origin.host && url.port == origin.port
        }
    }

    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction,
        decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        if isOurs(navigationAction.request.url) {
            decisionHandler(.allow)
        } else {
            // TODO(#67): open links that leave the app's origin in the default browser.
            AppLog.note("blocked navigation to \(navigationAction.request.url?.absoluteString ?? "?")")
            decisionHandler(.cancel)
        }
    }

    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration,
        for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        // TODO(#67): target="_blank" and window.open go to the default browser.
        nil
    }

    // TODO(#67): WKDownloadDelegate for downloads and runOpenPanelWith for file inputs.

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        loaded = true
        AppLog.note("page loaded: \(webView.url?.absoluteString ?? "?")")
    }

    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        AppLog.note("page failed: \(error.localizedDescription)")
    }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        AppLog.note("page failed to load: \(error.localizedDescription)")
    }

    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        AppLog.note("the web content process ended: reloading")
        webView.reload()
    }
}
