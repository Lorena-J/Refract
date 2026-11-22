import AppKit
import RefractCore
import WebKit

/// The web app in a WKWebView, kept to the server's own origin.
final class WebViewController: NSViewController, WKNavigationDelegate, WKUIDelegate, WKDownloadDelegate {
    private let origin: URL
    private let webView: WKWebView
    private var loaded = false
    private var downloadTargets: [ObjectIdentifier: URL] = [:]

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

    /// Opens a URL outside the app, only for the schemes LinkPolicy allows. Nothing of the
    /// web view (its token cookie) goes along: the browser gets the bare URL.
    private func openExternally(_ url: URL?) {
        guard let url, LinkPolicy.classify(url, origin: origin) == .external else {
            AppLog.note("refused to open \(url?.scheme ?? "?"): link")
            return
        }
        NSWorkspace.shared.open(url)
    }

    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction,
        decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        let url = navigationAction.request.url
        switch LinkPolicy.classify(url, origin: origin) {
        case .ours:
            decisionHandler(navigationAction.shouldPerformDownload ? .download : .allow)
        case .external:
            openExternally(url)
            decisionHandler(.cancel)
        case .refused:
            AppLog.note("blocked navigation to a \(url?.scheme ?? "?"): link")
            decisionHandler(.cancel)
        }
    }

    func webView(_ webView: WKWebView, decidePolicyFor navigationResponse: WKNavigationResponse,
        decisionHandler: @escaping (WKNavigationResponsePolicy) -> Void) {
        let disposition = (navigationResponse.response as? HTTPURLResponse)?
            .value(forHTTPHeaderField: "Content-Disposition")?.lowercased() ?? ""
        let attachment = disposition.hasPrefix("attachment")
        decisionHandler(attachment || !navigationResponse.canShowMIMEType ? .download : .allow)
    }

    func webView(_ webView: WKWebView, navigationAction: WKNavigationAction, didBecome download: WKDownload) {
        download.delegate = self
    }

    func webView(_ webView: WKWebView, navigationResponse: WKNavigationResponse, didBecome download: WKDownload) {
        download.delegate = self
    }

    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration,
        for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        // target="_blank" and window.open never make a window: elsewhere goes to the browser,
        // the app's own pages load in this view.
        let url = navigationAction.request.url
        switch LinkPolicy.classify(url, origin: origin) {
        case .ours: if navigationAction.shouldPerformDownload { return nil } else { webView.load(navigationAction.request) }
        case .external: openExternally(url)
        case .refused: AppLog.note("blocked a new window for a \(url?.scheme ?? "?"): link")
        }
        return nil
    }

    // MARK: Downloads

    func download(_ download: WKDownload, decideDestinationUsing response: URLResponse, suggestedFilename: String,
        completionHandler: @escaping (URL?) -> Void) {
        let panel = NSSavePanel()
        panel.nameFieldStringValue = DownloadName.sanitize(suggestedFilename)
        panel.directoryURL = FileManager.default.urls(for: .downloadsDirectory, in: .userDomainMask).first
        panel.canCreateDirectories = true
        let finish: (NSApplication.ModalResponse) -> Void = { result in
            if result == .OK, let url = panel.url {
                // The panel already asked about replacing; WKDownload wants the path free.
                try? FileManager.default.removeItem(at: url)
                self.downloadTargets[ObjectIdentifier(download)] = url
                completionHandler(url)
            } else {
                completionHandler(nil)
            }
        }
        if let window = view.window {
            panel.beginSheetModal(for: window, completionHandler: finish)
        } else {
            finish(panel.runModal())
        }
    }

    func downloadDidFinish(_ download: WKDownload) {
        downloadTargets[ObjectIdentifier(download)] = nil
    }

    func download(_ download: WKDownload, didFailWithError error: Error, resumeData: Data?) {
        let target = downloadTargets.removeValue(forKey: ObjectIdentifier(download))
        // Cancelling the save panel ends the download with a cancellation: not an error.
        if (error as NSError).code == NSURLErrorCancelled, target == nil { return }
        AppLog.note("download failed: \(error.localizedDescription)")
        let alert = NSAlert()
        alert.alertStyle = .warning
        alert.messageText = L("download.failed.title", "The download failed")
        alert.informativeText = L("download.failed.message", "Refract couldn't save the file: %@")
            .replacingOccurrences(of: "%@", with: error.localizedDescription)
        alert.addButton(withTitle: L("download.failed.ok", "OK"))
        if let window = view.window { alert.beginSheetModal(for: window) } else { alert.runModal() }
    }

    // MARK: Dialogs

    /// window.confirm answers false without this: the web app asks before leaving a page with
    /// unsaved text.
    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String,
        initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        let alert = NSAlert()
        alert.messageText = message
        alert.addButton(withTitle: L("dialog.ok", "OK"))
        alert.addButton(withTitle: L("dialog.cancel", "Cancel"))
        if let window = view.window {
            alert.beginSheetModal(for: window) { completionHandler($0 == .alertFirstButtonReturn) }
        } else {
            completionHandler(alert.runModal() == .alertFirstButtonReturn)
        }
    }

    // MARK: File inputs

    func webView(_ webView: WKWebView, runOpenPanelWith parameters: WKOpenPanelParameters,
        initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping ([URL]?) -> Void) {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = parameters.allowsMultipleSelection
        // A directory input (webkitdirectory) picks folders only; a file input picks files.
        panel.canChooseDirectories = parameters.allowsDirectories
        panel.canChooseFiles = !parameters.allowsDirectories
        let finish: (NSApplication.ModalResponse) -> Void = { result in
            completionHandler(result == .OK ? panel.urls : nil)
        }
        if let window = view.window {
            panel.beginSheetModal(for: window, completionHandler: finish)
        } else {
            finish(panel.runModal())
        }
    }

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
