import AppKit
import RefractCore

final class AppDelegate: NSObject, NSApplicationDelegate {
    private var supervisor: ServerSupervisor!
    private var root: RootViewController!
    private var window: NSWindow!
    private var quitting = false
    private let portKey = "serverPort"
    /// `REFRACT_APP_DEFAULTS_SUITE` names a separate UserDefaults suite (scratch runs and tests).
    private static let defaults: UserDefaults = {
        if let suite = ProcessInfo.processInfo.environment["REFRACT_APP_DEFAULTS_SUITE"], !suite.isEmpty,
           let d = UserDefaults(suiteName: suite) { return d }
        return .standard
    }()

    func applicationDidFinishLaunching(_ notification: Notification) {
        if activateRunningInstance() {
            exit(0)
        }
        installMenu()

        let layout = BundleLayout()
        var config = ServerSupervisor.Config(
            executable: layout.server,
            environment: ServerSupervisor.serverEnvironment(from: ProcessInfo.processInfo.environment),
            log: AppLog.store,
            layout: layout)
        // Same data folder as `refract local`: no REFRACT_DATA_DIR, so the server uses
        // ~/Library/Application Support/Refract.
        config.workingDirectory = NSHomeDirectory()
        supervisor = ServerSupervisor(config: config)
        let saved = Self.defaults.integer(forKey: portKey)
        supervisor.preferredPort = saved > 0 ? saved : nil

        root = RootViewController(
            restart: { [weak self] in self?.supervisor.restart() },
            showLogs: { NSWorkspace.shared.activateFileViewerSelecting([AppLog.store.file]) },
            reinstall: { NSWorkspace.shared.open(URL(string: "https://github.com/ponytojas/Refract/releases/latest")!) })
        window = NSWindow(contentViewController: root)
        window.styleMask = [.titled, .closable, .miniaturizable, .resizable]
        window.title = "Refract"
        window.minSize = NSSize(width: 640, height: 420)
        window.setContentSize(NSSize(width: 1280, height: 820))
        window.setFrameAutosaveName("RefractMainWindow")
        if !window.setFrameUsingName("RefractMainWindow") { window.center() }
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)

        supervisor.onChange = { [weak self] in self?.apply($0) }
        root.showLoading(L("loading.starting", "Starting Refract…"))
        supervisor.start()
    }

    /// A second launch of the app (a copy elsewhere, or a launch from the terminal) brings
    /// the first one forward instead of starting a second server on the same data folder.
    private func activateRunningInstance() -> Bool {
        guard let id = Bundle.main.bundleIdentifier else { return false }
        let others = NSRunningApplication.runningApplications(withBundleIdentifier: id)
            .filter { $0.processIdentifier != ProcessInfo.processInfo.processIdentifier }
        guard let other = others.first else { return false }
        other.activate(options: [.activateIgnoringOtherApps])
        return true
    }

    private func apply(_ state: ServerState) {
        guard !quitting else { return }
        switch state {
        case .idle, .starting:
            root.showLoading(L("loading.starting", "Starting Refract…"))
        case .stopping, .stopped:
            root.showLoading(L("loading.restarting", "Restarting Refract…"))
        case .ready(let url, let owns):
            if owns, let port = url.port {
                Self.defaults.set(port, forKey: portKey)
                supervisor.preferredPort = port
            }
            let token = owns ? supervisor.token : nil
            root.showWeb(url: url, token: token)
            if owns { checkTools(url: url, token: token) }
        case .failed(let failure):
            // Not the failure itself: its log tail is already in the log.
            AppLog.note("showing the error view: \(String(describing: failure).prefix(while: { $0 != "(" }))")
            root.showError(failure)
        }
    }

    /// The server logs and reports (/api/me) the tools it couldn't find: that is a damaged
    /// install, whatever the person's PATH has.
    private func checkTools(url: URL, token: String?) {
        Task { @MainActor in
            let missing = await fetchMissingTools(url: url, token: token)
            guard !missing.isEmpty, case .ready(let now, true) = supervisor.state, now == url else { return }
            AppLog.note("the server reports missing tools: \(missing.joined(separator: ", "))")
            supervisor.reportMissingTools(missing)
        }
    }

    // MARK: Quitting

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard let supervisor else { return .terminateNow }
        if quitting { return .terminateLater }
        // A terminal instance has nothing to stop, but its page may hold unsaved typing.
        switch supervisor.state {
        case .starting, .stopping, .ready: break
        default: return .terminateNow
        }
        quitting = true
        Task { @MainActor in
            // What was typed in the last moment only exists in the page: save it first.
            if case .ready = supervisor.state, await root.flushPage() == false {
                let alert = NSAlert()
                alert.alertStyle = .warning
                alert.messageText = L("quit.unsaved.title", "Some changes couldn't be saved")
                alert.informativeText = L("quit.unsaved.message", "Refract couldn't save everything you typed (the project may be locked or gone). Quit anyway and lose it?")
                alert.addButton(withTitle: L("quit.unsaved.cancel", "Don't Quit"))
                alert.addButton(withTitle: L("quit.unsaved.quit", "Quit Anyway"))
                if alert.runModal() == .alertFirstButtonReturn {
                    quitting = false
                    NSApp.reply(toApplicationShouldTerminate: false)
                    return
                }
            }
            // Progress only when stopping isn't instant.
            let root = self.root!
            let slow = Timer.scheduledTimer(withTimeInterval: 1, repeats: false) { _ in
                MainActor.assumeIsolated { root.showLoading(L("loading.stopping", "Saving and closing Refract…")) }
            }
            supervisor.stop { outcome in
                slow.invalidate()
                AppLog.note("quitting after the server stopped (\(outcome))")
                NSApp.reply(toApplicationShouldTerminate: true)
            }
        }
        return .terminateLater
    }

    // MARK: Menu (a minimum: ⌘Q, ⌘W and the editing commands the web view needs; #68 replaces it)

    private func installMenu() {
        let main = NSMenu()
        func add(_ menu: NSMenu, _ title: String, _ action: Selector?, _ key: String = "", _ mods: NSEvent.ModifierFlags = .command) {
            let item = menu.addItem(withTitle: title, action: action, keyEquivalent: key)
            item.keyEquivalentModifierMask = mods
        }
        func submenu(_ title: String) -> NSMenu {
            let item = main.addItem(withTitle: title, action: nil, keyEquivalent: "")
            let menu = NSMenu(title: title)
            main.setSubmenu(menu, for: item)
            return menu
        }
        let app = submenu("Refract")
        add(app, L("menu.about", "About Refract"), #selector(NSApplication.orderFrontStandardAboutPanel(_:)))
        app.addItem(.separator())
        add(app, L("menu.hide", "Hide Refract"), #selector(NSApplication.hide(_:)), "h")
        add(app, L("menu.hideOthers", "Hide Others"), #selector(NSApplication.hideOtherApplications(_:)), "h", [.command, .option])
        add(app, L("menu.showAll", "Show All"), #selector(NSApplication.unhideAllApplications(_:)))
        app.addItem(.separator())
        add(app, L("menu.quit", "Quit Refract"), #selector(NSApplication.terminate(_:)), "q")

        let edit = submenu(L("menu.edit", "Edit"))
        add(edit, L("menu.undo", "Undo"), Selector(("undo:")), "z")
        add(edit, L("menu.redo", "Redo"), Selector(("redo:")), "z", [.command, .shift])
        edit.addItem(.separator())
        add(edit, L("menu.cut", "Cut"), #selector(NSText.cut(_:)), "x")
        add(edit, L("menu.copy", "Copy"), #selector(NSText.copy(_:)), "c")
        add(edit, L("menu.paste", "Paste"), #selector(NSText.paste(_:)), "v")
        add(edit, L("menu.selectAll", "Select All"), #selector(NSText.selectAll(_:)), "a")

        let windowMenu = submenu(L("menu.window", "Window"))
        add(windowMenu, L("menu.minimize", "Minimize"), #selector(NSWindow.performMiniaturize(_:)), "m")
        add(windowMenu, L("menu.zoom", "Zoom"), #selector(NSWindow.performZoom(_:)))
        add(windowMenu, L("menu.close", "Close Window"), #selector(NSWindow.performClose(_:)), "w")
        NSApp.mainMenu = main
        NSApp.windowsMenu = windowMenu
    }
}
