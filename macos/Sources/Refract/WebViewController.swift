import AppKit
import RefractCore
import WebKit

/// The web app in a WKWebView, kept to the server's own origin.
final class WebViewController: NSViewController, WKNavigationDelegate, WKUIDelegate {
    private let origin: URL
    private let webView: WKWebView
    private var loaded = false
    /// When the page process last ended: reloading stops after 3 in a minute.
    private var reloads: [Date] = []
    /// Called when the page can't be shown safely (the origin rules didn't compile).
    private let failed: (String) -> Void

    init(origin: URL, failed: @escaping (String) -> Void) {
        self.origin = origin
        self.failed = failed
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
            // Nothing outside the origin loads, subresources included: the cookie would go
            // with it. Without the rules nothing loads at all.
            guard let rules = originOnlyRules(origin: origin) else {
                return refuse("no port in \(origin.absoluteString)")
            }
            do {
                let list = try await WKContentRuleListStore.default().compileContentRuleList(
                    forIdentifier: "refract-origin-only", encodedContentRuleList: rules)
                guard let list else { return refuse("the origin rules compiled to nothing") }
                webView.configuration.userContentController.removeAllContentRuleLists()
                webView.configuration.userContentController.add(list)
            } catch {
                return refuse("the origin rules don't compile: \(error.localizedDescription)")
            }
            let store = webView.configuration.websiteDataStore.httpCookieStore
            // Cookies ignore ports: drop the token of an earlier launch so that it never
            // goes to another server on this host (a terminal instance gets none).
            for old in await store.allCookies() where old.name == embedCookieName && old.domain == origin.host {
                await store.deleteCookie(old)
            }
            if let token {
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

    private func refuse(_ reason: String) {
        AppLog.note("not loading the web app: \(reason)")
        failed(reason)
    }

    /// Calls window.refractFlush(): true when everything typed is saved, false when some
    /// of it couldn't be (or the page didn't answer in time), nil when no page is loaded.
    func flush() async -> Bool? {
        guard loaded else { return nil }
        // callAsyncJavaScript ignores cancellation, so a task group would wait for it past the
        // limit: whichever of the call and the timer finishes first resumes, once.
        return await withCheckedContinuation { (continuation: CheckedContinuation<Bool, Never>) in
            var resumed = false
            let finish: @MainActor (Bool) -> Void = { value in
                guard !resumed else { return }
                resumed = true
                continuation.resume(returning: value)
            }
            Task { @MainActor in
                let result = try? await self.webView.callAsyncJavaScript(
                    "return await window.refractFlush?.() ?? true", arguments: [:], in: nil, contentWorld: .page)
                finish((result as? Bool) ?? false)
            }
            Task { @MainActor in
                try? await Task.sleep(nanoseconds: 8_000_000_000)
                finish(false)
            }
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
        let now = Date()
        reloads = reloads.filter { now.timeIntervalSince($0) < 60 }
        guard reloads.count < 3 else {
            refuse("the web content process keeps ending")
            return
        }
        reloads.append(now)
        AppLog.note("the web content process ended: reloading")
        webView.reload()
    }
}
