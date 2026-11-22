import Foundation
import RefractCore

/// The app's own lines in the same file as the server's (`~/Library/Logs/Refract/refract.log`).
enum AppLog {
    static let store = LogStore.standard

    static func note(_ text: String) {
        guard let fd = try? store.open(rotating: false) else { return }
        store.note(text, fd: fd)
        close(fd)
    }
}
